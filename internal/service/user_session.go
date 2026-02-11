package service

import (
	"fmt"
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/redis"
	"github.com/gin-gonic/gin"
)

// UserSessionService manages user sessions backed by Redis.
type UserSessionService struct {
	store         redis.Store
	cookieOptions sessions.Options
}

// sessionKeyUserID is the key used to store and retrieve the user ID in the session.
// It is used internally by UserSessionService methods like SetUserSession and GetUserID.
const sessionKeyUserID = "uid"

// NewUserSessionService creates a new UserSessionService.
// The `isDev` flag controls whether cookies are marked Secure.
func NewUserSessionService(isDev bool, redisAddr string) (*UserSessionService, error) {
	// Create Redis session store
	store, err := redis.NewStoreWithDB(10, "tcp", redisAddr, "", "", "0",
		[]byte("nZCowo9+aofuYO/54sK2mca+aj8M9XA2zVLrP1kh6uk=") /* TODO(security): rotate key */)
	if err != nil {
		return nil, fmt.Errorf("new store: %w", err)
	}

	cookieOptions := sessions.Options{
		Path:     "/api",
		MaxAge:   4 * 3600,
		Secure:   !isDev,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
	store.Options(cookieOptions)

	return &UserSessionService{store: store, cookieOptions: cookieOptions}, nil
}

// Middleware attaches session handling.
func (s *UserSessionService) Middleware() gin.HandlerFunc {
	return sessions.Sessions("sid" /* Cookie name */, s.store)
}

// SetUserSession stores the given user ID in the session and persists it.
func (s *UserSessionService) SetUserSession(session sessions.Session, uid string) error {
	session.Set(sessionKeyUserID, uid)

	if err := session.Save(); err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return nil
}

// ClearUserSession clears all session data and expires the cookie.
func (s *UserSessionService) ClearUserSession(session sessions.Session) error {
	session.Clear()

	opts := s.cookieOptions
	opts.MaxAge = -1
	session.Options(opts)

	if err := session.Save(); err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return nil
}

// GetUserID returns the user ID from the given session.
// It reports false if no valid user ID is present.
func (s *UserSessionService) GetUserID(session sessions.Session) (string, bool) {
	uid, ok := session.Get(sessionKeyUserID).(string)
	if !ok || uid == "" {
		return "", false
	}
	return uid, true
}
