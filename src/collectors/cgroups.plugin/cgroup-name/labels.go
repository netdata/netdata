// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"strconv"
	"strings"
)

type label struct {
	name  string
	value string
}

type labelSet struct {
	items []label
}

func (s *labelSet) add(name, value string) {
	s.items = append(s.items, label{name: name, value: value})
}

func (s labelSet) clone() labelSet {
	return labelSet{items: append([]label(nil), s.items...)}
}

func (s labelSet) value(name string) (string, bool) {
	for _, item := range s.items {
		if item.name == name {
			return item.value, true
		}
	}
	return "", false
}

// valueOrNull preserves the helper's diagnostic representation for missing
// Kubernetes metadata. Resolution validity is determined from value presence.
func (s labelSet) valueOrNull(name string) string {
	if value, ok := s.value(name); ok {
		return value
	}
	return "null"
}

func (s labelSet) without(name string) labelSet {
	out := labelSet{items: make([]label, 0, len(s.items))}
	for _, item := range s.items {
		if item.name != name {
			out.items = append(out.items, item)
		}
	}
	return out
}

func (s labelSet) prefixed(prefix string) labelSet {
	out := labelSet{items: make([]label, 0, len(s.items))}
	for _, item := range s.items {
		out.items = append(out.items, label{name: prefix + item.name, value: item.value})
	}
	return out
}

func (s labelSet) String() string {
	if len(s.items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(s.items))
	for _, item := range s.items {
		parts = append(parts, formatLabel(item))
	}
	return strings.Join(parts, ",")
}

func formatLabel(item label) string {
	var b strings.Builder
	b.WriteString(item.name)
	b.WriteString(`="`)
	for _, r := range item.value {
		switch r {
		case '"', '\\':
			b.WriteRune('\\')
			b.WriteRune(r)
		case '\t', '\n', '\r':
			b.WriteRune(' ')
		default:
			if r < ' ' || r == 0x7f {
				b.WriteRune(' ')
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteRune('"')
	return b.String()
}

func parseLabelSet(raw string) labelSet {
	var out labelSet
	for _, part := range splitLabelPairs(raw) {
		name, value, ok := parseLabelPair(part)
		if ok {
			out.add(name, value)
		}
	}
	return out
}

func splitLabelPairs(raw string) []string {
	if raw == "" {
		return nil
	}

	var out []string
	start := 0
	inQuote := false
	escaped := false
	for i := 0; i < len(raw); i++ {
		switch raw[i] {
		case '\\':
			if inQuote {
				escaped = !escaped
				continue
			}
		case '"':
			if !escaped {
				inQuote = !inQuote
			}
		case ',':
			if !inQuote {
				if part := strings.TrimSpace(raw[start:i]); part != "" {
					out = append(out, part)
				}
				start = i + 1
			}
		}
		if raw[i] != '\\' || !inQuote {
			escaped = false
		}
	}
	if part := strings.TrimSpace(raw[start:]); part != "" {
		out = append(out, part)
	}
	return out
}

func parseLabelPair(raw string) (string, string, bool) {
	name, value, ok := strings.Cut(raw, "=")
	if !ok || name == "" || value == "" {
		return "", "", false
	}

	if unquoted, err := strconv.Unquote(value); err == nil {
		return name, unquoted, true
	}
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		return name, value[1 : len(value)-1], true
	}
	return name, value, true
}

func inspectLabelValue(value string) string {
	if unquoted, err := strconv.Unquote(value); err == nil {
		return unquoted
	}
	return value
}

func formatLabelSets(sets []labelSet) string {
	lines := make([]string, 0, len(sets))
	for _, set := range sets {
		lines = append(lines, set.String())
	}
	return strings.Join(lines, "\n")
}

func findLabelSetByValue(sets []labelSet, name, value string) (labelSet, bool) {
	if name == "" || value == "" {
		return labelSet{}, false
	}
	for _, set := range sets {
		if candidate, ok := set.value(name); ok && candidate == value {
			return set.clone(), true
		}
	}
	return labelSet{}, false
}
