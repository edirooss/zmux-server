package channel

import (
	"errors"

	"github.com/edirooss/zmux-server/pkg/utils/avurl"
)

// validateInputURL
// Validation for *input* URLs (i.e., where media comes from).
//
// Policy:
//   - Require a protocol (FFmpeg's default to local files when schema missing; Zmux must not accpet this kind of inputs).
//
// Returns: error on validation failure, or nil if valid.
func validateInputURL(raw string) error {
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

// ValidateOutputURL
// Validation for *output* URLs (i,e where media goes).
//
// Policy:
//   - Require a protocol (FFmpeg's default to local files when schema missing; Zmux must not accpet this kind of outputs).
//   - If protocol is `udp`, require a valid `udp` absolute uri of media outputs (i.e., hostname & port required).

// Returns: error on validation failure, or nil if valid.
func validateOutputURL(raw string) error {
	// Parse URL
	url, err := avurl.Parse(raw)
	if err != nil {
		return err
	}

	if /* Note: This condition will be removed in the future when Zmux would support different output format then MPEG-TS
	Right now, we require protocol `udp` for all output URLs to ensure compatiabillty with remux hard-coded output format [-f mpegts]
	*/url.Schema != "udp" {
		return errors.New("only `udp` protocol allowed for media output")
	}

	// We require protocol; fallback to local file is forbidden for media output (i,e. writing to a local file)
	if url.Schema == "" {
		return errors.New("missing protocol")
	}

	// For `udp` outputs, require hostname & port
	if url.Schema == "udp" {
		if url.Host == "" {
			return errors.New("missing host for 'udp' media output")
		}
		if url.Port == "" {
			return errors.New("missing port for 'udp' media output")
		}
	}

	return nil
}
