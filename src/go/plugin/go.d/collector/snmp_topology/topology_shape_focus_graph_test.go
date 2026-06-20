// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"
	"time"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/stretchr/testify/require"
)

func TestTopologyFocusGraphBuildAndDepthTraversal(t *testing.T) {
	now := time.Now().UTC()
	data := &topologyData{
		SchemaVersion: topologySchemaVersion,
		CollectedAt:   now,
		Actors: []topologyActor{
			{
				ActorID:   "device-a",
				ActorType: "device",
				Layer:     "2",
				Source:    "snmp",
				Match: topologyMatch{
					IPAddresses: []string{"10.0.0.1"},
				},
			},
			{
				ActorID:   "device-b",
				ActorType: "device",
				Layer:     "2",
				Source:    "snmp",
				Match: topologyMatch{
					IPAddresses: []string{"10.0.0.2"},
				},
			},
			{
				ActorID:   "segment-1",
				ActorType: "segment",
				Layer:     "2",
				Source:    "snmp",
			},
			{
				ActorID:   "device-c",
				ActorType: "device",
				Layer:     "2",
				Source:    "snmp",
				Match: topologyMatch{
					IPAddresses: []string{"10.0.0.3"},
				},
			},
		},
		Links: []topologyLink{
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

func TestTopologyActorHasIPMatchesMatchAndManagementAddresses(t *testing.T) {
	actor := topologyActor{
		Match: topologyMatch{
			IPAddresses: []string{"10.0.0.1"},
		},
		Detail: topologyActorDetail{
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

func TestRecordTopologyFocusStatsNormalizesDepthAndFilteredCounts(t *testing.T) {
	data := &topologyData{
		Actors: []topologyActor{
			{ActorID: "device-a", ActorType: "device"},
			{ActorID: "device-b", ActorType: "device"},
		},
		Links: []topologyLink{
			{Protocol: "lldp", Direction: "bidirectional", SrcActorID: "device-a", DstActorID: "device-b"},
		},
	}

	recordTopologyFocusStats(data, topologyQueryOptions{
		ManagedDeviceFocus: "ip:10.0.0.1",
		Depth:              topologyDepthAllInternal,
	}, 5, 4)

	require.Equal(t, "ip:10.0.0.1", topologyStatsToV1(data.Stats)["managed_snmp_device_focus"])
	require.Equal(t, topologyDepthAll, topologyStatsToV1(data.Stats)["depth"])
	require.Equal(t, 3, topologyStatsToV1(data.Stats)["actors_focus_depth_filtered"])
	require.Equal(t, 3, topologyStatsToV1(data.Stats)["links_focus_depth_filtered"])
	require.Equal(t, len(data.Links), intStatValue(topologyStatsToV1(data.Stats)["links_total"]))
}

func TestRecordTopologyFocusAllDevicesStatsKeepsAllDepth(t *testing.T) {
	data := &topologyData{
		Links: []topologyLink{
			{Protocol: "lldp", Direction: "bidirectional", SrcActorID: "device-a", DstActorID: "device-b"},
		},
	}

	recordTopologyFocusAllDevicesStats(data, topologyQueryOptions{
		ManagedDeviceFocus: topologyManagedFocusAllDevices,
	})

	require.Equal(t, topologyManagedFocusAllDevices, topologyStatsToV1(data.Stats)["managed_snmp_device_focus"])
	require.Equal(t, topologyDepthAll, topologyStatsToV1(data.Stats)["depth"])
	require.Equal(t, 0, topologyStatsToV1(data.Stats)["actors_focus_depth_filtered"])
	require.Equal(t, 0, topologyStatsToV1(data.Stats)["links_focus_depth_filtered"])
	require.Equal(t, len(data.Links), intStatValue(topologyStatsToV1(data.Stats)["links_total"]))
}
