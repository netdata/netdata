// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

func applyMapTypePolicy(data *topologymodel.Data, mapType string) int {
	switch topologyoptions.NormalizeMapType(mapType) {
	case topologyoptions.MapTypeManagedFabric:
		return applyManagedFabricMapPolicy(data)
	case topologyoptions.MapTypeLLDPCDPManaged:
		return applyLLDPCDPManagedMapPolicy(data)
	case topologyoptions.MapTypeHighConfidenceInferred:
		return suppressUnlinkedInferredEndpoints(data)
	default:
		return 0
	}
}

func applyManagedFabricMapPolicy(data *topologymodel.Data) int {
	if data == nil || len(data.Actors) == 0 {
		return 0
	}

	actorsByHandle := make(map[topologymodel.ActorHandle]topologymodel.Actor, len(data.Actors))
	managedHandles := make(map[topologymodel.ActorHandle]struct{})
	segmentHandles := make(map[topologymodel.ActorHandle]struct{})
	for _, actor := range data.Actors {
		actorsByHandle[actor.ActorHandle] = actor
		if topologymodel.IsManagedSNMPDeviceActor(actor) {
			managedHandles[actor.ActorHandle] = struct{}{}
		}
		if topologymodel.ActorSegmentKind(actor) == topologymodel.SegmentKindBroadcastDomain {
			segmentHandles[actor.ActorHandle] = struct{}{}
		}
	}

	managedNeighborsBySegment := make(map[topologymodel.ActorHandle]map[topologymodel.ActorHandle]struct{})
	for _, link := range data.Links {
		segmentHandle, managedHandle, ok := managedFabricSegmentLeg(link, segmentHandles, managedHandles)
		if !ok {
			continue
		}
		neighbors := managedNeighborsBySegment[segmentHandle]
		if neighbors == nil {
			neighbors = make(map[topologymodel.ActorHandle]struct{})
			managedNeighborsBySegment[segmentHandle] = neighbors
		}
		neighbors[managedHandle] = struct{}{}
	}

	qualifiedSegments := make(map[topologymodel.ActorHandle]struct{})
	for segmentHandle, neighbors := range managedNeighborsBySegment {
		if len(neighbors) >= 2 {
			qualifiedSegments[segmentHandle] = struct{}{}
		}
	}

	keptHandles := make(map[topologymodel.ActorHandle]struct{}, len(managedHandles)+len(qualifiedSegments))
	for handle := range managedHandles {
		keptHandles[handle] = struct{}{}
	}
	keptLinks := make([]topologymodel.Link, 0, len(data.Links))
	for _, link := range data.Links {
		protocol := strings.ToLower(strings.TrimSpace(link.Protocol))
		keep := false
		switch protocol {
		case "lldp", "cdp":
			keep = true
		case "stp":
			_, srcManaged := managedHandles[link.SrcActorHandle]
			_, dstManaged := managedHandles[link.DstActorHandle]
			keep = srcManaged && dstManaged
		case "bridge", "fdb":
			_, _, keep = managedFabricSegmentLeg(link, qualifiedSegments, managedHandles)
		}
		if !keep {
			continue
		}
		keptLinks = append(keptLinks, link)
		if _, ok := actorsByHandle[link.SrcActorHandle]; ok {
			keptHandles[link.SrcActorHandle] = struct{}{}
		}
		if _, ok := actorsByHandle[link.DstActorHandle]; ok {
			keptHandles[link.DstActorHandle] = struct{}{}
		}
	}

	keptActors := make([]topologymodel.Actor, 0, len(keptHandles))
	for _, actor := range data.Actors {
		if _, ok := keptHandles[actor.ActorHandle]; ok {
			keptActors = append(keptActors, actor)
		}
	}
	removed := len(data.Actors) - len(keptActors)
	data.Actors = keptActors
	data.Links = keptLinks
	return removed
}

