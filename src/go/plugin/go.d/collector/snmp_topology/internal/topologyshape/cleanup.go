// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology/worklimit"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

func eliminateNonIPInferredActors(data *topologymodel.Data) int {
	removed, _ := eliminateNonIPInferredActorsWithLimiter(data, nil)
	return removed
}

func eliminateNonIPInferredActorsWithLimiter(data *topologymodel.Data, limiter worklimit.Limiter) (int, error) {
	if data == nil || len(data.Actors) == 0 {
		return 0, nil
	}
	if err := limiter.Charge(uint64(len(data.Actors))); err != nil {
		return 0, err
	}

	removedHandles := make(map[topologymodel.ActorHandle]struct{})
	keptActors := make([]topologymodel.Actor, 0, len(data.Actors))
	for _, actor := range data.Actors {
		matchIPs, err := topologymodel.NormalizedMatchIPsWithLimiter(actor.Match, limiter)
		if err != nil {
			return 0, err
		}
		if topologymodel.ActorIsInferred(actor) && len(matchIPs) == 0 {
			removedHandles[actor.ActorHandle] = struct{}{}
			continue
		}
		keptActors = append(keptActors, actor)
	}

	if len(removedHandles) == 0 {
		return 0, nil
	}

	data.Actors = keptActors
	if err := limiter.Charge(uint64(len(data.Links))); err != nil {
		return 0, err
	}
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
	return len(removedHandles), nil
}

func pruneSparseSegments(data *topologymodel.Data, threshold int) int {
	removed, _ := pruneSparseSegmentsWithLimiter(data, threshold, nil)
	return removed
}

func pruneSparseSegmentsWithLimiter(data *topologymodel.Data, threshold int, limiter worklimit.Limiter) (int, error) {
	if data == nil || len(data.Actors) == 0 {
		return 0, nil
	}

	removedTotal := 0
	for {
		if err := limiter.Charge(uint64(len(data.Actors))); err != nil {
			return 0, err
		}
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
			return removedTotal, nil
		}

		if err := limiter.Charge(uint64(len(segmentSet))); err != nil {
			return 0, err
		}
		neighborSet := make(map[topologymodel.ActorHandle]map[topologymodel.ActorHandle]struct{}, len(segmentSet))
		for segmentHandle := range segmentSet {
			neighborSet[segmentHandle] = make(map[topologymodel.ActorHandle]struct{})
		}
		if err := limiter.Charge(uint64(len(data.Links))); err != nil {
			return 0, err
		}
		for _, link := range data.Links {
			if _, ok := segmentSet[link.SrcActorHandle]; ok {
				neighborSet[link.SrcActorHandle][link.DstActorHandle] = struct{}{}
			}
			if _, ok := segmentSet[link.DstActorHandle]; ok {
				neighborSet[link.DstActorHandle][link.SrcActorHandle] = struct{}{}
			}
		}

		protectedSegments, err := l3SubnetSegmentsWithMembershipLinksWithLimiter(data.Links, l3SegmentSet, limiter)
		if err != nil {
			return 0, err
		}
		if err := limiter.Charge(uint64(len(neighborSet))); err != nil {
			return 0, err
		}
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
			return removedTotal, nil
		}
		removedTotal += len(removeSegments)

		if err := limiter.Charge(uint64(len(data.Actors))); err != nil {
			return 0, err
		}
		filteredActors := make([]topologymodel.Actor, 0, len(data.Actors)-len(removeSegments))
		for _, actor := range data.Actors {
			if _, drop := removeSegments[actor.ActorHandle]; drop {
				continue
			}
			filteredActors = append(filteredActors, actor)
		}
		data.Actors = filteredActors

		if err := limiter.Charge(uint64(len(data.Links))); err != nil {
			return 0, err
		}
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
	protected, _ := l3SubnetSegmentsWithMembershipLinksWithLimiter(links, l3SegmentSet, nil)
	return protected
}

func l3SubnetSegmentsWithMembershipLinksWithLimiter(
	links []topologymodel.Link,
	l3SegmentSet map[topologymodel.ActorHandle]struct{},
	limiter worklimit.Limiter,
) (map[topologymodel.ActorHandle]struct{}, error) {
	protected := make(map[topologymodel.ActorHandle]struct{})
	if len(l3SegmentSet) == 0 {
		return protected, nil
	}
	if err := limiter.Charge(uint64(len(links))); err != nil {
		return nil, err
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
	return protected, nil
}

func filterDanglingLinks(data *topologymodel.Data) {
	_ = filterDanglingLinksWithLimiter(data, nil)
}

func filterDanglingLinksWithLimiter(data *topologymodel.Data, limiter worklimit.Limiter) error {
	if data == nil || len(data.Links) == 0 {
		return nil
	}
	if err := limiter.Charge(uint64(len(data.Actors))); err != nil {
		return err
	}
	actorSet := make(map[topologymodel.ActorHandle]struct{}, len(data.Actors))
	for _, actor := range data.Actors {
		if !actor.ActorHandle.IsZero() {
			actorSet[actor.ActorHandle] = struct{}{}
		}
	}
	if len(actorSet) == 0 {
		data.Links = nil
		return nil
	}
	if err := limiter.Charge(uint64(len(data.Links))); err != nil {
		return err
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
	return nil
}
