package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/edirooss/zmux-server/internal/env"
	"github.com/edirooss/zmux-server/internal/http/handlers"
	"github.com/edirooss/zmux-server/pkg/utils/avurl"
	"github.com/edirooss/zmux-server/services"
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/secure"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/redis"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ============================================================================
// Auth & CSRF
// ============================================================================

type principal struct {
	Kind string // "session" or "apikey"
	UID  string
}

func setPrincipal(c *gin.Context, p *principal) { c.Set("principal", p) }
func getPrincipal(c *gin.Context) *principal {
	v, ok := c.Get("principal")
	if !ok || v == nil {
		return nil
	}
	p, _ := v.(*principal)
	return p
}

// --- Internal auth helpers (no side-effects) ---

func tryAPIKey(key string) *principal {
	if key == "" {
		return nil
	}
	if uid, ok := validateAPIKey(key); ok {
		return &principal{Kind: "apikey", UID: uid}
	}
	return nil
}

func trySession(sess sessions.Session) *principal {
	uid, _ := sess.Get("uid").(string)
	if uid == "" {
		return nil
	}

	// Sliding TTL — refresh touch marker; store.Save() extends TTL
	now := time.Now().Unix()
	last, _ := sess.Get("last_touch").(int64)
	if last == 0 || now-last > 15*60 /* session touch interval seconds (15 minutes) */ {
		// TODO(reliability): check error from Save() and log warn/metrics; Redis flaps shouldn't fail silently.
		sess.Set("last_touch", now)
		_ = sess.Save()
	}

	return &principal{Kind: "session", UID: uid}
}

// --- Authentication handlers (invasive) ---
// Blocks unauthenticated requests outright

// AuthAPIKey authenticates the request using an API key.
//   - If an API key is provided in the X-API-Key header,
//     it sets a principal on the gin.Context and continues request processing.
//   - If no valid key is found, it aborts the request
//     with 404 (obscurity pattern — don’t leak that endpoint exists yet auth failed).
func AuthAPIKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		if p := tryAPIKey(c.GetHeader("X-API-Key")); p != nil {
			setPrincipal(c, p)
			c.Next()
			return
		}
		c.AbortWithStatus(http.StatusNotFound)
	}
}

// AuthSession authenticates the request using a session cookie.
//   - If a valid session exists, it sets a principal on the gin.Context
//     and continues request processing.
//   - If no valid session is found,  it aborts the request
//     with 404 (obscurity pattern — don’t leak that endpoint exists yet auth failed).
func AuthSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		if p := trySession(sessions.Default(c)); p != nil {
			setPrincipal(c, p)

			c.Next()
			return
		}
		c.AbortWithStatus(http.StatusNotFound)
	}
}

// --- Prefer API key, fallback to session, else 404 for obscurity ---
func AuthEither() gin.HandlerFunc {
	return func(c *gin.Context) {
		if p := tryAPIKey(c.GetHeader("X-API-Key")); p != nil {
			setPrincipal(c, p)

			c.Next()
			return
		}

		if p := trySession(sessions.Default(c)); p != nil {
			setPrincipal(c, p)

			c.Next()
			return
		}
		c.AbortWithStatus(http.StatusNotFound)
	}
}

// CSRFIfSession ensures CSRF validation for mutating requests with session auth.
// - Only applies to session-authenticated principals (API keys skip CSRF).
// - Only applies if request method is mutating (POST, PUT, PATCH, DELETE).
// - Rejects if the X-CSRF-Token header is missing or invalid.
func CSRFIfSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only enforce CSRF if principal is a session user
		p := getPrincipal(c)
		if p == nil || p.Kind != "session" {
			c.Next()
			return
		}

		// Skip non-mutating methods
		if !isMutating(c.Request.Method) {
			c.Next()
			return
		}

		// Fetch expected token from session
		sess := sessions.Default(c)
		want, _ := sess.Get("csrf").(string)
		got := c.GetHeader("X-CSRF-Token")

		if want == "" || got == "" ||
			subtle.ConstantTimeCompare([]byte(want), []byte(got)) != 1 {
			c.AbortWithStatusJSON(http.StatusBadRequest,
				gin.H{"message": "invalid_csrf"})
			return
		}

		c.Next()
	}
}

