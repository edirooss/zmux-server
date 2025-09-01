package service

import (
	"crypto/subtle"

	"github.com/edirooss/zmux-server/internal/domain/principal"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

type contextKey string

const principalKey contextKey = "auth.principal"

// AuthService handles authentication logic.
type AuthService struct {
	UsrSessionSvc *UserSessionService
}

// NewAuthService creates a new AuthService.
func NewAuthService(sessionService *UserSessionService) *AuthService {
	return &AuthService{UsrSessionSvc: sessionService}
}

// AuthenticateWithPassword authenticates using username and password.
// On success, it sets and returns the Principal.
func (s *AuthService) AuthenticateWithPassword(c *gin.Context, username, password string) (*principal.Principal, bool) {
	if username == "hozi" && password == "Zz1234$#@!" {
		p := &principal.Principal{
			ID:   "hozi",
			Kind: principal.Admin,
		}
		s.setPrincipal(c, p)
		return p, true
	}
	return nil, false
}

// AuthenticateWithSession reads session from context and authenticates user ID.
func (s *AuthService) AuthenticateWithSession(c *gin.Context) (*principal.Principal, bool) {
	session := sessions.Default(c)
	uid, ok := s.UsrSessionSvc.GetUserID(session)
	if !ok {
		return nil, false
	}

	if uid == "hozi" {
		p := &principal.Principal{
			ID:   "hozi",
			Kind: principal.Admin,
		}
		s.setPrincipal(c, p)
		return p, true
	}
	return nil, false
}

// AuthenticateWithBearerToken authenticates using a bearer token.
func (s *AuthService) AuthenticateWithBearerToken(c *gin.Context, token string) (*principal.Principal, bool) {
	if subtle.ConstantTimeCompare([]byte(token), []byte("svc_test_2vV7Q2hksN8KzLpXWq3jUm5Ay4oRxE9b")) == 1 {
		p := &principal.Principal{
			ID:   "service-account-test-1",
			Kind: principal.ServiceAccount,
		}
		s.setPrincipal(c, p)
		return p, true
	}
	return nil, false
}

// WhoAmI returns the authenticated Principal from the Gin context.
// Returns nil if no principal is set.
func (s *AuthService) WhoAmI(c *gin.Context) *principal.Principal {
	if v, ok := c.Get(string(principalKey)); ok {
		if p, ok := v.(*principal.Principal); ok {
			return p
		}
	}
	return nil
}

// setPrincipal attaches the Principal to the Gin context (private).
func (s *AuthService) setPrincipal(c *gin.Context, p *principal.Principal) {
	c.Set(string(principalKey), p)
}
