package avurl

import "strconv"

// isPort checks if the string represents a valid port number (0–65535).
func isPort(s string) bool {
	// reject leading zeros
	if len(s) > 1 && s[0] == '0' {
		return false
	}

	port, err := strconv.Atoi(s)
	if err != nil {
		return false
	}
	return port >= 0 && port <= 65535
}
