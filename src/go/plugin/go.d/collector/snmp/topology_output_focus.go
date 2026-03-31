// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"fmt"
	"sort"
	"strings"
)

func applyTopologyDepthFocusFilter(data *topologyData, options topologyQueryOptions) {
	if data == nil || len(data.Actors) == 0 {
		return
	}
	options = normalizeTopologyQueryOptions(options)
	focusIPs := topologyManagedFocusSelectedIPs(options.ManagedDeviceFocus)

	beforeActors := len(data.Actors)
	beforeLinks := len(data.Links)

	if isTopologyManagedFocusAllDevices(options.ManagedDeviceFocus) {
		if data.Stats == nil {
			data.Stats = make(map[string]any)
		}
		data.Stats["managed_snmp_device_focus"] = options.ManagedDeviceFocus
		data.Stats["depth"] = topologyDepthAll
		data.Stats["actors_focus_depth_filtered"] = 0
		data.Stats["links_focus_depth_filtered"] = 0
		recomputeTopologyLinkStats(data)
		return
	}

	actorByID := make(map[string]topologyActor, len(data.Actors))
	segmentSet := make(map[string]struct{})
	nonSegmentSet := make(map[string]struct{})
	for _, actor := range data.Actors {
		id := strings.TrimSpace(actor.ActorID)
		if id == "" {
			continue
		}
		actorByID[id] = actor
		if strings.EqualFold(strings.TrimSpace(actor.ActorType), "segment") {
			segmentSet[id] = struct{}{}
		} else {
			nonSegmentSet[id] = struct{}{}
		}
	}
	if len(nonSegmentSet) == 0 || len(focusIPs) == 0 {
		recomputeTopologyLinkStats(data)
		return
	}

	roots := make(map[string]struct{})
	for actorID, actor := range actorByID {
		if _, ok := nonSegmentSet[actorID]; !ok {
			continue
		}
		if !isManagedSNMPDeviceActor(actor) {
			continue
		}
		for _, focusIP := range focusIPs {
			if !topologyActorHasIP(actor, focusIP) {
				continue
			}
			roots[actorID] = struct{}{}
			break
		}
	}
	if len(roots) == 0 {
		recordTopologyFocusStats(data, options, beforeActors, beforeLinks)
		return
	}

	nonSegmentAdj := make(map[string]map[string]struct{}, len(nonSegmentSet))
	nodeSegments := make(map[string]map[string]struct{}, len(nonSegmentSet))
	segmentNeighbors := make(map[string]map[string]struct{}, len(segmentSet))
	for actorID := range nonSegmentSet {
		nonSegmentAdj[actorID] = make(map[string]struct{})
		nodeSegments[actorID] = make(map[string]struct{})
	}
	for segmentID := range segmentSet {
		segmentNeighbors[segmentID] = make(map[string]struct{})
	}

	for _, link := range data.Links {
		src := strings.TrimSpace(link.SrcActorID)
		dst := strings.TrimSpace(link.DstActorID)
		if src == "" || dst == "" || src == dst {
			continue
		}
		_, srcSegment := segmentSet[src]
		_, dstSegment := segmentSet[dst]
		_, srcNonSegment := nonSegmentSet[src]
		_, dstNonSegment := nonSegmentSet[dst]

		switch {
		case srcNonSegment && dstNonSegment:
			nonSegmentAdj[src][dst] = struct{}{}
			nonSegmentAdj[dst][src] = struct{}{}
		case srcSegment && dstNonSegment:
			segmentNeighbors[src][dst] = struct{}{}
			nodeSegments[dst][src] = struct{}{}
		case dstSegment && srcNonSegment:
			segmentNeighbors[dst][src] = struct{}{}
			nodeSegments[src][dst] = struct{}{}
		}
	}

	distance := make(map[string]int, len(nonSegmentSet))
	queue := make([]string, 0, len(roots))
	for root := range roots {
		distance[root] = 0
		queue = append(queue, root)
	}
	segmentExpandedDepth := make(map[string]int)

	for head := 0; head < len(queue); head++ {
		current := queue[head]
		currentDepth := distance[current]
		if options.Depth != topologyDepthAllInternal && currentDepth >= options.Depth {
			continue
		}

		for neighbor := range nonSegmentAdj[current] {
			if _, seen := distance[neighbor]; seen {
				continue
			}
			distance[neighbor] = currentDepth + 1
			queue = append(queue, neighbor)
		}

		for segmentID := range nodeSegments[current] {
			if expandedAt, ok := segmentExpandedDepth[segmentID]; ok && expandedAt <= currentDepth {
				continue
			}
			segmentExpandedDepth[segmentID] = currentDepth
			for neighbor := range segmentNeighbors[segmentID] {
				if _, seen := distance[neighbor]; seen {
					continue
				}
				distance[neighbor] = currentDepth + 1
				queue = append(queue, neighbor)
			}
		}
	}

	includedNonSegment := make(map[string]struct{}, len(distance))
	for actorID, depth := range distance {
		if options.Depth == topologyDepthAllInternal || depth <= options.Depth {
			includedNonSegment[actorID] = struct{}{}
		}
	}
	if len(includedNonSegment) == 0 {
		recomputeTopologyLinkStats(data)
		return
	}

	includedActorsByDepth := make(map[string]struct{}, len(includedNonSegment)+len(segmentSet))
	for actorID := range includedNonSegment {
		includedActorsByDepth[actorID] = struct{}{}
	}
	if options.Depth == topologyDepthAllInternal || options.Depth > 0 {
		for segmentID, neighbors := range segmentNeighbors {
			for actorID := range neighbors {
				if _, ok := includedNonSegment[actorID]; ok {
					includedActorsByDepth[segmentID] = struct{}{}
					break
				}
			}
		}
	}

	shortestPathActors, shortestPathPairs := topologyShortestPathUnion(data, roots)
	includedActors := make(map[string]struct{}, len(includedActorsByDepth)+len(shortestPathActors))
	for actorID := range includedActorsByDepth {
		includedActors[actorID] = struct{}{}
	}
	for actorID := range shortestPathActors {
		includedActors[actorID] = struct{}{}
	}

	filteredLinks := make([]topologyLink, 0, len(data.Links))
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

	filteredActors := make([]topologyActor, 0, len(data.Actors))
	for _, actor := range data.Actors {
		if _, ok := includedActors[actor.ActorID]; ok {
			filteredActors = append(filteredActors, actor)
		}
	}
	data.Actors = filteredActors

	filterDanglingLinks(data)
	if options.EliminateNonIPInferred {
		pruneSparseSegments(data, 1)
		filterDanglingLinks(data)
	}

	sort.Slice(data.Actors, func(i, j int) bool {
		return canonicalMatchKey(data.Actors[i].Match) < canonicalMatchKey(data.Actors[j].Match)
	})
	sort.Slice(data.Links, func(i, j int) bool {
		return topologyLinkSortKey(data.Links[i]) < topologyLinkSortKey(data.Links[j])
	})

	recordTopologyFocusStats(data, options, beforeActors, beforeLinks)
}

