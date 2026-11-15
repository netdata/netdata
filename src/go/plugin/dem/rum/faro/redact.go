// SPDX-License-Identifier: GPL-3.0-or-later

package faro

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/redact"
)

// PathRule is a site's own path redaction: Re's matches become
// Replace, applied before the built-in rules.
type PathRule struct {
	Re      *regexp.Regexp
	Replace string
}

// redactPath removes personal data from a URL path: the site's rules first,
// then per segment email addresses (email), UUIDs and numbers of 6+ digits
// (id), and long token-like strings (token). A source path keeps a final
// script file name.
func redactPath(path string, rules []PathRule, source bool) string {
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
		case isToken(plain) && !(source && i == len(segs)-1 && isScriptFile(plain)):
			segs[i] = ":token"
		}
	}
	return strings.Join(segs, "/")
}

// redactURL strips credentials, query and fragment and applies the site's path
// rules. Relative paths follow the same policy.
func (o Options) redactURL(raw string) string {
	return o.redactLink(raw, false)
}

// redactSourceURL retains script basenames for source attribution and error
// fingerprints. Site rules and identifier redaction still apply; only the token
// heuristic is skipped for the final .js, .mjs or .cjs path segment.
func (o Options) redactSourceURL(raw string) string {
	return o.redactLink(raw, true)
}

func isScriptFile(name string) bool {
	return strings.HasSuffix(name, ".js") || strings.HasSuffix(name, ".mjs") || strings.HasSuffix(name, ".cjs")
}

func (o Options) redactLink(raw string, source bool) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil {
		var escapeErr url.EscapeError
		if errors.As(err, &escapeErr) {
			// Browsers accept literal percent signs in paths. Encode only malformed
			// escapes, then let the same URL parser remove credentials/query/fragment.
			u, err = url.Parse(escapeInvalidPercent(raw))
		}
	}
	if err != nil || u.Opaque != "" {
		return ""
	}
	u.User, u.RawQuery, u.Fragment, u.RawFragment = nil, "", "", ""
	u.ForceQuery = false
	// Site path rules can insert a malformed escape or bytes an escaped path
	// cannot hold; net/url would then escape the whole path again.
	path := validEscapedPath(redactPath(u.EscapedPath(), o.PathRules, source))
	plain, err := url.PathUnescape(path)
	if err != nil {
		return "" // unreachable: every escape is valid
	}
	u.Path, u.RawPath = plain, path
	if strings.IndexByte(path, '%') >= 0 {
		// The final pass reads the escaped URL, where an escape can hide a
		// credential (/a/Bearer%20token), so the redactor reads it decoded too.
		u.Path, u.RawPath = o.redactDecodedPath(path, plain)
	}
	if u.Host == "" && strings.HasPrefix(u.RawPath, "//") {
		// Removed control characters or a generalized segment can leave two
		// leading slashes; a relative path must not gain a host.
		u.RawPath = "/%2F" + u.RawPath[2:]
	}
	return u.String()
}

// redactDecodedPath reads an escaped path decoded, as the redactor reads text:
// it removes control characters and replaces every credential found with or
// without them by a literal marker, which net/url accepts in an escaped path.
// Every retained byte keeps its received escape, so the final pass over the
// escaped URL still reads the original escapes. plain is escaped decoded; it
// returns the new decoded and escaped path.
func (o Options) redactDecodedPath(escaped, plain string) (string, string) {
	// text is plain without the bytes removing its control characters would
	// join into another one; source maps its bytes back to plain.
	text, source := withoutJoins(plain)
	spans := o.redactor().RemovingSpans(text, unicode.IsControl)
	if len(spans) == 0 && strings.IndexFunc(text, unicode.IsControl) < 0 {
		return plain, escaped
	}
	// starts[i] is where decoded byte i begins in escaped.
	starts := make([]int, 0, len(plain)+1)
	for i := 0; i < len(escaped); {
		starts = append(starts, i)
		if escaped[i] == '%' && i+2 < len(escaped) && isHexByte(escaped[i+1]) && isHexByte(escaped[i+2]) {
			i += 3
		} else {
			i++
		}
	}
	starts = append(starts, len(escaped))
	if len(starts) != len(plain)+1 {
		// Unreachable for a decoded escaped path; escaping the redacted text keeps it redacted.
		return o.redactor().ApplyRemoving(text, unicode.IsControl), ""
	}
	var decoded, raw strings.Builder
	keep := func(start, end int) {
		for i := start; i < end; {
			c, n := utf8.DecodeRuneInString(text[i:end])
			if !unicode.IsControl(c) {
				decoded.WriteString(text[i : i+n])
				for j := i; j < i+n; j++ {
					at := j
					if source != nil {
						at = source[j]
					}
					raw.WriteString(escaped[starts[at]:starts[at+1]])
				}
			}
			i += n
		}
	}
	pos := 0
	for _, sp := range spans {
		keep(pos, sp.Start)
		decoded.WriteString(redact.Marker)
		raw.WriteString(redact.Marker)
		pos = sp.End
	}
	keep(pos, len(text))
	return decoded.String(), raw.String()
}

