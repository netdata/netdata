// SPDX-License-Identifier: GPL-3.0-or-later

package topologyenrich

import (
	"fmt"
	"strings"
	"testing"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
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
	assignTopologyEnrichTestHandles(t, &data)

	resolver := newTopologyL3ActorResolver(&data, nil)
	_, ok := resolver.resolve(topologymodel.L3Interface{
		DeviceID: "unknown-device",
		IP:       "198.51.100.1",
	})

	require.False(t, ok)
}

func TestTopologyL3ActorResolverSuppressesAmbiguousSnapshotIdentity(t *testing.T) {
	data := topologymodel.Data{
		Actors: []topologymodel.Actor{
			topologyL3ManagedActorForTest("router-a", nil, "198.51.100.1"),
			topologyL3ManagedActorForTest("router-b", nil, "198.51.100.2"),
		},
	}
	data.Actors[0].Match.SysName = "shared-router"
	data.Actors[1].Match.SysName = "shared-router"
	snapshots := []topologymodel.ObservationSnapshot{{
		LocalDeviceID: "shared-device-id",
		LocalDevice: topologymodel.Device{
			SysName:      "shared-router",
			OSPFRouterID: "192.0.2.100",
		},
	}}
	assignTopologyEnrichTestHandles(t, &data)

	resolver := newTopologyL3ActorResolver(&data, snapshots)
	_, deviceOK := resolver.resolveDeviceID("shared-device-id")
	_, routerOK := resolver.resolveRouterID("192.0.2.100")

	require.False(t, deviceOK)
	require.False(t, routerOK)
}

func TestTopologyL3ActorResolverIndexesOnlyManagedSNMPDevices(t *testing.T) {
	managed := topologyL3ManagedActorForTest("managed", nil, "198.51.100.1")
	nonSNMP := topologyL3ManagedActorForTest("non-snmp", nil, "198.51.100.1")
	nonSNMP.Source = "lldp"
	inferred := topologyL3ManagedActorForTest("inferred", map[string]any{"inferred": true}, "198.51.100.1")
	endpoint := topologyL3ManagedActorForTest("endpoint", nil, "198.51.100.1")
	endpoint.ActorType = "endpoint"
	data := topologymodel.Data{Actors: []topologymodel.Actor{nonSNMP, inferred, endpoint, managed}}
	snapshots := []topologymodel.ObservationSnapshot{{
		LocalDeviceID: "polled-device",
		LocalDevice:   topologymodel.Device{ManagementIP: "198.51.100.1"},
	}}
	assignTopologyEnrichTestHandles(t, &data)

	resolver := newTopologyL3ActorResolver(&data, snapshots)
	ref, ok := resolver.resolveDeviceID("polled-device")

	require.True(t, ok)
	require.Equal(t, "managed", ref.actorID)
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
			assignTopologyEnrichTestHandles(t, &tc.data)
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

func TestApplyTopologyL3SubnetEnrichmentEmitsSegmentMemberships(t *testing.T) {
	data := topologymodel.Data{
		Actors: []topologymodel.Actor{
			topologyL3ManagedActorForTest("router-a", nil, "203.0.113.1"),
			topologyL3ManagedActorForTest("router-b", nil, "203.0.113.2"),
			topologyL3ManagedActorForTest("router-c", nil, "203.0.113.3"),
		},
	}
	aggregate := topologymodel.ObservationAggregate{
		ProducerScopeID: "producer-a",
		L3Interfaces: []topologymodel.L3Interface{
			{DeviceID: "router-c", IP: "203.0.113.3", Netmask: "255.255.255.0", IfIndex: "3", IfName: "xe-0/0/3", IfDescr: "transit-c"},
			{DeviceID: "router-a", IP: "203.0.113.1", Netmask: "255.255.255.0", IfIndex: "1", IfName: "xe-0/0/1", IfDescr: "transit-a"},
			{DeviceID: "router-b", IP: "203.0.113.2", Netmask: "255.255.255.0", IfIndex: "2", IfName: "xe-0/0/2", IfDescr: "transit-b"},
			{DeviceID: "unmanaged", IP: "203.0.113.4", Netmask: "255.255.255.0", IfIndex: "4", IfName: "xe-0/0/4", IfDescr: "unmanaged"},
		},
	}
	handles := assignTopologyEnrichTestHandles(t, &data)

	stats := ApplyL3Subnet(&data, aggregate)

	require.Equal(t, 1, stats.EmittedSegments)
	require.Equal(t, 3, stats.EmittedMembershipLinks)
	require.Equal(t, 0, stats.EmittedLinks)
	require.Equal(t, 1, stats.SubnetStats.CandidateSegments)
	require.Equal(t, 4, stats.SubnetStats.CandidateMemberships)
	require.Equal(t, 1, stats.SuppressedMembershipUnresolvedActor)
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["l3_subnet_segment_emitted_segments"])
	require.Equal(t, 3, topologyStatsToV1ForTest(t, data.Stats)["l3_subnet_membership_visible_links"])
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["l3_subnet_membership_suppressed_unresolved_actor"])

	segments := topologyActorsByTypeForTest(data.Actors, topologymodel.L3SubnetSegmentActorType)
	require.Len(t, segments, 1)
	require.Equal(t, topologymodel.SegmentKindL3Subnet, segments[0].SegmentKind)
	require.Equal(t, "203.0.113.0/24", topologymodel.ActorDetailDisplayName(segments[0]))
	require.Equal(t, topologyengine.OptionalValue[int]{Value: 3, Has: true}, segments[0].Detail.L2.Segment.PortsTotal)
	require.Equal(t, topologyengine.OptionalValue[int]{Value: 3, Has: true}, segments[0].Detail.L2.Segment.EndpointsTotal)

	memberships := topologyLinksByTypeForTest(data.Links, topologymodel.L3SubnetMembershipLinkType)
	require.Len(t, memberships, 3)
	require.Equal(t, handles["router-a"], memberships[0].SrcActorHandle)
	require.Equal(t, segments[0].ActorHandle, memberships[0].DstActorHandle)
	require.NotNil(t, memberships[0].Detail.L3SubnetMembership)
	require.Equal(t, "203.0.113.0/24", memberships[0].Detail.L3SubnetMembership.Subnet)
	require.Equal(t, []topologymodel.L3SubnetMembershipInterface{
		{MemberIP: "203.0.113.1", IfIndex: 1, IfName: "xe-0/0/1", IfDescr: "transit-a"},
	}, memberships[0].Detail.L3SubnetMembership.Interfaces)
}

