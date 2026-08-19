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

	removedHandles := make(map[topologymodel.ActorHandle]struct{})
	keptActors := make([]topologymodel.Actor, 0, len(data.Actors))
	for _, actor := range data.Actors {
		if topologymodel.ActorIsInferred(actor) && len(topologymodel.NormalizedMatchIPs(actor.Match)) == 0 {
			removedHandles[actor.ActorHandle] = struct{}{}
			continue
		}
		keptActors = append(keptActors, actor)
	}

	if len(removedHandles) == 0 {
		return 0
	}

	data.Actors = keptActors
	links := make([]topologymodel.Link, 0, len(data.Links))
	for _, link := range data.Links {
		if _, removed := removedHandles[link.SrcActorHandle]; removed {
			continue
		}
		if _, removed := removedHandles[link.DstActorHandle]; removed {
			continue
		}
		links = append(links, link)
	}
	data.Links = links
	return len(removedHandles)
}

func pruneSparseSegments(data *topologymodel.Data, threshold int) int {
	if data == nil || len(data.Actors) == 0 {
		return 0
	}

	removedTotal := 0
	for {
		segmentSet := make(map[topologymodel.ActorHandle]struct{})
		l3SegmentSet := make(map[topologymodel.ActorHandle]struct{})
		for _, actor := range data.Actors {
			if !topologymodel.ActorIsSegment(actor) {
				continue
			}
			if actor.ActorHandle.IsZero() {
				continue
			}
			segmentSet[actor.ActorHandle] = struct{}{}
			if topologymodel.ActorIsL3SubnetSegment(actor) {
				l3SegmentSet[actor.ActorHandle] = struct{}{}
			}
		}
		if len(segmentSet) == 0 {
			return removedTotal
		}

		neighborSet := make(map[topologymodel.ActorHandle]map[topologymodel.ActorHandle]struct{}, len(segmentSet))
		for segmentHandle := range segmentSet {
			neighborSet[segmentHandle] = make(map[topologymodel.ActorHandle]struct{})
		}
		for _, link := range data.Links {
			if _, ok := segmentSet[link.SrcActorHandle]; ok {
				neighborSet[link.SrcActorHandle][link.DstActorHandle] = struct{}{}
			}
			if _, ok := segmentSet[link.DstActorHandle]; ok {
				neighborSet[link.DstActorHandle][link.SrcActorHandle] = struct{}{}
			}
		}

		protectedSegments := l3SubnetSegmentsWithMembershipLinks(data.Links, l3SegmentSet)
		removeSegments := make(map[topologymodel.ActorHandle]struct{})
		for segmentHandle, neighbors := range neighborSet {
			if _, protected := protectedSegments[segmentHandle]; protected {
				continue
			}
			if len(neighbors) <= threshold {
				removeSegments[segmentHandle] = struct{}{}
			}
		}
		if len(removeSegments) == 0 {
			return removedTotal
		}
		removedTotal += len(removeSegments)

		filteredActors := make([]topologymodel.Actor, 0, len(data.Actors)-len(removeSegments))
		for _, actor := range data.Actors {
			if _, drop := removeSegments[actor.ActorHandle]; drop {
				continue
			}
			filteredActors = append(filteredActors, actor)
		}
		data.Actors = filteredActors

		filteredLinks := make([]topologymodel.Link, 0, len(data.Links))
		for _, link := range data.Links {
			if _, drop := removeSegments[link.SrcActorHandle]; drop {
				continue
			}
			if _, drop := removeSegments[link.DstActorHandle]; drop {
				continue
			}
			filteredLinks = append(filteredLinks, link)
		}
		data.Links = filteredLinks
	}
}

func l3SubnetSegmentsWithMembershipLinks(links []topologymodel.Link, l3SegmentSet map[topologymodel.ActorHandle]struct{}) map[topologymodel.ActorHandle]struct{} {
	protected := make(map[topologymodel.ActorHandle]struct{})
	if len(l3SegmentSet) == 0 {
		return protected
	}
	for _, link := range links {
		if !strings.EqualFold(strings.TrimSpace(topologyutil.FirstNonEmptyString(link.LinkType, link.Protocol)), topologymodel.L3SubnetMembershipLinkType) {
			continue
		}
		if _, ok := l3SegmentSet[link.SrcActorHandle]; ok {
			protected[link.SrcActorHandle] = struct{}{}
		}
		if _, ok := l3SegmentSet[link.DstActorHandle]; ok {
			protected[link.DstActorHandle] = struct{}{}
		}
	}
	return protected
}

func filterDanglingLinks(data *topologymodel.Data) {
	if data == nil || len(data.Links) == 0 {
		return
	}
	actorSet := make(map[topologymodel.ActorHandle]struct{}, len(data.Actors))
	for _, actor := range data.Actors {
		if !actor.ActorHandle.IsZero() {
			actorSet[actor.ActorHandle] = struct{}{}
		}
	}
	if len(actorSet) == 0 {
		data.Links = nil
		return
	}
	filtered := make([]topologymodel.Link, 0, len(data.Links))
	for _, link := range data.Links {
		if _, ok := actorSet[link.SrcActorHandle]; !ok {
			continue
		}
		if _, ok := actorSet[link.DstActorHandle]; !ok {
			continue
		}
		filtered = append(filtered, link)
	}
	data.Links = filtered
}
