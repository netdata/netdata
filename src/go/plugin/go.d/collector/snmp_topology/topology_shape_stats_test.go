// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
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

	stats := topologyStatsToV1(data.Stats)
	keys := make([]string, 0, len(stats))
	for key := range stats {
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
			require.Equal(t, tc.want, topologyStatsToV1(data.Stats)[tc.key])
		})
	}
}

func TestRecomputeTopologyLinkStatsRefreshesExistingLogicalL3VisibleCounts(t *testing.T) {
	data := &topologyData{
		Stats: topologyStats{
			L3:      topologyL3EnrichmentStats{EmittedLinks: 2},
			HasL3:   true,
			OSPF:    topologyOSPFEnrichmentStats{EmittedLinks: 1},
			HasOSPF: true,
			BGP:     topologyBGPEnrichmentStats{EmittedLinks: 1},
			HasBGP:  true,
		},
		Links: []topologyLink{
			{Protocol: topologyL3SubnetLinkType, LinkType: topologyL3SubnetLinkType},
			{Protocol: topologyBGPAdjacencyLinkType, LinkType: topologyBGPAdjacencyLinkType},
			{Protocol: "lldp", LinkType: "lldp"},
		},
	}

	topologymodel.RecomputeLinkStats(data)

	expectedStats := map[string]struct {
		key  string
		want any
	}{
		"emitted-l3-subnet-links": {key: "l3_subnet_emitted_links", want: 2},
		"emitted-ospf-links":      {key: "ospf_adjacency_emitted_links", want: 1},
		"emitted-bgp-links":       {key: "bgp_adjacency_emitted_links", want: 1},
		"links-total":             {key: "links_total", want: 3},
		"visible-l3-subnet-links": {key: "l3_subnet_visible_links", want: 1},
		"visible-ospf-links":      {key: "ospf_adjacency_visible_links", want: 0},
		"visible-bgp-links":       {key: "bgp_adjacency_visible_links", want: 1},
	}
	for name, tc := range expectedStats {
		t.Run("stat/"+name, func(t *testing.T) {
			require.Equal(t, tc.want, topologyStatsToV1(data.Stats)[tc.key])
		})
	}
}
