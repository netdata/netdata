// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"testing"
	"time"

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
