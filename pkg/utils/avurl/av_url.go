package avurl

import (
	"errors"
	"fmt"

	"github.com/edirooss/zmux-server/pkg/utils/hostutil"
)

type URL struct {
	Schema   string `json:"schema"`
	Userinfo string `json:"userinfo"`
	Host     string `json:"host"`
	Port     string `json:"port"`
	Path     string `json:"path"`
}

// Parse takes a URL string, splits it into components, and validates
// the host and port. Returns a structured URL object on success.
func Parse(url string) (*URL, error) {
	schema, userinfo, host, port, path, _md := avurlSplit(url)

	/* invariant: url must equal re-joined parts; failure here means the split/join logic is broken */
	if url != avurlJoin(schema, userinfo, host, port, path, _md) {
		return nil, errors.New("unable to parse URL")
	}

	if _md.junk != "" /* leftover junk after ']' */ {
		return nil, errors.New("invalid URL")
	}

	if host != "" {
		if err := hostutil.ValidateHost(host); err != nil {
			return nil, err
		}
	}

	if port != "" && !isPort(port) {
		return nil, fmt.Errorf("bad port: '%s'", port)
	}

	return &URL{
		Schema:   schema,
		Userinfo: userinfo,
		Host:     host,
		Port:     port,
		Path:     path,
	}, nil
}

// RawParse splits a URL string into components without applying
// host and port validation checks.
func RawParse(url string) (*URL, error) {
	schema, userinfo, host, port, path, _md := avurlSplit(url)

	/* invariant: url must equal re-joined parts; failure here means the split/join logic is broken */
	if url != avurlJoin(schema, userinfo, host, port, path, _md) {
		return nil, errors.New("unable to parse URL")
	}

	return &URL{
		Schema:   schema,
		Userinfo: userinfo,
		Host:     host,
		Port:     port,
		Path:     path,
	}, nil
}
