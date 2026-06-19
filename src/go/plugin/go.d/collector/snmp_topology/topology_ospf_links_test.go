// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplyTopologyOSPFAdjacencyEnrichmentEmitsFullManagedLink(t *testing.T) {
	data := topologyData{
		Actors: []topologyActor{
			topologyOSPFManagedActorForTest("router-a", "device-a", "1.1.1.1", "198.51.100.1"),
			topologyOSPFManagedActorForTest("router-b", "device-b", "2.2.2.2", "198.51.100.2"),
		},
	}
	aggregate := topologyObservationAggregate{
		ospfNeighbors: []topologyOSPFNeighbor{
			ospfNeighborForTest("device-a", "1.1.1.1", "2.2.2.2", "198.51.100.2", "full"),
		},
	}

	stats := applyTopologyOSPFAdjacencyEnrichment(&data, aggregate)

	require.Equal(t, 1, stats.emittedLinks)
	require.Len(t, data.Links, 1)
	link := data.Links[0]
	require.Equal(t, topologyOSPFAdjacencyLinkType, link.LinkType)
	require.Equal(t, "full", link.State)
	require.Equal(t, "1.1.1.1", link.Src.Attributes["router_id"])
	require.Equal(t, "2.2.2.2", link.Dst.Attributes["router_id"])
	require.Equal(t, 1, data.Stats["ospf_neighbor_rows"])
	require.Equal(t, 1, data.Stats["ospf_neighbor_detail_rows"])
	require.Equal(t, 1, data.Stats["ospf_adjacency_emitted_links"])
	require.Equal(t, 1, data.Stats["ospf_adjacency_visible_links"])
	require.Len(t, data.Actors[0].Tables["ospf_neighbors"], 1)
	require.Equal(t, "router-b", data.Actors[0].Tables["ospf_neighbors"][0]["remote_actor_id"])
}

