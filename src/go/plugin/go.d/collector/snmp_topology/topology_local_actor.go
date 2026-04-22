// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import topologyengine "github.com/netdata/netdata/go/plugins/pkg/topology/engine"

func augmentLocalActorFromCache(data *topologyData, local topologyDevice) {
	if data == nil || len(data.Actors) == 0 {
		return
	}

	for i := range data.Actors {
		actor := &data.Actors[i]
		if !topologyengine.IsDeviceActorType(actor.ActorType) {
			continue
		}
		if !matchLocalTopologyActor(actor.Match, local) {
			continue
		}

		attrs := populateLocalActorAttributes(actor.Attributes, local)
		actor.Attributes = pruneNilAttributes(attrs)
		applyLocalActorLabels(actor, local)
		enrichLocalActorChartReferences(actor, local.InterfaceCharts)
		return
	}
}