// withoutJoins drops the bytes of plain that removing its control characters
// would join into another control character: a lone 0xC2 and a lone 0x80-0x9F
// byte that meet once the characters between them are gone (%C2%00%85). No
// other pair forms one. It works in one pass, like a stack, so joins that
// cascade cost no more. source lists the plain index of each byte of text; it
// is nil, and text is plain, when nothing joins.
func withoutJoins(plain string) (text string, source []int) {
	if strings.IndexFunc(plain, unicode.IsControl) < 0 {
		return plain, nil
	}
	var stack []int // plain indexes of the bytes kept so far, control characters excluded
	var joined []bool
	for i := 0; i < len(plain); {
		c, n := utf8.DecodeRuneInString(plain[i:])
		switch {
		case unicode.IsControl(c):
		case n == 1 && 0x80 <= plain[i] && plain[i] <= 0x9F && len(stack) > 0 && plain[stack[len(stack)-1]] == 0xC2:
			if joined == nil {
				joined = make([]bool, len(plain))
			}
			joined[stack[len(stack)-1]], joined[i] = true, true
			stack = stack[:len(stack)-1]
		default:
			for j := i; j < i+n; j++ {
				stack = append(stack, j)
			}
		}
		i += n
	}
	if joined == nil {
		return plain, nil
	}
	var out strings.Builder
	out.Grow(len(plain))
	source = make([]int, 0, len(plain))
	for i := range len(plain) {
		if !joined[i] {
			out.WriteByte(plain[i])
			source = append(source, i)
		}
	}
	return out.String(), source
}

// validEscapedPath escapes what net/url does not keep in an escaped path: a
// percent sign that does not start a valid escape, and every byte other than
// unreserved characters, sub-delims, ":", "@", "/", "[" and "]". Valid escapes
// stay; a valid path is returned as is.
func validEscapedPath(path string) string {
	valid := func(i int) (int, bool) {
		c := path[i]
		switch {
		case c == '%':
			if i+2 < len(path) && isHexByte(path[i+1]) && isHexByte(path[i+2]) {
				return 3, true
			}
			return 1, false
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
			return 1, true
		}
		return 1, strings.IndexByte("-._~!$&'()*+,;=:@/[]", c) >= 0
	}
	i := 0
	for i < len(path) {
		n, ok := valid(i)
		if !ok {
			break
		}
		i += n
	}
	if i == len(path) {
		return path
	}
	const hex = "0123456789ABCDEF"
	var out strings.Builder
	out.Grow(len(path) + 8)
	out.WriteString(path[:i])
	for i < len(path) {
		n, ok := valid(i)
		if ok {
			out.WriteString(path[i : i+n])
		} else {
			out.WriteByte('%')
			out.WriteByte(hex[path[i]>>4])
			out.WriteByte(hex[path[i]&15])
		}
		i += n
	}
	return out.String()
}

// escapeInvalidPercent encodes each percent sign that does not start a valid
// escape; a value without one is returned as is.
func escapeInvalidPercent(value string) string {
	invalid := func(i int) bool {
		return value[i] == '%' && (i+2 >= len(value) || !isHexByte(value[i+1]) || !isHexByte(value[i+2]))
	}
	first := 0
	for first < len(value) && !invalid(first) {
		first++
	}
	if first == len(value) {
		return value
	}
	var result strings.Builder
	result.Grow(len(value) + 2)
	result.WriteString(value[:first])
	for i := first; i < len(value); i++ {
		if invalid(i) {
			result.WriteString("%25")
		} else {
			result.WriteByte(value[i])
		}
	}
	return result.String()
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

// redactSelector removes email addresses, UUIDs and long numbers that can
// end up in element ids or classes.
func redactSelector(s string) string {
	s = selectorEmailRe.ReplaceAllString(s, ":email")
	s = selectorUUIDRe.ReplaceAllString(s, ":id")
	return selectorLongNum.ReplaceAllString(s, ":id")
}