func TestApplyTopologyOSPFAdjacencyEnrichmentKeepsSuppressedNeighborsAsDetailOnly(t *testing.T) {
	tests := map[string]struct {
		data                       topologyData
		aggregate                  topologyObservationAggregate
		wantNonFullState           int
		wantUnresolvedNeighbor     int
		wantSelfActor              int
		wantDetailState            string
		wantRemoteActorID          string
		wantRemoteActorIDAbsent    bool
		wantSuppressedStatsCounter string
	}{
		"non-full-neighbor": {
			data: topologyData{
				Actors: []topologyActor{
					topologyOSPFManagedActorForTest("router-a", "device-a", "1.1.1.1", "198.51.100.1"),
					topologyOSPFManagedActorForTest("router-b", "device-b", "2.2.2.2", "198.51.100.2"),
				},
			},
			aggregate: topologyObservationAggregate{
				ospfNeighbors: []topologyOSPFNeighbor{
					ospfNeighborForTest("device-a", "1.1.1.1", "2.2.2.2", "198.51.100.2", "twoWay"),
				},
			},
			wantNonFullState: 1,
			wantDetailState:  "twoWay",
		},
		"unresolved-neighbor": {
			data: topologyData{
				Actors: []topologyActor{
					topologyOSPFManagedActorForTest("router-a", "device-a", "1.1.1.1", "198.51.100.1"),
				},
			},
			aggregate: topologyObservationAggregate{
				ospfNeighbors: []topologyOSPFNeighbor{
					ospfNeighborForTest("device-a", "1.1.1.1", "2.2.2.2", "198.51.100.2", "full"),
				},
			},
			wantUnresolvedNeighbor:  1,
			wantRemoteActorIDAbsent: true,
		},
		"ambiguous-router-id-neighbor": {
			data: topologyData{
				Actors: []topologyActor{
					topologyOSPFManagedActorForTest("router-a", "device-a", "1.1.1.1", "198.51.100.1"),
					topologyOSPFManagedActorForTest("router-b", "device-b", "2.2.2.2", "198.51.100.2"),
					topologyOSPFManagedActorForTest("router-c", "device-c", "2.2.2.2", "198.51.100.3"),
				},
			},
			aggregate: topologyObservationAggregate{
				ospfNeighbors: []topologyOSPFNeighbor{
					ospfNeighborForTest("device-a", "1.1.1.1", "2.2.2.2", "0.0.0.0", "full"),
				},
			},
			wantUnresolvedNeighbor:  1,
			wantRemoteActorIDAbsent: true,
		},
		"self-actor": {
			data: topologyData{
				Actors: []topologyActor{
					topologyOSPFManagedActorForTest("router-a", "device-a", "1.1.1.1", "198.51.100.1"),
				},
			},
			aggregate: topologyObservationAggregate{
				ospfNeighbors: []topologyOSPFNeighbor{
					ospfNeighborForTest("device-a", "1.1.1.1", "1.1.1.1", "198.51.100.1", "full"),
				},
			},
			wantSelfActor:              1,
			wantRemoteActorID:          "router-a",
			wantSuppressedStatsCounter: "ospf_adjacency_suppressed_self_actor",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			stats := applyTopologyOSPFAdjacencyEnrichment(&tc.data, tc.aggregate)

			require.Zero(t, stats.emittedLinks)
			require.Equal(t, tc.wantNonFullState, stats.suppressedNonFullState)
			require.Equal(t, tc.wantUnresolvedNeighbor, stats.suppressedUnresolvedNeighbor)
			require.Equal(t, tc.wantSelfActor, stats.suppressedSelfActor)
			require.Empty(t, tc.data.Links)
			require.Len(t, tc.data.Actors[0].Tables["ospf_neighbors"], 1)
			row := tc.data.Actors[0].Tables["ospf_neighbors"][0]
			if tc.wantDetailState != "" {
				require.Equal(t, tc.wantDetailState, row["state"])
			}
			if tc.wantRemoteActorID != "" {
				require.Equal(t, tc.wantRemoteActorID, row["remote_actor_id"])
			}
			if tc.wantRemoteActorIDAbsent {
				require.NotContains(t, row, "remote_actor_id")
			}
			if tc.wantSuppressedStatsCounter != "" {
				require.Equal(t, 1, tc.data.Stats[tc.wantSuppressedStatsCounter])
			}
		})
	}
}

func TestApplyTopologyOSPFAdjacencyEnrichmentDeduplicatesBidirectionalObservations(t *testing.T) {
	data := topologyData{
		Actors: []topologyActor{
			topologyOSPFManagedActorForTest("router-a", "device-a", "1.1.1.1", "198.51.100.1"),
			topologyOSPFManagedActorForTest("router-b", "device-b", "2.2.2.2", "198.51.100.2"),
		},
	}
	aggregate := topologyObservationAggregate{
		ospfNeighbors: []topologyOSPFNeighbor{
			ospfNeighborForTest("device-a", "1.1.1.1", "2.2.2.2", "198.51.100.2", "full"),
			ospfNeighborForTest("device-b", "2.2.2.2", "1.1.1.1", "198.51.100.1", "full"),
		},
	}

	stats := applyTopologyOSPFAdjacencyEnrichment(&data, aggregate)

	require.Equal(t, 1, stats.emittedLinks)
	require.Equal(t, 1, stats.suppressedDuplicateLink)
	require.Len(t, data.Links, 1)
	require.Len(t, data.Actors[0].Tables["ospf_neighbors"], 1)
	require.Len(t, data.Actors[1].Tables["ospf_neighbors"], 1)
}

