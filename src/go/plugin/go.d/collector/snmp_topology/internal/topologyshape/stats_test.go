// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	"github.com/stretchr/testify/require"
)

func TestApplyPoliciesRecordsShapeStats(t *testing.T) {
	data := topologymodel.Data{
		Actors: []topologymodel.Actor{
			{ActorID: "device-a", ActorType: "device", Match: topologymodel.Match{IPAddresses: []string{"10.0.0.1"}}},
			{ActorID: "segment-a", ActorType: "segment", SegmentKind: topologymodel.SegmentKindBroadcastDomain},
			{ActorID: "endpoint-a", ActorType: "endpoint"},
		},
		Links: []topologymodel.Link{
			{SrcActorID: "device-a", DstActorID: "segment-a"},
			{SrcActorID: "segment-a", DstActorID: "endpoint-a"},
		},
	}

	ApplyPolicies(&data, topologyoptions.QueryOptions{
		EliminateNonIPInferred: true,
		InferenceStrategy:      topologyoptions.InferenceStrategySTPParentTree,
		MapType:                topologyoptions.MapTypeHighConfidenceInferred,
	})

	require.True(t, data.Stats.HasShape)
	require.Equal(t, topologyoptions.InferenceStrategySTPParentTree, data.Stats.Shape.InferenceStrategy)
	require.Equal(t, topologyoptions.MapTypeHighConfidenceInferred, data.Stats.Shape.MapType)
	require.Equal(t, 1, data.Stats.Shape.ActorsNonIPInferredSuppressed)
	require.Equal(t, 1, data.Stats.Shape.SegmentsSparseSuppressed)
	require.Equal(t, 0, data.Stats.Shape.ActorsMapTypeSuppressed)
	require.True(t, data.Stats.HasComputed)
	require.Equal(t, 1, data.Stats.Recomputed.ActorsTotal)
	require.Equal(t, 0, data.Stats.Recomputed.LinksTotal)
}

func TestRecomputeTopologyLinkStatsRefreshesExistingLogicalL3VisibleCounts(t *testing.T) {
	data := &topologymodel.Data{
		Stats: topologymodel.Stats{
			L3:      topologymodel.L3EnrichmentStats{EmittedLinks: 2},
			HasL3:   true,
			OSPF:    topologymodel.OSPFEnrichmentStats{EmittedLinks: 1},
			HasOSPF: true,
			BGP:     topologymodel.BGPEnrichmentStats{EmittedLinks: 1},
			HasBGP:  true,
		},
		Links: []topologymodel.Link{
			{Protocol: topologymodel.L3SubnetLinkType, LinkType: topologymodel.L3SubnetLinkType},
			{Protocol: topologymodel.BGPAdjacencyLinkType, LinkType: topologymodel.BGPAdjacencyLinkType},
			{Protocol: "lldp", LinkType: "lldp"},
		},
	}

	topologymodel.RecomputeLinkStats(data)

	require.Equal(t, 3, data.Stats.Recomputed.LinksTotal)
	require.Equal(t, 1, data.Stats.Recomputed.L3SubnetVisibleLinks)
	require.Equal(t, 0, data.Stats.Recomputed.OSPFAdjacencyVisibleLinks)
	require.Equal(t, 1, data.Stats.Recomputed.BGPAdjacencyVisibleLinks)
}
