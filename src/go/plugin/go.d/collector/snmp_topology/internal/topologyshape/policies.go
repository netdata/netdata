// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"sort"

	"github.com/netdata/netdata/go/plugins/pkg/topology/worklimit"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

func ApplyPolicies(data *topologymodel.Data, options topologyoptions.QueryOptions) error {
	if data == nil {
		return nil
	}
	if limiter := options.WorkLimiter; limiter != nil {
		items, err := worklimit.Sum(uint64(len(data.Actors)), uint64(len(data.Links)))
		if err != nil {
			return err
		}
		if err := limiter.ChargeProduct(items, 6); err != nil {
			return err
		}
		if err := limiter.ChargeSort(uint64(len(data.Actors))); err != nil {
			return err
		}
		if err := limiter.ChargeSort(uint64(len(data.Links))); err != nil {
			return err
		}
	}
	mapType := topologyoptions.NormalizeMapType(options.MapType)
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
	return nil
}
