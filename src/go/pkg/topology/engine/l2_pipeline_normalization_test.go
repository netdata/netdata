// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCSVToTopologySet_NormalizesAndDeduplicates(t *testing.T) {
	set := csvToTopologySet(" LLDP, cdp , , lldp, STP ")
	require.Equal(t, map[string]struct{}{
		"lldp": {},
		"cdp":  {},
		"stp":  {},
	}, set)
}

func TestCanonicalAddrType_PrefersIPFamilyAndFallsBack(t *testing.T) {
	require.Equal(t, "ipv4", canonicalAddrType("", "10.0.0.1"))
	require.Equal(t, "ipv6", canonicalAddrType("ipv4", "2001:db8::1"))
	require.Equal(t, "other", canonicalAddrType(" Other ", "not-an-ip"))
	require.Equal(t, "", canonicalAddrType("", "not-an-ip"))
}

func TestCanonicalARPProtocol_DefaultsToARP(t *testing.T) {
	require.Equal(t, "arp", canonicalARPProtocol(""))
	require.Equal(t, "arp", canonicalARPProtocol(" ARP "))
	require.Equal(t, "nd", canonicalARPProtocol(" ND "))
	require.Equal(t, "arp", canonicalARPProtocol("bogus"))
}

func TestDeriveRemoteDeviceID_PreferenceOrder(t *testing.T) {
	require.Equal(t, "switch-a", deriveRemoteDeviceID("Switch-A.", "00:11:22:33:44:55", "10.0.0.1", "fallback"))
	require.Equal(t, "chassis-001122334455", deriveRemoteDeviceID("", "00:11:22:33:44:55", "10.0.0.1", "fallback"))
	require.Equal(t, "ip-2001-db8--1", deriveRemoteDeviceID("", "", "2001:db8::1", "fallback"))
	require.Equal(t, "fallback-host", deriveRemoteDeviceID("", "", "", "Fallback-Host."))
	require.Equal(t, "discovered-unknown", deriveRemoteDeviceID("", "", "", ""))
}

func TestCanonicalToken_Strips0xPrefix(t *testing.T) {
	require.Equal(t, "001122334455", canonicalToken("0x00:11:22:33:44:55"))
	require.Equal(t, "001122334455", canonicalToken("00:11:22:33:44:55"))
	require.Equal(t, deriveRemoteDeviceID("", "0x00:11:22:33:44:55", "", ""), deriveRemoteDeviceID("", "00:11:22:33:44:55", "", ""))
}

func TestNormalizeLLDPPortHelpers_NormalizeBySubtype(t *testing.T) {
	require.Equal(t, "001122334455", normalizeLLDPPortIDForMatch("00:11:22:33:44:55", "macAddress"))
	require.Equal(t, "10.0.0.1", normalizeLLDPPortIDForMatch("0a000001", "networkAddress"))
	require.Equal(t, "Gi0/1", normalizeLLDPPortIDForMatch(" Gi0/1 ", "interfaceName"))
	require.Equal(t, "mac", normalizeLLDPPortSubtypeForMatch("3"))
	require.Equal(t, "network", normalizeLLDPPortSubtypeForMatch("networkAddress"))
	require.Equal(t, "local", normalizeLLDPPortSubtypeForMatch(" local "))
}

func TestNormalizeLLDPChassisAndMACToken(t *testing.T) {
	require.Equal(t, "10.0.0.1", normalizeLLDPChassisForMatch("0a000001"))
	require.Equal(t, "001122334455", normalizeLLDPChassisForMatch("00:11:22:33:44:55"))
	require.Equal(t, "switch-a", normalizeLLDPChassisForMatch("switch-a"))
	require.Equal(t, "001122334455", canonicalLLDPMACToken("0x001122334455"))
	require.Equal(t, "", canonicalLLDPMACToken("001122"))
}

func TestCanonicalBridgeAddr_RejectsAllZeroMAC(t *testing.T) {
	require.Equal(t, "00:11:22:33:44:55", canonicalBridgeAddr("00:00:00:00:00:00", "00:11:22:33:44:55"))
	require.Equal(t, "", canonicalBridgeAddr("00:00:00:00:00:00", "00:00:00:00:00:00"))
}
