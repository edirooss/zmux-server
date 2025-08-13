package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	models "github.com/edirooss/zmux-server/pkg/models/channel"
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
		log.Error("channel service creation failed", zap.Error(err))
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

	env := os.Getenv("ENV")
	var allowedOrigin string
	switch env {
	case "MR":
		allowedOrigin = "https://192.168.1.4:443"
	case "MZ":
		allowedOrigin = "https://192.168.2.4:443"
	default:
		allowedOrigin = "http://localhost:5173"
	}

	corsCfg := cors.Config{
		AllowOrigins:     []string{allowedOrigin},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Content-Type", "Authorization"},
		ExposeHeaders:    []string{"X-Total-Count", "Location"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour, // cache preflight
	}

	// Apply middlewares
	r.Use(gin.Recovery())    // Recovery first (outermost)
	r.Use(cors.New(corsCfg)) // “Edge” middleware next (CORS, etc.)
	r.Use(ZapLogger(log))    // Observability after that (logger, tracing)

	r.GET("/api/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "pong"})
	})

	r.POST("/api/channels", func(c *gin.Context) {
		req := models.NewCreateZmuxChannelReq()

		if err := c.ShouldBindJSON(&req); err != nil {
			_ = c.Error(err) // <-- attach
			c.JSON(http.StatusUnprocessableEntity, gin.H{"message": err.Error()})
			return
		}

		ch, err := channelService.CreateChannel(c.Request.Context(), &req)
		if err != nil {
			_ = c.Error(err) // <-- attach
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			return
		}

		c.Header("Location", fmt.Sprintf("/api/channels/%d", ch.ID))
		c.JSON(http.StatusCreated, ch)
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

	// main router setup (add)
	r.PUT("/api/channels/:id", func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			_ = c.Error(err)
			c.JSON(http.StatusBadRequest, gin.H{"message": "invalid id"})
			return
		}

		var req models.UpdateZmuxChannelReq
		if err := c.ShouldBindJSON(&req); err != nil {
			_ = c.Error(err)
			c.JSON(http.StatusUnprocessableEntity, gin.H{"message": err.Error()})
			return
		}

		ch, err := channelService.UpdateChannel(c.Request.Context(), id, &req)
		if err != nil {
			// If your repo returns a typed not-found error, convert here:
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

	// Run server
	switch env {
	case "MR", "MZ":
		certFile := os.Getenv("TLS_CERT_FILE") // e.g., /etc/ssl/certs/server.crt
		keyFile := os.Getenv("TLS_KEY_FILE")   // e.g., /etc/ssl/private/server.key
		if certFile == "" || keyFile == "" {
			log.Fatal("TLS_CERT_FILE and TLS_KEY_FILE must be set")
		}
		log.Info("running HTTPS server on :8443", zap.String("env", env), zap.String("cors_origin", allowedOrigin))
		if err := r.RunTLS(":8443", certFile, keyFile); err != nil {
			log.Fatal("server failed", zap.Error(err))
		}
	default:
		log.Info("running HTTP server on :8080", zap.String("env", env), zap.String("cors_origin", allowedOrigin))
		if err := r.Run(":8080"); err != nil {
			log.Fatal("server failed", zap.Error(err))
		}
	}
}
