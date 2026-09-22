// SPDX-License-Identifier: GPL-3.0-or-later

package statsd

import (
	"slices"
	"strings"
	"unicode"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

type presentation struct{ unit, title, family string }
type preparedRecord struct {
	record
	metadata presentation
	labelKey string
}

// prepareRecord is the single final-label boundary, after any profile replace.
// Metadata is removed before identity is formed. No output-ID preflight belongs here.
func prepareRecord(r record) (preparedRecord, error) {
	p := preparedRecord{
		record: r,
	}
	if !validName(r.name) {
		return p, rejectSyntax
	}
	p.labels = make([]metrix.Label, 0, len(r.labels))
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
	var key strings.Builder
	for i, l := range p.labels {
		if i > 0 && p.labels[i-1].Key == l.Key {
			return p, rejectLabels
		}
		// Neither '=' in keys nor NUL in values is admitted by the native grammar.
		key.WriteString(l.Key)
		key.WriteByte('=')
		key.WriteString(l.Value)
		key.WriteByte(0)
	}
	p.labelKey = key.String()
	return p, nil
}

func labelASCII(c rune) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("_-./[]", c)
}

func validLabelKey(s string) bool {
	// Native label buffers include their terminating NUL: 200/800 bytes.
	if len(s) == 0 || len(s) > 199 || strings.Trim(s, "_") == "" {
		return false
	}
	for _, c := range s {
		if !labelASCII(c) {
			return false
		}
	}
	return true
}

func validLabelValue(s string) bool {
	if len(s) == 0 || len(s) > 799 || !validText(s) || strings.Trim(s, "_") == "" ||
		strings.HasPrefix(s, " ") || strings.HasSuffix(s, " ") || strings.Contains(s, "  ") {
		return false
	}
	for _, c := range s {
		if c < 128 && !labelASCII(c) && !strings.ContainsRune(":+@() ", c) {
			return false
		}
	}
	return true
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
