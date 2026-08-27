// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

func ApplyPolicies(data *topologymodel.Data, options topologyoptions.QueryOptions) error {
	if data == nil {
		return nil
	}
	mapType := topologyoptions.NormalizeMapType(options.MapType)
	options.MapType = mapType

	collapsed := 0
	if options.CollapseActorsByIP {
		var err error
		collapsed, err = collapseActorsByIPWithLimiter(data, options.WorkLimiter)
		if err != nil {
			return err
		}
	}

	removedNonIP := 0
	if options.EliminateNonIPInferred {
		var err error
		removedNonIP, err = eliminateNonIPInferredActorsWithLimiter(data, options.WorkLimiter)
		if err != nil {
			return err
		}
	}

	if err := filterDanglingLinksWithLimiter(data, options.WorkLimiter); err != nil {
		return err
	}
	removedByMapType, err := applyMapTypePolicyWithLimiter(data, options.MapType, options.WorkLimiter)
	if err != nil {
		return err
	}
	if err := filterDanglingLinksWithLimiter(data, options.WorkLimiter); err != nil {
		return err
	}

	removedSparseSegments := 0
	if options.EliminateNonIPInferred {
		removedSparseSegments, err = pruneSparseSegmentsWithLimiter(data, 1, options.WorkLimiter)
		if err != nil {
			return err
		}
	}
	if err := filterDanglingLinksWithLimiter(data, options.WorkLimiter); err != nil {
		return err
	}

	if err := topologymodel.SortActors(options.WorkLimiter, data.Actors); err != nil {
		return err
	}
	if err := topologymodel.SortLinks(options.WorkLimiter, data.Links); err != nil {
		return err
	}

	data.Stats.Shape.ActorsCollapsedByIP = collapsed
	data.Stats.Shape.ActorsNonIPInferredSuppressed = removedNonIP
	data.Stats.Shape.ActorsMapTypeSuppressed = removedByMapType
	data.Stats.Shape.SegmentsSparseSuppressed = removedSparseSegments
	data.Stats.Shape.MapType = options.MapType
	if strategy := topologyoptions.NormalizeInferenceStrategy(options.InferenceStrategy); strategy != "" {
		data.Stats.Shape.InferenceStrategy = strategy
	}
	data.Stats.HasShape = true
	return topologymodel.RecomputeLinkStatsWithLimiter(data, options.WorkLimiter)
}
