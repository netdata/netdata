// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

func ApplyDepthFocusFilter(data *topologymodel.Data, options topologyoptions.QueryOptions) error {
	if data == nil || len(data.Actors) == 0 {
		return nil
	}
	var err error
	options, err = topologyoptions.PrepareQueryOptions(options)
	if err != nil {
		return err
	}
	focusIPs := options.ManagedFocusIPs()

	beforeActors := len(data.Actors)
	beforeLinks := len(data.Links)

	if options.ManagedFocusIsAllDevices() {
		return recordTopologyFocusAllDevicesStats(data, options)
	}
	graph, err := buildTopologyFocusGraphWithLimiter(data, options.WorkLimiter)
	if err != nil {
		return err
	}
	if len(graph.nonSegmentSet) == 0 || len(focusIPs) == 0 {
		return topologymodel.RecomputeLinkStatsWithLimiter(data, options.WorkLimiter)
	}
	roots, err := collectTopologyFocusRootsWithLimiter(graph, focusIPs, options.WorkLimiter)
	if err != nil {
		return err
	}
	if len(roots) == 0 {
		return recordTopologyFocusStats(data, options, beforeActors, beforeLinks)
	}

	distance, err := traverseTopologyFocusDepthWithLimiter(graph, roots, options.Depth, options.WorkLimiter)
	if err != nil {
		return err
	}
	includedNonSegment, includedActorsByDepth, err := collectTopologyFocusDepthSetsWithLimiter(graph, distance, options.Depth, options.WorkLimiter)
	if err != nil {
		return err
	}
	if len(includedNonSegment) == 0 {
		return topologymodel.RecomputeLinkStatsWithLimiter(data, options.WorkLimiter)
	}

	shortestPathActors, shortestPathPairs, err := topologyShortestPathUnion(data, roots, options.WorkLimiter)
	if err != nil {
		return err
	}
	if err := filterTopologyDataByFocusWithLimiter(data, includedActorsByDepth, shortestPathActors, shortestPathPairs, options.WorkLimiter); err != nil {
		return err
	}

	if err := filterDanglingLinksWithLimiter(data, options.WorkLimiter); err != nil {
		return err
	}
	if options.EliminateNonIPInferred {
		if _, err := pruneSparseSegmentsWithLimiter(data, 1, options.WorkLimiter); err != nil {
			return err
		}
		if err := filterDanglingLinksWithLimiter(data, options.WorkLimiter); err != nil {
			return err
		}
	}

	if err := topologymodel.SortActors(options.WorkLimiter, data.Actors); err != nil {
		return err
	}
	if err := topologymodel.SortLinks(options.WorkLimiter, data.Links); err != nil {
		return err
	}

	return recordTopologyFocusStats(data, options, beforeActors, beforeLinks)
}
