package urlutil

import "strings"

type metadata struct {
	hasSchema bool
	slashNum  int
	hasAtSign bool
	hasBrks   bool
	hasPort   bool
	junk      string
}

func avurlJoin(schema, userinfo, host, port, path string, _md metadata) (url string) {
	return schema + colon(_md.hasSchema) + strings.Repeat("/", _md.slashNum) + userinfo + atSign(_md.hasAtSign) + hostWithBrks(host, _md.hasBrks) + colon(_md.hasPort) + port + _md.junk + path
}

func atSign(b bool) string {
	if b {
		return "@"
	}

	return ""
}

func hostWithBrks(host string, _hasBrks bool) string {
	if _hasBrks {
		return "[" + host + "]"
	}
	return host
}

func colon(b bool) string {
	if b {
		return ":"
	}

	return ""
}
