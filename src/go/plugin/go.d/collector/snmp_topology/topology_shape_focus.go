// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sort"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

func applyTopologyDepthFocusFilter(data *topologyData, options topologyQueryOptions) {
	if data == nil || len(data.Actors) == 0 {
		return
	}
	options = topologyoptions.NormalizeQueryOptions(options)
	focusIPs := topologyoptions.ManagedFocusSelectedIPs(options.ManagedDeviceFocus)

	beforeActors := len(data.Actors)
	beforeLinks := len(data.Links)

	if topologyoptions.IsManagedFocusAllDevices(options.ManagedDeviceFocus) {
		recordTopologyFocusAllDevicesStats(data, options)
		return
	}

	graph := buildTopologyFocusGraph(data)
	if len(graph.nonSegmentSet) == 0 || len(focusIPs) == 0 {
		topologymodel.RecomputeLinkStats(data)
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
		topologymodel.RecomputeLinkStats(data)
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
		return topologymodel.CanonicalMatchKey(data.Actors[i].Match) < topologymodel.CanonicalMatchKey(data.Actors[j].Match)
	})
	sort.Slice(data.Links, func(i, j int) bool {
		return topologymodel.LinkSortKey(data.Links[i]) < topologymodel.LinkSortKey(data.Links[j])
	})

	recordTopologyFocusStats(data, options, beforeActors, beforeLinks)
}
