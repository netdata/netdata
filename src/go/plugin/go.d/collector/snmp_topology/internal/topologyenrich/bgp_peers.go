// SPDX-License-Identifier: GPL-3.0-or-later

package topologyenrich

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

func isBGPPeerEstablished(row topologymodel.BGPPeer) bool {
	return strings.EqualFold(strings.TrimSpace(row.State), "established")
}

func sortTopologyBGPPeerDetailRows(rows []topologymodel.BGPPeerDetailRow) {
	sortTopologyBGPPeerDetailRowsWithWork(nil, rows)
}

func sortTopologyBGPPeerDetailRowsWithWork(work *enrichmentWork, rows []topologymodel.BGPPeerDetailRow) {
	sortEnrichmentByPreparedStringKey(work, rows, topologyBGPPeerActorRowSortKeyWithWork)
}

func topologyBGPPeerActorRowSortKeyWithWork(work *enrichmentWork, row topologymodel.BGPPeerDetailRow) (string, bool) {
	if work != nil && !work.chargeStrings([]string{row.RoutingInstance, row.RemoteAS, row.NeighborIP, row.PeerIdentifier, row.State}) {
		return "", false
	}
	return topologyBGPPeerActorRowSortKey(row), true
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
