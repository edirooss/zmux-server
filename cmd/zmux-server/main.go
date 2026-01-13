package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/aviravitz/onvif-client/camera"
	"github.com/aviravitz/onvif-client/deviceservice"
	"github.com/edirooss/zmux-server/internal/config"
	"github.com/edirooss/zmux-server/internal/http/handler"
	mw "github.com/edirooss/zmux-server/internal/http/middleware"
	"github.com/edirooss/zmux-server/internal/infrastructure/processmgr"
	"github.com/edirooss/zmux-server/internal/service"
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/secure"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/yaml.v3"
)

type Config struct {
	RedisAddr      string `yaml:"redis_address"`
	ZmuxServerAddr string `yaml:"zmux_server_address"`
	Port           string `yaml:"port"`
}

var serverConfig *Config

func init() {
	// Handle version display
	handleVersion()
}

func main() {
	// Read env
	isDev := os.Getenv("ENV") == "dev"

	// Load config
	if err := loadConfig(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

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
	rdb := buildRedisClient(serverConfig.RedisAddr, 0)
	logmngr := processmgr.NewLogManager()
	b2bclntsvc, err := service.NewB2BClientService(context.TODO(), log, rdb, logmngr)
	if err != nil {
		log.Fatal("b2b client service creation failed", zap.Error(err))
	}
	chnlsvc, err := service.NewChannelService(context.TODO(), log, rdb, b2bclntsvc, logmngr)
	if err != nil {
		log.Fatal("channel service creation failed", zap.Error(err))
	}
	authsvc, err := service.NewAuthService(log, isDev, b2bclntsvc, serverConfig.RedisAddr)
	if err != nil {
		log.Fatal("auth service creation failed", zap.Error(err))
	}
	{
		r.Use(gin.Recovery()) // Recovery first (outermost)
		r.Use(mw.RequestID()) // Attach request ID for tracing; early in the chain so it's available everywhere

		if isDev { // Enable CORS for local Vite dev
			r.Use(cors.New(cors.Config{
				AllowOrigins:     []string{"http://localhost:5173", "http://localhost:4173", "http://localhost:3000", "http://127.0.0.1:3000", "https://" + serverConfig.ZmuxServerAddr},
				AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
				AllowHeaders:     []string{"X-Request-ID", "Content-Type", "X-CSRF-Token", "Authorization"},
				ExposeHeaders:    []string{"X-Request-ID", "X-Total-Count", "X-Cache", "X-Summary-Generated-At"},
				AllowCredentials: true, // Allow cookies in dev
				MaxAge:           12 * time.Hour,
			}))
		} else { // Behind Nginx + TLS
			r.SetTrustedProxies([]string{"127.0.0.1", serverConfig.ZmuxServerAddr})
			r.Use(secure.New(secure.Config{
				SSLProxyHeaders: map[string]string{
					"X-Forwarded-Proto": "https", // Fix scheme for secure cookies
				},
			}))
		}

		r.Use(authsvc.UserSession.Middleware()) // Attach user cookie-based session for auth

		r.Use(accessLog(zap.NewNop(), authsvc)) // Observability (logger, tracing)
		// r.Use(accessLog(log, authsvc)) // Observability (logger, tracing)

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
					admins.DELETE("/api/channels/adama/:mc", channelshndlr.DeleteChannelByMC)                            // delete one

					// --- Channel views ---
					admins.GET("/api/channels/summary", channelshndlr.Summary)
					authed.GET("/api/channels/status", channelshndlr.Status)
				}

				{
					// handler.ONVIFClientHandler
					//TODO: add middlware for hashed camera credentials?
					encryptedCameraDetailsGet := mw.RequireValidEncryptedCameraDetailsGet()
					encryptedCameraDetailsPost := mw.RequireValidEncryptedCameraDetailsPost()

					onvifclnthndlr, err := handler.NewONVIFClientHandler(log)
					if err != nil {
						log.Fatal("onvif client http handler creation failed", zap.Error(err))
					}

					// --- ONVIF Camera actions ---
					// admins.POST("/api/camera/create-camera", encryptedCameraDetailsPost, onvifclnthndlr.CreateCamera)
					//API endpoints for ONVIF device service
					admins.GET("/api/camera/:id/device-information", encryptedCameraDetailsGet, onvifclnthndlr.GetDeviceInformation)
					admins.GET("/api/camera/:id/system-date-time", encryptedCameraDetailsGet, onvifclnthndlr.GetSystemDateAndTime)
					admins.GET("/api/camera/:id/network-interfaces", encryptedCameraDetailsGet, onvifclnthndlr.GetNetworkInterfaces)
					admins.GET("/api/camera/:id/users", encryptedCameraDetailsGet, onvifclnthndlr.GetUsers)
					admins.GET("/api/camera/:id/dns", encryptedCameraDetailsGet, onvifclnthndlr.GetDNS)
					admins.GET("/api/camera/:id/scopes", encryptedCameraDetailsGet, onvifclnthndlr.GetScopes)
					admins.GET("/api/camera/:id/ntp", encryptedCameraDetailsGet, onvifclnthndlr.GetNTP)
					admins.POST("/api/camera/:id/reboot", encryptedCameraDetailsGet, onvifclnthndlr.RebootCamera)

					admins.GET("/api/camera/:id/profile-token", encryptedCameraDetailsGet, onvifclnthndlr.GetProfileToken)
					admins.GET("/api/camera/:id/sensor-token", encryptedCameraDetailsGet, onvifclnthndlr.GetSensorToken)
					admins.GET("/api/camera/:id/device-profiles", encryptedCameraDetailsGet, onvifclnthndlr.GetDeviceProfiles)
					admins.GET("/api/camera/:id/stream-uri", encryptedCameraDetailsGet, onvifclnthndlr.GetStreamUri)
					admins.GET("/api/camera/:id/snapshot-uri", encryptedCameraDetailsGet, onvifclnthndlr.GetSnapshotUri)
					admins.GET("/api/camera/:id/video-encoder-configurations", encryptedCameraDetailsGet, onvifclnthndlr.GetVideoEncoderConfigurations)
					admins.GET("/api/camera/:id/video-encoder-configuration", encryptedCameraDetailsGet, onvifclnthndlr.GetVideoEncoderConfiguration)
					admins.GET("/api/camera/:id/video-options", encryptedCameraDetailsGet, onvifclnthndlr.GetVideoOptions)
					admins.GET("/api/camera/:id/osds", encryptedCameraDetailsGet, onvifclnthndlr.GetOSDs)
					admins.GET("/api/camera/:id/osd-token", encryptedCameraDetailsGet, onvifclnthndlr.GetOSDTokenByText)
					admins.GET("/api/camera/:id/video-sources", encryptedCameraDetailsGet, onvifclnthndlr.GetVideoSources)
					admins.GET("/api/camera/:id/audio-encoders", encryptedCameraDetailsGet, onvifclnthndlr.GetAudioEncoders)
					admins.POST("/api/camera/:id/create-osd", encryptedCameraDetailsPost, onvifclnthndlr.CreateOSD)
					admins.POST("/api/camera/:id/set-osd-text", encryptedCameraDetailsPost, onvifclnthndlr.SetOSDText)
					admins.DELETE("/api/camera/:id/delete-osd", encryptedCameraDetailsPost, onvifclnthndlr.DeleteOSD)
					admins.POST("/api/camera/:id/synchronization-point", encryptedCameraDetailsPost, onvifclnthndlr.SetSynchronizationPoint)
					admins.POST("/api/camera/:id/modify-video-encoder-resolution", encryptedCameraDetailsPost, onvifclnthndlr.ModifyVideoEncoderResolution)
					admins.POST("/api/camera/:id/modify-video-encoder-quality", encryptedCameraDetailsPost, onvifclnthndlr.ModifyVideoEncoderQuality)

					admins.GET("/api/camera/:id/imaging-settings", encryptedCameraDetailsGet, onvifclnthndlr.GetImagingSettings)
					admins.GET("/api/camera/:id/imaging-options", encryptedCameraDetailsGet, onvifclnthndlr.GetImagingOptions)
					admins.GET("/api/camera/:id/imaging-status", encryptedCameraDetailsGet, onvifclnthndlr.GetImagingStatus)
					admins.POST("/api/camera/:id/set-imaging-settings", encryptedCameraDetailsPost, onvifclnthndlr.SetImagingSettings)
					admins.POST("/api/camera/:id/move-focus-absolute", encryptedCameraDetailsPost, onvifclnthndlr.MoveFocusAbsolute)
					admins.POST("/api/camera/:id/move-focus-relative", encryptedCameraDetailsPost, onvifclnthndlr.MoveFocusRelative)
					admins.POST("/api/camera/:id/set-focus-mode", encryptedCameraDetailsPost, onvifclnthndlr.SetFocusMode)
					admins.POST("/api/camera/:id/set-ir-cut-filter", encryptedCameraDetailsPost, onvifclnthndlr.SetIrCutFilter)
					admins.POST("/api/camera/:id/set-backlight-compensation", encryptedCameraDetailsPost, onvifclnthndlr.SetBacklightCompensation)
					admins.POST("/api/camera/:id/set-wide-dynamic-range", encryptedCameraDetailsPost, onvifclnthndlr.SetWideDynamicRange)
					admins.POST("/api/camera/:id/set-white-balance", encryptedCameraDetailsPost, onvifclnthndlr.SetWhiteBalance)
					admins.POST("/api/camera/:id/set-exposure-mode", encryptedCameraDetailsPost, onvifclnthndlr.SetExposureMode)
					admins.POST("/api/camera/:id/set-manual-exposure", encryptedCameraDetailsPost, onvifclnthndlr.SetManualExposure)
					admins.POST("/api/camera/:id/set-exposure-limits", encryptedCameraDetailsPost, onvifclnthndlr.SetExposureLimits)
					admins.GET("/api/camera/:id/is-manual-focus", encryptedCameraDetailsGet, onvifclnthndlr.IsManualFocus)

					admins.GET("/api/camera/:id/ptz-status", encryptedCameraDetailsGet, onvifclnthndlr.GetPTZStatus)
					admins.GET("/api/camera/:id/ptz-configurations", encryptedCameraDetailsGet, onvifclnthndlr.GetPTZConfigurations)
					admins.GET("/api/camera/:id/presets", encryptedCameraDetailsGet, onvifclnthndlr.GetPresets)
					admins.GET("/api/camera/:id/preset-token", encryptedCameraDetailsGet, onvifclnthndlr.GetPresetTokenByName)
					admins.GET("/api/camera/:id/ptz-nodes", encryptedCameraDetailsGet, onvifclnthndlr.GetPTZNodes)
					admins.GET("/api/camera/:id/preset-tours", encryptedCameraDetailsGet, onvifclnthndlr.GetPresetTours)
					admins.POST("/api/camera/:id/operate-preset-tour", encryptedCameraDetailsPost, onvifclnthndlr.OperatePresetTour)
					admins.POST("/api/camera/:id/goto-home-position", encryptedCameraDetailsPost, onvifclnthndlr.GotoHomePosition)
					admins.POST("/api/camera/:id/set-home-position", encryptedCameraDetailsPost, onvifclnthndlr.SetHomePosition)
					admins.POST("/api/camera/:id/absolute-move", encryptedCameraDetailsPost, onvifclnthndlr.AbsoluteMove)
					admins.POST("/api/camera/:id/relative-move", encryptedCameraDetailsPost, onvifclnthndlr.RelativeMove)
					admins.POST("/api/camera/:id/continuous-move", encryptedCameraDetailsPost, onvifclnthndlr.ContinuousMove)
					admins.POST("/api/camera/:id/goto-preset", encryptedCameraDetailsPost, onvifclnthndlr.GotoPreset)
					admins.POST("/api/camera/:id/set-preset", encryptedCameraDetailsPost, onvifclnthndlr.SetPreset)
					admins.DELETE("/api/camera/:id/remove-preset", encryptedCameraDetailsPost, onvifclnthndlr.RemovePreset)

					admins.GET("/api/camera/:id/relays", encryptedCameraDetailsGet, onvifclnthndlr.GetRelays)
					admins.GET("/api/camera/:id/digital-inputs", encryptedCameraDetailsGet, onvifclnthndlr.GetDigitalInputs)
					admins.POST("/api/camera/:id/trigger-relay", encryptedCameraDetailsPost, onvifclnthndlr.TriggerRelay)

					admins.POST("/api/camera/:id/start-subscription", encryptedCameraDetailsPost, onvifclnthndlr.StartSubscription)
					admins.GET("/api/camera/:id/fetch-events", encryptedCameraDetailsGet, onvifclnthndlr.FetchEvents)
					admins.POST("/api/camera/:id/renew-subscription", encryptedCameraDetailsPost, onvifclnthndlr.RenewSubscription)
					// api/create_camera with TTL 1 hour?
					// api/camera/:id/ptz-move
					// api/camera/:id/ptz-stop
					// api/camera/:id/ntp-set
					// api/camera/:id/system-log?type=access|system
				}

				{
					// B2B Client handler
					b2bclnthndlr := handler.NewB2BClientHandler(b2bclntsvc)

					// --- B2B Client collection ---
					admins.POST("/api/b2b-clients", b2bclnthndlr.CreateB2BClient)       // create one
					admins.GET("/api/b2b-clients", b2bclnthndlr.GetAllB2BClients)       // get all
					admins.GET("/api/b2b-clients/:id", b2bclnthndlr.GetB2BClient)       // get one
					admins.PUT("/api/b2b-clients/:id", b2bclnthndlr.UpdateB2BClient)    // update one
					admins.DELETE("/api/b2b-clients/:id", b2bclnthndlr.DeleteB2BClient) // delete one
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
		Addr:              serverConfig.ZmuxServerAddr + ":" + serverConfig.Port,
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

	camera, err := camera.CreateCamera("", "", "", "")
	if err != nil {
		log.Error("failed to create camera", zap.Error(err))
		return
	}
	deviceservice.GetDeviceInformation(camera)
	deviceservice.SetNTP(camera, false, "")
}

// handleVersion prints build metadata and exits when -v/--version is provided.
func handleVersion() {
	v := flag.Bool("v", false, "print version and exit")
	flag.BoolVar(v, "version", false, "print version and exit")
	flag.Parse()

	if *v {
		fmt.Printf("remux %s (commit %s, built %s)\n", config.Version, config.GitCommit, config.BuildDate)
		os.Exit(0)
	}
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

func loadConfig() error {
	data, err := os.ReadFile("zmux-server.yaml")
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(data, &serverConfig)
	if err != nil {
		return err
	}

	return nil
}
