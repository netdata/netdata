// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"cmp"
	"slices"
	"strings"
	"unicode/utf8"

	redact "github.com/netdata/netdata/go/plugins/plugin/dem/internal/redact"
)

type textRedactor struct {
	known    []string
	longest  int
	exact    *strings.Replacer
	patterns *redact.Redactor
}

func newRedactor(values map[string]string) *textRedactor {
	var known []string
	for _, value := range values {
		if value != "" {
			known = append(known, value)
		}
	}
	// Longest first, so a secret containing another is replaced whole.
	slices.SortFunc(known, func(a, b string) int { return cmp.Compare(len(b), len(a)) })
	var pairs []string
	for _, value := range known {
		pairs = append(pairs, value, "[REDACTED]")
	}
	longest := 1
	for _, value := range known {
		longest = max(longest, len(value))
	}
	return &textRedactor{
		known:    known,
		longest:  longest,
		exact:    strings.NewReplacer(pairs...),
		patterns: redact.NewRedactor(),
	}
}
func (r *textRedactor) text(value string) string {
	return clip(r.redacted(value))
}

// redacted leaves clipping to typed consumers that must disclose it.
func (r *textRedactor) redacted(value string) string {
	value = r.patterns.Apply(r.exact.Replace(value))
	// An upstream text cap or cancellation may remove a PEM closing marker.
	// Retain no suffix of an unfinished block just because it no longer matches.
	if start := strings.Index(asciiLower(value), "-----begin "); start >= 0 {
		value = value[:start] + "[REDACTED]"
	}
	return value
}

// asciiLower lowers ASCII letters only, so byte offsets match the input;
// strings.ToLower can change the encoded length of other characters.
func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + 'a' - 'A'
		}
	}
	return string(b)
}

func clip(s string) string {
	if utf8.RuneCountInString(s) <= textLimit {
		return s
	}
	return string([]rune(s)[:textLimit])
}
