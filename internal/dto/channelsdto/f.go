package channelsdto

import "encoding/json"

// F is a generic field wrapper that distinguishes between
// an omitted property, an explicit null, and a concrete value.
//
// States:
//   - Omitted: Set=false, Null=false
//   - Explicit null: Set=true, Null=true
//   - Value present: Set=true, Null=false, V holds the value
type F[T any] struct {
	V    T
	Set  bool
	Null bool
}

func (o *F[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	if len(b) == 4 && string(b) == "null" {
		o.Null = true
		return nil
	}
	return json.Unmarshal(b, &o.V)
}

// ValueOrNil returns a pointer to the underlying value if set and not null,
// otherwise returns nil.
func (f F[T]) ValueOrNil() *T {
	if !f.Set || f.Null {
		return nil
	}
	return &f.V
}
