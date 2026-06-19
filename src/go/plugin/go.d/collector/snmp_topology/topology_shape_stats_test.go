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

	expectedStats := map[string]struct {
		key  string
		want any
	}{
		"actors-total":                      {key: "actors_total", want: 1},
		"inference-strategy":                {key: "inference_strategy", want: topologyInferenceStrategySTPParentTree},
		"links-total":                       {key: "links_total", want: 0},
		"map-type":                          {key: "map_type", want: topologyMapTypeHighConfidenceInferred},
		"non-ip-inferred-actors-suppressed": {key: "actors_non_ip_inferred_suppressed", want: 1},
		"sparse-segments-suppressed":        {key: "segments_sparse_suppressed", want: 1},
		"suppressed-segments-after-cleanup": {key: "segments_suppressed", want: 1},
	}
	for name, tc := range expectedStats {
		t.Run("stat/"+name, func(t *testing.T) {
			require.Equal(t, tc.want, data.Stats[tc.key])
		})
	}
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

	expectedStats := map[string]struct {
		key  string
		want any
	}{
		"emitted-l3-subnet-links": {key: "l3_subnet_emitted_links", want: 2},
		"links-total":             {key: "links_total", want: 2},
		"visible-l3-subnet-links": {key: "l3_subnet_visible_links", want: 1},
	}
	for name, tc := range expectedStats {
		t.Run("stat/"+name, func(t *testing.T) {
			require.Equal(t, tc.want, data.Stats[tc.key])
		})
	}
}
