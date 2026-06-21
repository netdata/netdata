// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplyTopologyBGPAdjacencyEnrichmentEmitsEstablishedManagedLink(t *testing.T) {
	data := topologyData{
		Actors: []topologyActor{
			topologyBGPManagedActorForTest("router-a", "device-a", nil, "198.51.100.1"),
			topologyBGPManagedActorForTest("router-b", "device-b", nil, "198.51.100.2"),
		},
	}
	aggregate := topologyObservationAggregate{
		BGPPeers: []topologyBGPPeer{
			bgpPeerForTest("device-a", "default", "198.51.100.1", "198.51.100.2", "65001", "65002", "1.1.1.1", "2.2.2.2", "established"),
		},
	}

	stats := applyTopologyBGPAdjacencyEnrichment(&data, aggregate)

	require.Equal(t, 1, stats.EmittedLinks)
	require.Len(t, data.Links, 1)
	link := data.Links[0]
	require.Equal(t, topologyBGPAdjacencyLinkType, link.LinkType)
	require.Equal(t, "established", link.State)
	require.Equal(t, "65001", topologyBGPLocalAS(link))
	require.Equal(t, "65002", topologyBGPRemoteAS(link))
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["bgp_peer_rows"])
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["bgp_peer_detail_rows"])
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["bgp_adjacency_emitted_links"])
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["bgp_adjacency_visible_links"])
	require.Len(t, data.Actors[0].Detail.BGP, 1)
	require.Equal(t, "router-b", data.Actors[0].Detail.BGP[0].RemoteActorID)
}

