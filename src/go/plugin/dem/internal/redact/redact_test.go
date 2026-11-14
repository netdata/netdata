// SPDX-License-Identifier: GPL-3.0-or-later
package redact

import (
	"math/rand/v2"
	"net/url"
	"strings"
	"testing"

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

// Redacted text passes through the redactor and the credential patterns
// unchanged, so a later pattern pass over redacted output is a no-op. Inputs
// join fragments that interact: separators, word boundaries, keys, values and
// markers.
func TestApplyOutputIsStable(t *testing.T) {
	site := NewRedactor("configured-value")
	patterns := NewRedactor()
	fragments := []string{
		"bearer", "Bearer", " ", "\t", "\n", "token", "password", "api_key", "=", "&", "'", `"`, "akia", "AKIA",
		"0123456789abcdef", "ABCDEF", "-----BEGIN A KEY-----", "-----END A KEY-----", "[REDACTED]", "x", "_", ".", "/",
		"configured-value", "configured", "-value",
	}
	rng := rand.New(rand.NewPCG(1, 2))
	for range 20000 {
		var b strings.Builder
		for range 1 + rng.IntN(12) {
			b.WriteString(fragments[rng.IntN(len(fragments))])
		}
		out := site.Apply(b.String())
		require.Equal(t, out, site.Apply(out), "input %q", b.String())
		require.Equal(t, out, patterns.Apply(out), "input %q", b.String())
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
