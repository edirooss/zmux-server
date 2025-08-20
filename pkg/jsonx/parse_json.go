// parse_json.go
package jsonx

import (
	"encoding/json"
	"io"
)

// ParseJSONObject decodes one JSON value from src into dst.
//
// - Malformed JSON (bad tokens, empty/unterminated/truncated) => *json.SyntaxError, io.EOF, io.ErrUnexpectedEOF
// - Incorrect data type (field/value mismatch) => *json.UnmarshalTypeError
// - Unknown object fields => error("json: unknown field \"...\"") from encoding/json (no dedicated error type)
// - Other decode failures bubble up from encoding/json.
func ParseJSONObject[T any](src io.Reader, dst *T) error {
	dec := json.NewDecoder(src)
	dec.DisallowUnknownFields()
	// dec.UseNumber()

	if err := dec.Decode(dst); err != nil {
		return err
	}

	return nil
}
