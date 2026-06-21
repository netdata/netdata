// SPDX-License-Identifier: GPL-3.0-or-later

package topologyenrich

import (
	"fmt"
	"strings"
	"testing"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyshape"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
	"github.com/stretchr/testify/require"
)

func TestTopologyL3ActorResolverSuppressesAmbiguousIP(t *testing.T) {
	data := topologymodel.Data{
		Actors: []topologymodel.Actor{
			topologyL3ManagedActorForTest("router-a", nil, "198.51.100.1"),
			topologyL3ManagedActorForTest("router-b", nil, "198.51.100.1"),
		},
	}

	resolver := newTopologyL3ActorResolver(&data, nil)
	_, ok := resolver.resolve(topologymodel.L3Interface{
		DeviceID: "unknown-device",
		IP:       "198.51.100.1",
	})

	require.False(t, ok)
}

func TestApplyTopologyL3SubnetEnrichmentSuppressions(t *testing.T) {
	tests := map[string]struct {
		data                     topologymodel.Data
		aggregate                topologymodel.ObservationAggregate
		wantLinks                []topologymodel.Link
		wantUnresolvedActor      int
		wantSelfActor            int
		wantDuplicateLink        int
		wantVisibleLinks         int
		wantSuppressedMetricName string
	}{
		"unresolved-actor": {
			data: topologymodel.Data{
				Actors: []topologymodel.Actor{
					topologyL3ManagedActorForTest("device-a", nil, "198.51.100.1"),
				},
			},
			aggregate: topologymodel.ObservationAggregate{
				L3Interfaces: []topologymodel.L3Interface{
					l3InterfaceForTest("device-a", "198.51.100.1", "255.255.255.252", "2"),
					l3InterfaceForTest("device-b", "198.51.100.2", "255.255.255.252", "3"),
				},
			},
			wantUnresolvedActor:      1,
			wantSuppressedMetricName: "l3_subnet_suppressed_unresolved_actor",
		},
		"self-actor": {
			data: topologymodel.Data{
				Actors: []topologymodel.Actor{
					topologyL3ManagedActorForTest("router-a", nil, "198.51.100.1", "198.51.100.2"),
				},
			},
			aggregate: topologymodel.ObservationAggregate{
				L3Interfaces: []topologymodel.L3Interface{
					l3InterfaceForTest("device-a", "198.51.100.1", "255.255.255.252", "2"),
					l3InterfaceForTest("device-b", "198.51.100.2", "255.255.255.252", "3"),
				},
			},
			wantSelfActor:            1,
			wantSuppressedMetricName: "l3_subnet_suppressed_self_actor",
		},
		"duplicate-link": {
			data: topologymodel.Data{
				Actors: []topologymodel.Actor{
					topologyL3ManagedActorForTest("router-a", nil, "198.51.100.1"),
					topologyL3ManagedActorForTest("router-b", nil, "198.51.100.2"),
				},
				Links: []topologymodel.Link{topologyL3SubnetLinkForTest("router-a", "router-b", "198.51.100.0/30", uint64(30))},
			},
			aggregate: topologymodel.ObservationAggregate{
				L3Interfaces: []topologymodel.L3Interface{
					l3InterfaceForTest("router-a", "198.51.100.1", "255.255.255.252", "2"),
					l3InterfaceForTest("router-b", "198.51.100.2", "255.255.255.252", "3"),
				},
			},
			wantLinks:                []topologymodel.Link{topologyL3SubnetLinkForTest("router-a", "router-b", "198.51.100.0/30", uint64(30))},
			wantDuplicateLink:        1,
			wantVisibleLinks:         1,
			wantSuppressedMetricName: "l3_subnet_suppressed_duplicate_link",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			stats := ApplyL3Subnet(&tc.data, tc.aggregate)

			if tc.wantLinks == nil {
				require.Empty(t, tc.data.Links)
			} else {
				require.Equal(t, tc.wantLinks, tc.data.Links)
			}
			require.Equal(t, tc.wantUnresolvedActor, stats.SuppressedUnresolvedActor)
			require.Equal(t, tc.wantSelfActor, stats.SuppressedSelfActor)
			require.Equal(t, tc.wantDuplicateLink, stats.SuppressedDuplicateLink)
			require.Equal(t, 1, topologyStatsToV1ForTest(t, tc.data.Stats)["l3_subnet_candidate_links"])
			require.Equal(t, 0, topologyStatsToV1ForTest(t, tc.data.Stats)["l3_subnet_emitted_links"])
			require.Equal(t, tc.wantVisibleLinks, topologyStatsToV1ForTest(t, tc.data.Stats)["l3_subnet_visible_links"])
			require.Equal(t, 1, topologyStatsToV1ForTest(t, tc.data.Stats)[tc.wantSuppressedMetricName])
		})
	}
}