// isMutating returns true if the method changes state.
func isMutating(m string) bool {
	switch m {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func validateAPIKey(key string) (uid string, ok bool) {
	// TODO(auth): replace with real API key validation against a persistent store and return a stable key ID (not the raw key).
	// Accept any non-empty key for now to unblock partners during early development.
	if key == "" {
		return "", false
	}
	// Avoid logging the raw key via UID; use a generic placeholder for now.
	return "api", true
}

func randomTokenHex(nBytes int) string {
	b := make([]byte, nBytes)
	// TODO(security,bug): handle rand.Read error; on failure, return 500 rather than silently issuing a weak token.
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ============================================================================
// Server boot
// ============================================================================

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

	// Apply middlewares
	r.Use(gin.Recovery()) // Recovery first (outermost)

	if isDev { // Enable CORS for local Vite dev
		r.Use(cors.New(cors.Config{
			AllowOrigins:     []string{"http://localhost:5173"},
			AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowHeaders:     []string{"Content-Type", "Authorization", "X-CSRF-Token"},
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

	// Redis session store
	store, err := redis.NewStoreWithDB(10, "tcp", "127.0.0.1:6379", "", "", "0",
		[]byte("nZCowo9+aofuYO/54sK2mca+aj8M9XA2zVLrP1kh6uk=") /* TODO(security): rotate SESSION_KEY */)
	if err != nil {
		log.Fatal("redis session store init failed", zap.Error(err))
	}
	r.Use(sessions.Sessions("sid" /* session cookie name */, store))

	r.Use(accessLog(log)) // Observability after that (logger, tracing)

	// --- Public endpoints (no auth) ---
	{
		r.GET("/api/ping", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "pong"})
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

				c.JSON(http.StatusOK, url)
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

				c.JSON(http.StatusOK, url)
			})
		}

		// Auth endpoints for web console (cookie session)
		{
			r.POST("/api/login", func(c *gin.Context) {
				var req struct {
					Username string `json:"username"`
					Password string `json:"password"`
				}
				if err := bind(c.Request, &req); err != nil {
					c.Error(err)
					c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
					return
				}

				// TODO(auth): replace with real user lookup + password verify (bcrypt/argon2id) against a store.
				// Accept only env admin user credentials for now to unblock early development.
				if !(req.Username == env.Admin.Username && req.Password == env.Admin.Password) {
					c.JSON(http.StatusUnauthorized, gin.H{"message": "invalid credentials"})
					return
				}

				sess := sessions.Default(c)
				sess.Set("uid", req.Username)
				sess.Set("last_touch", time.Now().Unix())
				store.Options(sessions.Options{
					Path:     "/api",   // scope cookie strictly to /api
					MaxAge:   4 * 3600, /* 4 hours */ // session cookie lifetime
					Secure:   !isDev,   // must be true behind HTTPS in prod
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
				})
				if err := sess.Save(); err != nil {
					c.Error(err)
					c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
					return
				}

				c.Status(http.StatusOK)
			})

			r.POST("/api/logout", func(c *gin.Context) {
				sess := sessions.Default(c)
				sess.Clear()
				sess.Options(sessions.Options{
					Path:     "/api",
					MaxAge:   -1,
					Secure:   !isDev,
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
				})
				_ = sess.Save()
				c.Status(http.StatusNoContent)
			})
		}
	}

	// --- Protected endpoints (auth required) ---
	{
		// --- Only for cookie session (web console) ---
		sessAuthed := r.Group("", AuthSession(), CSRFIfSession())

		sessAuthed.GET("/api/me", func(c *gin.Context) {
			p := getPrincipal(c)
			if p == nil || p.Kind != "session" || p.UID == "" {
				c.AbortWithStatus(http.StatusInternalServerError) // Invariant broken: AuthSession should have guaranteed this.
				return
			}
			c.JSON(http.StatusOK, gin.H{"uid": p.UID})
		})

		sessAuthed.GET("/api/csrf", func(c *gin.Context) {
			sess := sessions.Default(c)
			token, _ := sess.Get("csrf").(string)
			if token == "" {
				token = randomTokenHex(32)
				sess.Set("csrf", token)
				_ = sess.Save()
			}

			// Avoid cache serving stale tokens
			c.Header("Cache-Control", "no-store")
			c.Header("Pragma", "no-cache")
			c.Header("Expires", "0")
			c.JSON(http.StatusOK, gin.H{"csrf": token})
		})

		sessAuthed.GET("/api/system/net/localaddrs", func(c *gin.Context) {
			localAddrs, err := localaddrLister.GetLocalAddrs(c.Request.Context())
			if err != nil {
				c.Error(err) // <-- attach
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}

			c.Header("X-Total-Count", strconv.Itoa(len(localAddrs))) // RA needs this
			c.JSON(http.StatusOK, localAddrs)
		})

		sessAuthed.GET("/api/channels/summary", func(c *gin.Context) {
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

		{
			sessAuthed.POST("/api/channels", channelshndlr.CreateChannel)       // Create new (Collection)
			sessAuthed.PUT("/api/channels/:id", channelshndlr.ReplaceChannel)   // Replace one (full update)
			sessAuthed.DELETE("/api/channels/:id", channelshndlr.DeleteChannel) // Delete one
		}

		// --- Shared API ---
		// Both session cookie (web console) and API key (partners) hit the same routes.
		sharedAuthed := r.Group("", AuthEither(), CSRFIfSession())

		{
			// TODO(authz): add per-key/per-user scopes for modify actions; read/list should be separable.
			sharedAuthed.GET("/api/channels", channelshndlr.GetChannelList)      // Get all (Collection)
			sharedAuthed.GET("/api/channels/:id", channelshndlr.GetChannel)      // Get one
			sharedAuthed.PATCH("/api/channels/:id", channelshndlr.ModifyChannel) // Modify one (partial update)
		}
	}

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
		if p := getPrincipal(c); p != nil {
			fields = append(fields, zap.Dict("auth", zap.String("kind", p.Kind), zap.String("uid", p.UID)))
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
