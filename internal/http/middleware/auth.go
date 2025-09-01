package middleware

import (
	"net/http"
	"strings"

	"github.com/edirooss/zmux-server/internal/service"
	"github.com/gin-gonic/gin"
)

// Authentication allows access if any of the following succeed:
// - Session-based user ID
// - Basic Auth credentials
// - Bearer token
//
// Responds with 401 Unauthorized if all checks fail.
func Authentication(svc *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if isSessionAuthenticated(c, svc) || isBasicAuthenticated(c, svc) || isBearerTokenAuthenticated(c, svc) {
			c.Next()
			return
		}
		c.AbortWithStatus(http.StatusUnauthorized)
	}
}

// isSessionAuthenticated authenticates using internal user ID set on a session (cookie-based).
func isSessionAuthenticated(c *gin.Context, svc *service.AuthService) bool {
	_, ok := svc.AuthenticateWithSession(c)
	return ok
}

// isBasicAuthenticated authenticates using username password from HTTP Basic Auth.
func isBasicAuthenticated(c *gin.Context, svc *service.AuthService) bool {
	user, pass, valid := c.Request.BasicAuth()
	if valid {
		_, ok := svc.AuthenticateWithPassword(c, user, pass)
		return ok
	}
	return false
}

// isBearerTokenAuthenticated authenticates using a token from Bearer authorization.
func isBearerTokenAuthenticated(c *gin.Context, svc *service.AuthService) bool {
	h := c.GetHeader("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return false
	}
	token := strings.TrimSpace(strings.TrimPrefix(h, prefix))
	if token == "" {
		return false
	}

	_, ok := svc.AuthenticateWithBearerToken(c, token)
	return ok
}