func TestApplyTopologyBGPAdjacencyEnrichmentKeepsSuppressedPeersAsDetailOnly(t *testing.T) {
	tests := map[string]struct {
		data                       topologyData
		aggregate                  topologyObservationAggregate
		wantNonEstablished         int
		wantUnresolvedNeighbor     int
		wantSelfActor              int
		wantDetailState            string
		wantRemoteActorID          string
		wantRemoteActorIDAbsent    bool
		wantSuppressedStatsCounter string
	}{
		"non-established-peer": {
			data: topologyData{
				Actors: []topologyActor{
					topologyBGPManagedActorForTest("router-a", "device-a", nil, "198.51.100.1"),
					topologyBGPManagedActorForTest("router-b", "device-b", nil, "198.51.100.2"),
				},
			},
			aggregate: topologyObservationAggregate{
				BGPPeers: []topologyBGPPeer{
					bgpPeerForTest("device-a", "default", "198.51.100.1", "198.51.100.2", "65001", "65002", "1.1.1.1", "2.2.2.2", "active"),
				},
			},
			wantNonEstablished: 1,
			wantDetailState:    "active",
			wantRemoteActorID:  "router-b",
		},
		"unresolved-neighbor": {
			data: topologyData{
				Actors: []topologyActor{
					topologyBGPManagedActorForTest("router-a", "device-a", nil, "198.51.100.1"),
				},
			},
			aggregate: topologyObservationAggregate{
				BGPPeers: []topologyBGPPeer{
					bgpPeerForTest("device-a", "default", "198.51.100.1", "203.0.113.2", "65001", "65002", "1.1.1.1", "2.2.2.2", "established"),
				},
			},
			wantUnresolvedNeighbor:  1,
			wantRemoteActorIDAbsent: true,
		},
		"self-actor": {
			data: topologyData{
				Actors: []topologyActor{
					topologyBGPManagedActorForTest("router-a", "device-a", nil, "198.51.100.1", "198.51.100.2"),
				},
			},
			aggregate: topologyObservationAggregate{
				BGPPeers: []topologyBGPPeer{
					bgpPeerForTest("device-a", "default", "198.51.100.1", "198.51.100.2", "65001", "65002", "1.1.1.1", "2.2.2.2", "established"),
				},
			},
			wantSelfActor:              1,
			wantRemoteActorID:          "router-a",
			wantSuppressedStatsCounter: "bgp_adjacency_suppressed_self_actor",
		},
		"ambiguous-peer-identifier": {
			data: topologyData{
				Actors: []topologyActor{
					topologyBGPManagedActorForTest("router-a", "device-a", nil, "198.51.100.1"),
					topologyBGPManagedActorForTest("router-b", "device-b", nil, "198.51.100.2"),
					topologyBGPManagedActorForTest("router-c", "device-c", nil, "198.51.100.3"),
				},
			},
			aggregate: topologyObservationAggregate{
				BGPPeers: []topologyBGPPeer{
					bgpPeerForTest("device-b", "default", "198.51.100.2", "203.0.113.1", "65002", "65001", "2.2.2.2", "", "idle"),
					bgpPeerForTest("device-c", "default", "198.51.100.3", "203.0.113.1", "65002", "65001", "2.2.2.2", "", "idle"),
					bgpPeerForTest("device-a", "default", "198.51.100.1", "", "65001", "65002", "1.1.1.1", "2.2.2.2", "established"),
				},
			},
			wantNonEstablished:      2,
			wantUnresolvedNeighbor:  1,
			wantRemoteActorIDAbsent: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			stats := applyTopologyBGPAdjacencyEnrichment(&tc.data, tc.aggregate)

			require.Zero(t, stats.EmittedLinks)
			require.Equal(t, tc.wantNonEstablished, stats.SuppressedNonEstablished)
			require.Equal(t, tc.wantUnresolvedNeighbor, stats.SuppressedUnresolvedNeighbor)
			require.Equal(t, tc.wantSelfActor, stats.SuppressedSelfActor)
			require.Empty(t, tc.data.Links)
			require.Len(t, tc.data.Actors[0].Detail.BGP, 1)
			row := tc.data.Actors[0].Detail.BGP[0]
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

func TestApplyTopologyBGPAdjacencyEnrichmentBuildsManagedLinks(t *testing.T) {
	tests := map[string]struct {
		data      topologyData
		aggregate topologyObservationAggregate
		validate  func(t *testing.T, data topologyData, stats topologyBGPEnrichmentStats)
	}{
		"deduplicates-asymmetric-identity-observations": {
			data: topologyData{
				Actors: []topologyActor{
					topologyBGPManagedActorForTest("router-a", "device-a", nil, "198.51.100.1"),
					topologyBGPManagedActorForTest("router-b", "device-b", nil, "198.51.100.2"),
				},
			},
			aggregate: topologyObservationAggregate{
				BGPPeers: []topologyBGPPeer{
					bgpPeerForTest("device-a", "default", "198.51.100.1", "198.51.100.2", "65001", "65002", "1.1.1.1", "2.2.2.2", "established"),
					bgpPeerForTest("device-b", "default", "198.51.100.2", "198.51.100.1", "65002", "65001", "", "", "established"),
				},
			},
			validate: func(t *testing.T, data topologyData, stats topologyBGPEnrichmentStats) {
				t.Helper()

				require.Equal(t, 1, stats.EmittedLinks)
				require.Equal(t, 1, stats.SuppressedDuplicateLink)
				require.Len(t, data.Links, 1)
				require.Len(t, data.Actors[0].Detail.BGP, 1)
				require.Len(t, data.Actors[1].Detail.BGP, 1)
				require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["bgp_adjacency_visible_links"])
			},
		},
		"deduplicates-asymmetric-local-ip-observations": {
			data: topologyData{
				Actors: []topologyActor{
					topologyBGPManagedActorForTest("router-a", "device-a", nil, "198.51.100.1"),
					topologyBGPManagedActorForTest("router-b", "device-b", nil, "198.51.100.2"),
				},
			},
			aggregate: topologyObservationAggregate{
				BGPPeers: []topologyBGPPeer{
					bgpPeerForTest("device-a", "default", "", "198.51.100.2", "65001", "65002", "1.1.1.1", "2.2.2.2", "established"),
					bgpPeerForTest("device-b", "default", "198.51.100.2", "198.51.100.1", "65002", "65001", "2.2.2.2", "1.1.1.1", "established"),
				},
			},
			validate: func(t *testing.T, data topologyData, stats topologyBGPEnrichmentStats) {
				t.Helper()

				require.Equal(t, 1, stats.EmittedLinks)
				require.Equal(t, 1, stats.SuppressedDuplicateLink)
				require.Len(t, data.Links, 1)
				require.Len(t, data.Actors[0].Detail.BGP, 1)
				require.Len(t, data.Actors[1].Detail.BGP, 1)
				require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["bgp_adjacency_visible_links"])
			},
		},
		"keeps-routing-instances-separate": {
			data: topologyData{
				Actors: []topologyActor{
					topologyBGPManagedActorForTest("router-a", "device-a", nil, "198.51.100.1"),
					topologyBGPManagedActorForTest("router-b", "device-b", nil, "198.51.100.2"),
				},
			},
			aggregate: topologyObservationAggregate{
				BGPPeers: []topologyBGPPeer{
					bgpPeerForTest("device-a", "blue", "198.51.100.1", "198.51.100.2", "65001", "65002", "1.1.1.1", "2.2.2.2", "established"),
					bgpPeerForTest("device-a", "red", "198.51.100.1", "198.51.100.2", "65001", "65002", "1.1.1.1", "2.2.2.2", "established"),
				},
			},
			validate: func(t *testing.T, data topologyData, stats topologyBGPEnrichmentStats) {
				t.Helper()

				require.Equal(t, 2, stats.EmittedLinks)
				require.Len(t, data.Links, 2)
				require.Equal(t, 2, topologyStatsToV1ForTest(t, data.Stats)["bgp_adjacency_visible_links"])
				require.ElementsMatch(t, []string{"blue", "red"}, []string{
					topologyBGPLinkRoutingInstance(data.Links[0]),
					topologyBGPLinkRoutingInstance(data.Links[1]),
				})
			},
		},
		"compacts-parallel-same-routing-instance-peers": {
			data: topologyData{
				Actors: []topologyActor{
					topologyBGPManagedActorForTest("router-a", "device-a", nil, "198.51.100.1", "198.51.100.5"),
					topologyBGPManagedActorForTest("router-b", "device-b", nil, "198.51.100.2", "198.51.100.6"),
				},
			},
			aggregate: topologyObservationAggregate{
				BGPPeers: []topologyBGPPeer{
					bgpPeerForTest("device-a", "default", "198.51.100.1", "198.51.100.2", "65001", "65002", "1.1.1.1", "2.2.2.2", "established"),
					bgpPeerForTest("device-a", "default", "198.51.100.5", "198.51.100.6", "65001", "65002", "1.1.1.1", "2.2.2.2", "established"),
				},
			},
			validate: func(t *testing.T, data topologyData, stats topologyBGPEnrichmentStats) {
				t.Helper()

				require.Equal(t, 1, stats.EmittedLinks)
				require.Equal(t, 1, stats.SuppressedDuplicateLink)
				require.Len(t, data.Links, 1)
				require.Len(t, data.Actors[0].Detail.BGP, 2)
				require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["bgp_adjacency_visible_links"])
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			stats := applyTopologyBGPAdjacencyEnrichment(&tc.data, tc.aggregate)

			tc.validate(t, tc.data, stats)
		})
	}
}

