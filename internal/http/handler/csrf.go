package handler

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// IssueSessionCSRF issues a CSRF token for the current session.
//
//   - Creates one if missing and stores it in the session.
//   - Returns the token in JSON with cache disabled.
func IssueSessionCSRF(c *gin.Context) {
	sess := sessions.Default(c)
	token, _ := sess.Get("csrf").(string)
	if token == "" {
		token = randomTokenHex(32)
		sess.Set("csrf", token)
		_ = sess.Save()
	}

	// Avoid cache serving stale tokens
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
	c.JSON(http.StatusOK, gin.H{"csrf": token})
}

func randomTokenHex(nBytes int) string {
	b := make([]byte, nBytes)
	// TODO(security,bug): handle rand.Read error; on failure, return 500 rather than silently issuing a weak token.
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