func TestApplyTopologyOSPFAdjacencyEnrichmentSuppressesMatchingL3SubnetEdge(t *testing.T) {
	l3Link := topologyLink{
		Protocol:   topologyL3SubnetLinkType,
		LinkType:   topologyL3SubnetLinkType,
		SrcActorID: "router-a",
		DstActorID: "router-b",
		Src: topologyLinkEndpoint{
			Attributes: map[string]any{"ip": "198.51.100.1"},
		},
		Dst: topologyLinkEndpoint{
			Attributes: map[string]any{"ip": "198.51.100.2"},
		},
		Metrics: map[string]any{
			"subnet": "198.51.100.0/30",
			"prefix": 30,
		},
	}
	data := topologyData{
		Stats: map[string]any{
			"l3_subnet_emitted_links": 1,
		},
		Actors: []topologyActor{
			topologyOSPFManagedActorForTest("router-a", "device-a", "1.1.1.1", "198.51.100.1"),
			topologyOSPFManagedActorForTest("router-b", "device-b", "2.2.2.2", "198.51.100.2"),
		},
		Links: []topologyLink{l3Link},
	}
	aggregate := topologyObservationAggregate{
		ospfNeighbors: []topologyOSPFNeighbor{
			ospfNeighborForTest("device-a", "1.1.1.1", "2.2.2.2", "198.51.100.2", "full"),
		},
	}

	stats := applyTopologyOSPFAdjacencyEnrichment(&data, aggregate)

	require.Equal(t, 1, stats.emittedLinks)
	require.Equal(t, 1, stats.suppressedL3SubnetOverlap)
	require.Len(t, data.Links, 1)
	require.Equal(t, topologyOSPFAdjacencyLinkType, data.Links[0].LinkType)
	require.Equal(t, 0, data.Stats["l3_subnet_visible_links"])
	require.Equal(t, 1, data.Stats["ospf_adjacency_visible_links"])
}

func TestApplyTopologyOSPFAdjacencyEnrichmentKeepsUnrelatedL3SubnetEdge(t *testing.T) {
	l3Link := topologyLink{
		Protocol:   topologyL3SubnetLinkType,
		LinkType:   topologyL3SubnetLinkType,
		SrcActorID: "router-a",
		DstActorID: "router-b",
		Src: topologyLinkEndpoint{
			Attributes: map[string]any{"ip": "203.0.113.1"},
		},
		Dst: topologyLinkEndpoint{
			Attributes: map[string]any{"ip": "203.0.113.2"},
		},
		Metrics: map[string]any{
			"subnet": "203.0.113.0/30",
			"prefix": 30,
		},
	}
	data := topologyData{
		Stats: map[string]any{
			"l3_subnet_emitted_links": 1,
		},
		Actors: []topologyActor{
			topologyOSPFManagedActorForTest("router-a", "device-a", "1.1.1.1", "198.51.100.1"),
			topologyOSPFManagedActorForTest("router-b", "device-b", "2.2.2.2", "198.51.100.2"),
		},
		Links: []topologyLink{l3Link},
	}
	neighbor := ospfNeighborForTest("device-a", "1.1.1.1", "2.2.2.2", "198.51.100.2", "full")
	neighbor.Network = ""
	neighbor.Netmask = ""
	neighbor.Subnet = ""
	neighbor.Prefix = 0
	aggregate := topologyObservationAggregate{
		ospfNeighbors: []topologyOSPFNeighbor{neighbor},
	}

	stats := applyTopologyOSPFAdjacencyEnrichment(&data, aggregate)

	require.Equal(t, 1, stats.emittedLinks)
	require.Zero(t, stats.suppressedL3SubnetOverlap)
	require.Len(t, data.Links, 2)
	require.Equal(t, 1, countTopologyLinksByType(data.Links, topologyL3SubnetLinkType))
	require.Equal(t, 1, countTopologyLinksByType(data.Links, topologyOSPFAdjacencyLinkType))
	require.Equal(t, 1, data.Stats["l3_subnet_visible_links"])
	require.Equal(t, 1, data.Stats["ospf_adjacency_visible_links"])
}

