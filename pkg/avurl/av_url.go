package avurl

import (
	"errors"
	"fmt"
	"strings"

	"github.com/edirooss/zmux-server/pkg/hostutil"
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

	if _md.hasAtSign {
		return nil, errors.New("userinfo should not be embedded in the URL")
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

func EmbeddUserinfo(url, username, password *string) *string {
	if url == nil {
		return nil
	}

	schema, _, host, port, path, _md := avurlSplit(*url)

	userinfo := ""
	if username != nil {
		_md.hasAtSign = true
		userinfo += escapUsername(*username)

		if password != nil {
			userinfo += ":" + escapPassword(*password)
		}
	}

	return ptr(avurlJoin(schema, userinfo, host, port, path, _md))
}

// escapUsername escapes only '/', '?', '#' and ':' using percent-encoding.
func escapUsername(input string) string {
	replacer := strings.NewReplacer(
		"/", "%2F",
		"?", "%3F",
		"#", "%23",
		":", "%3A",
	)
	return replacer.Replace(input)
}

// escapPassword escapes only '/', '?', and '#' using percent-encoding.
func escapPassword(input string) string {
	replacer := strings.NewReplacer(
		"/", "%2F",
		"?", "%3F",
		"#", "%23",
	)
	return replacer.Replace(input)
}

// ptr helper
func ptr(s string) *string { return &s }
