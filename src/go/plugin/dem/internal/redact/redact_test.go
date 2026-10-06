// SPDX-License-Identifier: GPL-3.0-or-later
package redact

import (
	"net/url"
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
