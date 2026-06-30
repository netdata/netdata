// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
)

func deviceToTopologyActor(
	dev model.Device,
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
		Detail: model.ProjectionActorDetail{
			Device: buildDeviceActorDetail(dev, localDeviceID, ifaceSummary, match),
		},
	}
}