func managedFabricSegmentLeg(
	link topologymodel.Link,
	segmentHandles map[topologymodel.ActorHandle]struct{},
	managedHandles map[topologymodel.ActorHandle]struct{},
) (topologymodel.ActorHandle, topologymodel.ActorHandle, bool) {
	protocol := strings.ToLower(strings.TrimSpace(link.Protocol))
	if protocol != "bridge" && protocol != "fdb" {
		return topologymodel.ActorHandle{}, topologymodel.ActorHandle{}, false
	}
	if _, segment := segmentHandles[link.SrcActorHandle]; segment {
		if _, managed := managedHandles[link.DstActorHandle]; managed {
			return link.SrcActorHandle, link.DstActorHandle, true
		}
	}
	if _, segment := segmentHandles[link.DstActorHandle]; segment {
		if _, managed := managedHandles[link.SrcActorHandle]; managed {
			return link.DstActorHandle, link.SrcActorHandle, true
		}
	}
	return topologymodel.ActorHandle{}, topologymodel.ActorHandle{}, false
}

func applyLLDPCDPManagedMapPolicy(data *topologymodel.Data) int {
	if data == nil || len(data.Actors) == 0 {
		return 0
	}

	managedHandles := make(map[topologymodel.ActorHandle]struct{})
	for _, actor := range data.Actors {
		if !topologymodel.IsManagedSNMPDeviceActor(actor) {
			continue
		}
		managedHandles[actor.ActorHandle] = struct{}{}
	}

	keepLink := func(link topologymodel.Link) bool {
		protocol := strings.ToLower(strings.TrimSpace(link.Protocol))
		return protocol == "lldp" || protocol == "cdp"
	}

	keptLinks := make([]topologymodel.Link, 0, len(data.Links))
	linkedHandles := make(map[topologymodel.ActorHandle]struct{}, len(managedHandles))
	for managedHandle := range managedHandles {
		linkedHandles[managedHandle] = struct{}{}
	}
	for _, link := range data.Links {
		if !keepLink(link) {
			continue
		}
		keptLinks = append(keptLinks, link)
		if !link.SrcActorHandle.IsZero() {
			linkedHandles[link.SrcActorHandle] = struct{}{}
		}
		if !link.DstActorHandle.IsZero() {
			linkedHandles[link.DstActorHandle] = struct{}{}
		}
	}
	data.Links = keptLinks

	keptActors := make([]topologymodel.Actor, 0, len(data.Actors))
	removed := 0
	for _, actor := range data.Actors {
		if _, ok := linkedHandles[actor.ActorHandle]; ok {
			keptActors = append(keptActors, actor)
			continue
		}
		removed++
	}
	data.Actors = keptActors
	return removed
}

func suppressUnlinkedInferredEndpoints(data *topologymodel.Data) int {
	if data == nil || len(data.Actors) == 0 {
		return 0
	}

	linked := make(map[topologymodel.ActorHandle]struct{}, len(data.Links)*2)
	for _, link := range data.Links {
		if !link.SrcActorHandle.IsZero() {
			linked[link.SrcActorHandle] = struct{}{}
		}
		if !link.DstActorHandle.IsZero() {
			linked[link.DstActorHandle] = struct{}{}
		}
	}

	removed := 0
	removedHandles := make(map[topologymodel.ActorHandle]struct{})
	kept := make([]topologymodel.Actor, 0, len(data.Actors))
	for _, actor := range data.Actors {
		if !strings.EqualFold(strings.TrimSpace(actor.ActorType), "endpoint") {
			kept = append(kept, actor)
			continue
		}
		if _, ok := linked[actor.ActorHandle]; ok {
			kept = append(kept, actor)
			continue
		}
		removed++
		removedHandles[actor.ActorHandle] = struct{}{}
	}
	if removed == 0 {
		return 0
	}
	data.Actors = kept
	if len(data.Links) == 0 {
		return removed
	}
	filtered := make([]topologymodel.Link, 0, len(data.Links))
	for _, link := range data.Links {
		if _, drop := removedHandles[link.SrcActorHandle]; drop {
			continue
		}
		if _, drop := removedHandles[link.DstActorHandle]; drop {
			continue
		}
		filtered = append(filtered, link)
	}
	data.Links = filtered
	return removed
}
