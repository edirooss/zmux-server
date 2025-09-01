package handler

import (
	"net/http"

	"github.com/edirooss/zmux-server/internal/service"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type UserSessionsHandler struct {
	log *zap.Logger
	svc *service.AuthService
}

func NewUserSessionsHandler(log *zap.Logger, authsvc *service.AuthService) *UserSessionsHandler {
	return &UserSessionsHandler{log.Named("usr_sessions"), authsvc}
}

// Login authenticates a user and creates a new session.
func (h *UserSessionsHandler) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := bind(c.Request, &req); err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	p, ok := h.svc.AuthenticateWithPassword(c, req.Username, req.Password)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "invalid credentials"})
		return
	}

	s := sessions.Default(c)
	if err := h.svc.UsrSessionSvc.SetUserSession(s, p.ID); err != nil {
		c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.Status(http.StatusOK)
}

// Logout clears the current session.
func (h *UserSessionsHandler) Logout(c *gin.Context) {
	s := sessions.Default(c)
	_ = h.svc.UsrSessionSvc.ClearUserSession(s)
	c.Status(http.StatusNoContent)
}
