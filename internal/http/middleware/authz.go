package middleware

import (
	"net/http"

	"github.com/edirooss/zmux-server/internal/domain/auth"
	"github.com/gin-gonic/gin"
)

// Authorization returns middleware that permits access only if the authenticated
// Principal's auth type is in the allowed list. Otherwise responds with 403 Forbidden.
func Authorization(allowed ...auth.AuthType) gin.HandlerFunc {
	allowedSet := make(map[auth.AuthType]struct{}, len(allowed))
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

		if _, ok := allowedSet[p.AuthType]; !ok {
			// Authenticated but not authorized
			c.AbortWithStatus(http.StatusForbidden)
			return
		}

		c.Next()
	}
}
