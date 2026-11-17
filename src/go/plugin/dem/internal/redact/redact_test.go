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
	redacted := Patterns.ApplyURL(raw)
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

// Both exact-value policies run before the credential patterns; they differ in
// which values they replace and how.
func TestValuePolicies(t *testing.T) {
	tests := map[string]struct {
		apply func(string) string
		input string
		want  string
	}{
		"site values shorter than four bytes are ignored": {
			apply: NewRedactor("abc", "", "abcd").Apply,
			input: "abc abcd",
			want:  "abc [REDACTED]",
		},
		"secrets of any length": {
			apply: NewSecrets("x", "").Apply,
			input: "x-ray",
			want:  "[REDACTED]-ray",
		},
		"a secret containing another is replaced whole": {
			apply: NewSecrets("pass", "password1").Apply,
			input: "password1 pass",
			want:  "[REDACTED] [REDACTED]",
		},
		"one pass never rescans a marker": {
			apply: NewSecrets("x", "RED").Apply,
			input: "x RED",
			want:  "[REDACTED] [REDACTED]",
		},
		"secrets then patterns": {
			apply: NewSecrets("abc").Apply,
			input: "token=abc Bearer xyz",
			want:  "token=[REDACTED] [REDACTED]",
		},
		"patterns alone": {
			apply: Patterns.Apply,
			input: "abcd password=abcd",
			want:  "abcd password=[REDACTED]",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) { assert.Equal(t, tc.want, tc.apply(tc.input)) })
	}
}

// Streamed matching relies on the secrets being listed longest first.
func TestSecretsValuesLongestFirst(t *testing.T) {
	assert.Equal(t, []string{"password1", "pass", "x"}, NewSecrets("x", "", "pass", "password1").Values())
}

// siteRedactor holds configured values that overlap each other and the
// credential patterns, so replacement order matters.
func siteRedactor() *Redactor { return NewRedactor("configured-value", "wxyz", "Bearer wxyzto") }

func TestApplyRemoving(t *testing.T) {
	r := siteRedactor()
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
		// An earlier configured value replaces text a later one would match.
		"Bearer wxyztoken=QXSECRET\x00": "Bearer [REDACTED]token=[REDACTED]",
	} {
		assert.Equal(t, want, r.ApplyRemoving(input, unicode.IsControl), "input %q", input)
	}
}

// Configured values are replaced in order, as in Apply: an earlier one can
// remove text a later one would match and give a key its word boundary.
func TestApplyRemovingFollowsValueOrder(t *testing.T) {
	r := NewRedactor("abcd", "Bearer abcdto")
	input := "Bearer abcdtoken=QXSECRET\x00"
	assert.Equal(t, "Bearer [REDACTED]token=[REDACTED]", r.ApplyRemoving(input, unicode.IsControl))
	assert.Equal(t, "Bearer [REDACTED]token=[REDACTED]", r.Apply(input))
	// A later value cannot match across an earlier replacement, whatever its bytes.
	r = NewRedactor("abcd", "\xfftok")
	input = "abcdtoken=QXSECRET\x00"
	assert.Equal(t, "[REDACTED]token=[REDACTED]", r.ApplyRemoving(input, unicode.IsControl))
}

// The span engine behind ApplyRemoving replaces what Apply replaces; only
// adjacent markers may merge.
func TestSpansMatchApply(t *testing.T) {
	r := siteRedactor()
	collapse := func(s string) string {
		for strings.Contains(s, Marker+Marker) {
			s = strings.ReplaceAll(s, Marker+Marker, Marker)
		}
		return s
	}
	keep := func(rune) bool { return false }
	rng := rand.New(rand.NewPCG(3, 4))
	for range 20000 {
		input := randomText(rng)
		require.Equal(
			t,
			collapse(r.Apply(input)),
			collapse(replaceSpans(input, r.spans(input), keep)),
			"input %q",
			input,
		)
	}
}

// RemovingSpans describes ApplyRemoving's result in the coordinates of its
// input: replacing the spans gives the same text, apart from adjacent markers.
func TestRemovingSpansMatchApplyRemoving(t *testing.T) {
	r := siteRedactor()
	collapse := func(s string) string {
		for strings.Contains(s, Marker+Marker) {
			s = strings.ReplaceAll(s, Marker+Marker, Marker)
		}
		return s
	}
	// Lone invalid UTF-8 bytes check the mapping around runes removal can cut or join.
	lone := []string{"\xc2", "\x85", "\xff", "\xe2\x82", "\u0085"}
	rng := rand.New(rand.NewPCG(7, 8))
	for i := range 50000 {
		input := randomText(rng)
		if i%2 == 1 {
			for range 1 + rng.IntN(3) {
				pos := rng.IntN(len(input) + 1)
				input = input[:pos] + lone[rng.IntN(len(lone))] + input[pos:]
			}
		}
		var spans []span
		previous := -1
		for _, sp := range r.RemovingSpans(input, unicode.IsControl) {
			require.Greater(t, sp.Start, previous, "input %q: spans are merged and ordered", input)
			require.LessOrEqual(t, sp.Start, sp.End, "input %q", input)
			previous = sp.End
			spans = append(spans, span{
				start: sp.Start,
				end:   sp.End,
			})
		}
		require.Equal(
			t,
			collapse(r.ApplyRemoving(input, unicode.IsControl)),
			collapse(replaceSpans(input, spans, unicode.IsControl)),
			"input %q",
			input,
		)
	}
}

// randomText joins fragments that interact: separators, removable characters,
// word boundaries, keys, values, markers and the configured value.
func randomText(rng *rand.Rand) string {
	fragments := []string{
		"bearer", "Bearer", " ", "\t", "\n", "\x00", "\x01", "\u0085", "token", "password", "api_key", "=", "&", "'",
		`"`, "akia", "AKIA", "0123456789abcdef", "ABCDEF", "-----BEGIN A KEY-----", "-----END A KEY-----", "[REDACTED]",
		"x", "_", ".", "/", "configured-value", "configured", "-value", "QXSECRET",
		"wxyz", "to", "Bearer wxyzto",
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
	site := siteRedactor()
	rng := rand.New(rand.NewPCG(1, 2))
	for range 20000 {
		input := randomText(rng)
		out := site.Apply(input)
		require.Equal(t, out, site.Apply(out), "input %q", input)
		require.Equal(t, out, Patterns.Apply(out), "input %q", input)
		removed := site.ApplyRemoving(input, unicode.IsControl)
		require.Equal(t, -1, strings.IndexFunc(removed, unicode.IsControl), "input %q", input)
		require.Equal(t, removed, site.Apply(removed), "input %q", input)
		require.Equal(t, removed, Patterns.Apply(removed), "input %q", input)
	}
}

// ApplyRemoving redacts at least what Apply redacts in either form: a planted
// token survives it no more often than it survives either form's Apply.
func TestApplyRemovingCoversBothForms(t *testing.T) {
	r := siteRedactor()
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
