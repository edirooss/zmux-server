package middleware

import (
	"net/http"

	"github.com/edirooss/zmux-server/internal/domain/principal"
	"github.com/edirooss/zmux-server/internal/service"
	"github.com/gin-gonic/gin"
)

// Authorization returns middleware that permits access only if the authenticated
// Principal's kind is in the allowed list. Otherwise responds with 403 Forbidden.
func Authorization(authsvc *service.AuthService, allowed ...principal.PrincipalKind) gin.HandlerFunc {
	allowedSet := make(map[principal.PrincipalKind]struct{}, len(allowed))
	for _, k := range allowed {
		allowedSet[k] = struct{}{}
	}

	return func(c *gin.Context) {
		p := authsvc.WhoAmI(c)
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
