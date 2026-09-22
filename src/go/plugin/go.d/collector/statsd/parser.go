// SPDX-License-Identifier: GPL-3.0-or-later

package statsd

import (
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

type wireType string

const (
	counter   wireType = "c"
	gauge     wireType = "g"
	timer     wireType = "ms"
	histogram wireType = "h"
	set       wireType = "s"
)

// rejection values are bounded diagnostic reasons, never input text.
type rejection string

func (r rejection) Error() string { return string(r) }

const (
	rejectSyntax      rejection = "syntax"
	rejectValue       rejection = "value"
	rejectRate        rejection = "rate"
	rejectLabels      rejection = "labels"
	rejectMetadata    rejection = "metadata"
	rejectType        rejection = "type_conflict"
	rejectBaseline    rejection = "gauge_baseline"
	rejectCapacity    rejection = "capacity"
	rejectOverflow    rejection = "overflow"
	rejectUnavailable rejection = "receiver_unavailable"
)

type record struct {
	name        string
	kind        wireType
	labels      []metrix.Label
	value, rate float64
	member      string
	delta       bool
}

// parseRecord accepts one complete payload. Transport framing owns the record
// size bound and removes LF/CRLF; there is no second byte quota here.
func parseRecord(line string) (record, error) {
	r := record{
		rate: 1,
	}
	if !validText(line) {
		return r, rejectSyntax
	}
	head, rest, ok := strings.Cut(line, "|")
	if !ok {
		return r, rejectSyntax
	}
	r.name, head, ok = strings.Cut(head, ":")
	if !ok || !validName(r.name) || head == "" {
		return r, rejectSyntax
	}
	kind, rest, hasOptions := strings.Cut(rest, "|")
	// Retained declarations/bindings must not keep a substring of the record.
	// Canonical constants also avoid an allocation on every accepted update.
	switch wireType(kind) {
	case counter:
		r.kind = counter
	case gauge:
		r.kind = gauge
	case timer:
		r.kind = timer
	case histogram:
		r.kind = histogram
	case set:
		r.kind = set
	default:
		return r, rejectSyntax
	}
	switch r.kind {
	case counter, gauge, timer, histogram:
		value, err := parseNumber(head)
		if err != nil || ((r.kind == counter || r.kind == timer) && value < 0) {
			return r, rejectValue
		}
		r.value = value
		r.delta = r.kind == gauge && (head[0] == '+' || head[0] == '-')
	case set:
		r.member = head
	}
	var seenRate, seenTags bool
	for hasOptions {
		var field string
		field, rest, hasOptions = strings.Cut(rest, "|")
		if field == "" {
			return r, rejectSyntax
		}
		switch field[0] {
		case '@':
			if seenRate {
				return r, rejectSyntax
			}
			seenRate = true
			var err error
			r.rate, err = parseNumber(field[1:])
			if err != nil || r.rate <= 0 || r.rate > 1 {
				return r, rejectRate
			}
		case '#':
			if seenTags {
				return r, rejectSyntax
			}
			seenTags = true
			var err error
			r.labels, err = parseTags(field[1:])
			if err != nil {
				return r, err
			}
		default:
			return r, rejectSyntax
		}
	}
	if (r.kind == gauge || r.kind == set) && r.rate != 1 {
		return r, rejectRate
	}
	return r, nil
}

func parseNumber(text string) (float64, error) {
	if text == "" {
		return 0, rejectValue
	}
	for _, c := range text {
		if (c < '0' || c > '9') && !strings.ContainsRune("+-.eE", c) {
			return 0, rejectValue
		}
	}
	v, err := strconv.ParseFloat(text, 64)
	if err != nil || !finite(v) {
		return 0, rejectValue
	}
	return v, nil
}

func parseTags(text string) ([]metrix.Label, error) {
	values := make(map[string]string)
	for tag := range strings.SplitSeq(text, ",") {
		key, value, ok := strings.Cut(tag, ":")
		if !ok || key == "" || value == "" {
			return nil, rejectSyntax
		}
		if key == "_collect_job" {
			return nil, rejectLabels
		}
		if old, exists := values[key]; exists && old != value {
			return nil, rejectLabels
		}
		values[key] = value
	}
	labels := make([]metrix.Label, 0, len(values))
	for k, v := range values {
		labels = append(labels, metrix.Label{
			Key:   k,
			Value: v,
		})
	}
	slices.SortFunc(labels, func(a, b metrix.Label) int { return strings.Compare(a.Key, b.Key) })
	return labels, nil
}

func validText(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, c := range s {
		if unicode.IsControl(c) {
			return false
		}
	}
	return true
}

func validName(s string) bool {
	if s == "" || !validText(s) {
		return false
	}
	for _, c := range s {
		if unicode.IsSpace(c) || c == ':' || c == '|' {
			return false
		}
	}
	return true
}
