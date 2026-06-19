// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplySNMPTopologyShapePolicies_EmitsStatsContractKeys(t *testing.T) {
	data := topologyData{
		Actors: []topologyActor{
			{ActorID: "device-a", ActorType: "device", Match: topologyMatch{IPAddresses: []string{"10.0.0.1"}}},
			{ActorID: "segment-a", ActorType: "segment"},
			{ActorID: "endpoint-a", ActorType: "endpoint"},
		},
		Links: []topologyLink{
			{SrcActorID: "device-a", DstActorID: "segment-a"},
			{SrcActorID: "segment-a", DstActorID: "endpoint-a"},
		},
	}

	applySNMPTopologyShapePolicies(&data, topologyQueryOptions{
		EliminateNonIPInferred: true,
		InferenceStrategy:      topologyInferenceStrategySTPParentTree,
		MapType:                topologyMapTypeHighConfidenceInferred,
	})

	keys := make([]string, 0, len(data.Stats))
	for key := range data.Stats {
		keys = append(keys, key)
	}
	require.ElementsMatch(t, []string{
		"actors_collapsed_by_ip",
		"actors_map_type_suppressed",
		"actors_non_ip_inferred_suppressed",
		"actors_total",
		"inference_strategy",
		"links_probable",
		"links_total",
		"map_type",
		"segments_sparse_suppressed",
		"segments_suppressed",
	}, keys)
	require.Equal(t, 1, data.Stats["actors_total"])
	require.Equal(t, 0, data.Stats["links_total"])
	require.Equal(t, 1, data.Stats["actors_non_ip_inferred_suppressed"])
	require.Equal(t, 1, data.Stats["segments_sparse_suppressed"])
	require.Equal(t, 1, data.Stats["segments_suppressed"])
	require.Equal(t, topologyMapTypeHighConfidenceInferred, data.Stats["map_type"])
	require.Equal(t, topologyInferenceStrategySTPParentTree, data.Stats["inference_strategy"])
}

func TestRecomputeTopologyLinkStatsRefreshesExistingL3VisibleCount(t *testing.T) {
	data := &topologyData{
		Stats: map[string]any{
			"l3_subnet_emitted_links": 2,
		},
		Links: []topologyLink{
			{Protocol: topologyL3SubnetLinkType, LinkType: topologyL3SubnetLinkType},
			{Protocol: "lldp", LinkType: "lldp"},
		},
	}

	recomputeTopologyLinkStats(data)

	require.Equal(t, 2, data.Stats["links_total"])
	require.Equal(t, 2, data.Stats["l3_subnet_emitted_links"])
	require.Equal(t, 1, data.Stats["l3_subnet_visible_links"])
}
