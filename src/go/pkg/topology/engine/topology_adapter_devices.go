// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import "github.com/netdata/netdata/go/plugins/pkg/topology"

func deviceToTopologyActor(
	dev Device,
	source, layer, localDeviceID string,
	ifaceSummary topologyDeviceInterfaceSummary,
	reporterAliases []string,
) topology.Actor {
	match := buildDeviceActorMatch(dev, reporterAliases)
	attrs := buildDeviceActorAttributes(dev, localDeviceID, ifaceSummary, match)
	tables := buildDeviceActorTables(ifaceSummary)

	return topology.Actor{
		ActorType:  resolveDeviceActorType(dev.Labels),
		Layer:      layer,
		Source:     source,
		Match:      match,
		Attributes: pruneTopologyAttributes(attrs),
		Labels:     cloneStringMap(dev.Labels),
		Tables:     tables,
	}
}
