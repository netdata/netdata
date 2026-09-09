// SPDX-License-Identifier: GPL-3.0-or-later

package topologyenrich

import (
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

func isBGPPeerEstablished(row topologymodel.BGPPeer) bool {
	return strings.EqualFold(strings.TrimSpace(row.State), "established")
}

func sortTopologyBGPPeerDetailRows(rows []topologymodel.BGPPeerDetailRow) {
	sort.Slice(rows, func(i, j int) bool {
		return topologyBGPPeerActorRowSortKey(rows[i]) < topologyBGPPeerActorRowSortKey(rows[j])
	})
}

func topologyBGPPeerActorRowSortKey(row topologymodel.BGPPeerDetailRow) string {
	return strings.Join([]string{
		row.RoutingInstance,
		row.RemoteAS,
		topologyutil.NormalizeBGPPeerAddress(row.NeighborIP),
		row.PeerIdentifier,
		row.State,
	}, "\x00")
}
