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
		"mac":             {normalize: NormalizeMAC, in: "hex-string: 00 11 22 33 44 55", want: "00:11:22:33:44:55"},
		"ip-address":      {normalize: NormalizeIPAddress, in: "0A14043C", want: "10.20.4.60"},
		"mapped-ipv4":     {normalize: NormalizeIPAddress, in: "::ffff:192.0.2.1", want: "192.0.2.1"},
		"mapped-ipv4-hex": {normalize: NormalizeIPAddress, in: "00000000000000000000ffffc0000201", want: "192.0.2.1"},
		"non-unspecified": {normalize: NormalizeNonUnspecifiedIPAddress, in: "::ffff:192.0.2.1", want: "192.0.2.1"},
		"hex-token":       {normalize: NormalizeHexToken, in: "31302E32302E342E323035", want: "10.20.4.205"},
		"hex-identifier":  {normalize: NormalizeHexIdentifier, in: "00:11:22:33:44:55", want: "001122334455"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.normalize(tc.in))
		})
	}
}

func TestNormalizeBGPPeerAddress(t *testing.T) {
	tests := map[string]struct {
		in   string
		want string
	}{
		"normalizes-hex-ip":             {in: "C0000202", want: "192.0.2.2"},
		"normalizes-ipv6-mapped-ipv4":   {in: "::ffff:192.0.2.2", want: "192.0.2.2"},
		"drops-unspecified-ip":          {in: "0.0.0.0"},
		"drops-unspecified-ipv6":        {in: "::"},
		"preserves-non-ip-diagnostic":   {in: "peer-token", want: "peer-token"},
		"trims-non-ip-diagnostic-token": {in: " peer-token ", want: "peer-token"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, NormalizeBGPPeerAddress(tc.in))
		})
	}
}

func TestNormalizeOSPFNeighborState(t *testing.T) {
	tests := map[string]struct {
		in   string
		want string
	}{
		"numeric-full":          {in: "8", want: "full"},
		"numeric-two-way":       {in: "4", want: "twoWay"},
		"snake-exstart":         {in: "exchange_start", want: "exchangeStart"},
		"short-exstart":         {in: "exstart", want: "exchangeStart"},
		"preserves-vendor":      {in: "vendorSpecific", want: "vendorSpecific"},
		"trims-preserved-value": {in: " vendorSpecific ", want: "vendorSpecific"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, NormalizeOSPFNeighborState(tc.in))
		})
	}
}
