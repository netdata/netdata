// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
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

func TestMergeRootDomainSets_MergesNodeMemberIntoExistingDesignatedRoot(t *testing.T) {
	rootToNodes := map[string]bridgeNodeSet{
		"designated-root": {"leaf-a": {}},
		"other-root":      {"node-x": {}, "leaf-b": {}},
	}

	mergeRootDomainSets(rootToNodes, "designated-root", "node-x")

	require.Len(t, rootToNodes, 1)
	require.Contains(t, rootToNodes, "designated-root")
	require.Equal(t, bridgeNodeSet{
		"leaf-a":     {},
		"other-root": {},
		"node-x":     {},
		"leaf-b":     {},
	}, rootToNodes["designated-root"])
}

func TestMergeRootDomainSets_MergesTwoNonRootMembersAcrossDomains(t *testing.T) {
	rootToNodes := map[string]bridgeNodeSet{
		"root-a": {"designated-member": {}, "leaf-a": {}},
		"root-b": {"node-member": {}, "leaf-b": {}},
	}

	mergeRootDomainSets(rootToNodes, "designated-member", "node-member")

	require.Len(t, rootToNodes, 1)
	require.Contains(t, rootToNodes, "root-a")
	require.Equal(t, bridgeNodeSet{
		"designated-member": {},
		"leaf-a":            {},
		"root-b":            {},
		"node-member":       {},
		"leaf-b":            {},
	}, rootToNodes["root-a"])
}

func TestCollectBridgeLinkRecords_DeduplicatesUndirectedAdjacencies(t *testing.T) {
	records := collectBridgeLinkRecords([]model.Adjacency{
		{
			Protocol:           "lldp",
			SourceID:           "a",
			SourcePort:         "Gi0/1",
			SourcePortEvidence: model.AdjacencyPortEvidence{IfIndex: 1, IfName: "Gi0/1"},
			TargetID:           "b",
			TargetPort:         "Gi0/2",
			TargetPortEvidence: model.AdjacencyPortEvidence{IfIndex: 2, IfName: "Gi0/2"},
		},
		{
			Protocol:           "lldp",
			SourceID:           "b",
			SourcePort:         "Gi0/2",
			SourcePortEvidence: model.AdjacencyPortEvidence{IfIndex: 2, IfName: "Gi0/2"},
			TargetID:           "a",
			TargetPort:         "Gi0/1",
			TargetPortEvidence: model.AdjacencyPortEvidence{IfIndex: 1, IfName: "Gi0/1"},
		},
	}, map[string]int{
		deviceIfNameKey("a", "Gi0/1"): 1,
		deviceIfNameKey("b", "Gi0/2"): 2,
	}, nil, bridgePortAliasIndex{}, topologyInferenceStrategyConfigFor(topologyInferenceStrategyFDBMinimumKnowledge))

	require.Len(t, records, 1)
	require.Equal(t, "a", records[0].designatedPort.deviceID)
	require.Equal(t, "b", records[0].port.deviceID)
}

func TestCollectBridgeLinkRecords_SkipsAdjacencyWithoutRemotePort(t *testing.T) {
	records := collectBridgeLinkRecords([]model.Adjacency{
		{
			Protocol:           "lldp",
			SourceID:           "a",
			SourcePort:         "Gi0/1",
			SourcePortEvidence: model.AdjacencyPortEvidence{IfIndex: 1, IfName: "Gi0/1"},
			TargetID:           "b",
			TargetPort:         "",
		},
	}, map[string]int{
		deviceIfNameKey("a", "Gi0/1"): 1,
	}, nil, bridgePortAliasIndex{}, topologyInferenceStrategyConfigFor(topologyInferenceStrategyFDBMinimumKnowledge))

	require.Empty(t, records)
}

