package principal

type Kind int     // principal kind (admin|service_account)
type AuthType int // principal auth type (session|basic|bearer)

type Principal struct {
	ID       string
	Kind     Kind
	AuthType AuthType
}

const (
	Admin          Kind = iota // Zmux Admin
	ServiceAccount             // Zmux Service Account
)

const (
	SessionAuth AuthType = iota // Auth via session cookie
	BasicAuth                   // Auth via http basic (username/password)
	BearerAuth                  // Auth via http bearer (token)
)

func (k Kind) String() string {
	switch k {
	case Admin:
		return "admin"
	case ServiceAccount:
		return "service_account"
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
