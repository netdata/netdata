// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import "strings"

func eliminateNonIPInferredActors(data *topologyData) int {
	if data == nil || len(data.Actors) == 0 {
		return 0
	}

	removedIDs := make(map[string]struct{})
	keptActors := make([]topologyActor, 0, len(data.Actors))
	for _, actor := range data.Actors {
		if topologyActorIsInferred(actor) && len(normalizedMatchIPs(actor.Match)) == 0 {
			removedIDs[actor.ActorID] = struct{}{}
			continue
		}
		keptActors = append(keptActors, actor)
	}

	if len(removedIDs) == 0 {
		return 0
	}

	data.Actors = keptActors
	links := make([]topologyLink, 0, len(data.Links))
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

func pruneSparseSegments(data *topologyData, threshold int) int {
	if data == nil || len(data.Actors) == 0 {
		return 0
	}

	removedTotal := 0
	for {
		segmentSet := make(map[string]struct{})
		for _, actor := range data.Actors {
			if strings.EqualFold(strings.TrimSpace(actor.ActorType), "segment") {
				segmentSet[actor.ActorID] = struct{}{}
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

		removeSegments := make(map[string]struct{})
		for segmentID, neighbors := range neighborSet {
			if len(neighbors) <= threshold {
				removeSegments[segmentID] = struct{}{}
			}
		}
		if len(removeSegments) == 0 {
			return removedTotal
		}
		removedTotal += len(removeSegments)

		filteredActors := make([]topologyActor, 0, len(data.Actors)-len(removeSegments))
		for _, actor := range data.Actors {
			if _, drop := removeSegments[actor.ActorID]; drop {
				continue
			}
			filteredActors = append(filteredActors, actor)
		}
		data.Actors = filteredActors

		filteredLinks := make([]topologyLink, 0, len(data.Links))
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

func filterDanglingLinks(data *topologyData) {
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
	filtered := make([]topologyLink, 0, len(data.Links))
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
