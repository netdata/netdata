// SPDX-License-Identifier: GPL-3.0-or-later

package topologyenrich

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/stretchr/testify/require"
)

func TestTopologyOSPFNeighborLinkKeyIgnoresUnspecifiedIPs(t *testing.T) {
	base := topologymodel.OSPFNeighbor{
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