func TestApplyTopologyL3SubnetEnrichmentDropsDuplicateSegmentMemberIP(t *testing.T) {
	data := topologymodel.Data{
		Actors: []topologymodel.Actor{
			topologyL3ManagedActorForTest("router-a", nil, "203.0.113.1"),
			topologyL3ManagedActorForTest("router-b", nil, "203.0.113.2"),
			topologyL3ManagedActorForTest("router-c", nil, "203.0.113.2"),
			topologyL3ManagedActorForTest("router-d", nil, "203.0.113.4"),
		},
	}
	aggregate := topologymodel.ObservationAggregate{
		ProducerScopeID: "producer-a",
		L3Interfaces: []topologymodel.L3Interface{
			{DeviceID: "router-c", IP: "203.0.113.2", Netmask: "255.255.255.0", IfIndex: "3", IfName: "xe-0/0/3"},
			{DeviceID: "router-a", IP: "203.0.113.1", Netmask: "255.255.255.0", IfIndex: "1", IfName: "xe-0/0/1"},
			{DeviceID: "router-d", IP: "203.0.113.4", Netmask: "255.255.255.0", IfIndex: "4", IfName: "xe-0/0/4"},
			{DeviceID: "router-b", IP: "203.0.113.2", Netmask: "255.255.255.0", IfIndex: "2", IfName: "xe-0/0/2"},
		},
	}
	assignTopologyEnrichTestHandles(t, &data)

	stats := ApplyL3Subnet(&data, aggregate)

	require.Equal(t, 1, stats.EmittedSegments)
	require.Equal(t, 3, stats.EmittedMembershipLinks)
	require.Equal(t, 1, stats.SubnetStats.SuppressedSegmentDuplicateIP)
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["l3_subnet_membership_suppressed_duplicate_ip"])
	require.Equal(t, 3, topologyStatsToV1ForTest(t, data.Stats)["l3_subnet_membership_visible_links"])
	require.Len(t, topologyActorsByTypeForTest(data.Actors, topologymodel.L3SubnetSegmentActorType), 1)

	memberships := topologyLinksByTypeForTest(data.Links, topologymodel.L3SubnetMembershipLinkType)
	require.Len(t, memberships, 3)
	require.Equal(t, []topologymodel.ActorHandle{
		topologyEnrichTestActorHandle("router-a"),
		topologyEnrichTestActorHandle("router-b"),
		topologyEnrichTestActorHandle("router-d"),
	}, topologyLinkSrcActorHandlesForTest(memberships))
}

