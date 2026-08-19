// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

func applyMapTypePolicy(data *topologymodel.Data, mapType string) int {
	switch topologyoptions.NormalizeMapType(mapType) {
	case topologyoptions.MapTypeLLDPCDPManaged:
		return applyLLDPCDPManagedMapPolicy(data)
	case topologyoptions.MapTypeHighConfidenceInferred:
		return suppressUnlinkedInferredEndpoints(data)
	default:
		return 0
	}
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
