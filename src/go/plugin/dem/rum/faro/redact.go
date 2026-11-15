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
	path := redactPath(u.EscapedPath(), o.PathRules, source)
	plain, err := url.PathUnescape(path)
	if err != nil {
		plain = path
	}
	u.Path, u.RawPath = plain, path
	escaped := path
	if plain == path {
		// Nothing was decoded, but the URL escapes text a site path rule inserted.
		escaped = u.EscapedPath()
	}
	if strings.IndexByte(escaped, '%') >= 0 {
		// The final pass reads the escaped URL, where an escape can hide a
		// credential (/a/Bearer%20token), so the redactor reads it decoded too.
		u.Path, u.RawPath = o.redactDecodedPath(escaped, u.Path)
		if u.Host == "" && strings.HasPrefix(u.RawPath, "//") {
			// Removed control characters can join two slashes; a relative path
			// must not become a network-path reference with a host.
			u.RawPath = "/%2F" + u.RawPath[2:]
		}
	}
	return u.String()
}

// redactDecodedPath reads an escaped path decoded, as the redactor reads text:
// it removes control characters and replaces every credential found with or
// without them by a literal marker, which net/url accepts in an escaped path.
// Every other byte keeps its received escaping, so the final pass over the
// escaped URL still reads the original escapes. plain is escaped decoded; it
// returns the new decoded and escaped path.
func (o Options) redactDecodedPath(escaped, plain string) (string, string) {
	redacted := o.redactor().ApplyRemoving(plain, unicode.IsControl)
	if redacted == plain {
		return plain, escaped
	}
	// starts[i] is where decoded byte i begins in escaped; kept lists the
	// decoded bytes that are not part of a control character.
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
		return redacted, ""
	}
	var keptText strings.Builder
	kept := make([]int, 0, len(plain))
	for i := 0; i < len(plain); {
		c, n := utf8.DecodeRuneInString(plain[i:])
		if !unicode.IsControl(c) {
			keptText.WriteString(plain[i : i+n])
			for j := i; j < i+n; j++ {
				kept = append(kept, j)
			}
		}
		i += n
	}
	// The redacted text is the kept text with ranges replaced by markers: place
	// each run between markers in the kept text and write its received escapes.
	text := keptText.String()
	runs := strings.Split(redacted, redact.Marker)
	var out strings.Builder
	pos := 0
	for i, run := range runs {
		at := pos
		switch {
		case i == len(runs)-1:
			at = len(text) - len(run)
		case i > 0:
			at = pos + strings.Index(text[pos:], run)
		}
		if at < pos || (i == 0 && at != 0) || !strings.HasPrefix(text[at:], run) {
			// Unreachable while the redactor only replaces ranges.
			return redacted, ""
		}
		if i > 0 {
			out.WriteString(redact.Marker)
		}
		for _, j := range kept[at : at+len(run)] {
			out.WriteString(escaped[starts[j]:starts[j+1]])
		}
		pos = at + len(run)
	}
	return redacted, out.String()
}

func escapeInvalidPercent(value string) string {
	var result strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == '%' && (i+2 >= len(value) || !isHexByte(value[i+1]) || !isHexByte(value[i+2])) {
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
