// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/stretchr/testify/require"
)

func TestTopologyL3ActorResolverSuppressesAmbiguousIP(t *testing.T) {
	data := topologyData{
		Actors: []topologyActor{
			topologyL3ManagedActorForTest("router-a", nil, "198.51.100.1"),
			topologyL3ManagedActorForTest("router-b", nil, "198.51.100.1"),
		},
	}

	resolver := newTopologyL3ActorResolver(&data, nil)
	_, ok := resolver.resolve(topologyL3Interface{
		DeviceID: "unknown-device",
		IP:       "198.51.100.1",
	})

	require.False(t, ok)
}

func TestApplyTopologyL3SubnetEnrichmentSuppressions(t *testing.T) {
	tests := map[string]struct {
		data                     topologyData
		aggregate                topologyObservationAggregate
		wantLinks                []topologyLink
		wantUnresolvedActor      int
		wantSelfActor            int
		wantDuplicateLink        int
		wantVisibleLinks         int
		wantSuppressedMetricName string
	}{
		"unresolved-actor": {
			data: topologyData{
				Actors: []topologyActor{
					topologyL3ManagedActorForTest("device-a", nil, "198.51.100.1"),
				},
			},
			aggregate: topologyObservationAggregate{
				l3Interfaces: []topologyL3Interface{
					l3InterfaceForTest("device-a", "198.51.100.1", "255.255.255.252", "2"),
					l3InterfaceForTest("device-b", "198.51.100.2", "255.255.255.252", "3"),
				},
			},
			wantUnresolvedActor:      1,
			wantSuppressedMetricName: "l3_subnet_suppressed_unresolved_actor",
		},
		"self-actor": {
			data: topologyData{
				Actors: []topologyActor{
					topologyL3ManagedActorForTest("router-a", nil, "198.51.100.1", "198.51.100.2"),
				},
			},
			aggregate: topologyObservationAggregate{
				l3Interfaces: []topologyL3Interface{
					l3InterfaceForTest("device-a", "198.51.100.1", "255.255.255.252", "2"),
					l3InterfaceForTest("device-b", "198.51.100.2", "255.255.255.252", "3"),
				},
			},
			wantSelfActor:            1,
			wantSuppressedMetricName: "l3_subnet_suppressed_self_actor",
		},
		"duplicate-link": {
			data: topologyData{
				Actors: []topologyActor{
					topologyL3ManagedActorForTest("router-a", nil, "198.51.100.1"),
					topologyL3ManagedActorForTest("router-b", nil, "198.51.100.2"),
				},
				Links: []topologyLink{topologyL3SubnetLinkForTest("router-a", "router-b", "198.51.100.0/30", uint64(30))},
			},
			aggregate: topologyObservationAggregate{
				l3Interfaces: []topologyL3Interface{
					l3InterfaceForTest("router-a", "198.51.100.1", "255.255.255.252", "2"),
					l3InterfaceForTest("router-b", "198.51.100.2", "255.255.255.252", "3"),
				},
			},
			wantLinks:                []topologyLink{topologyL3SubnetLinkForTest("router-a", "router-b", "198.51.100.0/30", uint64(30))},
			wantDuplicateLink:        1,
			wantVisibleLinks:         1,
			wantSuppressedMetricName: "l3_subnet_suppressed_duplicate_link",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			stats := applyTopologyL3SubnetEnrichment(&tc.data, tc.aggregate)

			if tc.wantLinks == nil {
				require.Empty(t, tc.data.Links)
			} else {
				require.Equal(t, tc.wantLinks, tc.data.Links)
			}
			require.Equal(t, tc.wantUnresolvedActor, stats.suppressedUnresolvedActor)
			require.Equal(t, tc.wantSelfActor, stats.suppressedSelfActor)
			require.Equal(t, tc.wantDuplicateLink, stats.suppressedDuplicateLink)
			require.Equal(t, 1, topologyStatsToV1(tc.data.Stats)["l3_subnet_candidate_links"])
			require.Equal(t, 0, topologyStatsToV1(tc.data.Stats)["l3_subnet_emitted_links"])
			require.Equal(t, tc.wantVisibleLinks, topologyStatsToV1(tc.data.Stats)["l3_subnet_visible_links"])
			require.Equal(t, 1, topologyStatsToV1(tc.data.Stats)[tc.wantSuppressedMetricName])
		})
	}
}

