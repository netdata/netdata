// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeManagementAddress_DecodesHexAndASCIIIPs(t *testing.T) {
	addr, addrType := normalizeManagementAddress("0A14043C", "1")
	require.Equal(t, "10.20.4.60", addr)
	require.Equal(t, "ipv4", addrType)

	addr, addrType = normalizeManagementAddress("31302E32302E342E323035", "")
	require.Equal(t, "10.20.4.205", addr)
	require.Equal(t, "ipv4", addrType)
}

func TestNormalizeHexHelpers_ClassifyTokensDeterministically(t *testing.T) {
	require.Equal(t, "00:11:22:33:44:55", normalizeMAC("hex-string: 00 11 22 33 44 55"))
	require.Equal(t, "10.20.4.60", normalizeIPAddress("0A14043C"))
	require.Equal(t, "10.20.4.205", normalizeHexToken("31302E32302E342E323035"))
	require.Equal(t, "001122334455", normalizeHexIdentifier("00:11:22:33:44:55"))
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
