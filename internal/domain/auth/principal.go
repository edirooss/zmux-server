package auth

import "github.com/gin-gonic/gin"

//
// ─── TYPES ─────────────────────────────────────────────────────────────────────
//

type PrincipalKind int

type AuthType int

type Permission string

type PermissionSet map[Permission]bool

type Principal struct {
	ID            string
	AuthType      AuthType
	PrincipalKind PrincipalKind
	PermissionSet PermissionSet
}

//
// ─── CONSTANTS ─────────────────────────────────────────────────────────────────
//

const (
	AdminKind   PrincipalKind = iota // Zmux Admin User
	ServiceKind                      // Zmux Service Account
)

const (
	BasicAuth   AuthType = iota // Basic auth
	SessionAuth                 // Session cookie
	BearerAuth                  // Bearer token
)

const (
	PermAdmin         Permission = "*"
	PermChannelCreate Permission = "channel:create"
	PermChannelRead   Permission = "channel:read"
	PermChannelUpdate Permission = "channel:update"
	PermChannelDelete Permission = "channel:delete"
)

//
// ─── STRING CONVERTERS ─────────────────────────────────────────────────────────
//

func (k PrincipalKind) String() string {
	switch k {
	case AdminKind:
		return "user"
	case ServiceKind:
		return "service"
	default:
		return "unknown"
	}
}

func (a AuthType) String() string {
	switch a {
	case BasicAuth:
		return "basic"
	case SessionAuth:
		return "session"
	case BearerAuth:
		return "bearer"
	default:
		return "unknown"
	}
}

//
// ─── PRINCIPAL HELPERS ─────────────────────────────────────────────────────────
//

func (p *Principal) Has(perm Permission) bool {
	return p != nil && p.PermissionSet[perm]
}

func (p *Principal) GetPermissions() []string {
	if p == nil || len(p.PermissionSet) == 0 {
		return []string{}
	}

	perms := make([]string, 0, len(p.PermissionSet))
	for k, v := range p.PermissionSet {
		if v {
			perms = append(perms, string(k))
		}
	}
	return perms
}

//
// ─── PERMISSION SET UTILS ──────────────────────────────────────────────────────
//

func NewPermissionSet(perms ...Permission) PermissionSet {
	set := make(PermissionSet, len(perms))
	for _, perm := range perms {
		set[perm] = true
	}
	return set
}

//
// ─── CONTEXT HELPERS ───────────────────────────────────────────────────────────
//

const principalKey = "auth.principal"

func SetPrincipal(c *gin.Context, id string, authType AuthType, kind PrincipalKind, perms PermissionSet) {
	c.Set(principalKey, &Principal{
		ID:            id,
		AuthType:      authType,
		PrincipalKind: kind,
		PermissionSet: perms,
	})
}

func GetPrincipal(c *gin.Context) *Principal {
	if v, ok := c.Get(principalKey); ok {
		if p, ok := v.(*Principal); ok {
			return p
		}
	}
	return nil
}
