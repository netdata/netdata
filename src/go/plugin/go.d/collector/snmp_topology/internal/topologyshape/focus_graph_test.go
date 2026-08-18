// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"testing"
	"time"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	"github.com/stretchr/testify/require"
)

func TestTopologyFocusGraphBuildAndDepthTraversal(t *testing.T) {
	now := time.Now().UTC()
	data := &topologymodel.Data{
		SchemaVersion: topologymodel.SchemaVersion,
		CollectedAt:   now,
		Actors: []topologymodel.Actor{
			{
				ActorID:   "device-a",
				ActorType: "device",
				Layer:     "2",
				Source:    "snmp",
				Match: topologymodel.Match{
					IPAddresses: []string{"10.0.0.1"},
				},
			},
			{
				ActorID:   "device-b",
				ActorType: "device",
				Layer:     "2",
				Source:    "snmp",
				Match: topologymodel.Match{
					IPAddresses: []string{"10.0.0.2"},
				},
			},
			{
				ActorID:     "segment-1",
				ActorType:   "segment",
				SegmentKind: topologymodel.SegmentKindBroadcastDomain,
				Layer:       "2",
				Source:      "snmp",
			},
			{
				ActorID:   "device-c",
				ActorType: "device",
				Layer:     "2",
				Source:    "snmp",
				Match: topologymodel.Match{
					IPAddresses: []string{"10.0.0.3"},
				},
			},
		},
		Links: []topologymodel.Link{
			{
				Layer:          "2",
				Protocol:       "lldp",
				LinkType:       "device",
				SrcActorHandle: topologyShapeTestActorHandle("device-a"),
				DstActorHandle: topologyShapeTestActorHandle("device-b"),
			},
			{
				Layer:          "2",
				Protocol:       "fdb",
				LinkType:       "segment",
				SrcActorHandle: topologyShapeTestActorHandle("device-b"),
				DstActorHandle: topologyShapeTestActorHandle("segment-1"),
			},
			{
				Layer:          "2",
				Protocol:       "fdb",
				LinkType:       "segment",
				SrcActorHandle: topologyShapeTestActorHandle("segment-1"),
				DstActorHandle: topologyShapeTestActorHandle("device-c"),
			},
		},
	}
	handles := assignTopologyShapeTestHandles(t, data)

	graph := buildTopologyFocusGraph(data)
	require.Contains(t, graph.nonSegmentSet, handles["device-a"])
	require.Contains(t, graph.nonSegmentSet, handles["device-b"])
	require.Contains(t, graph.nonSegmentSet, handles["device-c"])
	require.Contains(t, graph.segmentSet, handles["segment-1"])
	require.Contains(t, graph.nonSegmentAdj[handles["device-a"]], handles["device-b"])
	require.Contains(t, graph.nonSegmentAdj[handles["device-b"]], handles["device-a"])
	require.Contains(t, graph.nodeSegments[handles["device-b"]], handles["segment-1"])
	require.Contains(t, graph.segmentNeighbors[handles["segment-1"]], handles["device-b"])
	require.Contains(t, graph.segmentNeighbors[handles["segment-1"]], handles["device-c"])

	roots := map[topologymodel.ActorHandle]struct{}{handles["device-a"]: {}}

	distanceDepth1 := traverseTopologyFocusDepth(graph, roots, 1)
	require.Equal(t, map[topologymodel.ActorHandle]int{
		handles["device-a"]: 0,
		handles["device-b"]: 1,
	}, distanceDepth1)

	includedNonSegmentDepth1, includedActorsDepth1 := collectTopologyFocusDepthSets(graph, distanceDepth1, 1)
	require.Equal(t, map[topologymodel.ActorHandle]struct{}{
		handles["device-a"]: {},
		handles["device-b"]: {},
	}, includedNonSegmentDepth1)
	require.Equal(t, map[topologymodel.ActorHandle]struct{}{
		handles["device-a"]:  {},
		handles["device-b"]:  {},
		handles["segment-1"]: {},
	}, includedActorsDepth1)

	distanceDepth2 := traverseTopologyFocusDepth(graph, roots, 2)
	require.Equal(t, map[topologymodel.ActorHandle]int{
		handles["device-a"]: 0,
		handles["device-b"]: 1,
		handles["device-c"]: 2,
	}, distanceDepth2)

	includedNonSegmentDepth2, includedActorsDepth2 := collectTopologyFocusDepthSets(graph, distanceDepth2, 2)
	require.Equal(t, map[topologymodel.ActorHandle]struct{}{
		handles["device-a"]: {},
		handles["device-b"]: {},
		handles["device-c"]: {},
	}, includedNonSegmentDepth2)
	require.Equal(t, map[topologymodel.ActorHandle]struct{}{
		handles["device-a"]:  {},
		handles["device-b"]:  {},
		handles["device-c"]:  {},
		handles["segment-1"]: {},
	}, includedActorsDepth2)
}

