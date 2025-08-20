package jsonx

import "encoding/json"

// ---------- Field[T] (tri-state) ----------

type Field[T any] struct {
	set  bool
	null bool
	val  T
}

func (o Field[T]) IsSet() bool      { return o.set }
func (o Field[T]) IsNull() bool     { return o.set && o.null }
func (o Field[T]) Value() (T, bool) { return o.val, o.set && !o.null }

func (o *Field[T]) UnmarshalJSON(b []byte) error {
	// A small, allocation-friendly implementation is fine.
	// We only need to detect explicit null vs value.
	switch string(bytesTrimSpace(b)) {
	case "null":
		o.set, o.null = true, true
		var zero T
		o.val = zero
		return nil
	default:
		var v T
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		o.set, o.null, o.val = true, false, v
		return nil
	}
}