func topologyL3SubnetLinkForTest(srcActorID, dstActorID, subnet string, prefix any) topologyLink {
	return topologyLink{
		Protocol:   topologyL3SubnetLinkType,
		LinkType:   topologyL3SubnetLinkType,
		SrcActorID: srcActorID,
		DstActorID: dstActorID,
		Detail: topologyLinkDetail{
			L3Subnet: &topologyL3SubnetLinkDetail{
				Subnet: subnet,
				Prefix: intStatValue(prefix),
			},
		},
	}
}

func TestTopologyL3SubnetLinkKeySeparatesDelimitedFields(t *testing.T) {
	left := topologyLink{
		SrcActorID: "a|b",
		DstActorID: "c",
		Detail: topologyLinkDetail{
			L3Subnet: &topologyL3SubnetLinkDetail{
				Subnet: "198.51.100.0/30",
				Prefix: 30,
			},
		},
	}
	right := topologyLink{
		SrcActorID: "a",
		DstActorID: "b|c",
		Detail: topologyLinkDetail{
			L3Subnet: &topologyL3SubnetLinkDetail{
				Subnet: "198.51.100.0/30",
				Prefix: 30,
			},
		},
	}

	require.NotEqual(t, topologyL3SubnetLinkKey(left), topologyL3SubnetLinkKey(right))
}

func TestApplyTopologyDepthFocusFilterKeepsIncidentL3SubnetLink(t *testing.T) {
	data := topologyData{
		Stats: topologyStats{
			L3:    topologyL3EnrichmentStats{emittedLinks: 1},
			HasL3: true,
		},
		Actors: []topologyActor{
			topologyL3ManagedActorForTest("router-a", nil, "198.51.100.1"),
			topologyL3ManagedActorForTest("router-b", nil, "198.51.100.2"),
		},
		Links: []topologyLink{
			{
				Protocol:   topologyL3SubnetLinkType,
				LinkType:   topologyL3SubnetLinkType,
				SrcActorID: "router-a",
				DstActorID: "router-b",
				Detail: topologyLinkDetail{
					L3Subnet: &topologyL3SubnetLinkDetail{
						Subnet: "198.51.100.0/30",
						Prefix: 30,
					},
				},
			},
		},
	}

	applyTopologyDepthFocusFilter(&data, topologyQueryOptions{
		ManagedDeviceFocus:     "ip:198.51.100.1",
		Depth:                  1,
		InferenceStrategy:      topologyInferenceStrategyFDBMinimumKnowledge,
		MapType:                topologyMapTypeHighConfidenceInferred,
		EliminateNonIPInferred: true,
	})

	require.Len(t, data.Actors, 2)
	require.Len(t, data.Links, 1)
	require.Equal(t, topologyL3SubnetLinkType, data.Links[0].LinkType)
	require.Equal(t, 1, topologyStatsToV1(data.Stats)["l3_subnet_emitted_links"])
	require.Equal(t, 1, topologyStatsToV1(data.Stats)["l3_subnet_visible_links"])
}

func topologyL3ManagedActorForTest(actorID string, attrs map[string]any, ips ...string) topologyActor {
	detail := topologyActorDetail{
		L2: topologyengine.ProjectionActorDetail{
			Device: topologyengine.ProjectionDeviceActorDetail{
				DeviceID:     topologyMetricValueString(attrs, "device_id"),
				ManagementIP: topologyMetricValueString(attrs, "management_ip"),
				Inferred:     boolStatValue(attrs["inferred"]),
			},
		},
		SNMP: topologySNMPActorDetail{
			OSPFRouterID: normalizeTopologyRouterID(anyStringValue(attrs[tagOSPFRouterID])),
		},
	}
	return topologyActor{
		ActorID:   actorID,
		ActorType: "router",
		Layer:     "network",
		Source:    "snmp",
		Match:     topologyMatch{IPAddresses: ips},
		Detail:    detail,
	}
}