func TestCollectBridgeLinkRecords_STPParentTreeUsesDesignatedTargetPort(t *testing.T) {
	records := collectBridgeLinkRecords([]model.Adjacency{
		{
			Protocol:           "stp",
			SourceID:           "child",
			SourcePort:         "10",
			SourcePortEvidence: model.AdjacencyPortEvidence{IfIndex: 10, IfName: "Gi0/10", BridgePort: "10"},
			TargetID:           "root",
			TargetPort:         "8001",
			TargetPortEvidence: model.AdjacencyPortEvidence{IfIndex: 1, IfName: "Gi0/1", BridgePort: "1"},
		},
	}, map[string]int{
		deviceIfNameKey("child", "Gi0/10"): 10,
		deviceIfNameKey("root", "Gi0/1"):   1,
	}, nil, bridgePortAliasIndex{}, topologyInferenceStrategyConfigFor(topologyInferenceStrategySTPParentTree))

	require.Len(t, records, 1)
	require.Equal(t, "root", records[0].designatedPort.deviceID)
	require.Equal(t, "child", records[0].port.deviceID)
	require.Equal(t, "stp", records[0].method)
}

func TestCollectBridgeLinkRecords_STPPreservesVLANScopes(t *testing.T) {
	adjacencies := []model.Adjacency{
		{
			Protocol: "stp", SourceID: "child", SourcePort: "10",
			SourcePortEvidence: model.AdjacencyPortEvidence{IfIndex: 10, IfName: "Gi0/10", BridgePort: "10"},
			TargetID:           "root", TargetPort: "8001",
			TargetPortEvidence: model.AdjacencyPortEvidence{IfIndex: 1, IfName: "Gi0/1", BridgePort: "1"},
			Labels:             map[string]string{"vlan_id": "100"},
		},
		{
			Protocol: "stp", SourceID: "child", SourcePort: "10",
			SourcePortEvidence: model.AdjacencyPortEvidence{IfIndex: 10, IfName: "Gi0/10", BridgePort: "10"},
			TargetID:           "root", TargetPort: "8001",
			TargetPortEvidence: model.AdjacencyPortEvidence{IfIndex: 1, IfName: "Gi0/1", BridgePort: "1"},
			Labels:             map[string]string{"vlan_id": "200"},
		},
	}
	records := collectBridgeLinkRecords(adjacencies, map[string]int{
		deviceIfNameKey("child", "Gi0/10"): 10,
		deviceIfNameKey("root", "Gi0/1"):   1,
	}, nil, bridgePortAliasIndex{}, topologyInferenceStrategyConfigFor(topologyInferenceStrategySTPParentTree))

	require.Len(t, records, 2)
	require.Equal(t, "vlan:100", bridgePortForwardingDomain(records[0].designatedPort))
	require.Equal(t, "vlan:200", bridgePortForwardingDomain(records[1].designatedPort))
}

func TestCollectBridgeLinkRecords_CDPHybridSkipsLLDPAdjacencies(t *testing.T) {
	records := collectBridgeLinkRecords([]model.Adjacency{
		{
			Protocol:           "lldp",
			SourceID:           "a",
			SourcePort:         "Gi0/1",
			SourcePortEvidence: model.AdjacencyPortEvidence{IfIndex: 1, IfName: "Gi0/1"},
			TargetID:           "b",
			TargetPort:         "Gi0/2",
			TargetPortEvidence: model.AdjacencyPortEvidence{IfIndex: 2, IfName: "Gi0/2"},
		},
		{
			Protocol:           "cdp",
			SourceID:           "a",
			SourcePort:         "Gi0/3",
			SourcePortEvidence: model.AdjacencyPortEvidence{IfIndex: 3, IfName: "Gi0/3"},
			TargetID:           "c",
			TargetPort:         "Gi0/4",
			TargetPortEvidence: model.AdjacencyPortEvidence{IfIndex: 4, IfName: "Gi0/4"},
		},
	}, map[string]int{
		deviceIfNameKey("a", "Gi0/1"): 1,
		deviceIfNameKey("b", "Gi0/2"): 2,
		deviceIfNameKey("a", "Gi0/3"): 3,
		deviceIfNameKey("c", "Gi0/4"): 4,
	}, nil, bridgePortAliasIndex{}, topologyInferenceStrategyConfigFor(topologyInferenceStrategyCDPFDBHybrid))

	require.Len(t, records, 1)
	require.Equal(t, "cdp", records[0].method)
	require.Equal(t, "a", records[0].designatedPort.deviceID)
	require.Equal(t, "c", records[0].port.deviceID)
}

