// decodestrict.go
package jsonx

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

var (
	ErrEmptyBody    = errors.New("empty body")
	ErrTrailingJSON = errors.New("trailing data")
)

// ParseStrictJSONBody reads and **strictly** decodes a JSON HTTP request body into dst.
//
// Intended HTTP mapping: return **400 Bad Request** when decoding fails due to
// **syntax/structural issues in the HTTP request or JSON payload** or JSON schema **shape**
// violations, including:
//
//   - Malformed JSON syntax (e.g., bad tokens, truncated body)
//   - Empty body (returns ErrEmptyBody)
//   - Oversized body (reader capped at 1MB by default; adjust as needed)
//   - Trailing data (ensures a *single* JSON value; ErrTrailingJSON)
//   - Disallowed additional properties (unknown/unexpected fields) via DisallowUnknownFields
//   - Field-type mismatches (e.g., string into int)
//
// Notes & scope alignment with 400:
//
//   - This function performs **only structural/shape validation** of the JSON payload.
//   - It **does not** validate HTTP headers, authentication, or request metadata.
//   - It **does not** validate the presence of required fields (zero values are accepted).
//   - It **does not** enforce semantic/business rules (ranges, cross-field logic, etc.).
//   - It **does not** perform field-level access control or redaction.
//
// Usage:
//
//	Use in security-sensitive handlers to bind **low-trust** JSON inputs with tight shape checks.
//	On any of the above failures, surface a detailed error and map it to **HTTP 400**.
//
// Returns:
//   - nil on success
//   - A descriptive error (e.g., ErrEmptyBody, ErrTrailingJSON, json.Decoder errors) when the
//     body content or structure is invalid in ways that should result in **400 Bad Request**.
func ParseStrictJSONBody[T any](r *http.Request, dst *T) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB cap; tweak as needed
	if err != nil {
		return err
	}
	if len(bytesTrimSpace(body)) == 0 {
		return ErrEmptyBody
	}

	dec := json.NewDecoder(bytesNewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	// Ensure no trailing JSON values
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return ErrTrailingJSON
	}
	return nil
}

// minimal local helpers to avoid extra imports in snippet
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
func bytesNewReader(b []byte) io.Reader { return bytes.NewReader(b) }