func TestTopologyFocusGraphDoesNotFanOutThroughL3SubnetSegment(t *testing.T) {
	data := &topologymodel.Data{
		Actors: []topologymodel.Actor{
			{ActorID: "router-a", ActorType: "router"},
			{ActorID: "router-b", ActorType: "router"},
			{ActorID: "router-c", ActorType: "router"},
			{ActorID: "subnet-a", ActorType: topologymodel.L3SubnetSegmentActorType, SegmentKind: topologymodel.SegmentKindL3Subnet},
		},
		Links: []topologymodel.Link{
			{SrcActorHandle: topologyShapeTestActorHandle("router-a"), DstActorHandle: topologyShapeTestActorHandle("subnet-a"), Protocol: topologymodel.L3SubnetMembershipLinkType, LinkType: topologymodel.L3SubnetMembershipLinkType},
			{SrcActorHandle: topologyShapeTestActorHandle("router-b"), DstActorHandle: topologyShapeTestActorHandle("subnet-a"), Protocol: topologymodel.L3SubnetMembershipLinkType, LinkType: topologymodel.L3SubnetMembershipLinkType},
			{SrcActorHandle: topologyShapeTestActorHandle("router-c"), DstActorHandle: topologyShapeTestActorHandle("subnet-a"), Protocol: topologymodel.L3SubnetMembershipLinkType, LinkType: topologymodel.L3SubnetMembershipLinkType},
		},
	}
	handles := assignTopologyShapeTestHandles(t, data)

	graph := buildTopologyFocusGraph(data)
	distance := traverseTopologyFocusDepth(graph, map[topologymodel.ActorHandle]struct{}{handles["router-a"]: {}}, topologyoptions.DepthAllInternal)
	_, includedActors := collectTopologyFocusDepthSets(graph, distance, topologyoptions.DepthAllInternal)

	require.Equal(t, map[topologymodel.ActorHandle]int{handles["router-a"]: 0}, distance)
	require.Contains(t, includedActors, handles["router-a"])
	require.Contains(t, includedActors, handles["subnet-a"])
	require.NotContains(t, includedActors, handles["router-b"])
	require.NotContains(t, includedActors, handles["router-c"])
}

func TestTopologyActorHasIPMatchesMatchAndManagementAddresses(t *testing.T) {
	actor := topologymodel.Actor{
		Match: topologymodel.Match{
			IPAddresses: []string{"10.0.0.1"},
		},
		Detail: topologymodel.ActorDetail{
			L2: topologyengine.ProjectionActorDetail{
				Device: topologyengine.ProjectionDeviceActorDetail{
					DeviceID:            "device-a",
					ManagementIP:        "10.0.0.2",
					ManagementAddresses: []string{"10.0.0.3", "not-an-ip"},
				},
			},
		},
	}

	tests := map[string]struct {
		ip   string
		want bool
	}{
		"match-ip":            {ip: "10.0.0.1", want: true},
		"management-ip":       {ip: "10.0.0.2", want: true},
		"management-address":  {ip: "10.0.0.3", want: true},
		"missing-ip":          {ip: "10.0.0.9"},
		"invalid-ip-rejected": {ip: "not-an-ip"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, topologyActorHasIP(actor, tc.ip))
		})
	}
}

