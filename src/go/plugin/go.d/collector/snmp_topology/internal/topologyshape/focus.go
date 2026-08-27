// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"sort"

	"github.com/netdata/netdata/go/plugins/pkg/topology/worklimit"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

func ApplyDepthFocusFilter(data *topologymodel.Data, options topologyoptions.QueryOptions) error {
	if data == nil || len(data.Actors) == 0 {
		return nil
	}
	options = topologyoptions.NormalizeQueryOptions(options)
	focusIPs := topologyoptions.ManagedFocusSelectedIPs(options.ManagedDeviceFocus)

	beforeActors := len(data.Actors)
	beforeLinks := len(data.Links)

	if topologyoptions.IsManagedFocusAllDevices(options.ManagedDeviceFocus) {
		recordTopologyFocusAllDevicesStats(data, options)
		return nil
	}
	items, err := worklimit.Sum(uint64(len(data.Actors)), uint64(len(data.Links)))
	if err != nil {
		return err
	}
	if err := options.WorkLimiter.ChargeProduct(items, 4); err != nil {
		return err
	}

	graph := buildTopologyFocusGraph(data)
	if len(graph.nonSegmentSet) == 0 || len(focusIPs) == 0 {
		topologymodel.RecomputeLinkStats(data)
		return nil
	}
	if err := options.WorkLimiter.ChargeProduct(uint64(len(graph.actorByHandle)), uint64(len(focusIPs))); err != nil {
		return err
	}

	roots := collectTopologyFocusRoots(graph, focusIPs)
	if len(roots) == 0 {
		recordTopologyFocusStats(data, options, beforeActors, beforeLinks)
		return nil
	}

	distance := traverseTopologyFocusDepth(graph, roots, options.Depth)
	includedNonSegment, includedActorsByDepth := collectTopologyFocusDepthSets(graph, distance, options.Depth)
	if len(includedNonSegment) == 0 {
		topologymodel.RecomputeLinkStats(data)
		return nil
	}

	shortestPathActors, shortestPathPairs, err := topologyShortestPathUnion(data, roots, options.WorkLimiter)
	if err != nil {
		return err
	}
	filterTopologyDataByFocus(data, includedActorsByDepth, shortestPathActors, shortestPathPairs)

	filterDanglingLinks(data)
	if options.EliminateNonIPInferred {
		pruneSparseSegments(data, 1)
		filterDanglingLinks(data)
	}

	if err := options.WorkLimiter.ChargeSort(uint64(len(data.Actors))); err != nil {
		return err
	}
	sort.Slice(data.Actors, func(i, j int) bool {
		return topologymodel.CanonicalMatchKey(data.Actors[i].Match) < topologymodel.CanonicalMatchKey(data.Actors[j].Match)
	})
	if err := options.WorkLimiter.ChargeSort(uint64(len(data.Links))); err != nil {
		return err
	}
	sort.Slice(data.Links, func(i, j int) bool {
		return topologymodel.LinkSortKey(data.Links[i]) < topologymodel.LinkSortKey(data.Links[j])
	})

	recordTopologyFocusStats(data, options, beforeActors, beforeLinks)
	return nil
}
