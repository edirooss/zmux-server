package middleware

import (
	"net/http"
	"time"

	"github.com/edirooss/zmux-server/internal/env"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// RequireBasicAuthOrSession blocks access unless a valid basic auth credentials or valid session exists.
// Responds with 401 Unauthorized if not authenticated.
func RequireBasicAuthOrSession(c *gin.Context) {
	if !isBasicAuthenticated(c) && !isSessionAuthenticated(c) {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	c.Next()
}

// RequireAPIKey blocks access unless a valid API key is provided.
// Responds with 401 Unauthorized if not authenticated.
func RequireAPIKey(c *gin.Context) {
	if !isAPIKeyValid(c) {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	c.Next()
}

// RequireAuth allows access if either a valid basic auth credentials exists or valid session exists or a valid API key is exists.
// Responds with 401 Unauthorized if both checks fail.
func RequireAuth(c *gin.Context) {
	if isBasicAuthenticated(c) || isSessionAuthenticated(c) || isAPIKeyValid(c) {
		c.Next()
		return
	}
	c.AbortWithStatus(http.StatusUnauthorized)
}

const contextKeySessionAuth = "SessionAuthenticated"

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

	c.Set(contextKeySessionAuth, struct{}{})
	return true
}

// isBasicAuthenticated checks the HTTP request for Basic Authentication credentials.
func isBasicAuthenticated(c *gin.Context) bool {
	user, pass, hasAuth := c.Request.BasicAuth()
	return hasAuth && user == env.Admin.Username && pass == env.Admin.Password
}

// isAPIKeyValid checks if the X-API-Key header matches the expected value.
// TODO: Replace with real validation logic.
func isAPIKeyValid(c *gin.Context) bool {
	apiKey := c.GetHeader("X-API-Key")
	return apiKey == "test-apikey-1234"
}