func recordTopologyFocusStats(data *topologyData, options topologyQueryOptions, beforeActors, beforeLinks int) {
	if data == nil {
		return
	}
	if data.Stats == nil {
		data.Stats = make(map[string]any)
	}
	data.Stats["managed_snmp_device_focus"] = options.ManagedDeviceFocus
	if options.Depth == topologyDepthAllInternal {
		data.Stats["depth"] = topologyDepthAll
	} else {
		data.Stats["depth"] = options.Depth
	}
	data.Stats["actors_focus_depth_filtered"] = beforeActors - len(data.Actors)
	data.Stats["links_focus_depth_filtered"] = beforeLinks - len(data.Links)
	recomputeTopologyLinkStats(data)
}

func topologyManagedFocusSelectedIP(value string) string {
	ips := topologyManagedFocusSelectedIPs(value)
	if len(ips) == 0 {
		return ""
	}
	return ips[0]
}

func topologyManagedFocusSelectedIPs(value string) []string {
	normalized := parseTopologyManagedFocuses(value)
	if len(normalized) == 1 && normalized[0] == topologyManagedFocusAllDevices {
		return nil
	}

	out := make([]string, 0, len(normalized))
	for _, focus := range normalized {
		if len(focus) <= len(topologyManagedFocusIPPrefix) {
			continue
		}
		if !strings.EqualFold(focus[:len(topologyManagedFocusIPPrefix)], topologyManagedFocusIPPrefix) {
			continue
		}
		ip := normalizeIPAddress(strings.TrimSpace(focus[len(topologyManagedFocusIPPrefix):]))
		if ip == "" {
			continue
		}
		out = append(out, ip)
	}
	return out
}

