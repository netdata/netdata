// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sort"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

func applySNMPTopologyShapePolicies(data *topologyData, options topologyQueryOptions) {
	if data == nil {
		return
	}
	mapType := topologyoptions.NormalizeMapType(options.MapType)
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
		return topologymodel.CanonicalMatchKey(data.Actors[i].Match) < topologymodel.CanonicalMatchKey(data.Actors[j].Match)
	})
	sort.Slice(data.Links, func(i, j int) bool {
		return topologymodel.LinkSortKey(data.Links[i]) < topologymodel.LinkSortKey(data.Links[j])
	})

	data.Stats.Shape.ActorsCollapsedByIP = collapsed
	data.Stats.Shape.ActorsNonIPInferredSuppressed = removedNonIP
	data.Stats.Shape.ActorsMapTypeSuppressed = removedByMapType
	data.Stats.Shape.SegmentsSparseSuppressed = removedSparseSegments
	data.Stats.Shape.MapType = options.MapType
	if strategy := topologyoptions.NormalizeInferenceStrategy(options.InferenceStrategy); strategy != "" {
		data.Stats.Shape.InferenceStrategy = strategy
	}
	data.Stats.HasShape = true
	topologymodel.RecomputeLinkStats(data)
}
