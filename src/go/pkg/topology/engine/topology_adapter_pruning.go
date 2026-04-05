// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
)

func pruneSegmentArtifacts(actors []topology.Actor, links []topology.Link) ([]topology.Actor, []topology.Link, int) {
	if len(actors) == 0 || len(links) == 0 {
		return actors, links, 0
	}

	segmentKeys := make(map[string]struct{})
	segmentOrder := make([]string, 0)
	for _, actor := range actors {
		if !strings.EqualFold(strings.TrimSpace(actor.ActorType), "segment") {
			continue
		}
		key := canonicalTopologyMatchKey(actor.Match)
		if key == "" {
			continue
		}
		if _, seen := segmentKeys[key]; seen {
			continue
		}
		segmentKeys[key] = struct{}{}
		segmentOrder = append(segmentOrder, key)
	}
	if len(segmentKeys) == 0 {
		return actors, links, 0
	}
	sort.Strings(segmentOrder)

	discoveryPairs := make(map[string]struct{})
	for _, link := range links {
		protocol := strings.ToLower(strings.TrimSpace(link.Protocol))
		if protocol != "lldp" && protocol != "cdp" {
			continue
		}
		src := canonicalTopologyMatchKey(link.Src.Match)
		dst := canonicalTopologyMatchKey(link.Dst.Match)
		if src == "" || dst == "" {
			continue
		}
		if _, srcSegment := segmentKeys[src]; srcSegment {
			continue
		}
		if _, dstSegment := segmentKeys[dst]; dstSegment {
			continue
		}
		if pair := topologyUndirectedPairKey(src, dst); pair != "" {
			discoveryPairs[pair] = struct{}{}
		}
	}

	suppressed := make(map[string]struct{})
	for {
		changed := false
		neighborsBySegment := make(map[string]map[string]struct{})
		for _, link := range links {
			src := canonicalTopologyMatchKey(link.Src.Match)
			dst := canonicalTopologyMatchKey(link.Dst.Match)
			if src == "" || dst == "" {
				continue
			}
			if _, srcSuppressed := suppressed[src]; srcSuppressed {
				continue
			}
			if _, dstSuppressed := suppressed[dst]; dstSuppressed {
				continue
			}

			_, srcSegment := segmentKeys[src]
			_, dstSegment := segmentKeys[dst]

			if srcSegment && !dstSegment {
				neighbors := neighborsBySegment[src]
				if neighbors == nil {
					neighbors = make(map[string]struct{})
					neighborsBySegment[src] = neighbors
				}
				neighbors[dst] = struct{}{}
			}
			if dstSegment && !srcSegment {
				neighbors := neighborsBySegment[dst]
				if neighbors == nil {
					neighbors = make(map[string]struct{})
					neighborsBySegment[dst] = neighbors
				}
				neighbors[src] = struct{}{}
			}
		}

		for _, segmentKey := range segmentOrder {
			if _, alreadySuppressed := suppressed[segmentKey]; alreadySuppressed {
				continue
			}

			neighbors := neighborsBySegment[segmentKey]
			if len(neighbors) < 2 {
				suppressed[segmentKey] = struct{}{}
				changed = true
				continue
			}

			if len(neighbors) == 2 {
				pairValues := make([]string, 0, 2)
				for neighbor := range neighbors {
					pairValues = append(pairValues, neighbor)
				}
				if len(pairValues) == 2 {
					if pair := topologyUndirectedPairKey(pairValues[0], pairValues[1]); pair != "" {
						if _, found := discoveryPairs[pair]; found {
							suppressed[segmentKey] = struct{}{}
							changed = true
						}
					}
				}
			}
		}

		if !changed {
			break
		}
	}

	if len(suppressed) == 0 {
		return actors, links, 0
	}

	filteredActors := make([]topology.Actor, 0, len(actors))
	for _, actor := range actors {
		key := canonicalTopologyMatchKey(actor.Match)
		if key == "" {
			filteredActors = append(filteredActors, actor)
			continue
		}
		if _, isSuppressed := suppressed[key]; isSuppressed && strings.EqualFold(strings.TrimSpace(actor.ActorType), "segment") {
			continue
		}
		filteredActors = append(filteredActors, actor)
	}

	filteredLinks := make([]topology.Link, 0, len(links))
	for _, link := range links {
		src := canonicalTopologyMatchKey(link.Src.Match)
		dst := canonicalTopologyMatchKey(link.Dst.Match)
		if src != "" {
			if _, srcSuppressed := suppressed[src]; srcSuppressed {
				continue
			}
		}
		if dst != "" {
			if _, dstSuppressed := suppressed[dst]; dstSuppressed {
				continue
			}
		}
		filteredLinks = append(filteredLinks, link)
	}

	return filteredActors, filteredLinks, len(suppressed)
}

func topologyUndirectedPairKey(left, right string) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return ""
	}
	if left <= right {
		return left + keySep + right
	}
	return right + keySep + left
}

type topologyLinkCounts struct {
	lldp           int
	cdp            int
	fdb            int
	arp            int
	bidirectional  int
	unidirectional int
}

func summarizeTopologyLinks(links []topology.Link) topologyLinkCounts {
	var counts topologyLinkCounts
	for _, link := range links {
		switch strings.ToLower(strings.TrimSpace(link.Protocol)) {
		case "lldp":
			counts.lldp++
		case "cdp":
			counts.cdp++
		case "bridge", "fdb":
			counts.fdb++
		case "arp":
			counts.arp++
		}

		switch strings.ToLower(strings.TrimSpace(link.Direction)) {
		case "bidirectional":
			counts.bidirectional++
		case "unidirectional":
			counts.unidirectional++
		}
	}
	return counts
}

func pruneManagedOverlapUnlinkedEndpointActors(
	actors []topology.Actor,
	links []topology.Link,
	suppressedEndpointIDs map[string]struct{},
) ([]topology.Actor, int) {
	if len(actors) == 0 || len(suppressedEndpointIDs) == 0 {
		return actors, 0
	}

	suppressedIdentityKeys := make(map[string]struct{})
	for endpointID := range suppressedEndpointIDs {
		endpointID = normalizeFDBEndpointID(endpointID)
		if endpointID == "" {
			continue
		}
		match := endpointMatchFromID(endpointID)
		for _, key := range topologyMatchIdentityKeys(match) {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			suppressedIdentityKeys[key] = struct{}{}
		}
	}
	if len(suppressedIdentityKeys) == 0 {
		return actors, 0
	}

	linkedIdentityKeys := make(map[string]struct{}, len(links)*2)
	for _, link := range links {
		for _, key := range topologyMatchIdentityKeys(link.Src.Match) {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			linkedIdentityKeys[key] = struct{}{}
		}
		for _, key := range topologyMatchIdentityKeys(link.Dst.Match) {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			linkedIdentityKeys[key] = struct{}{}
		}
	}

	filtered := make([]topology.Actor, 0, len(actors))
	suppressedCount := 0
	for _, actor := range actors {
		if !strings.EqualFold(strings.TrimSpace(actor.ActorType), "endpoint") {
			filtered = append(filtered, actor)
			continue
		}

		actorKeys := topologyMatchIdentityKeys(actor.Match)
		if !topologyIdentityKeysOverlap(actorKeys, suppressedIdentityKeys) {
			filtered = append(filtered, actor)
			continue
		}
		// Keep endpoint actors that still participate in at least one emitted link.
		if topologyIdentityKeysOverlap(actorKeys, linkedIdentityKeys) {
			filtered = append(filtered, actor)
			continue
		}
		suppressedCount++
	}
	return filtered, suppressedCount
}
