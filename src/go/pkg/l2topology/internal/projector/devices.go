// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import "github.com/netdata/netdata/go/plugins/pkg/topology/graph"

func deviceToTopologyActor(
	dev Device,
	source, layer, localDeviceID string,
	ifaceSummary topologyDeviceInterfaceSummary,
	reporterAliases []string,
) projectedActor {
	match := buildDeviceActorMatch(dev, reporterAliases)

	return projectedActor{
		Actor: graph.Actor{
			ActorType: resolveDeviceActorType(dev.Labels),
			Layer:     layer,
			Source:    source,
			Match:     match,
			Labels:    cloneStringMap(dev.Labels),
		},
		Detail: ProjectionActorDetail{
			Device: buildDeviceActorDetail(dev, localDeviceID, ifaceSummary, match),
		},
	}
}
