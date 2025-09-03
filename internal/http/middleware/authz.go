package middleware

import (
	"net/http"
	"strconv"

	"github.com/edirooss/zmux-server/internal/domain/principal"
	"github.com/edirooss/zmux-server/internal/env"
	"github.com/edirooss/zmux-server/internal/service"
	"github.com/gin-gonic/gin"
)

// Authorization restricts access to the given principal kinds.
//
//   - 401 if no principal (unauthenticated)
//   - 403 if principal kind not in allowed set (unauthorized)
//
// Admins are always allowed.
func Authorization(auth *service.AuthService, kinds ...principal.PrincipalKind) gin.HandlerFunc {
	// precompute set for O(1) membership check
	allowed := make(map[principal.PrincipalKind]struct{}, len(kinds))
	for _, k := range kinds {
		allowed[k] = struct{}{}
	}

	return func(c *gin.Context) {
		p := auth.WhoAmI(c)
		if p == nil {
			c.AbortWithStatus(http.StatusUnauthorized) // no session/token → stop
			return
		}

		if p.Kind == principal.Admin {
			c.Next() // Admins bypass all restrictions
			return
		}

		if _, ok := allowed[p.Kind]; !ok {
			c.AbortWithStatus(http.StatusForbidden) // role not permitted
			return
		}

		c.Next()
	}
}

// RequireB2BClient allows only B2BClient principals.
//
//   - 401 if unauthenticated
//   - 422 if authenticated but not a B2B client (semantic mismatch)
func RequireB2BClient(auth *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := auth.WhoAmI(c)
		if p == nil {
			c.AbortWithStatus(http.StatusUnauthorized) // unauthenticated
			return
		}
		if p.Kind != principal.B2BClient {
			c.AbortWithStatus(http.StatusUnprocessableEntity) // not a b2b client
			return
		}
		c.Next()
	}
}

// AuthorizeChannelIDAccess ensures a B2B client is bound to the requested channel ID.
//
//   - 401 if unauthenticated
//   - 403 if not authorized for the channel
//
// Admins always bypass this check.
func AuthorizeChannelIDAccess(auth *service.AuthService, channels env.B2BClientChannelIDs) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := auth.WhoAmI(c)
		if p == nil {
			c.AbortWithStatus(http.StatusUnauthorized) // no principal found
			return
		}

		if p.Kind == principal.Admin {
			c.Next() // Admins always allowed
			return
		}
		if p.Kind != principal.B2BClient {
			c.AbortWithStatus(http.StatusForbidden) // wrong role
			return
		}

		// extract :id param (route param already validated by other middleware)
		id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		if !channels.Has(p.ID, id) {
			c.AbortWithStatus(http.StatusForbidden) // client not bound to this channel
			return
		}

		c.Next()
	}
}
