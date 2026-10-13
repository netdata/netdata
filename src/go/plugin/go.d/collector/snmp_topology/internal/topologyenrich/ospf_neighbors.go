// SPDX-License-Identifier: GPL-3.0-or-later

package topologyenrich

import (
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

func isOSPFNeighborFull(row topologymodel.OSPFNeighbor) bool {
	return strings.EqualFold(topologyutil.NormalizeOSPFNeighborState(row.State), "full")
}

type topologyOSPFNeighborLinkKey struct {
	srcActor       topologymodel.ActorHandle
	dstActor       topologymodel.ActorHandle
	localRouterID  string
	remoteRouterID string
	discriminator  string
}

func (k topologyOSPFNeighborLinkKey) reversed() topologyOSPFNeighborLinkKey {
	k.srcActor, k.dstActor = k.dstActor, k.srcActor
	return k
}

func topologyOSPFNeighborLinkKeyParts(row topologymodel.OSPFNeighbor, srcActor, dstActor topologymodel.ActorHandle) topologyOSPFNeighborLinkKey {
	localRouterID := topologyutil.NormalizeTopologyRouterID(row.LocalRouterID)
	neighborRouterID := topologyutil.NormalizeTopologyRouterID(row.NeighborRouterID)
	if localRouterID > neighborRouterID {
		localRouterID, neighborRouterID = neighborRouterID, localRouterID
	}

	return topologyOSPFNeighborLinkKey{
		srcActor:       srcActor,
		dstActor:       dstActor,
		localRouterID:  localRouterID,
		remoteRouterID: neighborRouterID,
		discriminator:  topologyOSPFAdjacencyDiscriminator(row),
	}
}

func topologyOSPFAdjacencyDiscriminator(row topologymodel.OSPFNeighbor) string {
	if _, subnet, prefix, ok := topologyOSPFSubnetMatch(row); ok {
		return topologyutil.JoinKeyParts("subnet", subnet, strconv.Itoa(prefix))
	}

	localIP := topologyOSPFDedupIP(row.LocalIP)
	neighborIP := topologyOSPFDedupIP(row.NeighborIP)
	if localIP != "" && neighborIP != "" {
		if localIP > neighborIP {
			localIP, neighborIP = neighborIP, localIP
		}
		return topologyutil.JoinKeyParts("ip_pair", localIP, neighborIP)
	}

	return "router_id"
}

func topologyOSPFDedupIP(value string) string {
	return topologyutil.NormalizeNonUnspecifiedIPAddress(value)
}

func topologyOSPFSubnetMatch(row topologymodel.OSPFNeighbor) (network, subnet string, prefix int, ok bool) {
	if row.Subnet == "" || row.Prefix <= 0 {
		return "", "", 0, false
	}
	return row.Network, row.Subnet, row.Prefix, true
}
