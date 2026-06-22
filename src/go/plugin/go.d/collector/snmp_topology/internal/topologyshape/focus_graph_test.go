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
				Layer:      "2",
				Protocol:   "lldp",
				LinkType:   "device",
				SrcActorID: "device-a",
				DstActorID: "device-b",
			},
			{
				Layer:      "2",
				Protocol:   "fdb",
				LinkType:   "segment",
				SrcActorID: "device-b",
				DstActorID: "segment-1",
			},
			{
				Layer:      "2",
				Protocol:   "fdb",
				LinkType:   "segment",
				SrcActorID: "segment-1",
				DstActorID: "device-c",
			},
		},
	}

	graph := buildTopologyFocusGraph(data)
	require.Contains(t, graph.nonSegmentSet, "device-a")
	require.Contains(t, graph.nonSegmentSet, "device-b")
	require.Contains(t, graph.nonSegmentSet, "device-c")
	require.Contains(t, graph.segmentSet, "segment-1")
	require.Contains(t, graph.nonSegmentAdj["device-a"], "device-b")
	require.Contains(t, graph.nonSegmentAdj["device-b"], "device-a")
	require.Contains(t, graph.nodeSegments["device-b"], "segment-1")
	require.Contains(t, graph.segmentNeighbors["segment-1"], "device-b")
	require.Contains(t, graph.segmentNeighbors["segment-1"], "device-c")

	roots := map[string]struct{}{"device-a": {}}

	distanceDepth1 := traverseTopologyFocusDepth(graph, roots, 1)
	require.Equal(t, map[string]int{
		"device-a": 0,
		"device-b": 1,
	}, distanceDepth1)

	includedNonSegmentDepth1, includedActorsDepth1 := collectTopologyFocusDepthSets(graph, distanceDepth1, 1)
	require.Equal(t, map[string]struct{}{
		"device-a": {},
		"device-b": {},
	}, includedNonSegmentDepth1)
	require.Equal(t, map[string]struct{}{
		"device-a":  {},
		"device-b":  {},
		"segment-1": {},
	}, includedActorsDepth1)

	distanceDepth2 := traverseTopologyFocusDepth(graph, roots, 2)
	require.Equal(t, map[string]int{
		"device-a": 0,
		"device-b": 1,
		"device-c": 2,
	}, distanceDepth2)

	includedNonSegmentDepth2, includedActorsDepth2 := collectTopologyFocusDepthSets(graph, distanceDepth2, 2)
	require.Equal(t, map[string]struct{}{
		"device-a": {},
		"device-b": {},
		"device-c": {},
	}, includedNonSegmentDepth2)
	require.Equal(t, map[string]struct{}{
		"device-a":  {},
		"device-b":  {},
		"device-c":  {},
		"segment-1": {},
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
			{SrcActorID: "router-a", DstActorID: "subnet-a", Protocol: topologymodel.L3SubnetMembershipLinkType, LinkType: topologymodel.L3SubnetMembershipLinkType},
			{SrcActorID: "router-b", DstActorID: "subnet-a", Protocol: topologymodel.L3SubnetMembershipLinkType, LinkType: topologymodel.L3SubnetMembershipLinkType},
			{SrcActorID: "router-c", DstActorID: "subnet-a", Protocol: topologymodel.L3SubnetMembershipLinkType, LinkType: topologymodel.L3SubnetMembershipLinkType},
		},
	}

	graph := buildTopologyFocusGraph(data)
	distance := traverseTopologyFocusDepth(graph, map[string]struct{}{"router-a": {}}, topologyoptions.DepthAllInternal)
	_, includedActors := collectTopologyFocusDepthSets(graph, distance, topologyoptions.DepthAllInternal)

	require.Equal(t, map[string]int{"router-a": 0}, distance)
	require.Contains(t, includedActors, "router-a")
	require.Contains(t, includedActors, "subnet-a")
	require.NotContains(t, includedActors, "router-b")
	require.NotContains(t, includedActors, "router-c")
}

func TestTopologyActorHasIPMatchesMatchAndManagementAddresses(t *testing.T) {
	actor := topologymodel.Actor{
		Match: topologymodel.Match{
			IPAddresses: []string{"10.0.0.1"},
		},
		Detail: topologymodel.ActorDetail{
			L2: topologyengine.ProjectionActorDetail{
				Device: topologyengine.ProjectionDeviceActorDetail{
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
				},
			},
			L2: topologyengine.ProjectionActorDetail{
				Device: topologyengine.ProjectionDeviceActorDetail{
					ManagementAddresses: []string{"::", "::ffff:192.0.2.2", "192.0.2.2"},
				},
			},
		},
	}

	require.Equal(t, []string{"192.0.2.1", "192.0.2.2"}, topologymodel.ActorDetailManagementIPs(actor))
}

func TestRecordTopologyFocusStatsNormalizesDepthAndFilteredCounts(t *testing.T) {
	data := &topologymodel.Data{
		Actors: []topologymodel.Actor{
			{ActorID: "device-a", ActorType: "device"},
			{ActorID: "device-b", ActorType: "device"},
		},
		Links: []topologymodel.Link{
			{Protocol: "lldp", Direction: "bidirectional", SrcActorID: "device-a", DstActorID: "device-b"},
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
			{Protocol: "lldp", Direction: "bidirectional", SrcActorID: "device-a", DstActorID: "device-b"},
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
