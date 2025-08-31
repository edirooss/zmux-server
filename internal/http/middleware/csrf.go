package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/edirooss/zmux-server/internal/principal"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// ValidateSessionCSRF checks CSRF tokens for session-authenticated requests.
//
//   - Skips validation for non-session-authenticated requests (e.g. Basic, API key).
//   - Applies only to mutating methods (POST, PUT, PATCH, DELETE).
//   - Aborts with 400 Bad Request if the token is missing or invalid.
func ValidateSessionCSRF(c *gin.Context) {
	// Skip for non session-authenticated requests
	if p := principal.GetPrincipal(c); p != nil && p.CredentialType != principal.Session {
		c.Next()
		return
	}

	// Skip if method is not mutating
	switch c.Request.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		// continue
	default:
		c.Next()
		return
	}

	want, _ := sessions.Default(c).Get("csrf").(string)
	got := c.GetHeader("X-CSRF-Token")

	if want == "" || got == "" ||
		subtle.ConstantTimeCompare([]byte(want), []byte(got)) != 1 {
		c.AbortWithStatusJSON(http.StatusBadRequest,
			gin.H{"message": "invalid csrf token"})
		return
	}

	c.Next()
}
