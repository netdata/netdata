// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeSNMPHexText_StripsPrefixesAndQuotes(t *testing.T) {
	tests := map[string]struct {
		in   string
		want string
	}{
		"quoted-hex-string":   {in: `"hex-string: 00 11 22 33"`, want: "00 11 22 33"},
		"octet-string-prefix": {in: "octet string: 0A14043C", want: "0A14043C"},
		"quoted-string":       {in: `'string: abc'`, want: "abc"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, normalizeSNMPHexText(tc.in))
		})
	}
}

func TestDecodeLLDPCapabilities(t *testing.T) {
	require.Equal(t,
		[]string{"bridge", "router"},
		decodeLLDPCapabilities("28"),
	)
}

func TestInferCategoryFromCapabilities(t *testing.T) {
	tests := map[string]struct {
		capabilities []string
		want         string
	}{
		"access-point": {capabilities: []string{"wlanAccessPoint"}, want: "access point"},
		"router":       {capabilities: []string{"bridge", "router"}, want: "router"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, inferCategoryFromCapabilities(tc.capabilities))
		})
	}
}
