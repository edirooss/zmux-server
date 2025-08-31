package service

import (
	"crypto/subtle"

	"github.com/edirooss/zmux-server/internal/principal"
)

// AuthService handles authentication logic
type AuthService struct{}

// NewAuthService creates a new auth service
func NewAuthService() *AuthService {
	return &AuthService{}
}

// ValidateUsernamePassword validates username/password (used credentials for auth via login form and basic)
func (s *AuthService) ValidateUsernamePassword(username, password string, ct principal.CredentialType) (*principal.Principal, bool) {
	if username == "hozi" && password == "Zz1234$#@!" {
		return &principal.Principal{
			ID:             "admin-1",
			CredentialType: ct,
			PrincipalType:  principal.Admin,
		}, true
	}
	return nil, false
}

// ValidateSession validates a session user ID (from session store)
func (s *AuthService) ValidateSession(uid string) (*principal.Principal, bool) {
	if uid == "admin-1" {
		return &principal.Principal{
			ID:             "admin-1",
			CredentialType: principal.Session,
			PrincipalType:  principal.Admin,
		}, true
	}
	return nil, false
}

// ValidateBearerToken validates a bearer token
func (s *AuthService) ValidateBearerToken(token string) (*principal.Principal, bool) {
	if subtle.ConstantTimeCompare([]byte(token), []byte("sk_test_2vV7Q2hksN8KzLpXWq3jUm5Ay4oRxE9b")) == 1 {
		return &principal.Principal{
			ID:             "service-account-1",
			CredentialType: principal.Bearer,
			PrincipalType:  principal.ServiceAccount,
		}, true
	}
	return nil, false
}
