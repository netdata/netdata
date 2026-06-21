// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
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
		OSPFNeighbors: []topologyOSPFNeighbor{
			ospfNeighborForTest("device-a", "1.1.1.1", "2.2.2.2", "198.51.100.2", "full"),
		},
	}

	stats := applyTopologyOSPFAdjacencyEnrichment(&data, aggregate)

	require.Equal(t, 1, stats.EmittedLinks)
	require.Len(t, data.Links, 1)
	link := data.Links[0]
	require.Equal(t, topologyOSPFAdjacencyLinkType, link.LinkType)
	require.Equal(t, "full", link.State)
	require.Equal(t, "1.1.1.1", topologyOSPFLocalRouterID(link))
	require.Equal(t, "2.2.2.2", topologyOSPFNeighborRouterID(link))
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["ospf_neighbor_rows"])
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["ospf_neighbor_detail_rows"])
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["ospf_adjacency_emitted_links"])
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["ospf_adjacency_visible_links"])
	require.Len(t, data.Actors[0].Detail.OSPF, 1)
	require.Equal(t, "router-b", data.Actors[0].Detail.OSPF[0].RemoteActorID)
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
				OSPFNeighbors: []topologyOSPFNeighbor{
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
				OSPFNeighbors: []topologyOSPFNeighbor{
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
				OSPFNeighbors: []topologyOSPFNeighbor{
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
				OSPFNeighbors: []topologyOSPFNeighbor{
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

			require.Zero(t, stats.EmittedLinks)
			require.Equal(t, tc.wantNonFullState, stats.SuppressedNonFullState)
			require.Equal(t, tc.wantUnresolvedNeighbor, stats.SuppressedUnresolvedNeighbor)
			require.Equal(t, tc.wantSelfActor, stats.SuppressedSelfActor)
			require.Empty(t, tc.data.Links)
			require.Len(t, tc.data.Actors[0].Detail.OSPF, 1)
			row := tc.data.Actors[0].Detail.OSPF[0]
			if tc.wantDetailState != "" {
				require.Equal(t, tc.wantDetailState, row.State)
			}
			if tc.wantRemoteActorID != "" {
				require.Equal(t, tc.wantRemoteActorID, row.RemoteActorID)
			}
			if tc.wantRemoteActorIDAbsent {
				require.Empty(t, row.RemoteActorID)
			}
			if tc.wantSuppressedStatsCounter != "" {
				require.Equal(t, 1, topologyStatsToV1ForTest(t, tc.data.Stats)[tc.wantSuppressedStatsCounter])
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
		OSPFNeighbors: []topologyOSPFNeighbor{
			ospfNeighborForTest("device-a", "1.1.1.1", "2.2.2.2", "198.51.100.2", "full"),
			ospfNeighborForTest("device-b", "2.2.2.2", "1.1.1.1", "198.51.100.1", "full"),
		},
	}

	stats := applyTopologyOSPFAdjacencyEnrichment(&data, aggregate)

	require.Equal(t, 1, stats.EmittedLinks)
	require.Equal(t, 1, stats.SuppressedDuplicateLink)
	require.Len(t, data.Links, 1)
	require.Len(t, data.Actors[0].Detail.OSPF, 1)
	require.Len(t, data.Actors[1].Detail.OSPF, 1)
}

func TestApplyTopologyOSPFAdjacencyEnrichmentKeepsMatchingL3SubnetEdge(t *testing.T) {
	l3Link := topologyLink{
		Protocol:   topologyL3SubnetLinkType,
		LinkType:   topologyL3SubnetLinkType,
		SrcActorID: "router-a",
		DstActorID: "router-b",
		Src: topologyLinkEndpoint{
			Match: topologyMatch{IPAddresses: []string{"198.51.100.1"}},
		},
		Dst: topologyLinkEndpoint{
			Match: topologyMatch{IPAddresses: []string{"198.51.100.2"}},
		},
		Detail: topologyLinkDetail{
			L3Subnet: &topologyL3SubnetLinkDetail{
				Subnet: "198.51.100.0/30",
				Prefix: 30,
			},
		},
	}
	data := topologyData{
		Stats: topologyStats{
			L3:    topologyL3EnrichmentStats{EmittedLinks: 1},
			HasL3: true,
		},
		Actors: []topologyActor{
			topologyOSPFManagedActorForTest("router-a", "device-a", "1.1.1.1", "198.51.100.1"),
			topologyOSPFManagedActorForTest("router-b", "device-b", "2.2.2.2", "198.51.100.2"),
		},
		Links: []topologyLink{l3Link},
	}
	aggregate := topologyObservationAggregate{
		OSPFNeighbors: []topologyOSPFNeighbor{
			ospfNeighborForTest("device-a", "1.1.1.1", "2.2.2.2", "198.51.100.2", "full"),
		},
	}

	stats := applyTopologyOSPFAdjacencyEnrichment(&data, aggregate)

	require.Equal(t, 1, stats.EmittedLinks)
	require.Len(t, data.Links, 2)
	require.Equal(t, 1, countTopologyLinksByType(data.Links, topologyL3SubnetLinkType))
	require.Equal(t, 1, countTopologyLinksByType(data.Links, topologyOSPFAdjacencyLinkType))
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["l3_subnet_visible_links"])
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["ospf_adjacency_visible_links"])
}

func TestApplyTopologyOSPFAdjacencyEnrichmentKeepsUnrelatedL3SubnetEdge(t *testing.T) {
	l3Link := topologyLink{
		Protocol:   topologyL3SubnetLinkType,
		LinkType:   topologyL3SubnetLinkType,
		SrcActorID: "router-a",
		DstActorID: "router-b",
		Src: topologyLinkEndpoint{
			Match: topologyMatch{IPAddresses: []string{"203.0.113.1"}},
		},
		Dst: topologyLinkEndpoint{
			Match: topologyMatch{IPAddresses: []string{"203.0.113.2"}},
		},
		Detail: topologyLinkDetail{
			L3Subnet: &topologyL3SubnetLinkDetail{
				Subnet: "203.0.113.0/30",
				Prefix: 30,
			},
		},
	}
	data := topologyData{
		Stats: topologyStats{
			L3:    topologyL3EnrichmentStats{EmittedLinks: 1},
			HasL3: true,
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
		OSPFNeighbors: []topologyOSPFNeighbor{neighbor},
	}

	stats := applyTopologyOSPFAdjacencyEnrichment(&data, aggregate)

	require.Equal(t, 1, stats.EmittedLinks)
	require.Len(t, data.Links, 2)
	require.Equal(t, 1, countTopologyLinksByType(data.Links, topologyL3SubnetLinkType))
	require.Equal(t, 1, countTopologyLinksByType(data.Links, topologyOSPFAdjacencyLinkType))
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["l3_subnet_visible_links"])
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["ospf_adjacency_visible_links"])
}

