package principal

type Kind int           // principal kind (admin|service_account)
type CredentialType int // principal credential type (login|session|basic|bearer)

type Principal struct {
	ID             string // user id for admins, account id for service accounts
	PrincipalType  Kind
	CredentialType CredentialType
}

const (
	Admin          Kind = iota // Zmux Admin
	ServiceAccount             // Zmux Service Account
)

const (
	Login   CredentialType = iota // Auth via login form (username/password)
	Session                       // Auth via cookie-based session
	Basic                         // Auth via http basic (username/password)
	Bearer                        // Auth via http bearer (token)
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

func (a CredentialType) String() string {
	switch a {
	case Login:
		return "login"
	case Session:
		return "session"
	case Basic:
		return "basic"
	case Bearer:
		return "bearer"
	default:
		return "unknown"
	}
}
