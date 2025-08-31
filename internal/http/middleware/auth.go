package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"github.com/edirooss/zmux-server/internal/domain/auth"
	"github.com/edirooss/zmux-server/internal/env"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// Authentication allows access if either valid Basic credentials, valid session, or valid Bearer token exists.
// Responds with 401 Unauthorized if none validate.
func Authentication(c *gin.Context) {
	if isBasicAuthenticated(c) || isSessionAuthenticated(c) || isBearerTokenValid(c) {
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

// isBearerTokenValid validates Authorization: Bearer <token> against a demo secret.
// TODO(bearer): replace with lookup+hash compare for production.
func isBearerTokenValid(c *gin.Context) bool {
	h := c.GetHeader("Authorization")
	if h == "" {
		return false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return false
	}
	token := strings.TrimSpace(strings.TrimPrefix(h, prefix))
	demoBearerToken := "sk_test_2vV7Q2hksN8KzLpXWq3jUm5Ay4oRxE9b"
	if token == "" || demoBearerToken == "" {
		return false
	}
	// constant-time compare to avoid timing side channels
	if subtle.ConstantTimeCompare([]byte(token), []byte(demoBearerToken)) == 1 {
		auth.SetPrincipal(c, &auth.Principal{Kind: auth.Token, ID: redactToken(token)})
		return true
	}
	return false
}

// redactToken keeps logs safe while still traceable.
func redactToken(tok string) string {
	if len(tok) <= 8 {
		return "****"
	}
	return tok[:4] + "…" + tok[len(tok)-4:]
}
