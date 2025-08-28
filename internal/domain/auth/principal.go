package auth

import "github.com/gin-gonic/gin"

// Kind represents the type of principal — basic, session or API key.
type Kind int

const (
	Basic   Kind = iota // basic
	Session             // session
	APIKey              // apikey
)

func (k Kind) String() string {
	switch k {
	case Basic:
		return "basic"
	case Session:
		return "session"
	case APIKey:
		return "apikey"
	default:
		return ""
	}
}

// Principal represents an authenticated entity with a kind and ID.
type Principal struct {
	Kind Kind
	ID   string
}

// principalKey is the key used to store/retrieve Principal in gin.Context.
const principalKey = "auth.principal"

// SetPrincipal attaches a Principal to the gin.Context.
func SetPrincipal(c *gin.Context, p *Principal) {
	c.Set(principalKey, p)
}

// GetPrincipal retrieves the Principal from the gin.Context.
// Returns nil if none is set.
func GetPrincipal(c *gin.Context) *Principal {
	if v, ok := c.Get(principalKey); ok {
		if p, ok := v.(*Principal); ok {
			return p
		}
	}
	return nil
}
