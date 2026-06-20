// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTopologyOSPFNeighborLinkKeyIgnoresUnspecifiedIPs(t *testing.T) {
	base := topologyOSPFNeighbor{
		LocalRouterID:    "1.1.1.1",
		NeighborRouterID: "2.2.2.2",
	}
	withUnspecifiedIPs := base
	withUnspecifiedIPs.LocalIP = "::"
	withUnspecifiedIPs.NeighborIP = "0.0.0.0"

	require.Equal(
		t,
		topologyOSPFNeighborLinkKeyParts(base, "router-a", "router-b"),
		topologyOSPFNeighborLinkKeyParts(withUnspecifiedIPs, "router-a", "router-b"),
	)
}

func TestTopologyCache_OSPFNeighborDropsUnspecifiedOnlyNeighborIdentity(t *testing.T) {
	cache := newTopologyCache()

	cache.updateOSPFNeighbor(map[string]string{
		tagOSPFNeighborRouterID: "0.0.0.0",
		tagOSPFNeighborIP:       "0.0.0.0",
		tagOSPFNeighborState:    "full",
	})

	require.Empty(t, cache.ospfNeighborsByKey)
}

func TestTopologyCache_MatchOSPFNeighborLocalInterfaceUsesLongestPrefix(t *testing.T) {
	cache := newTopologyCache()
	cache.l3InterfacesByIP = map[string]topologyL3Interface{
		"10.0.0.1": {
			IP:      "10.0.0.1",
			Netmask: "255.255.0.0",
			IfIndex: "1",
		},
		"10.0.1.1": {
			IP:      "10.0.1.1",
			Netmask: "255.255.255.252",
			IfIndex: "2",
		},
	}

	match, ok := cache.matchOSPFNeighborLocalInterface("10.0.1.2")

	require.True(t, ok)
	require.Equal(t, "10.0.1.1", match.IP)
	require.Equal(t, "10.0.1.0", match.Network)
	require.Equal(t, "255.255.255.252", match.Netmask)
	require.Equal(t, "10.0.1.0/30", match.Subnet)
	require.Equal(t, 30, match.Prefix)
}