func TestInferFDBPairwiseBridgeLinks_ReciprocalUniquePortPerSide(t *testing.T) {
	attachments := []model.Attachment{
		{
			DeviceID:   "sw-a",
			IfIndex:    1,
			EndpointID: "mac:bb:bb:bb:bb:bb:bb",
			Method:     "fdb",
		},
		{
			DeviceID:   "sw-b",
			IfIndex:    2,
			EndpointID: "mac:aa:aa:aa:aa:aa:aa",
			Method:     "fdb",
		},
	}
	ifaceByDeviceIndex := map[string]model.Interface{
		deviceIfIndexKey("sw-a", 1): {DeviceID: "sw-a", IfIndex: 1, IfName: "Gi0/1"},
		deviceIfIndexKey("sw-b", 2): {DeviceID: "sw-b", IfIndex: 2, IfName: "Gi0/2"},
	}
	reporterAliases := map[string][]string{
		"sw-a": {"mac:aa:aa:aa:aa:aa:aa"},
		"sw-b": {"mac:bb:bb:bb:bb:bb:bb"},
	}

	records := inferFDBPairwiseBridgeLinks(attachments, ifaceByDeviceIndex, reporterAliases)
	require.Len(t, records, 1)
	require.Equal(t, "fdb_pairwise", records[0].method)
	require.Equal(t, "sw-a", records[0].designatedPort.deviceID)
	require.Equal(t, "sw-b", records[0].port.deviceID)
}

func TestInferFDBPairwiseBridgeLinks_RejectsAmbiguousAliasOwners(t *testing.T) {
	attachments := []model.Attachment{
		{DeviceID: "sw-a", IfIndex: 1, EndpointID: "mac:bb:bb:bb:bb:bb:bb", Method: "fdb"},
		{DeviceID: "sw-b", IfIndex: 2, EndpointID: "mac:aa:aa:aa:aa:aa:aa", Method: "fdb"},
		{DeviceID: "sw-c", IfIndex: 3, EndpointID: "mac:aa:aa:aa:aa:aa:aa", Method: "fdb"},
	}
	ifaceByDeviceIndex := map[string]model.Interface{
		deviceIfIndexKey("sw-a", 1): {DeviceID: "sw-a", IfIndex: 1, IfName: "Gi0/1"},
		deviceIfIndexKey("sw-b", 2): {DeviceID: "sw-b", IfIndex: 2, IfName: "Gi0/2"},
		deviceIfIndexKey("sw-c", 3): {DeviceID: "sw-c", IfIndex: 3, IfName: "Gi0/3"},
	}
	reporterAliases := map[string][]string{
		"sw-a": {"mac:aa:aa:aa:aa:aa:aa"},
		"sw-b": {"mac:bb:bb:bb:bb:bb:bb"},
		"sw-c": {"mac:bb:bb:bb:bb:bb:bb"},
	}

	require.Empty(t, inferFDBPairwiseBridgeLinks(attachments, ifaceByDeviceIndex, reporterAliases))
}

