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
	values     []string
	standalone *regexp.Regexp // whole match → [REDACTED]
	keyvalue   *regexp.Regexp // keep the key, redact the value
}

var standalonePattern = regexp.MustCompile(
	`(?i)(` +
		`bearer\s+[a-z0-9._~+/-]+=*` + // Bearer tokens
		`|akia[0-9a-z]{16}` + // AWS access keys
		`|-----begin [a-z ]+-----[\s\S]*?-----end [a-z ]+-----` + // PEM blocks
		`)`,
)

const credentialKeys = `api_key|apikey|access_token|client_secret|token|password|secret`

var keyValuePattern = regexp.MustCompile(`(?i)\b((?:` + credentialKeys + `))=[^&\s"']*`)
var queryKeyPattern = regexp.MustCompile(`(?i)^(?:` + credentialKeys + `)$`)

func NewRedactor(values ...string) *Redactor {
	return &Redactor{
		values:     append([]string(nil), values...),
		standalone: standalonePattern,
		keyvalue:   keyValuePattern,
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
	if r.standalone.MatchString(out) {
		out = r.standalone.ReplaceAllString(out, redacted)
	}
	if r.keyvalue.MatchString(out) {
		out = r.keyvalue.ReplaceAllString(out, "$1="+redacted)
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

// spans lists the byte ranges Apply replaces in s, stage by stage in Apply's
// order. A later stage reads earlier replacements as filler (see fill).
func (r *Redactor) spans(s string) []span {
	var out []span
	for _, v := range r.values {
		if len(v) < 4 {
			continue
		}
		for i := 0; ; {
			j := strings.Index(s[i:], v)
			if j < 0 {
				break
			}
			out = append(out, span{start: i + j, end: i + j + len(v)})
			i += j + len(v)
		}
	}
	if len(out) == 0 && !r.standalone.MatchString(s) && !r.keyvalue.MatchString(s) {
		return nil
	}
	masked := []byte(s)
	fill(masked, out)
	values := len(out)
	for _, m := range r.standalone.FindAllIndex(masked, -1) {
		out = append(out, span{start: m[0], end: m[1]})
	}
	fill(masked, out[values:])
	for _, m := range r.keyvalue.FindAllSubmatchIndex(masked, -1) {
		out = append(out, span{start: m[3] + 1, end: m[1]}) // the value after "key="
	}
	return out
}

// fill overwrites spans with a byte the patterns read like the marker: no
// pattern class holds it except a key=value value, and it is a word boundary
// on both sides.
func fill(b []byte, spans []span) {
	for _, sp := range spans {
		for i := sp.start; i < sp.end; i++ {
			b[i] = '['
		}
	}
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
			shifts = append(shifts, shift{kept: i - removed, removed: removed + n})
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
		out[i] = span{start: source(sp.start), end: source(sp.start)}
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