func TestTopologyActorDetailManagementIPsCanonicalizesBeforeDedup(t *testing.T) {
	actor := topologymodel.Actor{
		Detail: topologymodel.ActorDetail{
			SNMP: topologymodel.SNMPActorDetail{
				ManagementIP: "::ffff:192.0.2.1",
				ManagementAddresses: []topologymodel.ManagementAddress{
					{Address: "192.0.2.1"},
					{Address: "192.0.2.3", AddressType: "ipv4"},
					{Address: "c0000263", AddressType: "16"},
					{Address: "192.0.2.4", AddressType: "2"},
				},
			},
			L2: topologyengine.ProjectionActorDetail{
				Device: topologyengine.ProjectionDeviceActorDetail{
					DeviceID:            "device-a",
					ManagementAddresses: []string{"::", "::ffff:192.0.2.2", "192.0.2.2"},
				},
			},
		},
	}

	require.Equal(t, []string{"192.0.2.2"}, topologymodel.ActorDetailManagementIPs(actor))
}

func TestTopologyActorDetailManagementIPsUsesOnlySelectedSNMPFallback(t *testing.T) {
	snmpOnly := topologymodel.Actor{
		Detail: topologymodel.ActorDetail{
			SNMP: topologymodel.SNMPActorDetail{
				ManagementIP: "192.0.2.1",
				ManagementAddresses: []topologymodel.ManagementAddress{
					{Address: "192.0.2.2", AddressType: "ipv4"},
				},
			},
		},
	}
	require.Equal(t, "192.0.2.1", topologymodel.ActorDetailManagementIP(snmpOnly))
	require.Equal(t, []string{"192.0.2.1"}, topologymodel.ActorDetailManagementIPs(snmpOnly))

	l2WithoutPrimary := snmpOnly
	l2WithoutPrimary.Detail.L2.Device.DeviceID = "device-a"
	require.Empty(t, topologymodel.ActorDetailManagementIP(l2WithoutPrimary))
	require.Empty(t, topologymodel.ActorDetailManagementIPs(l2WithoutPrimary))
	require.False(t, topologyActorHasIP(l2WithoutPrimary, "192.0.2.1"))
	require.False(t, topologyActorHasIP(l2WithoutPrimary, "192.0.2.2"))
}

func TestRecordTopologyFocusStatsNormalizesDepthAndFilteredCounts(t *testing.T) {
	data := &topologymodel.Data{
		Actors: []topologymodel.Actor{
			{ActorID: "device-a", ActorType: "device"},
			{ActorID: "device-b", ActorType: "device"},
		},
		Links: []topologymodel.Link{
			{Protocol: "lldp", Direction: "bidirectional", SrcActorHandle: topologyShapeTestActorHandle("device-a"), DstActorHandle: topologyShapeTestActorHandle("device-b")},
		},
	}

	recordTopologyFocusStats(data, topologyoptions.QueryOptions{
		ManagedDeviceFocus: "ip:10.0.0.1",
		Depth:              topologyoptions.DepthAllInternal,
	}, 5, 4)

	require.Equal(t, "ip:10.0.0.1", data.Stats.Focus.ManagedSNMPDeviceFocus)
	require.True(t, data.Stats.Focus.Depth.All)
	require.Equal(t, 3, data.Stats.Focus.ActorsDepthFiltered)
	require.Equal(t, 3, data.Stats.Focus.LinksDepthFiltered)
	require.Equal(t, len(data.Links), data.Stats.Recomputed.LinksTotal)
}

func TestRecordTopologyFocusAllDevicesStatsKeepsAllDepth(t *testing.T) {
	data := &topologymodel.Data{
		Links: []topologymodel.Link{
			{Protocol: "lldp", Direction: "bidirectional", SrcActorHandle: topologyShapeTestActorHandle("device-a"), DstActorHandle: topologyShapeTestActorHandle("device-b")},
		},
	}

	recordTopologyFocusAllDevicesStats(data, topologyoptions.QueryOptions{
		ManagedDeviceFocus: topologyoptions.ManagedFocusAllDevices,
	})

	require.Equal(t, topologyoptions.ManagedFocusAllDevices, data.Stats.Focus.ManagedSNMPDeviceFocus)
	require.True(t, data.Stats.Focus.Depth.All)
	require.Equal(t, 0, data.Stats.Focus.ActorsDepthFiltered)
	require.Equal(t, 0, data.Stats.Focus.LinksDepthFiltered)
	require.Equal(t, len(data.Links), data.Stats.Recomputed.LinksTotal)
}
