package middleware

import (
	"net/http"
	"time"

	"github.com/edirooss/zmux-server/internal/domain/auth"
	"github.com/edirooss/zmux-server/internal/env"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// Authentication allows access if either a valid basic auth credentials exists or valid session exists or a valid API key is exists.
// Responds with 401 Unauthorized if both checks fail.
func Authentication(c *gin.Context) {
	if isBasicAuthenticated(c) || isSessionAuthenticated(c) || isAPIKeyValid(c) {
		c.Next()
		return
	}
	c.AbortWithStatus(http.StatusUnauthorized)
}

// isBasicAuthenticated checks the HTTP request for Basic Authentication credentials.
func isBasicAuthenticated(c *gin.Context) bool {
	user, pass, hasAuth := c.Request.BasicAuth()
	if hasAuth && user == env.Admin.Username && pass == env.Admin.Password {
		auth.SetPrincipal(c, &auth.Principal{Kind: auth.Basic, ID: user})
		return true
	}
	return false
}

// isSessionAuthenticated returns true if the session is valid.
// Also updates the session's "last_touch" timestamp if older than 15 minutes.
func isSessionAuthenticated(c *gin.Context) bool {
	session := sessions.Default(c)
	userID, _ := session.Get("uid").(string)
	if userID == "" {
		return false
	}

	const sessionTTL = 15 * 60 // 15 minutes
	now := time.Now().Unix()
	lastTouch, _ := session.Get("last_touch").(int64)
	if lastTouch == 0 || now-lastTouch > sessionTTL {
		session.Set("last_touch", now)
		_ = session.Save()
	}

	auth.SetPrincipal(c, &auth.Principal{Kind: auth.Session, ID: userID})
	return true
}

// isAPIKeyValid checks if the X-API-Key header matches the expected value.
// TODO: Replace with real validation logic.
func isAPIKeyValid(c *gin.Context) bool {
	apiKey := c.GetHeader("X-API-Key")
	if apiKey == "test-apikey-1234" {
		auth.SetPrincipal(c, &auth.Principal{Kind: auth.APIKey, ID: apiKey})
		return true

	}
	return false
}

// AuthorizedAuth returns middleware that permits access only if the authenticated
// Principal's Kind is in the allowed list. Otherwise responds with 403 Forbidden.
func AuthorizedAuth(allowed ...auth.Kind) gin.HandlerFunc {
	allowedSet := make(map[auth.Kind]struct{}, len(allowed))
	for _, k := range allowed {
		allowedSet[k] = struct{}{}
	}

	return func(c *gin.Context) {
		p := auth.GetPrincipal(c)
		if p == nil {
			// No principal found — authentication middleware wasn’t applied
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		if _, ok := allowedSet[p.Kind]; !ok {
			// Authenticated but not authorized
			c.AbortWithStatus(http.StatusForbidden)
			return
		}

		c.Next()
	}
}
