package middleware

import (
	"net/http"
	"strconv"

	"github.com/edirooss/zmux-server/internal/domain/principal"
	"github.com/edirooss/zmux-server/internal/env"
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

// ServiceAccountOnly permits only ServiceAccount principals.
// - 401 if no principal (auth missing)
// - 422 if principal exists but is not applicable (e.g., Admin trying a service-account-only endpoint)
func ServiceAccountOnly(authsvc *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := authsvc.WhoAmI(c)
		if p == nil {
			// Unauthenticated
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		if p.Kind != principal.ServiceAccount {
			// Authenticated but the endpoint is not relevant/applicable to this principal kind
			// Using 422 Unprocessable Content to signal semantic mismatch
			c.AbortWithStatus(http.StatusUnprocessableEntity)
			return
		}

		c.Next()
	}
}

// RequireChannelIDAccess enforces that a service account is bound
// to the channel ID. Admin principals bypass this check entirely.
func RequireChannelIDAccess(authsvc *service.AuthService, idx env.ServiceAccountChannelIDs) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := authsvc.WhoAmI(c)
		if p == nil {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		// Admins always allowed
		if p.Kind == principal.Admin {
			c.Next()
			return
		}

		// Must be a service account
		if p.Kind != principal.ServiceAccount {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}

		id, _ := strconv.ParseInt(c.Param("id"), 10, 64) // extract :id (already validated by middleware)
		if !idx.Has(p.ID, id) {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}

		c.Next()
	}
}