func TestApplyTopologyBGPAdjacencyEnrichmentCanonicalizesUndirectedLinkEndpoints(t *testing.T) {
	data := topologyData{
		Actors: []topologyActor{
			topologyBGPManagedActorForTest("router-a", "device-a", nil, "198.51.100.1"),
			topologyBGPManagedActorForTest("router-b", "device-b", nil, "198.51.100.2"),
		},
	}
	aggregate := topologyObservationAggregate{
		BGPPeers: []topologyBGPPeer{
			bgpPeerForTest("device-b", "default", "198.51.100.2", "198.51.100.1", "65002", "65001", "2.2.2.2", "1.1.1.1", "established"),
		},
	}

	stats := applyTopologyBGPAdjacencyEnrichment(&data, aggregate)

	require.Equal(t, 1, stats.EmittedLinks)
	require.Len(t, data.Links, 1)
	link := data.Links[0]
	require.Equal(t, "router-a", link.SrcActorID)
	require.Equal(t, "router-b", link.DstActorID)
	require.Equal(t, "1.1.1.1", topologyBGPLocalIdentifier(link))
	require.Equal(t, "2.2.2.2", topologyBGPPeerIdentifier(link))
	require.Equal(t, "198.51.100.1", topologyBGPLocalIP(link))
	require.Equal(t, "198.51.100.2", topologyBGPNeighborIP(link))
	require.Equal(t, "65001", topologyBGPLocalAS(link))
	require.Equal(t, "65002", topologyBGPRemoteAS(link))
}

func TestApplyTopologyBGPAdjacencyEnrichmentResolvesPeerIdentifierByRouterID(t *testing.T) {
	data := topologyData{
		Actors: []topologyActor{
			topologyBGPManagedActorForTest("router-a", "device-a", nil, "198.51.100.1"),
			topologyBGPManagedActorForTest("router-b", "device-b", map[string]any{tagOSPFRouterID: "2.2.2.2"}, "198.51.100.2"),
		},
	}
	aggregate := topologyObservationAggregate{
		BGPPeers: []topologyBGPPeer{
			bgpPeerForTest("device-a", "default", "198.51.100.1", "", "65001", "65002", "1.1.1.1", "2.2.2.2", "established"),
		},
	}

	stats := applyTopologyBGPAdjacencyEnrichment(&data, aggregate)

	require.Equal(t, 1, stats.EmittedLinks)
	require.Len(t, data.Links, 1)
	require.Equal(t, "router-b", data.Links[0].DstActorID)
	require.Equal(t, "2.2.2.2", topologyBGPPeerIdentifier(data.Links[0]))
	require.Len(t, data.Actors[0].Detail.BGP, 1)
	require.Equal(t, "router-b", data.Actors[0].Detail.BGP[0].RemoteActorID)
}

func topologyBGPManagedActorForTest(actorID, deviceID string, attrs map[string]any, ips ...string) topologyActor {
	if attrs == nil {
		attrs = make(map[string]any)
	}
	attrs["device_id"] = deviceID
	return topologyL3ManagedActorForTest(actorID, attrs, ips...)
}

func bgpPeerForTest(deviceID, routingInstance, localIP, neighborIP, localAS, remoteAS, localIdentifier, peerIdentifier, state string) topologyBGPPeer {
	return topologyBGPPeer{
		DeviceID:        deviceID,
		RoutingInstance: routingInstance,
		LocalIP:         localIP,
		NeighborIP:      neighborIP,
		LocalAS:         localAS,
		RemoteAS:        remoteAS,
		LocalIdentifier: localIdentifier,
		PeerIdentifier:  peerIdentifier,
		State:           state,
	}
}
