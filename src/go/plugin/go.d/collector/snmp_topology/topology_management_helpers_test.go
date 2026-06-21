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
		"hex-ipv4":        {addr: "0A14043C", addrType: "1", wantAddr: "10.20.4.60", wantType: "ipv4"},
		"ascii-ipv4":      {addr: "31302E32302E342E323035", wantAddr: "10.20.4.205", wantType: "ipv4"},
		"mapped-ipv4":     {addr: "::ffff:192.0.2.1", addrType: "2", wantAddr: "192.0.2.1", wantType: "ipv4"},
		"mapped-ipv4-hex": {addr: "00000000000000000000ffffc0000201", addrType: "2", wantAddr: "192.0.2.1", wantType: "ipv4"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			addr, addrType := normalizeManagementAddress(tc.addr, tc.addrType)
			require.Equal(t, tc.wantAddr, addr)
			require.Equal(t, tc.wantType, addrType)
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
