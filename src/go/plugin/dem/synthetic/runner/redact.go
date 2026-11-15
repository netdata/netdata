// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	redact "github.com/netdata/netdata/go/plugins/plugin/dem/internal/redact"
)

// textRedactor applies the journey secret policy (redact.Secrets) to runner
// text.
type textRedactor struct {
	secrets *redact.Secrets
	// known (longest first) and longest let the diagnostic stream match secrets
	// split across chunks.
	known   []string
	longest int
	// patterns redacts the run target URL.
	patterns *redact.Redactor
}

func newRedactor(values map[string]string) *textRedactor {
	secrets := redact.NewSecrets(slices.Collect(maps.Values(values))...)
	longest := 1
	for _, value := range secrets.Values() {
		longest = max(longest, len(value))
	}
	return &textRedactor{
		secrets:  secrets,
		known:    secrets.Values(),
		longest:  longest,
		patterns: redact.Patterns,
	}
}

func (r *textRedactor) text(value string) string {
	return clip(r.redacted(value))
}

// redacted leaves clipping to typed consumers that must disclose it.
func (r *textRedactor) redacted(value string) string {
	value = r.secrets.Apply(value)
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
