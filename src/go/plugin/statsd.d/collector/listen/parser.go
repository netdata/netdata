// SPDX-License-Identifier: GPL-3.0-or-later

package listen

import (
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

// wireType is the StatsD type token. Parsing returns these canonical constants,
// so retained state never holds a substring of the receive record.
type wireType string

const (
	counter   wireType = "c"
	gauge     wireType = "g"
	timer     wireType = "ms"
	histogram wireType = "h"
	set       wireType = "s"
)

// record is one parsed line. value is the numeric payload of c/g/ms/h, member
// the set payload; delta marks a signed gauge update.
type record struct {
	name        string
	kind        wireType
	labels      []metrix.Label
	value, rate float64
	member      string
	delta       bool
}

// parseRecord accepts one complete payload. Transport framing owns the record
// size bound and removes LF/CRLF; there is no second byte quota here. Parsed
// labels are appended to tags[:0], so they may share the caller's storage.
func parseRecord(line string, tags []metrix.Label) (record, error) {
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
			r.labels, err = parseTags(field[1:], tags[:0])
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
	for i := range len(text) {
		switch c := text[i]; {
		case c >= '0' && c <= '9', c == '+', c == '-', c == '.', c == 'e', c == 'E':
		default:
			return 0, rejectValue
		}
	}
	v, err := strconv.ParseFloat(text, 64)
	if err != nil || !finite(v) {
		return 0, rejectValue
	}
	return v, nil
}

// parseTags appends the tags, sorted by key, to labels. Identical repeats
// collapse. A conflicting repeat rejects as labels, and takes precedence over a
// later malformed tag, which the record never reaches. The storage behind labels
// keeps no string beyond the result, so a caller clearing the result's length
// retains nothing.
func parseTags(text string, labels []metrix.Label) ([]metrix.Label, error) {
	start, storage := len(labels), labels[len(labels):cap(labels)]
	var err error
	for tag := range strings.SplitSeq(text, ",") {
		key, value, ok := strings.Cut(tag, ":")
		if !ok || key == "" || value == "" {
			err = rejectSyntax
			break
		}
		if key == "_collect_job" {
			err = rejectLabels
			break
		}
		labels = append(labels, metrix.Label{
			Key:   key,
			Value: value,
		})
	}
	tags := labels[start:]
	slices.SortFunc(tags, func(a, b metrix.Label) int { return strings.Compare(a.Key, b.Key) })
	n, conflict := 0, false
	for _, l := range tags {
		if n > 0 && tags[n-1].Key == l.Key {
			conflict = conflict || tags[n-1].Value != l.Value
			continue
		}
		tags[n] = l
		n++
	}
	clear(tags[n:])
	if cap(labels) > start+len(storage) {
		clear(storage) // Outgrown: the tags now live in a new array.
	}
	labels = labels[:start+n]
	if conflict {
		return labels, rejectLabels
	}
	return labels, err
}

// validText reports valid UTF-8 without control characters. ASCII is checked
// bytewise; the rest of a string from its first non-ASCII byte takes the rune path.
func validText(s string) bool {
	for i := range len(s) {
		c := s[i]
		if c >= utf8.RuneSelf {
			return validUnicodeText(s[i:])
		}
		if c < ' ' || c == 0x7f {
			return false
		}
	}
	return true
}

func validUnicodeText(s string) bool {
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

// validName is validText excluding whitespace, ':' and '|'. ASCII whitespace is
// space or a control character.
func validName(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		c := s[i]
		if c >= utf8.RuneSelf {
			return validUnicodeName(s[i:])
		}
		if c <= ' ' || c == 0x7f || c == ':' || c == '|' {
			return false
		}
	}
	return true
}

func validUnicodeName(s string) bool {
	if !validUnicodeText(s) {
		return false
	}
	for _, c := range s {
		if unicode.IsSpace(c) || c == ':' || c == '|' {
			return false
		}
	}
	return true
}
