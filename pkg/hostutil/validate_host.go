package hostutil

import (
	"fmt"
	"net"
	"strings"
	"unicode"
)

func ValidateHost(raw string) error {
	switch {
	case looksLikeIPv4(raw):
		if !validateIPv4(raw) {
			return fmt.Errorf("bad IP: '%s'", raw)
		}
	case looksLikeIPv6(raw):
		if !validateIPv6(raw) {
			return fmt.Errorf("bad IPv6: '%s'", raw)
		}
	default:
		if !validateHostname(raw) {
			return fmt.Errorf("bad hostname: '%s'", raw)
		}
	}
	return nil
}

// looksLikeIPv4 checks if raw looks like dotted quad
func looksLikeIPv4(raw string) bool {
	parts := strings.Split(raw, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		for _, r := range p {
			if !unicode.IsDigit(r) {
				return false
			}
		}
	}
	return true
}

// validateIPv4 parses and ensures all octets in range
func validateIPv4(raw string) bool {
	ip := net.ParseIP(raw)
	if ip == nil {
		return false
	}
	return ip.To4() != nil
}

// looksLikeIPv6 checks if raw looks like IPv6 literal
func looksLikeIPv6(raw string) bool {
	// simplest heuristic: has ':' or wrapped in []
	return strings.Contains(raw, ":") || (strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]"))
}

// validateIPv6 parses as IPv6
func validateIPv6(raw string) bool {
	ip := net.ParseIP(raw)
	if ip == nil {
		return false
	}
	return ip.To16() != nil && ip.To4() == nil
}

// validateHostname checks DNS label rules (RFC 1123)
func validateHostname(raw string) bool {
	if len(raw) > 253 {
		return false
	}
	labels := strings.Split(raw, ".")
	for _, label := range labels {
		if len(label) < 1 || len(label) > 63 {
			return false
		}
		// must be alnum or hyphen in the middle
		for i, r := range label {
			if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-') {
				return false
			}
			// no leading/trailing hyphen
			if (i == 0 || i == len(label)-1) && r == '-' {
				return false
			}
		}
	}
	return true
}
