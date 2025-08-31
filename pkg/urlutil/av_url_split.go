package urlutil

import (
	"strings"
)

// avurlSplit is a Go port of FFmpeg's av_url_split(...) (libavformat/utils.c),
// without buffer-size truncation.
//
// Port handling difference:
//   - C: port parsed with atoi (non-digits ignored, empty→0): "123abc" → 123, "" → 0, "+42" → 42.
//   - Go: port kept as raw substring: "123abc", "", "+42".
func avurlSplit(url string) ( /* initialization: empty strings */ schema, userinfo, host, port, path string, _md metadata) {
	/* traversal helper */
	var cursor int

	/* -------- parse scheme (protocol) --------
	   example: "http:", "file:", "rtsp:"
	   copies everything up to ':' (exclusive) into `schema`
	   then skip ':', and optionally up to two leading '/'.
	*/
	if colon := strings.IndexByte(url, ':'); colon != -1 {
		_md.hasSchema = true
		schema = url[:colon]

		cursor = colon + 1 // skip ':'
		if cursor == len(url) {
			return
		}

		if url[cursor] == '/' {
			cursor++
			_md.slashNum++
			if cursor == len(url) {
				return
			}
		}
		if url[cursor] == '/' {
			cursor++
			_md.slashNum++
			if cursor == len(url) {
				return
			}
		}
	} else {
		/* no ':' found; treat entire url as a plain path/filename */
		path = url
		return
	}

	/* -------- split authority vs path/query/fragment --------
	   copy everything from the first '/', '?', or '#' (inclusive) after `cursor` to the end into `path`.
	*/

	/* scans from `cursor` until '/', '?' or '#'.
	   If none found, `pathAt` == len(url), i.e., no path/query/fragment */
	pathAt := cursor + strcspn(url[cursor:], "/?#") // pathAt ALWAYS <= len(url)
	path = url[pathAt:]

	/* -------- parse authority: [userinfo@]host[:port] -------- */

	/* if pathAt == cursor, the cursor is at '/', '?' or '#' (i.e., ).
	   That means there's no authority — just schema:[//](/|?|#)... */
	if pathAt != cursor {
		/* ---- extract userinfo (user[:pass]@...) ----
		   userinfo is everything from `cursor` up to the LAST '@' (exclusive)
		*/
		userinfoAt := cursor
		for {
			atSignRel := strings.IndexByte(url[cursor:pathAt], '@')
			if atSignRel == -1 {
				// '@' not found
				break
			}
			_md.hasAtSign = true
			atSignAbs := cursor + atSignRel
			userinfo = url[userinfoAt:atSignAbs] // overwrite until the last '@'
			cursor = atSignAbs + 1               // skip '@'
			if cursor == len(url) {
				return
			}
		}

		/* ---- IPv6 literal: [host]:port ---- */
		if closingBracketRel := strings.IndexByte(url[cursor:pathAt], ']'); closingBracketRel != -1 && url[cursor] == '[' {
			_md.hasBrks = true
			brkAbs := cursor + closingBracketRel
			host = url[cursor+1 : brkAbs] // stripped '[]'
			cursor = brkAbs + 1

			if cursor == len(url) {
				return
			}

			// check port
			if url[cursor] == ':' {
				_md.hasPort = true
				port = url[cursor+1 : pathAt]
			} else if cursor != pathAt {
				// leftover junk between ']' and pathAt
				_md.junk = url[cursor:pathAt]
			}
		} else if /* ---- IPv4/hostname with :port ---- */
		colonRel := strings.IndexByte(url[cursor:pathAt], ':'); colonRel != -1 {
			_md.hasPort = true
			colonAbs := cursor + colonRel
			host = url[cursor:colonAbs]
			port = url[colonAbs+1 : pathAt]
		} else /* ---- IPv4/hostname with no port ---- */ {
			host = url[cursor:pathAt]
		}
	}

	return
}

// strcspn returns the length of the initial segment of s that
// contains none of the bytes in reject.
func strcspn(s, reject string) int {
	if idx := strings.IndexAny(s, reject); idx != -1 {
		return idx
	}
	return len(s)
}
