package main

import (
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/edirooss/zmux-server/internal/domain/principal"
	"github.com/edirooss/zmux-server/internal/http/handler"
	mw "github.com/edirooss/zmux-server/internal/http/middleware"
	"github.com/edirooss/zmux-server/internal/service"
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/secure"
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
	authsvc, err := service.NewAuthService(isDev)
	if err != nil {
		log.Fatal("auth service creation failed", zap.Error(err))
	}
	{
		r.Use(gin.Recovery()) // Recovery first (outermost)

		if isDev { // Enable CORS for local Vite dev
			r.Use(cors.New(cors.Config{
				AllowOrigins:     []string{"http://localhost:5173", "http://localhost:4173", "http://127.0.0.1:3000"},
				AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
				AllowHeaders:     []string{"Content-Type", "X-CSRF-Token", "Authorization"},
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

		r.Use(authsvc.UserSession.Middleware()) // Attach user cookie-based session for auth

		r.Use(accessLog(log, authsvc)) // Observability (logger, tracing)

		r.Use(func(c *gin.Context) {
			// Enforce a hard 10MB max request body.
			// Protects against oversized or drip-fed request body ("slow body" / RUDY DoS)
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 10<<20)
			c.Next()
		})
	}

	// Register route handlers
	{
		// --- Public endpoints (no auth) ---
		{
			r.GET("/api/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"message": "pong"}) })

			{
				usrsesshndler := handler.NewUserSessionsHandler(log, authsvc)
				r.POST("/api/login", usrsesshndler.Login)
				r.POST("/api/logout", usrsesshndler.Logout)
			}
		}

		// --- Protected endpoints (auth required) ---
		{
			authed := r.Group("", mw.Authentication(authsvc)) // Any authenticated principal required (admin|service_account)
			authed.GET("/api/me", handler.Me(authsvc))

			authzed := authed.Group("", mw.Authorization(authsvc, principal.Admin)) // Only admin principal

			{
				// HTTP Handler for channel CRUD + summary
				channelshndlr, err := handler.NewChannelsHandler(log)
				if err != nil {
					log.Fatal("channels http handler creation failed", zap.Error(err))
				}

				{
					authed.GET("/api/channels", channelshndlr.GetChannelList)                              // Get all (Collection)
					authzed.POST("/api/channels", mw.ConcurrentCap(10), channelshndlr.CreateChannel)       // Create new (Collection)
					authed.GET("/api/channels/:id", channelshndlr.GetChannel)                              // Get one
					authzed.PUT("/api/channels/:id", mw.ConcurrentCap(10), channelshndlr.ReplaceChannel)   // Replace one (full update)
					authed.PATCH("/api/channels/:id", mw.ConcurrentCap(10), channelshndlr.ModifyChannel)   // Modify one (partial update)
					authzed.DELETE("/api/channels/:id", mw.ConcurrentCap(10), channelshndlr.DeleteChannel) // Delete one
				}

				authzed.GET("/api/channels/summary", channelshndlr.Summary) // Get status+ifmt+metrics (Collection)
				authed.GET("/api/channels/status", channelshndlr.Status)    // Get status (Collection)
			}

			authzed.GET("/api/system/net/localaddrs", handler.NewLocalAddrHandler(log).GetLocalAddrList)
		}
	}

	httpsrv := &http.Server{
		Addr:              "127.0.0.1:8080",
		Handler:           r,
		ReadHeaderTimeout: 2 * time.Second,  // kills header-drip Slowloris
		ReadTimeout:       10 * time.Second, // full request read (incl. body)
		WriteTimeout:      15 * time.Second, // avoid forever-hangs on writes
		IdleTimeout:       60 * time.Second, // keep-alive cap
		MaxHeaderBytes:    1 << 20,          // 1MB cap
	}

	log.Info("running HTTP server", zap.String("addr", httpsrv.Addr))
	if err := httpsrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal("server failed", zap.Error(err))
	}
	log.Info("server closed")
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
func accessLog(log *zap.Logger, authsvc *service.AuthService) gin.HandlerFunc {
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
		if p := authsvc.WhoAmI(c); p != nil {
			fields = append(fields, zap.Dict("auth",
				zap.String("id", p.ID),
				zap.String("kind", p.Kind.String()),
			))
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
