package middleware

import (
	"net/http"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// RequireSessionAuth blocks requests without a valid session.
//
// - Aborts with 401 Unauthorized otherwise.
func RequireSessionAuth(c *gin.Context) {
	sess := sessions.Default(c)
	uid, _ := sess.Get("uid").(string)
	if uid == "" {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	// Sliding TTL — refresh touch marker; store.Save() extends TTL
	now := time.Now().Unix()
	last, _ := sess.Get("last_touch").(int64)
	if last == 0 || now-last > 15*60 {
		sess.Set("last_touch", now)
		_ = sess.Save() // TODO(reliability): check error and log warning
	}
	c.Next()
}
