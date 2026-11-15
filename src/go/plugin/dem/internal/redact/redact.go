// SPDX-License-Identifier: GPL-3.0-or-later

package redact

import (
	"cmp"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"
)

// Redactor removes secret values and common credential shapes from text
// before egress. It replaces exact supplied values of at least four bytes, then
// applies credential-pattern matches regardless of their value length.
type Redactor struct {
	values []string
}

// standalonePattern matches credentials that are replaced whole.
var standalonePattern = regexp.MustCompile(
	`(?i)(` +
		`bearer\s+[a-z0-9._~+/-]+=*` + // Bearer tokens
		`|akia[0-9a-z]{16}` + // AWS access keys
		`|-----begin [a-z ]+-----[\s\S]*?-----end [a-z ]+-----` + // PEM blocks
		`)`,
)

const credentialKeys = `api_key|apikey|access_token|client_secret|token|password|secret`

// keyValuePattern matches credential key=value pairs; the key is kept and the value redacted.
var keyValuePattern = regexp.MustCompile(`(?i)\b((?:` + credentialKeys + `))=[^&\s"']*`)
var queryKeyPattern = regexp.MustCompile(`(?i)^(?:` + credentialKeys + `)$`)

func NewRedactor(values ...string) *Redactor {
	return &Redactor{
		values: append([]string(nil), values...),
	}
}

const redacted = "[REDACTED]"

// Apply redacts from a copy of s.
func (r *Redactor) Apply(s string) string {
	out := s
	for _, v := range r.values {
		if len(v) >= 4 { // avoid redacting trivially short values everywhere
			out = strings.ReplaceAll(out, v, redacted)
		}
	}
	// Most diagnostic strings contain no credentials. Avoid allocating copies
	// when the replacement would leave them unchanged.
	if standalonePattern.MatchString(out) {
		out = standalonePattern.ReplaceAllString(out, redacted)
	}
	if keyValuePattern.MatchString(out) {
		out = keyValuePattern.ReplaceAllString(out, "$1="+redacted)
	}
	return out
}

// ApplyRemoving returns s without the runes remove selects. It redacts every
// credential Apply finds in s or in s without those runes: removing a rune can
// join a credential ("tok\x00en=...") or break one that needs it
// ("Bearer\tTOKEN", a word boundary). Kept runes keep their bytes.
func (r *Redactor) ApplyRemoving(s string, remove func(rune) bool) string {
	if strings.IndexFunc(s, remove) < 0 {
		return r.Apply(s)
	}
	var kept strings.Builder
	kept.Grow(len(s))
	writeKept(&kept, s, remove)
	spans := r.spans(s)
	if keptSpans := r.spans(kept.String()); len(keptSpans) > 0 {
		spans = append(spans, keptToSource(keptSpans, s, remove)...)
	}
	if len(spans) == 0 {
		return kept.String()
	}
	// A replacement can give its neighbor a word boundary the original lacked.
	return r.Apply(replaceSpans(s, spans, remove))
}

// span is a byte range Apply would replace with one marker; an empty span
// inserts one (a credential key without a value).
type span struct{ start, end int }

// spans lists the byte ranges Apply replaces in s, step by step in Apply's
// order: each configured value, then the standalone patterns, then key=value.
// Each step reads the text with the earlier replacements filled (see filled).
// Unlike Apply, a configured value never matches across an earlier marker,
// which matters only for values that contain the marker's text.
func (r *Redactor) spans(s string) []span {
	var out []span
	text := s
	for _, v := range r.values {
		if len(v) < 4 {
			continue
		}
		earlier := len(out)
		for i := 0; ; {
			j := strings.Index(text[i:], v)
			if j < 0 {
				break
			}
			sp := span{
				start: i + j,
				end:   i + j + len(v),
			}
			if overlaps(sp, out[:earlier]) {
				i += j + 1 // Apply sees a marker there, not the filler
				continue
			}
			out = append(out, sp)
			i = sp.end
		}
		text = filled(text, out[earlier:])
	}
	n := len(out)
	if standalonePattern.MatchString(text) {
		for _, m := range standalonePattern.FindAllStringIndex(text, -1) {
			out = append(out, span{
				start: m[0],
				end:   m[1],
			})
		}
		text = filled(text, out[n:])
	}
	if keyValuePattern.MatchString(text) {
		for _, m := range keyValuePattern.FindAllStringSubmatchIndex(text, -1) {
			out = append(out, span{
				start: m[3] + 1,
				end:   m[1],
			}) // the value after "key="
		}
	}
	return out
}

