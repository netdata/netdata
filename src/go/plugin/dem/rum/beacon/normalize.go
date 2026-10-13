// SPDX-License-Identifier: GPL-3.0-or-later

package beacon

import (
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxPathLen  = 512
	maxGroupLen = 200
)

// Path reduces a page URL to its path: query and fragment are dropped
// before any other use, control characters removed, length capped.
// Unparsable or empty input yields "/".
func Path(raw string) string {
	raw = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return -1
		}
		return r
	}, raw)
	if raw == "" {
		return "/"
	}
	p := raw
	if u, err := url.Parse(raw); err == nil && (u.Scheme != "" || strings.HasPrefix(raw, "/")) {
		p = u.EscapedPath()
	} else if i := strings.IndexAny(raw, "?#"); i >= 0 {
		p = raw[:i]
	}
	if p == "" || p[0] != '/' {
		p = "/" + p
	}
	if len(p) > 1 {
		p = strings.TrimRight(p, "/")
		if p == "" {
			p = "/"
		}
	}
	return Truncate(p, maxPathLen)
}

// PageGroup replaces volatile path segments (decimal numbers, UUIDs,
// hex strings of 8+ chars) with ":id" so /users/42 and /users/43 share
// one instance.
func PageGroup(path string) string {
	if path == "" || path == "/" {
		return "/"
	}
	segs := strings.Split(path, "/")
	for i, s := range segs {
		if s == "" {
			continue
		}
		if isNumeric(s) || isUUID(s) || isLongHex(s) {
			segs[i] = ":id"
		}
	}
	g := strings.Join(segs, "/")
	return Truncate(g, maxGroupLen)
}

func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return s != ""
}

func isHexByte(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func isLongHex(s string) bool {
	if len(s) < 8 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isHexByte(s[i]) {
			return false
		}
	}
	return true
}

// isUUID matches 8-4-4-4-12 hex groups.
func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < len(s); i++ {
		switch i {
		case 8, 13, 18, 23:
			if s[i] != '-' {
				return false
			}
		default:
			if !isHexByte(s[i]) {
				return false
			}
		}
	}
	return true
}

// Clean bounds a free-form string coming from the client (browser name,
// OS, event names): control characters dropped, length capped.
func Clean(s string, max int) string {
	s = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s))
	return Truncate(s, max)
}

// Truncate limits valid UTF-8 text to n bytes without splitting a rune.
func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
