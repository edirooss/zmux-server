package principal

import (
	"encoding/json"
	"fmt"
)

type PrincipalKind int // principal kind (admin|service_account)

const (
	Admin          PrincipalKind = iota // Zmux Admin
	ServiceAccount                      // Zmux Service Account
)

func (k PrincipalKind) String() string {
	switch k {
	case Admin:
		return "admin"
	case ServiceAccount:
		return "service_account"
	default:
		return "unknown"
	}
}

// MarshalJSON makes PrincipalKind serialize as string
func (k PrincipalKind) MarshalJSON() ([]byte, error) {
	return json.Marshal(k.String())
}

// UnmarshalJSON makes PrincipalKind deserialize from string
func (k *PrincipalKind) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch s {
	case "admin":
		*k = Admin
	case "service_account":
		*k = ServiceAccount
	default:
		return fmt.Errorf("invalid PrincipalKind: %s", s)
	}
	return nil
}

type Principal struct {
	ID   string        `json:"id"`   // user id for admins, account id for service accounts
	Kind PrincipalKind `json:"kind"` // string marshaled
}
