// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import "strings"

func attachTopologyOSPFNeighborRows(data *topologyData, rowsByActor map[string][]topologyOSPFNeighborDetailRow) {
	if data == nil || len(rowsByActor) == 0 {
		return
	}
	for i := range data.Actors {
		actor := &data.Actors[i]
		rows := rowsByActor[strings.TrimSpace(actor.ActorID)]
		if len(rows) == 0 {
			continue
		}
		sortTopologyOSPFNeighborDetailRows(rows)
		actor.Detail.OSPF = rows
	}
}

func attachTopologyBGPPeerRows(data *topologyData, rowsByActor map[string][]topologyBGPPeerDetailRow) {
	if data == nil || len(rowsByActor) == 0 {
		return
	}
	for i := range data.Actors {
		actor := &data.Actors[i]
		rows := rowsByActor[strings.TrimSpace(actor.ActorID)]
		if len(rows) == 0 {
			continue
		}
		sortTopologyBGPPeerDetailRows(rows)
		actor.Detail.BGP = rows
	}
}
