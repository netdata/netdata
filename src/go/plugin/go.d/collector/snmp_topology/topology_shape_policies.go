// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import "sort"

func applySNMPTopologyShapePolicies(data *topologyData, options topologyQueryOptions) {
	if data == nil {
		return
	}
	mapType := normalizeTopologyMapType(options.MapType)
	if mapType == "" {
		mapType = topologyMapTypeAllDevicesLowConfidence
	}
	options.MapType = mapType

	collapsed := 0
	if options.CollapseActorsByIP {
		collapsed = collapseActorsByIP(data)
	}

	removedNonIP := 0
	if options.EliminateNonIPInferred {
		removedNonIP = eliminateNonIPInferredActors(data)
	}

	filterDanglingLinks(data)
	removedByMapType := applyMapTypePolicy(data, options.MapType)
	filterDanglingLinks(data)

	removedSparseSegments := 0
	if options.EliminateNonIPInferred {
		removedSparseSegments = pruneSparseSegments(data, 1)
	}
	filterDanglingLinks(data)

	sort.Slice(data.Actors, func(i, j int) bool {
		return canonicalMatchKey(data.Actors[i].Match) < canonicalMatchKey(data.Actors[j].Match)
	})
	sort.Slice(data.Links, func(i, j int) bool {
		return topologyLinkSortKey(data.Links[i]) < topologyLinkSortKey(data.Links[j])
	})

	data.Stats.Shape.ActorsCollapsedByIP = collapsed
	data.Stats.Shape.ActorsNonIPInferredSuppressed = removedNonIP
	data.Stats.Shape.ActorsMapTypeSuppressed = removedByMapType
	data.Stats.Shape.SegmentsSparseSuppressed = removedSparseSegments
	data.Stats.Shape.MapType = options.MapType
	if strategy := normalizeTopologyInferenceStrategy(options.InferenceStrategy); strategy != "" {
		data.Stats.Shape.InferenceStrategy = strategy
	}
	data.Stats.HasShape = true
	recomputeTopologyLinkStats(data)
}
