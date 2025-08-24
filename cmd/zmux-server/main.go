package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/edirooss/zmux-server/internal/http/handlers"
	"github.com/edirooss/zmux-server/pkg/utils/avurl"
	"github.com/edirooss/zmux-server/services"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	// Read env
	isDev := os.Getenv("ENV") == "dev"

	// Create Zap logger
	logConfig := zap.NewDevelopmentConfig()
	logConfig.EncoderConfig.TimeKey = ""
	logConfig.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	logConfig.DisableStacktrace = true
	logConfig.DisableCaller = true
	logConfig.Level.SetLevel(zap.DebugLevel)
	log := zap.Must(logConfig.Build())
	defer log.Sync()
	log = log.Named("main")

	// HTTP Handler for channel CRUD
	channelshndlr, err := handlers.NewChannelsHandler(log)
	if err != nil {
		log.Fatal("channels http handler creation failed", zap.Error(err))
	}

	// Service for reading local addresses
	localaddrLister := services.NewLocalAddrLister(services.LocalAddrListerOptions{})

	// Service for generating channel summaries
	summarySvc := services.NewSummaryService(
		log,
		services.SummaryOptions{
			TTL:            1000 * time.Millisecond, // tune as needed
			RefreshTimeout: 500 * time.Millisecond,
		},
	)

	if !isDev {
		gin.SetMode(gin.ReleaseMode)
	}

	// Configure Gin's logger
	gin.DefaultWriter = zap.NewStdLog(log.Named("gin")).Writer()

	// Create Gin router
	r := gin.New()

	// Trust reverse proxy
	_ = r.SetTrustedProxies([]string{"127.0.0.1"})

	// Apply middlewares
	r.Use(gin.Recovery()) // Recovery first (outermost)

	if isDev {
		// Configure permissive CORS (dev only)
		r.Use(cors.New(cors.Config{
			AllowOrigins:     []string{"*"},
			AllowMethods:     []string{"*"},
			AllowHeaders:     []string{"*"},
			ExposeHeaders:    []string{"*"},
			AllowCredentials: true,
			MaxAge:           12 * time.Hour,
		}))
	}

	r.Use(accessLog(log)) // Observability after that (logger, tracing)

	r.GET("/api/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "pong"})
	})

	{
		r.POST("/api/url/parse", func(c *gin.Context) {
			var req struct {
				URL string `json:"url"`
			}
			if err := bind(c.Request, &req); err != nil {
				c.Error(err)
				c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
				return
			}
			url, err := avurl.Parse(req.URL)
			if err != nil {
				c.Error(err)
				c.JSON(http.StatusUnprocessableEntity, gin.H{"message": err.Error()})
				return
			}

			c.JSON(200, url)
		})

		r.POST("/api/url/parse/raw", func(c *gin.Context) {
			var req struct {
				URL string `json:"url"`
			}
			if err := bind(c.Request, &req); err != nil {
				c.Error(err)
				c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
				return
			}
			url, err := avurl.RawParse(req.URL)
			if err != nil {
				c.Error(err)
				c.JSON(http.StatusUnprocessableEntity, gin.H{"message": err.Error()})
				return
			}

			c.JSON(200, url)
		})
	}

	{
		r.GET("/api/channels", channelshndlr.GetChannelList)       // Get all (Collection)
		r.POST("/api/channels", channelshndlr.CreateChannel)       // Create new (Collection)
		r.GET("/api/channels/:id", channelshndlr.GetChannel)       // Get one
		r.PUT("/api/channels/:id", channelshndlr.ReplaceChannel)   // Replace one (full update)
		r.PATCH("/api/channels/:id", channelshndlr.ModifyChannel)  // Modify one (partial update)
		r.DELETE("/api/channels/:id", channelshndlr.DeleteChannel) // Delete one
	}

	r.GET("/api/system/net/localaddrs", func(c *gin.Context) {
		localAddrs, err := localaddrLister.GetLocalAddrs(c.Request.Context())
		if err != nil {
			c.Error(err) // <-- attach
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
			c.Error(err)
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

// accessLog is a Gin middleware that records HTTP request/response details with Zap after handling.
func accessLog(log *zap.Logger) gin.HandlerFunc {
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

func bind(req *http.Request, obj any) error {
	if req == nil || req.Body == nil {
		return errors.New("invalid request")
	}
	return decodeJSON(req.Body, obj)
}

func decodeJSON(r io.Reader, obj any) error {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(obj); err != nil {
		return err
	}
	return nil
}
