// SPDX-License-Identifier: GPL-3.0-or-later

package topologyutil

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
			require.Equal(t, tc.want, NormalizeSNMPHexText(tc.in))
		})
	}
}

func TestNormalizeRouterIDDropsUnspecifiedAddresses(t *testing.T) {
	tests := map[string]struct {
		in   string
		want string
	}{
		"ipv4-unspecified": {in: "0.0.0.0"},
		"ipv6-unspecified": {in: "::"},
		"hex-unspecified":  {in: "00000000"},
		"valid-router-id":  {in: " 1.2.3.4 ", want: "1.2.3.4"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, NormalizeTopologyRouterID(tc.in))
		})
	}
}

func TestNormalizeHexHelpers_ClassifyTokensDeterministically(t *testing.T) {
	tests := map[string]struct {
		normalize func(string) string
		in        string
		want      string
	}{
		"mac":            {normalize: NormalizeMAC, in: "hex-string: 00 11 22 33 44 55", want: "00:11:22:33:44:55"},
		"ip-address":     {normalize: NormalizeIPAddress, in: "0A14043C", want: "10.20.4.60"},
		"mapped-ipv4":    {normalize: NormalizeNonUnspecifiedIPAddress, in: "::ffff:192.0.2.1", want: "192.0.2.1"},
		"hex-token":      {normalize: NormalizeHexToken, in: "31302E32302E342E323035", want: "10.20.4.205"},
		"hex-identifier": {normalize: NormalizeHexIdentifier, in: "00:11:22:33:44:55", want: "001122334455"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.normalize(tc.in))
		})
	}
}
