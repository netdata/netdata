// SPDX-License-Identifier: GPL-3.0-or-later

package statsd

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

// presentation is name-wide chart metadata from nd_unit, nd_title and nd_family.
type presentation struct{ unit, title, family string }

// preparedRecord is a record after final preparation: metadata extracted, labels
// validated and sorted, and id the series identity key.
type preparedRecord struct {
	record
	metadata presentation
	id       []byte
}

// prepareRecord is the single final-label boundary, after any profile replace.
// Metadata is removed before identity is formed. No output-ID preflight belongs here.
// Final labels are appended to labels[:0] and the identity key to id[:0], so both
// may share the caller's storage.
func prepareRecord(r record, labels []metrix.Label, id []byte) (preparedRecord, error) {
	p := preparedRecord{
		record: r,
	}
	if !validName(r.name) {
		return p, rejectSyntax
	}
	p.labels = labels[:0]
	for _, l := range r.labels {
		if strings.HasPrefix(l.Key, "nd_") {
			if !validMetadata(l.Value) {
				return p, rejectMetadata
			}
			switch l.Key {
			case "nd_unit":
				p.metadata.unit = l.Value
			case "nd_title":
				p.metadata.title = l.Value
			case "nd_family":
				p.metadata.family = l.Value
			default:
				return p, rejectMetadata
			}
			continue
		}
		if l.Key == "_collect_job" || l.Key == metrix.MeasureSetFieldLabel || !validLabelKey(l.Key) ||
			!validLabelValue(l.Value) {
			return p, rejectLabels
		}
		p.labels = append(p.labels, l)
	}
	slices.SortFunc(p.labels, func(a, b metrix.Label) int { return strings.Compare(a.Key, b.Key) })
	// The final name, NUL, then the canonical labels. Names exclude controls and the
	// native grammar admits neither '=' in keys nor NUL in values.
	p.id = append(append(id[:0], r.name...), 0)
	for i, l := range p.labels {
		if i > 0 && p.labels[i-1].Key == l.Key {
			return p, rejectLabels
		}
		p.id = append(append(append(append(p.id, l.Key...), '='), l.Value...), 0)
	}
	return p, nil
}

func labelASCII(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
		c == '_' || c == '-' || c == '.' || c == '/' || c == '[' || c == ']'
}

func validLabelKey(s string) bool {
	// Native label buffers include their terminating NUL: 200/800 bytes.
	if len(s) == 0 || len(s) > 199 {
		return false
	}
	underscores := true
	for i := range len(s) {
		if !labelASCII(s[i]) {
			return false
		}
		underscores = underscores && s[i] == '_'
	}
	return !underscores
}

// validLabelValue checks ASCII bytes in one pass; non-ASCII text is preserved
// when the whole value is valid UTF-8 without control characters.
func validLabelValue(s string) bool {
	if len(s) == 0 || len(s) > 799 || s[0] == ' ' || s[len(s)-1] == ' ' {
		return false
	}
	underscores, unicodeText := true, false
	for i := range len(s) {
		c := s[i]
		underscores = underscores && c == '_'
		switch {
		case c >= utf8.RuneSelf:
			unicodeText = true
		case c == ' ':
			if s[i-1] == ' ' {
				return false
			}
		case !labelASCII(c) && c != ':' && c != '+' && c != '@' && c != '(' && c != ')':
			return false
		}
	}
	return !underscores && (!unicodeText || validUnicodeText(s))
}

func validMetadata(s string) bool {
	return s != "" && validText(s) && !strings.ContainsAny(s, "\"'\\") &&
		strings.TrimFunc(s, unicode.IsSpace) == s && !strings.Contains(s, "  ") && strings.Trim(s, "_") != ""
}

func (p presentation) conflicts(other presentation) bool {
	return other.unit != "" && other.unit != p.unit || other.title != "" && other.title != p.title ||
		other.family != "" && other.family != p.family
}

func defaultPresentation(p preparedRecord) presentation {
	m := p.metadata
	if m.unit == "" {
		switch p.kind {
		case counter:
			m.unit = "events"
		case set:
			m.unit = "members"
		case timer:
			m.unit = "milliseconds"
		default:
			m.unit = "value"
		}
	}
	if m.title == "" {
		m.title = encodeName(p.name)
	}
	if m.family == "" {
		m.family = "statsd"
	}
	return m
}

func encodeName(s string) string {
	const hex = "0123456789abcdef"
	var b strings.Builder
	b.Grow(len(s))
	for i := range len(s) {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '-' {
			b.WriteByte(c)
		} else {
			b.WriteByte('_')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&15])
		}
	}
	return b.String()
}