func TestApplyTopologyL3SubnetEnrichmentAggregatesMultipleMemberInterfaces(t *testing.T) {
	data := topologymodel.Data{
		Actors: []topologymodel.Actor{
			topologyL3ManagedActorForTest("router-a", nil, "203.0.113.1", "203.0.113.5"),
			topologyL3ManagedActorForTest("router-b", nil, "203.0.113.2"),
		},
	}
	aggregate := topologymodel.ObservationAggregate{
		ProducerScopeID: "producer-a",
		L3Interfaces: []topologymodel.L3Interface{
			{DeviceID: "router-a", IP: "203.0.113.5", Netmask: "255.255.255.0", IfIndex: "5", IfName: "xe-0/0/5", IfDescr: "transit-a-secondary"},
			{DeviceID: "router-b", IP: "203.0.113.2", Netmask: "255.255.255.0", IfIndex: "2", IfName: "xe-0/0/2", IfDescr: "transit-b"},
			{DeviceID: "router-a", IP: "203.0.113.1", Netmask: "255.255.255.0", IfIndex: "1", IfName: "xe-0/0/1", IfDescr: "transit-a-primary"},
		},
	}
	assignTopologyEnrichTestHandles(t, &data)

	stats := ApplyL3Subnet(&data, aggregate)

	require.Equal(t, 1, stats.EmittedSegments)
	require.Equal(t, 2, stats.EmittedMembershipLinks)
	require.Equal(t, 3, stats.SubnetStats.CandidateMemberships)

	segments := topologyActorsByTypeForTest(data.Actors, topologymodel.L3SubnetSegmentActorType)
	require.Len(t, segments, 1)
	require.Equal(t, topologyengine.OptionalValue[int]{Value: 3, Has: true}, segments[0].Detail.L2.Segment.PortsTotal)
	require.Equal(t, topologyengine.OptionalValue[int]{Value: 2, Has: true}, segments[0].Detail.L2.Segment.EndpointsTotal)

	memberships := topologyLinksByTypeForTest(data.Links, topologymodel.L3SubnetMembershipLinkType)
	require.Len(t, memberships, 2)
	routerALink := requireTopologyLinkBySrcActorIDForTest(t, memberships, "router-a")
	require.NotNil(t, routerALink.Detail.L3SubnetMembership)
	require.Equal(t, []topologymodel.L3SubnetMembershipInterface{
		{MemberIP: "203.0.113.1", IfIndex: 1, IfName: "xe-0/0/1", IfDescr: "transit-a-primary"},
		{MemberIP: "203.0.113.5", IfIndex: 5, IfName: "xe-0/0/5", IfDescr: "transit-a-secondary"},
	}, routerALink.Detail.L3SubnetMembership.Interfaces)
}

