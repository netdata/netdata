// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strings"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/topology/engine"
)

func applyMapTypePolicy(data *topologyData, mapType string) int {
	switch normalizeTopologyMapType(mapType) {
	case topologyMapTypeLLDPCDPManaged:
		return applyLLDPCDPManagedMapPolicy(data)
	case topologyMapTypeHighConfidenceInferred:
		return suppressUnlinkedInferredEndpoints(data)
	default:
		return 0
	}
}

func applyLLDPCDPManagedMapPolicy(data *topologyData) int {
	if data == nil || len(data.Actors) == 0 {
		return 0
	}

	managedIDs := make(map[string]struct{})
	for _, actor := range data.Actors {
		if !isManagedSNMPDeviceActor(actor) {
			continue
		}
		managedIDs[actor.ActorID] = struct{}{}
	}

	keepLink := func(link topologyLink) bool {
		protocol := strings.ToLower(strings.TrimSpace(link.Protocol))
		return protocol == "lldp" || protocol == "cdp"
	}

	keptLinks := make([]topologyLink, 0, len(data.Links))
	linkedIDs := make(map[string]struct{}, len(managedIDs))
	for managedID := range managedIDs {
		linkedIDs[managedID] = struct{}{}
	}
	for _, link := range data.Links {
		if !keepLink(link) {
			continue
		}
		keptLinks = append(keptLinks, link)
		if strings.TrimSpace(link.SrcActorID) != "" {
			linkedIDs[link.SrcActorID] = struct{}{}
		}
		if strings.TrimSpace(link.DstActorID) != "" {
			linkedIDs[link.DstActorID] = struct{}{}
		}
	}
	data.Links = keptLinks

	keptActors := make([]topologyActor, 0, len(data.Actors))
	removed := 0
	for _, actor := range data.Actors {
		if _, ok := linkedIDs[actor.ActorID]; ok {
			keptActors = append(keptActors, actor)
			continue
		}
		removed++
	}
	data.Actors = keptActors
	return removed
}

func suppressUnlinkedInferredEndpoints(data *topologyData) int {
	if data == nil || len(data.Actors) == 0 {
		return 0
	}

	linked := make(map[string]struct{}, len(data.Links)*2)
	for _, link := range data.Links {
		if strings.TrimSpace(link.SrcActorID) != "" {
			linked[link.SrcActorID] = struct{}{}
		}
		if strings.TrimSpace(link.DstActorID) != "" {
			linked[link.DstActorID] = struct{}{}
		}
	}

	removed := 0
	removedIDs := make(map[string]struct{})
	kept := make([]topologyActor, 0, len(data.Actors))
	for _, actor := range data.Actors {
		if !strings.EqualFold(strings.TrimSpace(actor.ActorType), "endpoint") {
			kept = append(kept, actor)
			continue
		}
		if _, ok := linked[actor.ActorID]; ok {
			kept = append(kept, actor)
			continue
		}
		removed++
		removedIDs[actor.ActorID] = struct{}{}
	}
	if removed == 0 {
		return 0
	}
	data.Actors = kept
	if len(data.Links) == 0 {
		return removed
	}
	filtered := make([]topologyLink, 0, len(data.Links))
	for _, link := range data.Links {
		if _, drop := removedIDs[link.SrcActorID]; drop {
			continue
		}
		if _, drop := removedIDs[link.DstActorID]; drop {
			continue
		}
		filtered = append(filtered, link)
	}
	data.Links = filtered
	return removed
}

func isManagedSNMPDeviceActor(actor topologyActor) bool {
	if !topologyengine.IsDeviceActorType(actor.ActorType) {
		return false
	}
	if strings.ToLower(strings.TrimSpace(actor.Source)) != "snmp" {
		return false
	}
	return !topologyActorIsInferred(actor)
}
