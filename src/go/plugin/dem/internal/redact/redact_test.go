// SPDX-License-Identifier: GPL-3.0-or-later
package redact

import (
	"math/rand/v2"
	"net/url"
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestURLCredentialParameters(t *testing.T) {
	raw := "https://example.org/?client_secret=first&client%5Fsecret=second&CLIENT_SECRET=third&view=home&token=fourth"
	redacted := NewRedactor().ApplyURL(raw)
	for _, secret := range []string{"first", "second", "third", "fourth"} {
		require.NotContains(t, redacted, secret)
	}
	parsed, err := url.Parse(redacted)
	require.NoError(t, err)
	require.Equal(t, []string{"[REDACTED]", "[REDACTED]"}, parsed.Query()["client_secret"])
	require.Equal(t, "home", parsed.Query().Get("view"))
	require.Equal(t, "[REDACTED]", parsed.Query().Get("CLIENT_SECRET"))
}

func TestTextCredentialPatterns(t *testing.T) {
	r := NewRedactor("configured-value")
	for _, tc := range []struct{ input, want string }{
		{"", ""},
		{"ordinary checkout failure", "ordinary checkout failure"},
		{"configured-value Bearer abc.def token=one password=two", "[REDACTED] [REDACTED] token=[REDACTED] password=[REDACTED]"},
		{"key AKIA0123456789ABCDEF", "key [REDACTED]"},
		{"-----BEGIN PRIVATE KEY-----\nsynthetic\n-----END PRIVATE KEY-----", "[REDACTED]"},
		{"token=[REDACTED]", "token=[REDACTED]"},
	} {
		require.Equal(t, tc.want, r.Apply(tc.input))
	}
}

func TestApplyRemoving(t *testing.T) {
	r := NewRedactor("configured-value")
	for input, want := range map[string]string{
		"":                                "",
		"ordinary text":                   "ordinary text",
		"line one\nline two\x00":          "line oneline two",
		"tok\x00en=hunter2":               "token=[REDACTED]",
		"Bea\u0085rer abcdef":             "[REDACTED]",
		"configu\x1bred-value":            "[REDACTED]",
		"Bearer abcdef\x00ghijkl":         "[REDACTED]",
		"Bearer configured\t-value":       "[REDACTED]",
		"Bearer\tabcdef":                  "[REDACTED]",
		"a\x01token=hunter2":              "atoken=[REDACTED]",
		"token=\x00":                      "token=[REDACTED]",
		"AKIA0123456789ABCDEF\x01token=x": "[REDACTED]token=[REDACTED]",
		"\x7fkey Bearer\nabc tok\x00en=x": "key [REDACTED] token=[REDACTED]",
		// Configured values are replaced before the patterns run, as in Apply.
		"Bearer configured-value-----BEGIN A KEY-----body-----END A KEY-----\x00": "Bearer [REDACTED]",
		"Bearer configured-valuetoken=abc\x00":                                    "Bearer [REDACTED]token=[REDACTED]",
	} {
		assert.Equal(t, want, r.ApplyRemoving(input, unicode.IsControl), "input %q", input)
	}
}

// The span engine behind ApplyRemoving replaces what Apply replaces; only
// adjacent markers may merge.
func TestSpansMatchApply(t *testing.T) {
	r := NewRedactor("configured-value")
	collapse := func(s string) string {
		for strings.Contains(s, redacted+redacted) {
			s = strings.ReplaceAll(s, redacted+redacted, redacted)
		}
		return s
	}
	keep := func(rune) bool { return false }
	rng := rand.New(rand.NewPCG(3, 4))
	for range 20000 {
		input := randomText(rng)
		require.Equal(t, collapse(r.Apply(input)), collapse(replaceSpans(input, r.spans(input), keep)), "input %q", input)
	}
}

// randomText joins fragments that interact: separators, removable characters,
// word boundaries, keys, values, markers and the configured value.
func randomText(rng *rand.Rand) string {
	fragments := []string{
		"bearer", "Bearer", " ", "\t", "\n", "\x00", "\x01", "\u0085", "token", "password", "api_key", "=", "&", "'",
		`"`, "akia", "AKIA", "0123456789abcdef", "ABCDEF", "-----BEGIN A KEY-----", "-----END A KEY-----", "[REDACTED]",
		"x", "_", ".", "/", "configured-value", "configured", "-value", "QXSECRET",
	}
	var b strings.Builder
	for range 1 + rng.IntN(12) {
		b.WriteString(fragments[rng.IntN(len(fragments))])
	}
	return b.String()
}

// Redacted text passes through the redactor and the credential patterns
// unchanged, so a later pattern pass over redacted output is a no-op.
func TestApplyOutputIsStable(t *testing.T) {
	site := NewRedactor("configured-value")
	patterns := NewRedactor()
	rng := rand.New(rand.NewPCG(1, 2))
	for range 20000 {
		input := randomText(rng)
		out := site.Apply(input)
		require.Equal(t, out, site.Apply(out), "input %q", input)
		require.Equal(t, out, patterns.Apply(out), "input %q", input)
		removed := site.ApplyRemoving(input, unicode.IsControl)
		require.Equal(t, -1, strings.IndexFunc(removed, unicode.IsControl), "input %q", input)
		require.Equal(t, removed, site.Apply(removed), "input %q", input)
		require.Equal(t, removed, patterns.Apply(removed), "input %q", input)
	}
}

// ApplyRemoving redacts at least what Apply redacts in either form: a planted
// token survives it no more often than it survives either form's Apply.
func TestApplyRemovingCoversBothForms(t *testing.T) {
	r := NewRedactor("configured-value")
	strip := func(s string) string {
		return strings.Map(func(c rune) rune {
			if unicode.IsControl(c) {
				return -1
			}
			return c
		}, s)
	}
	rng := rand.New(rand.NewPCG(5, 6))
	for range 20000 {
		input := randomText(rng)
		got := strings.Count(r.ApplyRemoving(input, unicode.IsControl), "QXSECRET")
		limit := min(strings.Count(strip(r.Apply(input)), "QXSECRET"), strings.Count(r.Apply(strip(input)), "QXSECRET"))
		require.LessOrEqual(t, got, limit, "input %q", input)
	}
}

// Ingestion checks every descriptive string; ordinary values should not allocate
// replacement copies. Timings are development-machine trends, not CI gates.
func BenchmarkApply(b *testing.B) {
	r := NewRedactor("configured-value")
	for _, tc := range []struct{ name, value string }{
		{"ordinary", "ordinary checkout failure"},
		{"credentials", "configured-value Bearer abc.def token=one password=two"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				r.Apply(tc.value)
			}
		})
	}
}

// Text without removable characters costs one Apply; removable characters add
// one copy and a pattern check of the cleaned text. Timings are
// development-machine trends, not CI gates.
func BenchmarkApplyRemoving(b *testing.B) {
	r := NewRedactor("configured-value")
	for _, tc := range []struct{ name, value string }{
		{"ordinary", "ordinary checkout failure"},
		{"multiline", "checkout failed\nretry later\npayment widget unavailable"},
		{"credentials", "request failed\ntok\x00en=one Bearer abc.def"},
		{"long credentials", strings.Repeat("request failed\n", 64) + "tok\x00en=one Bearer abc.def"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				r.ApplyRemoving(tc.value, unicode.IsControl)
			}
		})
	}
}
