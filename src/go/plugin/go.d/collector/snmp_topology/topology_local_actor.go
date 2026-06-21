// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
)

func augmentLocalActorFromCache(data *topologymodel.Data, local topologymodel.Device) {
	if data == nil || len(data.Actors) == 0 {
		return
	}

	for i := range data.Actors {
		actor := &data.Actors[i]
		if !topologyengine.IsDeviceActorType(actor.ActorType) {
			continue
		}
		if !topologymodel.MatchLocalActor(actor.Match, local) {
			continue
		}

		actor.Detail.SNMP = topologySNMPActorDetailFromDevice(local)
		applyLocalActorLabels(actor, local)
		enrichLocalActorChartReferences(actor, local.InterfaceCharts)
		return
	}
}