func overlaps(sp span, spans []span) bool {
	for _, o := range spans {
		if sp.start < o.end && o.start < sp.end {
			return true
		}
	}
	return false
}

// filler stands in for a replaced range. The patterns read it like the marker:
// only a key=value value holds it, and it is not a word character. Configured
// values never match across it (see spans).
const filler = 0xFF

// filled returns text with spans overwritten by filler, or text itself when
// there are none.
func filled(text string, spans []span) string {
	if len(spans) == 0 {
		return text
	}
	b := []byte(text)
	for _, sp := range spans {
		for i := sp.start; i < sp.end; i++ {
			b[i] = filler
		}
	}
	return string(b)
}

// keptToSource maps spans of s without the removed runes onto s.
func keptToSource(spans []span, s string, remove func(rune) bool) []span {
	// Each removed rune shifts the kept bytes after it.
	type shift struct{ kept, removed int } // kept offset of the next kept byte, removed bytes so far
	var shifts []shift
	removed := 0
	for i := 0; i < len(s); {
		c, n := utf8.DecodeRuneInString(s[i:])
		if remove(c) {
			shifts = append(shifts, shift{
				kept:    i - removed,
				removed: removed + n,
			})
			removed += n
		}
		i += n
	}
	source := func(k int) int { // offset in s of kept byte k, or len(s) past the end
		j := sort.Search(len(shifts), func(j int) bool { return shifts[j].kept > k })
		if j == 0 {
			return k
		}
		return k + shifts[j-1].removed
	}
	out := make([]span, len(spans))
	for i, sp := range spans {
		out[i] = span{
			start: source(sp.start),
			end:   source(sp.start),
		}
		if sp.end > sp.start {
			out[i].end = source(sp.end-1) + 1
		}
	}
	return out
}

// replaceSpans writes s without the removed runes, each run of overlapping or
// touching spans replaced by one marker.
func replaceSpans(s string, spans []span, remove func(rune) bool) string {
	slices.SortFunc(spans, func(a, b span) int { return cmp.Or(a.start-b.start, a.end-b.end) })
	var out strings.Builder
	out.Grow(len(s) + len(redacted))
	pos := 0
	for i := 0; i < len(spans); {
		merged := spans[i]
		for i++; i < len(spans) && spans[i].start <= merged.end; i++ {
			merged.end = max(merged.end, spans[i].end)
		}
		writeKept(&out, s[pos:merged.start], remove)
		out.WriteString(redacted)
		pos = merged.end
	}
	writeKept(&out, s[pos:], remove)
	return out.String()
}

func writeKept(out *strings.Builder, s string, remove func(rune) bool) {
	for i := 0; i < len(s); {
		c, n := utf8.DecodeRuneInString(s[i:])
		if !remove(c) {
			out.WriteString(s[i : i+n])
		}
		i += n
	}
}

// ApplyURL recognizes query names after URL decoding, including repeated keys.
// Only the display/history target changes; navigation uses the original URL.
func (r *Redactor) ApplyURL(value string) string {
	u, err := url.Parse(value)
	if err != nil {
		return r.Apply(value)
	}
	parts := strings.Split(u.RawQuery, "&")
	for i, part := range parts {
		key, _, hasValue := strings.Cut(part, "=")
		decoded, err := url.QueryUnescape(key)
		if err == nil && hasValue && queryKeyPattern.MatchString(decoded) {
			parts[i] = key + "=" + url.QueryEscape(redacted)
		}
	}
	u.RawQuery = strings.Join(parts, "&")
	return r.Apply(u.String())
}