func TestApplyTopologyL3SubnetEnrichmentOmitsSegmentsWithoutProducerScope(t *testing.T) {
	data := topologymodel.Data{
		Actors: []topologymodel.Actor{
			topologyL3ManagedActorForTest("router-a", nil, "203.0.113.1"),
			topologyL3ManagedActorForTest("router-b", nil, "203.0.113.2"),
		},
	}
	aggregate := topologymodel.ObservationAggregate{
		L3Interfaces: []topologymodel.L3Interface{
			l3InterfaceForTest("router-a", "203.0.113.1", "255.255.255.0", "1"),
			l3InterfaceForTest("router-b", "203.0.113.2", "255.255.255.0", "2"),
		},
	}
	assignTopologyEnrichTestHandles(t, &data)

	stats := ApplyL3Subnet(&data, aggregate)

	require.Equal(t, 1, stats.SuppressedNoProducerScope)
	require.Empty(t, topologyActorsByTypeForTest(data.Actors, topologymodel.L3SubnetSegmentActorType))
	require.Empty(t, topologyLinksByTypeForTest(data.Links, topologymodel.L3SubnetMembershipLinkType))
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["l3_subnet_segment_suppressed_no_producer_scope"])
	require.Equal(t, 0, topologyStatsToV1ForTest(t, data.Stats)["l3_subnet_membership_visible_links"])
}

func topologyL3SubnetLinkForTest(srcActorID, dstActorID, subnet string, prefix any) topologymodel.Link {
	return topologymodel.Link{
		Protocol:       topologymodel.L3SubnetLinkType,
		LinkType:       topologymodel.L3SubnetLinkType,
		SrcActorHandle: topologyEnrichTestActorHandle(srcActorID),
		DstActorHandle: topologyEnrichTestActorHandle(dstActorID),
		Detail: topologymodel.LinkDetail{
			L3Subnet: &topologymodel.L3SubnetLinkDetail{
				Subnet: subnet,
				Prefix: testTopologyIntValue(prefix),
			},
		},
	}
}

func topologyActorsByTypeForTest(actors []topologymodel.Actor, actorType string) []topologymodel.Actor {
	var out []topologymodel.Actor
	for _, actor := range actors {
		if strings.EqualFold(strings.TrimSpace(actor.ActorType), actorType) {
			out = append(out, actor)
		}
	}
	return out
}

func topologyLinksByTypeForTest(links []topologymodel.Link, linkType string) []topologymodel.Link {
	var out []topologymodel.Link
	for _, link := range links {
		if strings.EqualFold(strings.TrimSpace(topologyutil.FirstNonEmptyString(link.LinkType, link.Protocol)), linkType) {
			out = append(out, link)
		}
	}
	return out
}

func topologyLinkSrcActorHandlesForTest(links []topologymodel.Link) []topologymodel.ActorHandle {
	out := make([]topologymodel.ActorHandle, 0, len(links))
	for _, link := range links {
		out = append(out, link.SrcActorHandle)
	}
	return out
}

func requireTopologyLinkBySrcActorIDForTest(t *testing.T, links []topologymodel.Link, actorID string) topologymodel.Link {
	t.Helper()
	want := topologyEnrichTestActorHandle(actorID)
	for _, link := range links {
		if link.SrcActorHandle == want {
			return link
		}
	}
	require.Failf(t, "missing link", "src actor id %q", actorID)
	return topologymodel.Link{}
}

func TestTopologyL3SubnetLinkKeySeparatesDelimitedFields(t *testing.T) {
	handles := graph.NewActorHandleAllocator()
	first := handles.Next()
	second := handles.Next()
	third := handles.Next()
	left := topologymodel.Link{
		SrcActorHandle: first,
		DstActorHandle: second,
		Detail: topologymodel.LinkDetail{
			L3Subnet: &topologymodel.L3SubnetLinkDetail{
				Subnet: "198.51.100.0/30",
				Prefix: 30,
			},
		},
	}
	right := topologymodel.Link{
		SrcActorHandle: first,
		DstActorHandle: third,
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
				Protocol:       topologymodel.L3SubnetLinkType,
				LinkType:       topologymodel.L3SubnetLinkType,
				SrcActorHandle: topologyEnrichTestActorHandle("router-a"),
				DstActorHandle: topologyEnrichTestActorHandle("router-b"),
				Detail: topologymodel.LinkDetail{
					L3Subnet: &topologymodel.L3SubnetLinkDetail{
						Subnet: "198.51.100.0/30",
						Prefix: 30,
					},
				},
			},
		},
	}
	assignTopologyEnrichTestHandles(t, &data)

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