func topologyShortestPathUnion(
	data *topologyData,
	roots map[string]struct{},
) (map[string]struct{}, map[string]struct{}) {
	includedActors := make(map[string]struct{})
	includedPairs := make(map[string]struct{})
	if data == nil || len(roots) < 2 {
		return includedActors, includedPairs
	}

	adjacency := make(map[string]map[string]struct{})
	for _, link := range data.Links {
		src := strings.TrimSpace(link.SrcActorID)
		dst := strings.TrimSpace(link.DstActorID)
		if src == "" || dst == "" || src == dst {
			continue
		}
		if _, ok := adjacency[src]; !ok {
			adjacency[src] = make(map[string]struct{})
		}
		if _, ok := adjacency[dst]; !ok {
			adjacency[dst] = make(map[string]struct{})
		}
		adjacency[src][dst] = struct{}{}
		adjacency[dst][src] = struct{}{}
	}

	rootIDs := make([]string, 0, len(roots))
	for actorID := range roots {
		rootIDs = append(rootIDs, actorID)
	}
	sort.Strings(rootIDs)

	for i := 0; i < len(rootIDs); i++ {
		source := rootIDs[i]
		if _, ok := adjacency[source]; !ok {
			continue
		}

		parents, distance := topologyShortestParents(adjacency, source)
		for j := i + 1; j < len(rootIDs); j++ {
			target := rootIDs[j]
			if _, ok := distance[target]; !ok {
				continue
			}

			visited := make(map[string]struct{})
			stack := []string{target}
			for len(stack) > 0 {
				node := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if _, seen := visited[node]; seen {
					continue
				}
				visited[node] = struct{}{}
				includedActors[node] = struct{}{}
				if node == source {
					continue
				}

				for _, parent := range parents[node] {
					includedActors[parent] = struct{}{}
					includedPairs[topologyActorPairKey(node, parent)] = struct{}{}
					stack = append(stack, parent)
				}
			}
		}
	}

	return includedActors, includedPairs
}

func topologyShortestParents(
	adjacency map[string]map[string]struct{},
	source string,
) (map[string][]string, map[string]int) {
	parents := make(map[string][]string)
	distance := map[string]int{source: 0}
	queue := []string{source}

	for head := 0; head < len(queue); head++ {
		current := queue[head]
		neighbors := make([]string, 0, len(adjacency[current]))
		for neighbor := range adjacency[current] {
			neighbors = append(neighbors, neighbor)
		}
		sort.Strings(neighbors)
		for _, neighbor := range neighbors {
			nextDepth := distance[current] + 1
			currentDepth, seen := distance[neighbor]
			if !seen {
				distance[neighbor] = nextDepth
				parents[neighbor] = []string{current}
				queue = append(queue, neighbor)
				continue
			}
			if nextDepth == currentDepth {
				parents[neighbor] = append(parents[neighbor], current)
			}
		}
	}

	for node := range parents {
		sort.Strings(parents[node])
	}

	return parents, distance
}

func topologyActorPairKey(left, right string) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return ""
	}
	if left > right {
		left, right = right, left
	}
	return left + "|" + right
}

func topologyActorHasIP(actor topologyActor, ip string) bool {
	ip = normalizeIPAddress(ip)
	if ip == "" {
		return false
	}
	for _, candidate := range normalizedMatchIPs(actor.Match) {
		if candidate == ip {
			return true
		}
	}
	if ip == normalizeIPAddress(topologyMetricValueString(actor.Attributes, "management_ip")) {
		return true
	}
	if raw, ok := actor.Attributes["management_addresses"]; ok {
		switch values := raw.(type) {
		case []string:
			for _, value := range values {
				if ip == normalizeIPAddress(value) {
					return true
				}
			}
		case []any:
			for _, value := range values {
				if ip == normalizeIPAddress(fmt.Sprint(value)) {
					return true
				}
			}
		}
	}
	return false
}
