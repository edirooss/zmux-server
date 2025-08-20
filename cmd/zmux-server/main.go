package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/edirooss/zmux-server/pkg/jsonx"
	"github.com/edirooss/zmux-server/pkg/models/channelmodel"
	"github.com/edirooss/zmux-server/redis"
	"github.com/edirooss/zmux-server/services"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Custom Gin middleware that logs using Zap
func ZapLogger(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		status := c.Writer.Status()
		latency := time.Since(start)
		route := c.FullPath()
		if route == "" {
			route = c.Request.URL.Path
		}

		// collect all errors from Gin context
		var errs []error
		for _, ge := range c.Errors {
			if ge.Err != nil {
				errs = append(errs, ge.Err)
			}
		}
		// errors.Join returns nil if errs is empty
		joinedErr := errors.Join(errs...)

		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("route", route),
			zap.Int("status", status),
			zap.String("client_ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
			zap.Duration("latency", latency),
		}
		if joinedErr != nil {
			fields = append(fields, zap.Error(joinedErr))
		}

		switch {
		case status >= 500:
			log.Error("request", fields...)
		case status >= 400:
			log.Warn("request", fields...)
		default:
			log.Info("request", fields...)
		}
	}
}

func main() {
	// Create Zap logger
	logConfig := zap.NewDevelopmentConfig()
	logConfig.EncoderConfig.TimeKey = ""
	logConfig.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	logConfig.DisableStacktrace = true
	logConfig.DisableCaller = true
	log := zap.Must(logConfig.Build())
	defer log.Sync()

	// Enable strict JSON decoding (must be before binding happens)
	binding.EnableDecoderDisallowUnknownFields = true

	// Service for channel CRUD
	channelService, err := services.NewChannelService(log)
	if err != nil {
		log.Fatal("channel service creation failed", zap.Error(err))
	}

	// Service for reading local addresses
	localaddrLister := services.NewLocalAddrLister(services.LocalAddrListerOptions{})

	// Service for generating channel summaries
	summarySvc := services.NewSummaryService(
		log,
		redis.NewChannelRepository(log),
		redis.NewRemuxRepository(log),
		services.SummaryOptions{
			TTL:            1000 * time.Millisecond, // tune as needed
			RefreshTimeout: 500 * time.Millisecond,
		},
	)

	// Create Gin router
	r := gin.New()

	// Trust reverse proxy
	_ = r.SetTrustedProxies([]string{"127.0.0.1"})

	// Apply middlewares
	r.Use(gin.Recovery()) // Recovery first (outermost)

	// CORS (dev only)
	if os.Getenv("ENV") == "dev" {
		r.Use(cors.New(cors.Config{
			AllowOrigins:     []string{"http://localhost:5173"},
			AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowHeaders:     []string{"Content-Type", "Authorization"},
			ExposeHeaders:    []string{"X-Total-Count", "Location"},
			AllowCredentials: false,
			MaxAge:           12 * time.Hour, // cache preflight
		}))
	}

	r.Use(ZapLogger(log)) // Observability after that (logger, tracing)

	r.GET("/api/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "pong"})
	})

	r.GET("/api/channels/:id", func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			_ = c.Error(err) // <-- attach parse error too (helps observability)
			c.JSON(http.StatusBadRequest, gin.H{"message": "invalid id"})
			return
		}

		ch, err := channelService.GetChannel(c.Request.Context(), id)
		if err != nil {
			if errors.Is(err, redis.ErrChannelNotFound) {
				_ = c.Error(err) // <-- attach 404 cause as well (optional)
				c.JSON(http.StatusNotFound, gin.H{"message": redis.ErrChannelNotFound.Error()})
				return
			}
			_ = c.Error(err) // <-- attach
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			return
		}

		c.JSON(http.StatusOK, ch)
	})

	r.GET("/api/channels", func(c *gin.Context) {
		chs, err := channelService.ListChannels(c.Request.Context())
		if err != nil {
			_ = c.Error(err) // <-- attach
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			return
		}
		c.Header("X-Total-Count", strconv.Itoa(len(chs))) // RA needs this
		c.JSON(http.StatusOK, chs)
	})

	r.POST("/api/channels", func(c *gin.Context) {
		// Content-Type guard
		if requireContentType(c, "application/json", "application/json; charset=utf-8"); err != nil {
			_ = c.Error(err) // <-- attach
			c.JSON(http.StatusUnsupportedMediaType, gin.H{"message": err.Error()})
			return
		}

		var req channelmodel.CreateZmuxChannelReq
		if err := jsonx.ParseStrictJSONBody(c.Request, &req); err != nil { /* schema mismatch: malformed JSON, unknown fields, missing required fields, wrong data type at JSON level */
			_ = c.Error(err) // <-- attach
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}

		ch, err := req.ToDomain()
		if err != nil { /* schema matched but misused (well-formed json, but content invalid: null in non-nullable / parse field format failure / invalid field values) */
			_ = c.Error(err) // <-- attach
			c.JSON(http.StatusUnprocessableEntity, gin.H{"message": err.Error()})
			return
		}

		if err := channelService.CreateChannel(c.Request.Context(), ch); err != nil {
			_ = c.Error(err) // <-- attach
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			return
		}

		c.Header("Location", fmt.Sprintf("/api/channels/%d", ch.ID))
		c.JSON(http.StatusCreated, ch)
	})

	r.PUT("/api/channels/:id", func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			_ = c.Error(err)
			c.JSON(http.StatusBadRequest, gin.H{"message": "invalid id"})
			return
		}

		// Content-Type guard
		if err := requireContentType(c, "application/json", "application/json; charset=utf-8"); err != nil {
			_ = c.Error(err) // <-- attach
			c.JSON(http.StatusUnsupportedMediaType, gin.H{"message": err.Error()})
			return
		}

		var req channelmodel.ReplaceZmuxChannelReq
		if err := jsonx.ParseStrictJSONBody(c.Request, &req); err != nil { /* schema mismatch: malformed JSON, unknown fields, missing required fields, wrong data type at JSON level */
			_ = c.Error(err) // <-- attach
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}

		ch, err := req.ToDomain(id)
		if err != nil { /* schema matched but misused (well-formed json, but content invalid: null in non-nullable / parse field format failure / invalid field values) */
			_ = c.Error(err) // <-- attach
			c.JSON(http.StatusUnprocessableEntity, gin.H{"message": err.Error()})
			return
		}

		if err := channelService.UpdateChannel(c.Request.Context(), ch); err != nil {
			if errors.Is(err, redis.ErrChannelNotFound) {
				_ = c.Error(err)
				c.JSON(http.StatusNotFound, gin.H{"message": redis.ErrChannelNotFound.Error()})
				return
			}
			_ = c.Error(err)
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			return
		}

		c.JSON(http.StatusOK, ch)
	})

	r.DELETE("/api/channels/:id", func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			_ = c.Error(err)
			c.JSON(http.StatusBadRequest, gin.H{"message": "invalid id"})
			return
		}

		if err := channelService.DeleteChannel(c.Request.Context(), id); err != nil {
			if errors.Is(err, redis.ErrChannelNotFound) {
				_ = c.Error(err)
				c.JSON(http.StatusNotFound, gin.H{"message": redis.ErrChannelNotFound.Error()})
				return
			}
			_ = c.Error(err)
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			return
		}

		// RA-friendly response
		c.JSON(http.StatusOK, gin.H{"id": id})
	})

	r.GET("/api/system/net/localaddrs", func(c *gin.Context) {
		localAddrs, err := localaddrLister.GetLocalAddrs(c.Request.Context())
		if err != nil {
			_ = c.Error(err) // <-- attach
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			return
		}

		c.Header("X-Total-Count", strconv.Itoa(len(localAddrs))) // RA needs this
		c.JSON(http.StatusOK, localAddrs)
	})

	r.GET("/api/channels/summary", func(c *gin.Context) {
		// Optional query to bypass cache for admin/diagnostics: ?force=1
		force := c.Query("force") == "1"

		var (
			res services.SummaryResult
			err error
		)
		if force {
			// Force a refresh by temporarily setting TTL=0 via a context trick:
			// Simply call summarySvc.Get with expired cache by invalidating before.
			// Safer: expose a public Invalidate(). We'll do that:
			summarySvc.Invalidate()
		}

		res, err = summarySvc.Get(c.Request.Context())
		if err != nil {
			_ = c.Error(err)
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			return
		}

		// Friendly cache headers for debugging/observability
		c.Header("X-Cache", map[bool]string{true: "HIT", false: "MISS"}[res.CacheHit])
		c.Header("X-Summary-Generated-At", strconv.FormatInt(res.GeneratedAt.UnixMilli(), 10))
		c.Header("X-Total-Count", strconv.Itoa(len(res.Data)))

		c.JSON(http.StatusOK, res.Data)
	})

	log.Info("running HTTP server on 127.0.0.1:8080")
	if err := r.Run("127.0.0.1:8080"); err != nil {
		log.Fatal("server failed", zap.Error(err))
	}
}

// requireContentType ensures the request's Content-Type header exactly matches one of the allowed values.
func requireContentType(c *gin.Context, allowedContentTypes ...string) error {
	contentType := c.GetHeader("Content-Type")
	for _, allowed := range allowedContentTypes {
		if contentType == allowed {
			return nil
		}
	}
	return fmt.Errorf("invalid Content-Type %q; must be one of: %v", contentType, allowedContentTypes)
}