func TestApplyTopologyOSPFAdjacencyEnrichmentResolvesUnnumberedNeighborByRouterID(t *testing.T) {
	data := topologyData{
		Actors: []topologyActor{
			topologyOSPFManagedActorForTest("router-a", "device-a", "1.1.1.1", "198.51.100.1"),
			topologyOSPFManagedActorForTest("router-b", "device-b", "2.2.2.2", "198.51.100.2"),
		},
	}
	aggregate := topologyObservationAggregate{
		ospfNeighbors: []topologyOSPFNeighbor{
			{
				DeviceID:         "device-a",
				LocalRouterID:    "1.1.1.1",
				NeighborRouterID: "2.2.2.2",
				NeighborIP:       "0.0.0.0",
				AddresslessIndex: "7",
				State:            "full",
			},
		},
	}

	stats := applyTopologyOSPFAdjacencyEnrichment(&data, aggregate)

	require.Equal(t, 1, stats.emittedLinks)
	require.Len(t, data.Links, 1)
	require.Equal(t, topologyOSPFAdjacencyLinkType, data.Links[0].LinkType)
	require.Equal(t, "2.2.2.2", data.Links[0].Dst.Attributes["router_id"])
	require.NotContains(t, data.Links[0].Dst.Attributes, "ip")
	require.NotContains(t, data.Links[0].Metrics, "neighbor_ip")
	require.NotContains(t, data.Actors[0].Tables["ospf_neighbors"][0], "neighbor_ip")
}

func TestApplyTopologyOSPFAdjacencyEnrichmentDeduplicatesBidirectionalUnnumberedObservations(t *testing.T) {
	data := topologyData{
		Actors: []topologyActor{
			topologyOSPFManagedActorForTest("router-a", "device-a", "1.1.1.1", "198.51.100.1"),
			topologyOSPFManagedActorForTest("router-b", "device-b", "2.2.2.2", "198.51.100.2"),
		},
	}
	aggregate := topologyObservationAggregate{
		ospfNeighbors: []topologyOSPFNeighbor{
			{
				DeviceID:         "device-a",
				LocalRouterID:    "1.1.1.1",
				NeighborRouterID: "2.2.2.2",
				NeighborIP:       "0.0.0.0",
				AddresslessIndex: "7",
				State:            "full",
			},
			{
				DeviceID:         "device-b",
				LocalRouterID:    "2.2.2.2",
				NeighborRouterID: "1.1.1.1",
				NeighborIP:       "0.0.0.0",
				AddresslessIndex: "9",
				State:            "full",
			},
		},
	}

	stats := applyTopologyOSPFAdjacencyEnrichment(&data, aggregate)

	require.Equal(t, 1, stats.emittedLinks)
	require.Equal(t, 1, stats.suppressedDuplicateLink)
	require.Len(t, data.Links, 1)
	require.Equal(t, topologyOSPFAdjacencyLinkType, data.Links[0].LinkType)
	require.Len(t, data.Actors[0].Tables["ospf_neighbors"], 1)
	require.Len(t, data.Actors[1].Tables["ospf_neighbors"], 1)
}

func countTopologyLinksByType(links []topologyLink, linkType string) int {
	count := 0
	for _, link := range links {
		if firstNonEmptyString(link.LinkType, link.Protocol) == linkType {
			count++
		}
	}
	return count
}

func topologyOSPFManagedActorForTest(actorID, deviceID, routerID string, ips ...string) topologyActor {
	attrs := map[string]any{
		"device_id":     deviceID,
		tagOSPFRouterID: routerID,
	}
	return topologyL3ManagedActorForTest(actorID, attrs, ips...)
}

func ospfNeighborForTest(deviceID, localRouterID, neighborRouterID, neighborIP, state string) topologyOSPFNeighbor {
	localIP := "198.51.100.1"
	if deviceID == "device-b" {
		localIP = "198.51.100.2"
	}
	return topologyOSPFNeighbor{
		DeviceID:         deviceID,
		LocalRouterID:    localRouterID,
		NeighborRouterID: neighborRouterID,
		NeighborIP:       neighborIP,
		State:            state,
		LocalIP:          localIP,
		Network:          "198.51.100.0",
		Netmask:          "255.255.255.252",
		Subnet:           "198.51.100.0/30",
		Prefix:           30,
	}
}