func topologyL3SubnetLinkForTest(srcActorID, dstActorID, subnet string, prefix any) topologymodel.Link {
	return topologymodel.Link{
		Protocol:   topologymodel.L3SubnetLinkType,
		LinkType:   topologymodel.L3SubnetLinkType,
		SrcActorID: srcActorID,
		DstActorID: dstActorID,
		Detail: topologymodel.LinkDetail{
			L3Subnet: &topologymodel.L3SubnetLinkDetail{
				Subnet: subnet,
				Prefix: testTopologyIntValue(prefix),
			},
		},
	}
}

func TestTopologyL3SubnetLinkKeySeparatesDelimitedFields(t *testing.T) {
	left := topologymodel.Link{
		SrcActorID: "a|b",
		DstActorID: "c",
		Detail: topologymodel.LinkDetail{
			L3Subnet: &topologymodel.L3SubnetLinkDetail{
				Subnet: "198.51.100.0/30",
				Prefix: 30,
			},
		},
	}
	right := topologymodel.Link{
		SrcActorID: "a",
		DstActorID: "b|c",
		Detail: topologymodel.LinkDetail{
			L3Subnet: &topologymodel.L3SubnetLinkDetail{
				Subnet: "198.51.100.0/30",
				Prefix: 30,
			},
		},
	}

	require.NotEqual(t, topologyL3SubnetLinkKey(left), topologyL3SubnetLinkKey(right))
}

func TestApplyTopologyDepthFocusFilterKeepsIncidentL3SubnetLink(t *testing.T) {
	data := topologymodel.Data{
		Stats: topologymodel.Stats{
			L3:    topologymodel.L3EnrichmentStats{EmittedLinks: 1},
			HasL3: true,
		},
		Actors: []topologymodel.Actor{
			topologyL3ManagedActorForTest("router-a", nil, "198.51.100.1"),
			topologyL3ManagedActorForTest("router-b", nil, "198.51.100.2"),
		},
		Links: []topologymodel.Link{
			{
				Protocol:   topologymodel.L3SubnetLinkType,
				LinkType:   topologymodel.L3SubnetLinkType,
				SrcActorID: "router-a",
				DstActorID: "router-b",
				Detail: topologymodel.LinkDetail{
					L3Subnet: &topologymodel.L3SubnetLinkDetail{
						Subnet: "198.51.100.0/30",
						Prefix: 30,
					},
				},
			},
		},
	}

	topologyshape.ApplyDepthFocusFilter(&data, topologyoptions.QueryOptions{
		ManagedDeviceFocus:     "ip:198.51.100.1",
		Depth:                  1,
		InferenceStrategy:      topologyoptions.InferenceStrategyFDBMinimumKnowledge,
		MapType:                topologyoptions.MapTypeHighConfidenceInferred,
		EliminateNonIPInferred: true,
	})

	require.Len(t, data.Actors, 2)
	require.Len(t, data.Links, 1)
	require.Equal(t, topologymodel.L3SubnetLinkType, data.Links[0].LinkType)
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["l3_subnet_emitted_links"])
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["l3_subnet_visible_links"])
}

func topologyL3ManagedActorForTest(actorID string, attrs map[string]any, ips ...string) topologymodel.Actor {
	detail := topologymodel.ActorDetail{
		L2: topologyengine.ProjectionActorDetail{
			Device: topologyengine.ProjectionDeviceActorDetail{
				DeviceID:     topologyL3TestString(attrs, "device_id"),
				ManagementIP: topologyL3TestString(attrs, "management_ip"),
				Inferred:     testTopologyBoolValue(attrs["inferred"]),
			},
		},
		SNMP: topologymodel.SNMPActorDetail{
			OSPFRouterID: topologyutil.NormalizeTopologyRouterID(topologyL3TestString(attrs, topologymodel.LabelOSPFRouterID)),
		},
	}
	return topologymodel.Actor{
		ActorID:   actorID,
		ActorType: "router",
		Layer:     "network",
		Source:    "snmp",
		Match:     topologymodel.Match{IPAddresses: ips},
		Detail:    detail,
	}
}

func testTopologyBoolValue(value any) bool {
	typed, _ := value.(bool)
	return typed
}

func testTopologyIntValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case uint64:
		return int(typed)
	default:
		return 0
	}
}

func topologyL3TestString(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
