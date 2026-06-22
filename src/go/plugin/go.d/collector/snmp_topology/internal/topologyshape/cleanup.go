// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

func eliminateNonIPInferredActors(data *topologymodel.Data) int {
	if data == nil || len(data.Actors) == 0 {
		return 0
	}

	removedIDs := make(map[string]struct{})
	keptActors := make([]topologymodel.Actor, 0, len(data.Actors))
	for _, actor := range data.Actors {
		if topologymodel.ActorIsInferred(actor) && len(topologymodel.NormalizedMatchIPs(actor.Match)) == 0 {
			removedIDs[actor.ActorID] = struct{}{}
			continue
		}
		keptActors = append(keptActors, actor)
	}

	if len(removedIDs) == 0 {
		return 0
	}

	data.Actors = keptActors
	links := make([]topologymodel.Link, 0, len(data.Links))
	for _, link := range data.Links {
		if _, removed := removedIDs[link.SrcActorID]; removed {
			continue
		}
		if _, removed := removedIDs[link.DstActorID]; removed {
			continue
		}
		links = append(links, link)
	}
	data.Links = links
	return len(removedIDs)
}

func pruneSparseSegments(data *topologymodel.Data, threshold int) int {
	if data == nil || len(data.Actors) == 0 {
		return 0
	}

	removedTotal := 0
	for {
		segmentSet := make(map[string]struct{})
		l3SegmentSet := make(map[string]struct{})
		for _, actor := range data.Actors {
			if !topologymodel.ActorIsSegment(actor) {
				continue
			}
			actorID := strings.TrimSpace(actor.ActorID)
			if actorID == "" {
				continue
			}
			segmentSet[actorID] = struct{}{}
			if topologymodel.ActorIsL3SubnetSegment(actor) {
				l3SegmentSet[actorID] = struct{}{}
			}
		}
		if len(segmentSet) == 0 {
			return removedTotal
		}

		neighborSet := make(map[string]map[string]struct{}, len(segmentSet))
		for segmentID := range segmentSet {
			neighborSet[segmentID] = make(map[string]struct{})
		}
		for _, link := range data.Links {
			if _, ok := segmentSet[link.SrcActorID]; ok {
				neighborSet[link.SrcActorID][link.DstActorID] = struct{}{}
			}
			if _, ok := segmentSet[link.DstActorID]; ok {
				neighborSet[link.DstActorID][link.SrcActorID] = struct{}{}
			}
		}

		protectedSegments := l3SubnetSegmentsWithMembershipLinks(data.Links, l3SegmentSet)
		removeSegments := make(map[string]struct{})
		for segmentID, neighbors := range neighborSet {
			if _, protected := protectedSegments[segmentID]; protected {
				continue
			}
			if len(neighbors) <= threshold {
				removeSegments[segmentID] = struct{}{}
			}
		}
		if len(removeSegments) == 0 {
			return removedTotal
		}
		removedTotal += len(removeSegments)

		filteredActors := make([]topologymodel.Actor, 0, len(data.Actors)-len(removeSegments))
		for _, actor := range data.Actors {
			if _, drop := removeSegments[actor.ActorID]; drop {
				continue
			}
			filteredActors = append(filteredActors, actor)
		}
		data.Actors = filteredActors

		filteredLinks := make([]topologymodel.Link, 0, len(data.Links))
		for _, link := range data.Links {
			if _, drop := removeSegments[link.SrcActorID]; drop {
				continue
			}
			if _, drop := removeSegments[link.DstActorID]; drop {
				continue
			}
			filteredLinks = append(filteredLinks, link)
		}
		data.Links = filteredLinks
	}
}

func l3SubnetSegmentsWithMembershipLinks(links []topologymodel.Link, l3SegmentSet map[string]struct{}) map[string]struct{} {
	protected := make(map[string]struct{})
	if len(l3SegmentSet) == 0 {
		return protected
	}
	for _, link := range links {
		if !strings.EqualFold(strings.TrimSpace(topologyutil.FirstNonEmptyString(link.LinkType, link.Protocol)), topologymodel.L3SubnetMembershipLinkType) {
			continue
		}
		if _, ok := l3SegmentSet[strings.TrimSpace(link.SrcActorID)]; ok {
			protected[strings.TrimSpace(link.SrcActorID)] = struct{}{}
		}
		if _, ok := l3SegmentSet[strings.TrimSpace(link.DstActorID)]; ok {
			protected[strings.TrimSpace(link.DstActorID)] = struct{}{}
		}
	}
	return protected
}

func filterDanglingLinks(data *topologymodel.Data) {
	if data == nil || len(data.Links) == 0 {
		return
	}
	actorSet := make(map[string]struct{}, len(data.Actors))
	for _, actor := range data.Actors {
		if id := strings.TrimSpace(actor.ActorID); id != "" {
			actorSet[id] = struct{}{}
		}
	}
	if len(actorSet) == 0 {
		data.Links = nil
		return
	}
	filtered := make([]topologymodel.Link, 0, len(data.Links))
	for _, link := range data.Links {
		if _, ok := actorSet[strings.TrimSpace(link.SrcActorID)]; !ok {
			continue
		}
		if _, ok := actorSet[strings.TrimSpace(link.DstActorID)]; !ok {
			continue
		}
		filtered = append(filtered, link)
	}
	data.Links = filtered
}
