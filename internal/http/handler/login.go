package handler

import (
	"net/http"
	"time"

	"github.com/edirooss/zmux-server/internal/principal"
	"github.com/edirooss/zmux-server/internal/service"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type LoginHandler struct {
	log   *zap.Logger
	svc   *service.AuthService
	isDev bool
}

func NewLoginHandler(log *zap.Logger, authsvc *service.AuthService, isDev bool) *LoginHandler {
	return &LoginHandler{log.Named("login"), authsvc, isDev}
}

// Login authenticates a user and creates a new session.
func (h *LoginHandler) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := bind(c.Request, &req); err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	p, ok := h.svc.ValidateUsernamePassword(req.Username, req.Password, principal.Login)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "invalid credentials"})
		return
	}
	principal.SetPrincipal(c, p)

	sess := sessions.Default(c)
	sess.Set("uid", p.ID)
	sess.Set("last_touch", time.Now().Unix())
	if err := sess.Save(); err != nil {
		c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.Status(http.StatusOK)
}

// Logout clears the current session.
func (h *LoginHandler) Logout(c *gin.Context) {
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
	p := principal.GetPrincipal(c)
	if p == nil {
		// No principal found — authentication middleware wasn’t applied
		c.Status(http.StatusUnauthorized)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":              p.ID,
		"principal_type":  p.PrincipalType.String(),
		"credential_type": p.CredentialType.String(),
	})
}
