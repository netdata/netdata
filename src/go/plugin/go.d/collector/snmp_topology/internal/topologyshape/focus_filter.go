// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"github.com/netdata/netdata/go/plugins/pkg/topology/worklimit"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
)

func filterTopologyDataByFocus(
	data *topologymodel.Data,
	includedActorsByDepth map[topologymodel.ActorHandle]struct{},
	shortestPathActors map[topologymodel.ActorHandle]struct{},
	shortestPathPairs map[topologyActorPair]struct{},
) {
	_ = filterTopologyDataByFocusWithLimiter(data, includedActorsByDepth, shortestPathActors, shortestPathPairs, nil)
}

func filterTopologyDataByFocusWithLimiter(
	data *topologymodel.Data,
	includedActorsByDepth map[topologymodel.ActorHandle]struct{},
	shortestPathActors map[topologymodel.ActorHandle]struct{},
	shortestPathPairs map[topologyActorPair]struct{},
	limiter worklimit.Limiter,
) error {
	if err := limiter.Charge(uint64(len(includedActorsByDepth))); err != nil {
		return err
	}
	includedActors := make(map[topologymodel.ActorHandle]struct{}, len(includedActorsByDepth)+len(shortestPathActors))
	for actorHandle := range includedActorsByDepth {
		includedActors[actorHandle] = struct{}{}
	}
	if err := limiter.Charge(uint64(len(shortestPathActors))); err != nil {
		return err
	}
	for actorHandle := range shortestPathActors {
		includedActors[actorHandle] = struct{}{}
	}

	if err := limiter.Charge(uint64(len(data.Links))); err != nil {
		return err
	}
	filteredLinks := make([]topologymodel.Link, 0, len(data.Links))
	linkActors := make(map[topologymodel.ActorHandle]struct{})
	for _, link := range data.Links {
		srcActorHandle := link.SrcActorHandle
		dstActorHandle := link.DstActorHandle
		if srcActorHandle.IsZero() || dstActorHandle.IsZero() {
			continue
		}

		_, srcInDepth := includedActorsByDepth[srcActorHandle]
		_, dstInDepth := includedActorsByDepth[dstActorHandle]
		_, inShortestPath := shortestPathPairs[topologyActorPair{src: srcActorHandle, dst: dstActorHandle}]
		if !(srcInDepth && dstInDepth) && !inShortestPath {
			continue
		}

		filteredLinks = append(filteredLinks, link)
		linkActors[srcActorHandle] = struct{}{}
		linkActors[dstActorHandle] = struct{}{}
	}
	data.Links = filteredLinks

	if err := limiter.Charge(uint64(len(linkActors))); err != nil {
		return err
	}
	for actorHandle := range linkActors {
		includedActors[actorHandle] = struct{}{}
	}

	if err := limiter.Charge(uint64(len(data.Actors))); err != nil {
		return err
	}
	filteredActors := make([]topologymodel.Actor, 0, len(data.Actors))
	for _, actor := range data.Actors {
		if _, ok := includedActors[actor.ActorHandle]; ok {
			filteredActors = append(filteredActors, actor)
		}
	}
	data.Actors = filteredActors
	return nil
}
