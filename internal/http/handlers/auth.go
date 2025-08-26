package handlers

import (
	"net/http"
	"time"

	"github.com/edirooss/zmux-server/internal/env"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type AuthHandler struct {
	log   *zap.Logger
	isDev bool
}

func NewAuthHandler(log *zap.Logger, isDev bool) *AuthHandler {
	return &AuthHandler{log.Named("auth"), isDev}
}

// Login authenticates a user and creates a new session.
func (h *AuthHandler) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := bind(c.Request, &req); err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	// TODO(auth): replace with real user lookup + password verify (bcrypt/argon2id).
	if !(req.Username == env.Admin.Username && req.Password == env.Admin.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "invalid credentials"})
		return
	}

	sess := sessions.Default(c)
	sess.Set("uid", req.Username)
	sess.Set("last_touch", time.Now().Unix())
	sess.Options(sessions.Options{
		Path:     "/api",   // scope cookie strictly to /api
		MaxAge:   4 * 3600, // session cookie lifetime (4h)
		Secure:   !h.isDev, // must be true behind HTTPS in prod
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	if err := sess.Save(); err != nil {
		c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.Status(http.StatusOK)
}

// Logout clears the current session.
func (h *AuthHandler) Logout(c *gin.Context) {
	sess := sessions.Default(c)
	sess.Clear()
	sess.Options(sessions.Options{
		Path:     "/api",
		MaxAge:   -1,
		Secure:   !h.isDev,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	_ = sess.Save()
	c.Status(http.StatusNoContent)
}

func (h *AuthHandler) Me(c *gin.Context) {
	sess := sessions.Default(c)
	uid, _ := sess.Get("uid").(string)
	if uid == "" {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	// Sliding TTL — refresh touch marker; store.Save() extends TTL
	now := time.Now().Unix()
	last, _ := sess.Get("last_touch").(int64)
	if last == 0 || now-last > 15*60 {
		sess.Set("last_touch", now)
		_ = sess.Save() // TODO(reliability): check error and log warning
	}

	c.JSON(http.StatusOK, gin.H{"uid": uid})
}
