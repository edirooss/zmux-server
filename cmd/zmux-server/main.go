package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	log = log.Named("main")

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

	gin.SetMode(gin.ReleaseMode)

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

	r.POST("/api/channelsCOPY", enforceContentType("application/json; charset=utf-8"), func(c *gin.Context) {
		// Cap request size
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 5 /* 100*1024 100 KiB */)
		defer c.Request.Body.Close()

		dec := json.NewDecoder(c.Request.Body)
		dec.DisallowUnknownFields()

		var req channelmodel.CreateZmuxChannelReq
		for {
			// Decode exactly one JSON value with unknown-field rejection
			if err := dec.Decode(&req); err != nil {
				if err == io.EOF {
					break
				}

				if errors.As(err, new(*http.MaxBytesError)) {
					c.JSON(http.StatusRequestEntityTooLarge, gin.H{"message": err.Error()})
					return
				}

				c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
				return
			}
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

	r.POST("/api/channels1", enforceContentType("application/json; charset=utf-8"), func(c *gin.Context) {

		// Cap request size
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 0)
		defer func() {
			// should i add here draining?
			c.Request.Body.Close()
		}()

		dec := json.NewDecoder(c.Request.Body)
		var req channelmodel.CreateZmuxChannelReq

		// decode one JSON value;
		// note: .Decode() skips leading and trailing whitespace
		if err := dec.Decode(&req); err != nil {
			if errors.As(err, new(*http.MaxBytesError)) {
				c.Error(err)
				c.Header("Connection", "close")
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{"message": err.Error()})
				return
			}

			c.Error(err)
			c.Header("Connection", "close")
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		// ensure EOF
		if err := func() error {
			// try to decode another JSON value; extra syntax is rejected
			if dec.Decode(&struct{}{}) != io.EOF {
				return errors.New("expected EOF (trailing content not allowed)")
			}
			return nil
		}(); err != nil {
			c.Error(err)
			c.Header("Connection", "close")
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}

		// Business logic on req, then get chan
		ch := channelmodel.NewZmuxChannel(0)
		ch.Name = req.Name.Value()

		c.Header("Location", fmt.Sprintf("/api/channels/%d", ch.ID))
		c.JSON(http.StatusCreated, ch)
	})

	r.POST("/api/channels", withBodyCap(0), func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 5)
		_, err := io.ReadAll(c.Request.Body)

		if err != nil {
			c.Error(err)
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}

		// Business logic on req, then get chan
		ch := channelmodel.NewZmuxChannel(0)
		// ch.Name = req.Name.Value()

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
		if err := jsonx.ParseJSONObject(io.Reader(c.Request.Body), &req); err != nil { /* schema mismatch: malformed JSON, unknown fields, missing required fields, wrong data type at JSON level */
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

	httpserver := &http.Server{
		Addr:    "127.0.0.1:8080",
		Handler: r, // <- gin.Engine satisfies http.Handler

		// Configure timeouts (by default: it’s all basically “infinite timeouts”)
		ReadTimeout:  10 * time.Second, // → max time the server will wait to read the request (headers + body).
		WriteTimeout: 15 * time.Second, // → max time the server will wait to write the response to the client.
		IdleTimeout:  60 * time.Second, // → max time an idle keep-alive connection sits open between requests.

		// Header size constraint
		MaxHeaderBytes: 1 << 15, // 32 KB

		// Attach zap's logger
		ErrorLog: zap.NewStdLog(log.Named("http").WithOptions(zap.AddCallerSkip(1))),
	}

	log.Info("running HTTP server on 127.0.0.1:8080")
	if err := httpserver.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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

func peekByte(br *bufio.Reader, allowed ...byte) error {
	b, err := br.Peek(1)
	if len(allowed) == 0 /* expecting EOF */ {
		if err == io.EOF {
			return nil
		}
		// unexpected read failure
		if err != nil {
			return err
		}
		// unexpected read success
		return errors.New("trailing bytes not allowed")
	}
	// expecting allowed bytes

	if err != nil {
		return err // bubble up io.EOF or other error
	}

	next := b[0]
	for _, ch := range allowed {
		if next == ch {
			return nil // valid byte
		}
	}

	return errors.New("unexpected byte")
}

func enforceContentType(args ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		contentType := c.GetHeader("Content-Type")
		if err := checkContentType(contentType, args...); err != nil {
			_ = c.Error(err)
			c.AbortWithStatusJSON(http.StatusUnsupportedMediaType, gin.H{"message": err.Error()})
			return
		}
		c.Next()
	}
}

func checkContentType(contentType string, args ...string) error {
	// Optional: parse media type using `mime.ParseMediaType`; tolerate charset, etc.
	// Match against allowed list
	for _, allowed := range args {
		if contentType == allowed {
			return nil
		}
	}
	return errors.New("invalid content type")
}

func withBodyCap(maxBodyBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
		c.Next()
	}
}
