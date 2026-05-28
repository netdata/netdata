// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import "sort"

func applySNMPTopologyOutputPolicies(data *topologyData, options topologyQueryOptions) {
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

	if data.Stats == nil {
		data.Stats = make(map[string]any)
	}
	data.Stats["actors_collapsed_by_ip"] = collapsed
	data.Stats["actors_non_ip_inferred_suppressed"] = removedNonIP
	data.Stats["actors_map_type_suppressed"] = removedByMapType
	data.Stats["segments_sparse_suppressed"] = removedSparseSegments
	data.Stats["map_type"] = options.MapType
	if strategy := normalizeTopologyInferenceStrategy(options.InferenceStrategy); strategy != "" {
		data.Stats["inference_strategy"] = strategy
	}
	if removedSparseSegments > 0 {
		data.Stats["segments_suppressed"] = intStatValue(data.Stats["segments_suppressed"]) + removedSparseSegments
	}
	recomputeTopologyLinkStats(data)
}
