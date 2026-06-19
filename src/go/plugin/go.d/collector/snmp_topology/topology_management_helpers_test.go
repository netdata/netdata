// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeManagementAddress_DecodesHexAndASCIIIPs(t *testing.T) {
	tests := map[string]struct {
		addr     string
		addrType string
		wantAddr string
		wantType string
	}{
		"hex-ipv4":   {addr: "0A14043C", addrType: "1", wantAddr: "10.20.4.60", wantType: "ipv4"},
		"ascii-ipv4": {addr: "31302E32302E342E323035", wantAddr: "10.20.4.205", wantType: "ipv4"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			addr, addrType := normalizeManagementAddress(tc.addr, tc.addrType)
			require.Equal(t, tc.wantAddr, addr)
			require.Equal(t, tc.wantType, addrType)
		})
	}
}

func TestNormalizeHexHelpers_ClassifyTokensDeterministically(t *testing.T) {
	tests := map[string]struct {
		normalize func(string) string
		in        string
		want      string
	}{
		"mac":            {normalize: normalizeMAC, in: "hex-string: 00 11 22 33 44 55", want: "00:11:22:33:44:55"},
		"ip-address":     {normalize: normalizeIPAddress, in: "0A14043C", want: "10.20.4.60"},
		"hex-token":      {normalize: normalizeHexToken, in: "31302E32302E342E323035", want: "10.20.4.205"},
		"hex-identifier": {normalize: normalizeHexIdentifier, in: "00:11:22:33:44:55", want: "001122334455"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.normalize(tc.in))
		})
	}
}

func TestReconstructLldpRemMgmtAddrHex_FromOctets(t *testing.T) {
	require.Equal(t, "0a14043c", reconstructLldpRemMgmtAddrHex(map[string]string{
		tagLldpRemMgmtAddrLen:             "4",
		tagLldpRemMgmtAddrOctetPref + "1": "10",
		tagLldpRemMgmtAddrOctetPref + "2": "20",
		tagLldpRemMgmtAddrOctetPref + "3": "4",
		tagLldpRemMgmtAddrOctetPref + "4": "60",
	}))
}
