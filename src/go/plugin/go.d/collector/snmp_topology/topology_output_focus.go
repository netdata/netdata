// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import "sort"

func applyTopologyDepthFocusFilter(data *topologyData, options topologyQueryOptions) {
	if data == nil || len(data.Actors) == 0 {
		return
	}
	options = normalizeTopologyQueryOptions(options)
	focusIPs := topologyManagedFocusSelectedIPs(options.ManagedDeviceFocus)

	beforeActors := len(data.Actors)
	beforeLinks := len(data.Links)

	if isTopologyManagedFocusAllDevices(options.ManagedDeviceFocus) {
		recordTopologyFocusAllDevicesStats(data, options)
		return
	}

	graph := buildTopologyFocusGraph(data)
	if len(graph.nonSegmentSet) == 0 || len(focusIPs) == 0 {
		recomputeTopologyLinkStats(data)
		return
	}

	roots := collectTopologyFocusRoots(graph, focusIPs)
	if len(roots) == 0 {
		recordTopologyFocusStats(data, options, beforeActors, beforeLinks)
		return
	}

	distance := traverseTopologyFocusDepth(graph, roots, options.Depth)
	includedNonSegment, includedActorsByDepth := collectTopologyFocusDepthSets(graph, distance, options.Depth)
	if len(includedNonSegment) == 0 {
		recomputeTopologyLinkStats(data)
		return
	}

	shortestPathActors, shortestPathPairs := topologyShortestPathUnion(data, roots)
	filterTopologyDataByFocus(data, includedActorsByDepth, shortestPathActors, shortestPathPairs)

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
