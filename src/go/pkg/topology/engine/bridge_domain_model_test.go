// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildBridgeDomainModel_MergesRootSetsLikeEnlinkdGetAllPersisted(t *testing.T) {
	model := buildBridgeDomainModel(
		[]bridgeBridgeLinkRecord{
			{
				port:           bridgePortRef{deviceID: "node-a", bridgePort: "10", ifIndex: 10},
				designatedPort: bridgePortRef{deviceID: "node-b", bridgePort: "1", ifIndex: 1},
			},
			{
				port:           bridgePortRef{deviceID: "node-b", bridgePort: "2", ifIndex: 2},
				designatedPort: bridgePortRef{deviceID: "node-c", bridgePort: "1", ifIndex: 1},
			},
		},
		nil,
	)

	require.Len(t, model.domains, 1)
	domain := model.domains[0]
	require.Len(t, domain.bridges, 3)
	require.True(t, domain.bridges["node-c"].root)
	require.False(t, domain.bridges["node-a"].root)
	require.False(t, domain.bridges["node-b"].root)
	require.Len(t, domain.segments, 2)
}

func TestBuildBridgeDomainModel_AttachesMacsToMatchingBridgeSegments(t *testing.T) {
	model := buildBridgeDomainModel(
		[]bridgeBridgeLinkRecord{
			{
				port:           bridgePortRef{deviceID: "leaf", bridgePort: "2", ifIndex: 2, ifName: "Gi0/2"},
				designatedPort: bridgePortRef{deviceID: "root", bridgePort: "1", ifIndex: 1, ifName: "Gi0/1"},
			},
		},
		[]bridgeMacLinkRecord{
			{port: bridgePortRef{deviceID: "leaf", bridgePort: "2", ifIndex: 2, ifName: "Gi0/2"}, endpointID: "mac:00:11:22:33:44:55", method: "fdb"},
			{port: bridgePortRef{deviceID: "leaf", bridgePort: "9", ifIndex: 9, ifName: "Gi0/9"}, endpointID: "mac:aa:bb:cc:dd:ee:ff", method: "fdb"},
		},
	)

	require.Len(t, model.domains, 1)
	domain := model.domains[0]
	require.Len(t, domain.segments, 2)

	var shared *bridgeDomainSegment
	var standalone *bridgeDomainSegment
	for _, segment := range domain.segments {
		if segment == nil {
			continue
		}
		if len(segment.ports) == 2 {
			shared = segment
		}
		if len(segment.ports) == 1 {
			standalone = segment
		}
	}

	require.NotNil(t, shared)
	require.Contains(t, shared.endpointIDs, "mac:00:11:22:33:44:55")
	require.NotNil(t, standalone)
	require.Contains(t, standalone.endpointIDs, "mac:aa:bb:cc:dd:ee:ff")
}

func TestCollectBridgeLinkRecords_DeduplicatesUndirectedAdjacencies(t *testing.T) {
	records := collectBridgeLinkRecords([]Adjacency{
		{
			Protocol:   "lldp",
			SourceID:   "a",
			SourcePort: "Gi0/1",
			TargetID:   "b",
			TargetPort: "Gi0/2",
		},
		{
			Protocol:   "lldp",
			SourceID:   "b",
			SourcePort: "Gi0/2",
			TargetID:   "a",
			TargetPort: "Gi0/1",
		},
	}, map[string]int{
		deviceIfNameKey("a", "Gi0/1"): 1,
		deviceIfNameKey("b", "Gi0/2"): 2,
	})

	require.Len(t, records, 1)
	require.Equal(t, "a", records[0].designatedPort.deviceID)
	require.Equal(t, "b", records[0].port.deviceID)
}

func TestCollectBridgeLinkRecords_SkipsAdjacencyWithoutRemotePort(t *testing.T) {
	records := collectBridgeLinkRecords([]Adjacency{
		{
			Protocol:   "lldp",
			SourceID:   "a",
			SourcePort: "Gi0/1",
			TargetID:   "b",
			TargetPort: "",
		},
	}, map[string]int{
		deviceIfNameKey("a", "Gi0/1"): 1,
	})

	require.Empty(t, records)
}
