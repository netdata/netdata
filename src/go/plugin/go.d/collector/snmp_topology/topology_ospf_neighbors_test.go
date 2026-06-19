// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeOSPFRouterIDDropsUnspecifiedAddresses(t *testing.T) {
	require.Empty(t, normalizeOSPFRouterID("0.0.0.0"))
	require.Empty(t, normalizeOSPFRouterID("::"))
	require.Empty(t, normalizeOSPFRouterID("00000000"))
	require.Equal(t, "1.2.3.4", normalizeOSPFRouterID(" 1.2.3.4 "))
}

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
