package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/edirooss/zmux-server/internal/http/handler"
	mw "github.com/edirooss/zmux-server/internal/http/middleware"
	"github.com/edirooss/zmux-server/internal/service"
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/secure"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
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

	// Apply Gin middlewares
	rdb := buildRedisClient("127.0.0.1:6379", 0)
	chnlsvc, err := service.NewChannelService(context.TODO(), log, rdb)
	if err != nil {
		log.Fatal("channel service creation failed", zap.Error(err))
	}
	b2bclntsvc, err := service.NewB2BClientService(context.TODO(), log, chnlsvc, rdb)
	if err != nil {
		log.Fatal("b2b client service creation failed", zap.Error(err))
	}
	authsvc, err := service.NewAuthService(log, isDev, b2bclntsvc)
	if err != nil {
		log.Fatal("auth service creation failed", zap.Error(err))
	}
	{
		r.Use(gin.Recovery()) // Recovery first (outermost)
		r.Use(mw.RequestID()) // Attach request ID for tracing; early in the chain so it's available everywhere

		if isDev { // Enable CORS for local Vite dev
			r.Use(cors.New(cors.Config{
				AllowOrigins:     []string{"http://localhost:5173", "http://localhost:4173", "http://localhost:3000", "http://127.0.0.1:3000"},
				AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
				AllowHeaders:     []string{"X-Request-ID", "Content-Type", "X-CSRF-Token", "Authorization"},
				ExposeHeaders:    []string{"X-Request-ID", "X-Total-Count", "X-Cache", "X-Summary-Generated-At"},
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

		// r.Use(accessLog(zap.NewNop(), authsvc)) // Observability (logger, tracing)
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
			authed := r.Group("", mw.Authentication(authsvc)) // any authenticated principal (admin|b2b_client)
			authed.GET("/api/me", handler.Me(authsvc, b2bclntsvc))

			admins := authed.Group("", mw.Authorization(authsvc)) // only admins
			{
				{
					channelshndlr, err := handler.NewChannelsHandler(log, authsvc, chnlsvc, b2bclntsvc, service.NewRemuxRepository(log, rdb))
					if err != nil {
						log.Fatal("channels http handler creation failed", zap.Error(err))
					}
					// --- Channel collection ---
					admins.POST("/api/channels", channelshndlr.CreateChannel)    // create one
					authed.GET("/api/channels", channelshndlr.GetChannelList)    // get list, get many
					admins.DELETE("/api/channels", channelshndlr.DeleteChannels) // delete many
					admins.PATCH("/api/channels", channelshndlr.ModifyChannels)  // update many (modify/partial-update)

					// --- Channel resource ---
					requireValidID := mw.RequireValidChannelID()
					requireChannelAccess := mw.RequireChannelIDAccess(authsvc, b2bclntsvc)
					authed.GET("/api/channels/:id", requireValidID, requireChannelAccess, channelshndlr.GetChannel)      // get one
					admins.GET("/api/channels/:id/logs", requireValidID, channelshndlr.GetChannelLogs)                   // get one (logs)
					admins.PUT("/api/channels/:id", requireValidID, channelshndlr.ReplaceChannel)                        // update one (replace/full-update)
					authed.PATCH("/api/channels/:id", requireValidID, requireChannelAccess, channelshndlr.ModifyChannel) // update one (modify/partial-update)
					admins.DELETE("/api/channels/:id", requireValidID, channelshndlr.DeleteChannel)                      // delete one

					// --- Channel views ---
					admins.GET("/api/channels/summary", channelshndlr.Summary)
					authed.GET("/api/channels/status", channelshndlr.Status)
				}

				{
					// B2B Client handler
					b2bclnthndlr := handler.NewB2BClientHandler(b2bclntsvc, chnlsvc)

					// --- B2B Client collection ---
					admins.POST("/api/b2b-clients", b2bclnthndlr.CreateB2BClient)       // create one
					admins.GET("/api/b2b-clients", b2bclnthndlr.GetAllB2BClients)       // get all
					admins.GET("/api/b2b-clients/:id", b2bclnthndlr.GetB2BClient)       // get one
					admins.PUT("/api/b2b-clients/:id", b2bclnthndlr.UpdateB2BClient)    // update one
					admins.DELETE("/api/b2b-clients/:id", b2bclnthndlr.DeleteB2BClient) // delete one

					// bff helpers
					admins.GET("/api/b2b-clients/channels/available", b2bclnthndlr.GetChannelsAvailable)
					admins.GET("/api/b2b-clients/:id/channels", b2bclnthndlr.GetChannelsSelected)
					admins.GET("/api/b2b-clients/:id/channels/available-and-selected", b2bclnthndlr.GetChannelsAvailableAndSelected)
				}
			}

			// --- Outputs Ref ---
			admins.GET("/api/channels/outputs/ref", func(ctx *gin.Context) {
				type OutputRef struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				}
				ctx.JSON(http.StatusOK, []OutputRef{{"onprem_mr01", "onprem_mr01"}, {"onprem_mz01", "onprem_mz01"}, {"pubcloud_sky320", "pubcloud_sky320"}})
			})

			// --- System ---
			admins.GET("/api/system/net/localaddrs", handler.NewLocalAddrHandler(log).GetLocalAddrList) // GET local network addresses
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

// helpers

func buildLogger() *zap.Logger {
	logConfig := zap.NewDevelopmentConfig()
	logConfig.EncoderConfig.TimeKey = ""
	logConfig.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	logConfig.DisableStacktrace = true
	logConfig.DisableCaller = true
	logConfig.Level.SetLevel(zap.DebugLevel)
	return zap.Must(logConfig.Build())
}

func buildRedisClient(addr string, db int) *redis.Client {
	opts := &redis.Options{
		Addr:         addr,
		DB:           db,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
		MinIdleConns: 5,
		MaxRetries:   3,
	}

	return redis.NewClient(opts)
}
