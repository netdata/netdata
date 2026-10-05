// SPDX-License-Identifier: GPL-3.0-or-later

package beacon

import (
	"net/url"
	"regexp"
	"strings"
)

// PathRule is a site's own path redaction: Re's matches become
// Replace, applied before the built-in rules.
type PathRule struct {
	Re      *regexp.Regexp
	Replace string
}

// RedactPath removes personal data from a URL path: the site's
// rules first, then per segment email addresses (email), UUIDs and
// numbers of 6+ digits (id), and long token-like strings (token).
func RedactPath(path string, rules []PathRule) string {
	for _, r := range rules {
		path = r.Re.ReplaceAllString(path, r.Replace)
	}
	segs := strings.Split(path, "/")
	for i, s := range segs {
		if s == "" || s[0] == ':' {
			continue
		}
		plain := s
		if u, err := url.PathUnescape(s); err == nil {
			plain = u
		}
		switch {
		case isEmail(plain):
			segs[i] = ":email"
		case isUUID(plain) || (isNumeric(plain) && len(plain) >= 6):
			segs[i] = ":id"
		case isToken(plain):
			segs[i] = ":token"
		}
	}
	return strings.Join(segs, "/")
}

// RedactURL applies RedactPath to the path of an absolute URL, keeping
// scheme and host; query and fragment must already be gone.
func RedactURL(raw string, rules []PathRule) string {
	i := strings.Index(raw, "://")
	if i < 0 {
		return RedactPath(raw, rules)
	}
	rest := raw[i+3:]
	j := strings.IndexByte(rest, '/')
	if j < 0 {
		return raw
	}
	return raw[:i+3] + rest[:j] + RedactPath(rest[j:], rules)
}

func isEmail(s string) bool {
	at := strings.IndexByte(s, '@')
	return at > 0 && strings.Contains(s[at+1:], ".")
}

// isToken: 20+ characters of a token alphabet mixing letters and digits,
// with at most one hyphen, so readable slugs such as
// "top-10-tips-for-summer-2026" stay.
func isToken(s string) bool {
	if len(s) < 20 || strings.Count(s, "-") > 1 {
		return false
	}
	var letters, digits int
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
			digits++
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
			letters++
		case c == '_' || c == '-' || c == '.' || c == '~' || c == '=' || c == '+':
		default:
			return false
		}
	}
	return letters > 0 && digits > 0
}

var (
	selectorEmailRe = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	selectorUUIDRe  = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	selectorLongNum = regexp.MustCompile(`\d{6,}`)
)

// RedactSelector removes email addresses, UUIDs and long numbers that can
// end up in element ids or classes.
func RedactSelector(s string) string {
	s = selectorEmailRe.ReplaceAllString(s, ":email")
	s = selectorUUIDRe.ReplaceAllString(s, ":id")
	return selectorLongNum.ReplaceAllString(s, ":id")
}
