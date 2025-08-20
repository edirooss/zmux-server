package jsonx

import "encoding/json"

// ---------- Field[T] ----------

// Field[T] tracks presence (key appeared) and holds a pointer value:
//   - IsSet() == true  => key existed (even if it was null; allows null vs. undefined distinction)
//   - val == nil       => value was JSON null (ignored at this layer; may be used at other layers to enforce nullability/ clear a field if explicitly null)
type Field[T any] struct {
	set bool
	val *T
}

func (o Field[T]) IsSet() bool  { return o.set }
func (o Field[T]) IsNull() bool { return o.set && o.val == nil }
func (o Field[T]) Value() *T    { return o.val }

func (o *Field[T]) UnmarshalJSON(b []byte) error {
	switch string(bytesTrimSpace(b)) {
	case "null":
		o.set, o.val = true, nil
		return nil
	default:
		var v T
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		o.set, o.val = true, &v
		return nil
	}
}

func bytesTrimSpace(b []byte) []byte {
	i := 0
	for i < len(b) && (b[i] == ' ' || b[i] == '\n' || b[i] == '\t' || b[i] == '\r') {
		i++
	}

	j := len(b) - 1
	for j >= i && (b[j] == ' ' || b[j] == '\n' || b[j] == '\t' || b[j] == '\r') {
		j--
	}

	return b[i : j+1]
}

/*
This could be farther extened with reflection for automatic required-field validation.
func (Field[T]) _jsonxField() {}

// Required[T] marks a field as "key must be present".
type Required[T any] struct{ Field[T] }

func (Required[T]) _jsonxRequired() {}

// Optional[T] marks a field as "optional, key may be present".
type Optional[T any] struct{ Field[T] }

func (Optional[T]) _jsonxOptional() {}

// Marker interfaces for reflection-based validation.
type fieldlMarker interface{ _jsonxField() }
type requiredMarker interface{ _jsonxRequired() }
type optionalMarker interface{ _jsonxOptional() }
*/
