// SPDX-License-Identifier: GPL-3.0-or-later

package topologyenrich

import "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"

func attachTopologyOSPFNeighborRows(work *enrichmentWork, data *topologymodel.Data, rowsByActor map[topologymodel.ActorHandle][]topologymodel.OSPFNeighborDetailRow) {
	if data == nil || len(rowsByActor) == 0 {
		return
	}
	if !work.charge(uint64(len(data.Actors))) {
		return
	}
	for i := range data.Actors {
		actor := &data.Actors[i]
		rows := rowsByActor[actor.ActorHandle]
		if len(rows) == 0 {
			continue
		}
		sortTopologyOSPFNeighborDetailRowsWithWork(work, rows)
		actor.Detail.OSPF = rows
	}
}

func attachTopologyBGPPeerRows(work *enrichmentWork, data *topologymodel.Data, rowsByActor map[topologymodel.ActorHandle][]topologymodel.BGPPeerDetailRow) {
	if data == nil || len(rowsByActor) == 0 {
		return
	}
	if !work.charge(uint64(len(data.Actors))) {
		return
	}
	for i := range data.Actors {
		actor := &data.Actors[i]
		rows := rowsByActor[actor.ActorHandle]
		if len(rows) == 0 {
			continue
		}
		sortTopologyBGPPeerDetailRowsWithWork(work, rows)
		actor.Detail.BGP = rows
	}
}
