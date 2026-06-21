// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
)

func filterTopologyDataByFocus(
	data *topologymodel.Data,
	includedActorsByDepth map[string]struct{},
	shortestPathActors map[string]struct{},
	shortestPathPairs map[string]struct{},
) {
	includedActors := make(map[string]struct{}, len(includedActorsByDepth)+len(shortestPathActors))
	for actorID := range includedActorsByDepth {
		includedActors[actorID] = struct{}{}
	}
	for actorID := range shortestPathActors {
		includedActors[actorID] = struct{}{}
	}

	filteredLinks := make([]topologymodel.Link, 0, len(data.Links))
	linkActors := make(map[string]struct{})
	for _, link := range data.Links {
		srcActorID := strings.TrimSpace(link.SrcActorID)
		dstActorID := strings.TrimSpace(link.DstActorID)
		if srcActorID == "" || dstActorID == "" {
			continue
		}

		_, srcInDepth := includedActorsByDepth[srcActorID]
		_, dstInDepth := includedActorsByDepth[dstActorID]
		_, inShortestPath := shortestPathPairs[topologyActorPairKey(srcActorID, dstActorID)]
		if !(srcInDepth && dstInDepth) && !inShortestPath {
			continue
		}

		filteredLinks = append(filteredLinks, link)
		linkActors[srcActorID] = struct{}{}
		linkActors[dstActorID] = struct{}{}
	}
	data.Links = filteredLinks

	for actorID := range linkActors {
		includedActors[actorID] = struct{}{}
	}

	filteredActors := make([]topologymodel.Actor, 0, len(data.Actors))
	for _, actor := range data.Actors {
		if _, ok := includedActors[actor.ActorID]; ok {
			filteredActors = append(filteredActors, actor)
		}
	}
	data.Actors = filteredActors
}
