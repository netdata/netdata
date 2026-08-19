// SPDX-License-Identifier: GPL-3.0-or-later

package topologyenrich

import "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"

func attachTopologyOSPFNeighborRows(data *topologymodel.Data, rowsByActor map[topologymodel.ActorHandle][]topologymodel.OSPFNeighborDetailRow) {
	if data == nil || len(rowsByActor) == 0 {
		return
	}
	for i := range data.Actors {
		actor := &data.Actors[i]
		rows := rowsByActor[actor.ActorHandle]
		if len(rows) == 0 {
			continue
		}
		sortTopologyOSPFNeighborDetailRows(rows)
		actor.Detail.OSPF = rows
	}
}

func attachTopologyBGPPeerRows(data *topologymodel.Data, rowsByActor map[topologymodel.ActorHandle][]topologymodel.BGPPeerDetailRow) {
	if data == nil || len(rowsByActor) == 0 {
		return
	}
	for i := range data.Actors {
		actor := &data.Actors[i]
		rows := rowsByActor[actor.ActorHandle]
		if len(rows) == 0 {
			continue
		}
		sortTopologyBGPPeerDetailRows(rows)
		actor.Detail.BGP = rows
	}
}
