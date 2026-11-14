// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRedactedCutsUnfinishedPEMAtItsMarker finds an unfinished PEM block in
// the original text even when lowercasing earlier characters changes their
// byte length.
func TestRedactedCutsUnfinishedPEMAtItsMarker(t *testing.T) {
	grows := strings.Repeat("Ⱥ", 60) // lowercases to a longer UTF-8 encoding
	shrinks := strings.Repeat("İ", 60)
	tests := map[string]struct {
		value string
		want  string
	}{
		"ascii prefix":         {value: "key: -----BEGIN PRIVATE KEY-----\nMIIsecretkeymaterial", want: "key: [REDACTED]"},
		"lowercase marker":     {value: "-----begin private key-----\nMIIsecret", want: "[REDACTED]"},
		"longer when lowered":  {value: grows + "-----BEGIN PRIVATE KEY-----\nMIIsecretkeymaterialMIIsecretkeymaterial", want: grows + "[REDACTED]"},
		"marker near the end":  {value: grows + "-----BEGIN ", want: grows + "[REDACTED]"},
		"shorter when lowered": {value: shrinks + "-----BEGIN PRIVATE KEY-----\nMIIsecret", want: shrinks + "[REDACTED]"},
		"no marker":            {value: grows + " plain text", want: grows + " plain text"},
	}
	r := newRedactor(nil)
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var got string
			assert.NotPanics(t, func() { got = r.redacted(tc.value) })
			assert.Equal(t, tc.want, got)
			assert.NotContains(t, got, "MII")
		})
	}
}
