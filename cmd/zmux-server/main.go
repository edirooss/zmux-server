package main

import (
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/edirooss/zmux-server/internal/http/handlers"
	"github.com/edirooss/zmux-server/internal/http/middleware"
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/secure"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/redis"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	// Read env
	isDev := os.Getenv("ENV") == "dev"

	// Create Zap logger
	log := buildLogger()
	defer log.Sync()
	log = log.Named("main")

	// Create Gin router
	if !isDev {
		gin.SetMode(gin.ReleaseMode)
	}
	gin.DefaultWriter = zap.NewStdLog(log.Named("gin")).Writer() // Configure Gin's logger to use Zap
	r := gin.New()

	// Apply middlewares
	{
		r.Use(gin.Recovery()) // Recovery first (outermost)

		if isDev { // Enable CORS for local Vite dev
			r.Use(cors.New(cors.Config{
				AllowOrigins:     []string{"http://localhost:5173", "http://localhost:4173"},
				AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
				AllowHeaders:     []string{"Content-Type", "X-CSRF-Token"},
				ExposeHeaders:    []string{"X-Total-Count", "X-Cache", "X-Summary-Generated-At"},
				AllowCredentials: true, // Allow cookies in dev
				MaxAge:           12 * time.Hour,
			}))
		} else { // Behind Nginx + TLS
			r.SetTrustedProxies([]string{"127.0.0.1"})
			r.Use(secure.New(secure.Config{
				SSLProxyHeaders: map[string]string{
					"X-Forwarded-Proto": "https", // Fix scheme for secure cookies
				},
			}))
		}

		// Create Redis session store
		store, err := redis.NewStoreWithDB(10, "tcp", "127.0.0.1:6379", "", "", "0",
			[]byte("nZCowo9+aofuYO/54sK2mca+aj8M9XA2zVLrP1kh6uk=") /* TODO(security): rotate key */)
		if err != nil {
			log.Fatal("redis session store init failed", zap.Error(err))
		}
		store.Options(sessions.Options{
			Path:     "/api",   // scope cookie strictly to /api
			MaxAge:   4 * 3600, // session cookie lifetime (4h)
			Secure:   !isDev,   // must be true behind HTTPS in prod
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})

		r.Use(sessions.Sessions("sid" /* Session cookie name */, store))

		r.Use(accessLog(log)) // Observability (logger, tracing)
	}

	// Register route handlers
	{
		// --- Public endpoints (no auth) ---
		authhndler := handlers.NewAuthHandler(log, isDev)
		{
			r.GET("/api/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"message": "pong"}) })

			{
				r.POST("/api/login", authhndler.Login)
				r.POST("/api/logout", authhndler.Logout)
			}
		}

		// --- Protected endpoints (auth required) ---
		{
			authed := r.Group("", middleware.RequireAuth, middleware.ValidateSessionCSRF) // Any authentication method required (basic, session or API key)

			authed.GET("/api/me", authhndler.Me)

			authed.GET("/api/csrf", handlers.NewCSRFHandler(log).IssueSessionCSRF)

			{
				// HTTP Handler for channel CRUD + summary
				channelshndlr, err := handlers.NewChannelsHandler(log)
				if err != nil {
					log.Fatal("channels http handler creation failed", zap.Error(err))
				}

				{
					authed.GET("/api/channels", channelshndlr.GetChannelList)       // Get all (Collection)
					authed.POST("/api/channels", channelshndlr.CreateChannel)       // Create new (Collection)
					authed.GET("/api/channels/:id", channelshndlr.GetChannel)       // Get one
					authed.PUT("/api/channels/:id", channelshndlr.ReplaceChannel)   // Replace one (full update)
					authed.PATCH("/api/channels/:id", channelshndlr.ModifyChannel)  // Modify one (partial update)
					authed.DELETE("/api/channels/:id", channelshndlr.DeleteChannel) // Delete one
				}

				authed.GET("/api/channels/summary", channelshndlr.Summary) // Generate summary for admin dashboard (Collection)
			}

			authed.GET("/api/system/net/localaddrs", handlers.NewLocalAddrHandler(log).GetLocalAddrList)
		}
	}

	log.Info("running HTTP server on 127.0.0.1:8080")
	if err := r.Run("127.0.0.1:8080"); err != nil {
		log.Fatal("server failed", zap.Error(err))
	}
}

func buildLogger() *zap.Logger {
	logConfig := zap.NewDevelopmentConfig()
	logConfig.EncoderConfig.TimeKey = ""
	logConfig.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	logConfig.DisableStacktrace = true
	logConfig.DisableCaller = true
	logConfig.Level.SetLevel(zap.DebugLevel)
	return zap.Must(logConfig.Build())
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
		// if p := getPrincipal(c); p != nil {
		// 	fields = append(fields, zap.Dict("auth", zap.String("kind", p.Kind), zap.String("uid", p.UID)))
		// }
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