func TestInferFDBPairwiseBridgeLinks_EmitsOneRecordPerCompatibleVLANScope(t *testing.T) {
	attachments := []model.Attachment{
		{DeviceID: "sw-a", IfIndex: 1, EndpointID: "mac:bb:bb:bb:bb:bb:bb", Method: "fdb", Labels: map[string]string{"fdb_domain_id": "fdb:10", "vlan_id": "100"}},
		{DeviceID: "sw-a", IfIndex: 1, EndpointID: "mac:bb:bb:bb:bb:bb:bb", Method: "fdb", Labels: map[string]string{"fdb_domain_id": "fdb:20", "vlan_id": "200"}},
		{DeviceID: "sw-b", IfIndex: 2, EndpointID: "mac:aa:aa:aa:aa:aa:aa", Method: "fdb", Labels: map[string]string{"fdb_domain_id": "fdb:30", "vlan_id": "100"}},
		{DeviceID: "sw-b", IfIndex: 2, EndpointID: "mac:aa:aa:aa:aa:aa:aa", Method: "fdb", Labels: map[string]string{"fdb_domain_id": "fdb:40", "vlan_id": "200"}},
	}
	ifaceByDeviceIndex := map[string]model.Interface{
		deviceIfIndexKey("sw-a", 1): {DeviceID: "sw-a", IfIndex: 1, IfName: "Gi0/1"},
		deviceIfIndexKey("sw-b", 2): {DeviceID: "sw-b", IfIndex: 2, IfName: "Gi0/2"},
	}
	reporterAliases := map[string][]string{
		"sw-a": {"mac:aa:aa:aa:aa:aa:aa"},
		"sw-b": {"mac:bb:bb:bb:bb:bb:bb"},
	}

	records := inferFDBPairwiseBridgeLinks(attachments, ifaceByDeviceIndex, reporterAliases)
	require.Len(t, records, 2)
	require.Equal(t, "100", records[0].designatedPort.vlanID)
	require.Equal(t, "200", records[1].designatedPort.vlanID)
}

func TestInferFDBPairwiseBridgeLinks_RejectsIncompatibleDeviceLocalRawDomains(t *testing.T) {
	attachments := []model.Attachment{
		{DeviceID: "sw-a", IfIndex: 1, EndpointID: "mac:bb:bb:bb:bb:bb:bb", Method: "fdb", Labels: map[string]string{"fdb_domain_id": "fdb:500", "vlan_id": "100"}},
		{DeviceID: "sw-b", IfIndex: 2, EndpointID: "mac:aa:aa:aa:aa:aa:aa", Method: "fdb", Labels: map[string]string{"fdb_domain_id": "fdb:500", "vlan_id": "200"}},
	}
	ifaceByDeviceIndex := map[string]model.Interface{
		deviceIfIndexKey("sw-a", 1): {DeviceID: "sw-a", IfIndex: 1, IfName: "Gi0/1"},
		deviceIfIndexKey("sw-b", 2): {DeviceID: "sw-b", IfIndex: 2, IfName: "Gi0/2"},
	}
	reporterAliases := map[string][]string{
		"sw-a": {"mac:aa:aa:aa:aa:aa:aa"},
		"sw-b": {"mac:bb:bb:bb:bb:bb:bb"},
	}

	require.Empty(t, inferFDBPairwiseBridgeLinks(attachments, ifaceByDeviceIndex, reporterAliases))
}

func TestInferFDBPairwiseBridgeLinks_RejectsAmbiguousVLANAlias(t *testing.T) {
	attachments := []model.Attachment{
		{DeviceID: "sw-a", IfIndex: 1, EndpointID: "mac:bb:bb:bb:bb:bb:bb", Method: "fdb", Labels: map[string]string{"fdb_domain_id": "fdb:10", "vlan_id": "100"}},
		{DeviceID: "sw-a", IfIndex: 1, EndpointID: "mac:bb:bb:bb:bb:bb:bb", Method: "fdb", Labels: map[string]string{"fdb_domain_id": "fdb:20", "vlan_id": "100"}},
		{DeviceID: "sw-b", IfIndex: 2, EndpointID: "mac:aa:aa:aa:aa:aa:aa", Method: "fdb", Labels: map[string]string{"fdb_domain_id": "fdb:30", "vlan_id": "100"}},
	}
	ifaceByDeviceIndex := map[string]model.Interface{
		deviceIfIndexKey("sw-a", 1): {DeviceID: "sw-a", IfIndex: 1, IfName: "Gi0/1"},
		deviceIfIndexKey("sw-b", 2): {DeviceID: "sw-b", IfIndex: 2, IfName: "Gi0/2"},
	}
	reporterAliases := map[string][]string{
		"sw-a": {"mac:aa:aa:aa:aa:aa:aa"},
		"sw-b": {"mac:bb:bb:bb:bb:bb:bb"},
	}

	require.Empty(t, inferFDBPairwiseBridgeLinks(attachments, ifaceByDeviceIndex, reporterAliases))
}
