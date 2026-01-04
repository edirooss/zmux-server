package service

import (
	"fmt"
	"strconv"

	"github.com/edirooss/zmux-server/internal/domain/principal"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type contextKey string

const principalKey contextKey = "auth.principal"

// AuthService handles authentication logic.
type AuthService struct {
	log         *zap.Logger
	UserSession *UserSessionService
	b2bsvc      *B2BClientService
}

// NewAuthService creates a new AuthService.
func NewAuthService(log *zap.Logger, isDev bool, b2bsvc *B2BClientService, redisAddr string) (*AuthService, error) {
	log = log.Named("auth")
	usersesssvc, err := NewUserSessionService(isDev, redisAddr)
	if err != nil {
		return nil, fmt.Errorf("new user session service: %w", err)
	}

	return &AuthService{log: log, UserSession: usersesssvc, b2bsvc: b2bsvc}, nil
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
	uid, ok := s.UserSession.GetUserID(session)
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
// Looks up principal by bearer token in Redis and sets it on the request context.
// DEV: No constant-time compare here—token is used as a Redis key; errors are logged and treated as auth failures.
func (s *AuthService) AuthenticateWithBearerToken(c *gin.Context, token string) (*principal.Principal, bool) {
	b2bClient, ok := s.b2bsvc.LookupByToken(token)
	if !ok {
		return nil, false
	}
	p := &principal.Principal{
		ID:   strconv.FormatInt(b2bClient.ID, 10),
		Kind: principal.B2BClient,
	}
	s.setPrincipal(c, p)
	return p, true
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
