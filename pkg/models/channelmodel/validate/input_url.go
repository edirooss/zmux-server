package validate

import (
	"errors"

	"github.com/edirooss/zmux-server/pkg/utils/avurl"
)

// ValidateInputURL
// Validation for *input* URLs (i.e., where media comes from).
//
// Policy:
//   - Require a protocol (FFmpeg's default to local files when schema missing; Zmux must not accpet this kind of inputs).
//
// Returns: error on validation failure, or nil if valid.
func ValidateInputURL(raw string) error {
	// Parse URL
	url, err := avurl.Parse(raw)
	if err != nil {
		return err
	}

	// We require protocol; fallback to local file is forbidden for media source input (i,e. reading from a local file)
	if url.Schema == "" {
		return errors.New("missing protocol")
	}

	return nil
}
