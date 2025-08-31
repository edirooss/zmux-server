package handlers

import (
	"net/http"
	"time"

	"github.com/edirooss/zmux-server/internal/domain/auth"
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
	uid := req.Username

	sess := sessions.Default(c)
	sess.Set("uid", uid)
	sess.Set("last_touch", time.Now().Unix())
	if err := sess.Save(); err != nil {
		c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	auth.SetPrincipal(c, &auth.Principal{Kind: auth.Session, ID: uid})
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

func Me(c *gin.Context) {
	p := auth.GetPrincipal(c)
	if p == nil {
		// No principal found — authentication middleware wasn’t applied
		c.Status(http.StatusUnauthorized)
		return
	}

	c.JSON(http.StatusOK, gin.H{"kind": p.Kind.String(), "id": p.ID})
}
