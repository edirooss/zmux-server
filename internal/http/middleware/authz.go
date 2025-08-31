package middleware

import (
	"net/http"

	"github.com/edirooss/zmux-server/internal/principal"
	"github.com/gin-gonic/gin"
)

// Authorization returns middleware that permits access only if the authenticated
// Principal's kind is in the allowed list. Otherwise responds with 403 Forbidden.
func Authorization(allowed ...principal.Kind) gin.HandlerFunc {
	allowedSet := make(map[principal.Kind]struct{}, len(allowed))
	for _, k := range allowed {
		allowedSet[k] = struct{}{}
	}

	return func(c *gin.Context) {
		p := principal.GetPrincipal(c)
		if p == nil {
			// No principal found — authentication middleware wasn’t applied
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		if _, ok := allowedSet[p.PrincipalType]; !ok {
			// Authenticated but not authorized
			c.AbortWithStatus(http.StatusForbidden)
			return
		}

		c.Next()
	}
}