func TestApplyTopologyOSPFAdjacencyEnrichmentResolvesUnnumberedNeighborByRouterID(t *testing.T) {
	data := topologyData{
		Actors: []topologyActor{
			topologyOSPFManagedActorForTest("router-a", "device-a", "1.1.1.1", "198.51.100.1"),
			topologyOSPFManagedActorForTest("router-b", "device-b", "2.2.2.2", "198.51.100.2"),
		},
	}
	aggregate := topologyObservationAggregate{
		OSPFNeighbors: []topologyOSPFNeighbor{
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

	require.Equal(t, 1, stats.EmittedLinks)
	require.Len(t, data.Links, 1)
	require.Equal(t, topologyOSPFAdjacencyLinkType, data.Links[0].LinkType)
	require.Equal(t, "2.2.2.2", topologyOSPFNeighborRouterID(data.Links[0]))
	require.Empty(t, topologyOSPFNeighborIP(data.Links[0]))
	require.Empty(t, data.Actors[0].Detail.OSPF[0].NeighborIP)
}

func TestApplyTopologyOSPFAdjacencyEnrichmentDeduplicatesBidirectionalUnnumberedObservations(t *testing.T) {
	data := topologyData{
		Actors: []topologyActor{
			topologyOSPFManagedActorForTest("router-a", "device-a", "1.1.1.1", "198.51.100.1"),
			topologyOSPFManagedActorForTest("router-b", "device-b", "2.2.2.2", "198.51.100.2"),
		},
	}
	aggregate := topologyObservationAggregate{
		OSPFNeighbors: []topologyOSPFNeighbor{
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

	require.Equal(t, 1, stats.EmittedLinks)
	require.Equal(t, 1, stats.SuppressedDuplicateLink)
	require.Len(t, data.Links, 1)
	require.Equal(t, topologyOSPFAdjacencyLinkType, data.Links[0].LinkType)
	require.Len(t, data.Actors[0].Detail.OSPF, 1)
	require.Len(t, data.Actors[1].Detail.OSPF, 1)
}

func countTopologyLinksByType(links []topologyLink, linkType string) int {
	count := 0
	for _, link := range links {
		if topologyutil.FirstNonEmptyString(link.LinkType, link.Protocol) == linkType {
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
