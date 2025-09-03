package principal

import (
	"encoding/json"
	"fmt"
)

type PrincipalKind int // principal kind (admin|b2b_client)

const (
	Admin     PrincipalKind = iota // Zmux Admins
	B2BClient                      // Zmux B2B Clients
)

func (k PrincipalKind) String() string {
	switch k {
	case Admin:
		return "admin"
	case B2BClient:
		return "b2b_client"
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
	case "b2b_client":
		*k = B2BClient
	default:
		return fmt.Errorf("invalid PrincipalKind: %s", s)
	}
	return nil
}

type Principal struct {
	ID   string        `json:"id"`   // user id for admins, client id for b2b clients
	Kind PrincipalKind `json:"kind"` // string marshaled
}
