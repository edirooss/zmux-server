package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/edirooss/zmux-server/internal/principal"
	"github.com/edirooss/zmux-server/internal/service"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// Authentication allows access if either valid Basic credentials, valid session, or valid Bearer token exists.
// Responds with 401 Unauthorized if none validate.
func Authentication(svc *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if isBasicAuthenticated(c, svc) || isSessionAuthenticated(c, svc) || isBearerTokenValid(c, svc) {
			c.Next()
			return
		}
		c.AbortWithStatus(http.StatusUnauthorized)
	}
}

// isBasicAuthenticated checks the HTTP request for Basic Authentication credentials.
func isBasicAuthenticated(c *gin.Context, svc *service.AuthService) bool {
	user, pass, hasAuth := c.Request.BasicAuth()
	if hasAuth {
		if p, ok := svc.ValidateUsernamePassword(user, pass, principal.Basic); ok {
			principal.SetPrincipal(c, p)
			return true
		}
	}
	return false
}

// isSessionAuthenticated returns true if the session is valid.
// Also updates the session's "last_touch" timestamp if older than 15 minutes.
func isSessionAuthenticated(c *gin.Context, svc *service.AuthService) bool {
	session := sessions.Default(c)
	userID, _ := session.Get("uid").(string)
	p, ok := svc.ValidateSession(userID)
	if !ok {
		return false
	}
	principal.SetPrincipal(c, p)

	const sessionTTL = 15 * 60 // 15 minutes
	now := time.Now().Unix()
	lastTouch, _ := session.Get("last_touch").(int64)
	if lastTouch == 0 || now-lastTouch > sessionTTL {
		session.Set("last_touch", now)
		_ = session.Save()
	}

	return true
}

// isBearerTokenValid validates Authorization: Bearer <token> against a demo secret.
// TODO(bearer): replace with lookup+hash compare for production.
func isBearerTokenValid(c *gin.Context, svc *service.AuthService) bool {
	h := c.GetHeader("Authorization")
	if h == "" {
		return false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return false
	}
	token := strings.TrimSpace(strings.TrimPrefix(h, prefix))
	if token == "" {
		return false
	}
	// constant-time compare to avoid timing side channels
	if p, ok := svc.ValidateBearerToken(token); ok {
		principal.SetPrincipal(c, p)
		return true
	}
	return false
}
