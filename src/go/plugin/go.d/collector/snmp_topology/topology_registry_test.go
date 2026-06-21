// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"
	"time"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyshape"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTopologyRegistry_SnapshotAggregatesAcrossCaches(t *testing.T) {
	registry := newTopologyRegistry()

	cacheA := newTopologyCache()
	cacheA.updateTime = time.Now()
	cacheA.lastUpdate = cacheA.updateTime
	cacheA.agentID = "agent-test"
	cacheA.localDevice = topologyDevice{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		SysName:       "sw-a",
		ManagementIP:  "10.0.0.1",
	}
	cacheA.lldpLocPorts["1"] = &lldpLocPort{
		portNum:       "1",
		portID:        "Gi0/1",
		portIDSubtype: "interfaceName",
	}
	cacheA.lldpRemotes["1:1"] = &lldpRemote{
		localPortNum:     "1",
		remIndex:         "1",
		chassisID:        "aa:bb:cc:dd:ee:ff",
		chassisIDSubtype: "macAddress",
		portID:           "Gi0/2",
		portIDSubtype:    "interfaceName",
		sysName:          "sw-b",
		managementAddr:   "10.0.0.2",
	}

	cacheB := newTopologyCache()
	cacheB.updateTime = time.Now().Add(time.Second)
	cacheB.lastUpdate = cacheB.updateTime
	cacheB.agentID = "agent-test"
	cacheB.localDevice = topologyDevice{
		ChassisID:     "aa:bb:cc:dd:ee:ff",
		ChassisIDType: "macAddress",
		SysName:       "sw-b",
		ManagementIP:  "10.0.0.2",
	}
	cacheB.lldpLocPorts["1"] = &lldpLocPort{
		portNum:       "1",
		portID:        "Gi0/2",
		portIDSubtype: "interfaceName",
	}
	cacheB.lldpRemotes["1:1"] = &lldpRemote{
		localPortNum:     "1",
		remIndex:         "1",
		chassisID:        "00:11:22:33:44:55",
		chassisIDSubtype: "macAddress",
		portID:           "Gi0/1",
		portIDSubtype:    "interfaceName",
		sysName:          "sw-a",
		managementAddr:   "10.0.0.1",
	}

	registry.register(cacheA)
	registry.register(cacheB)

	data, ok := snapshotTopologyRegistryForTest(registry)
	require.True(t, ok)
	require.Equal(t, "2", data.Layer)
	require.Equal(t, "snmp", data.Source)
	require.Equal(t, "summary", data.View)

	require.GreaterOrEqual(t, topologyStatsToV1(data.Stats)["devices_total"].(int), 2)
	require.GreaterOrEqual(t, topologyStatsToV1(data.Stats)["links_total"].(int), 1)
	require.GreaterOrEqual(t, topologyStatsToV1(data.Stats)["links_lldp"].(int), 1)
}

func TestTopologyRegistry_SnapshotSingleCacheKeepsLLDPUnidirectional(t *testing.T) {
	registry := newTopologyRegistry()

	cache := newTopologyCache()
	cache.updateTime = time.Now()
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent-test"
	cache.localDevice = topologyDevice{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		SysName:       "sw-a",
		ManagementIP:  "10.0.0.1",
	}
	cache.lldpLocPorts["1"] = &lldpLocPort{
		portNum:       "1",
		portID:        "Gi0/1",
		portIDSubtype: "interfaceName",
	}
	cache.lldpRemotes["1:1"] = &lldpRemote{
		localPortNum:     "1",
		remIndex:         "1",
		chassisID:        "aa:bb:cc:dd:ee:ff",
		chassisIDSubtype: "macAddress",
		portID:           "Gi0/2",
		portIDSubtype:    "interfaceName",
		sysName:          "sw-b",
		managementAddr:   "10.0.0.2",
	}

	registry.register(cache)

	data, ok := snapshotTopologyRegistryForTest(registry)
	require.True(t, ok)
	require.Len(t, data.Links, 1)
	require.Equal(t, "lldp", data.Links[0].Protocol)
	require.Equal(t, "unidirectional", data.Links[0].Direction)
	require.Nil(t, data.Links[0].L2)
	require.Equal(t, 1, topologyStatsToV1(data.Stats)["links_unidirectional"].(int))
	require.Equal(t, 0, topologyStatsToV1(data.Stats)["links_bidirectional"].(int))
}

func TestTopologyRegistry_DefaultMapEmitsL3SubnetForManagedRoutersWithoutLLDP(t *testing.T) {
	registry := newTopologyRegistry()

	cacheA := newTopologyCache()
	cacheA.updateTime = time.Now()
	cacheA.lastUpdate = cacheA.updateTime
	cacheA.agentID = "agent-test"
	cacheA.localDevice = topologyDevice{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		SysName:       "router-a",
		ManagementIP:  "10.0.0.1",
	}
	cacheA.updateIfIndexByIP(map[string]string{
		tagTopoIfIndex: "2",
		tagTopoIPAddr:  "198.51.100.1",
		tagTopoIPMask:  "255.255.255.252",
	})
	cacheA.updateIfNameByIndex(map[string]string{
		tagTopoIfIndex: "2",
		tagTopoIfName:  "wan0",
	})

	cacheB := newTopologyCache()
	cacheB.updateTime = time.Now().Add(time.Second)
	cacheB.lastUpdate = cacheB.updateTime
	cacheB.agentID = "agent-test"
	cacheB.localDevice = topologyDevice{
		ChassisID:     "aa:bb:cc:dd:ee:ff",
		ChassisIDType: "macAddress",
		SysName:       "router-b",
		ManagementIP:  "10.0.0.2",
	}
	cacheB.updateIfIndexByIP(map[string]string{
		tagTopoIfIndex: "7",
		tagTopoIPAddr:  "198.51.100.2",
		tagTopoIPMask:  "255.255.255.252",
	})
	cacheB.updateIfNameByIndex(map[string]string{
		tagTopoIfIndex: "7",
		tagTopoIfName:  "wan7",
	})

	registry.register(cacheA)
	registry.register(cacheB)

	data, ok := snapshotTopologyRegistryForTest(registry)

	require.True(t, ok)
	require.Len(t, data.Actors, 2)
	require.Len(t, data.Links, 1)
	link := data.Links[0]
	require.Equal(t, "3", link.Layer)
	require.Equal(t, topologyL3SubnetLinkType, link.Protocol)
	require.Equal(t, topologyL3SubnetLinkType, link.LinkType)
	require.Equal(t, "observed", link.Direction)
	require.Equal(t, "198.51.100.0/30", topologyL3Subnet(link))
	require.Equal(t, "shared_subnet", topologymodel.LinkInferenceValue(link))
	require.Equal(t, "logical_l3_subnet", topologymodel.LinkAttachmentModeValue(link))
	require.Equal(t, "198.51.100.1", topologyEvidenceSrcIP(link))
	require.Equal(t, "198.51.100.2", topologyEvidenceDstIP(link))
	require.Equal(t, 1, topologyStatsToV1(data.Stats)["l3_subnet_emitted_links"])
	require.Equal(t, 1, topologyStatsToV1(data.Stats)["l3_subnet_visible_links"])
	require.Equal(t, 1, topologyStatsToV1(data.Stats)["links_total"])
}

func TestTopologyRegistry_OSPFSnapshotEnrichesSubnetAfterNeighborIngest(t *testing.T) {
	registry := newTopologyRegistry()

	cacheA := newTopologyCache()
	cacheA.updateTime = time.Now()
	cacheA.lastUpdate = cacheA.updateTime
	cacheA.agentID = "agent-test"
	cacheA.localDevice = topologyDevice{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		SysName:       "router-a",
		ManagementIP:  "10.0.0.1",
	}
	cacheA.updateTopologyProfileTags([]*ddsnmp.ProfileMetrics{{
		DeviceMetadata: map[string]ddsnmp.MetaTag{
			tagOSPFRouterID: {Value: "1.1.1.1"},
		},
	}})
	cacheA.updateTopologyCacheEntry(ddsnmp.Metric{
		TopologyKind: ddsnmp.KindOSPFNeighbor,
		Tags: map[string]string{
			tagOSPFNeighborRouterID:         "2.2.2.2",
			tagOSPFNeighborIP:               "198.51.100.2",
			tagOSPFNeighborAddresslessIndex: "0",
			tagOSPFNeighborState:            "full",
		},
	})
	cacheA.updateIfIndexByIP(map[string]string{
		tagTopoIfIndex: "2",
		tagTopoIPAddr:  "198.51.100.1",
		tagTopoIPMask:  "255.255.255.252",
	})

	cacheB := newTopologyCache()
	cacheB.updateTime = time.Now().Add(time.Second)
	cacheB.lastUpdate = cacheB.updateTime
	cacheB.agentID = "agent-test"
	cacheB.localDevice = topologyDevice{
		ChassisID:     "aa:bb:cc:dd:ee:ff",
		ChassisIDType: "macAddress",
		SysName:       "router-b",
		ManagementIP:  "10.0.0.2",
	}
	cacheB.updateTopologyProfileTags([]*ddsnmp.ProfileMetrics{{
		DeviceMetadata: map[string]ddsnmp.MetaTag{
			tagOSPFRouterID: {Value: "2.2.2.2"},
		},
	}})
	cacheB.updateIfIndexByIP(map[string]string{
		tagTopoIfIndex: "7",
		tagTopoIPAddr:  "198.51.100.2",
		tagTopoIPMask:  "255.255.255.252",
	})

	registry.register(cacheA)
	registry.register(cacheB)

	data, ok := snapshotTopologyRegistryForTest(registry)

	require.True(t, ok)
	require.Len(t, data.Links, 2)
	require.Equal(t, 1, countTopologyLinksByType(data.Links, topologyL3SubnetLinkType))
	require.Equal(t, 1, countTopologyLinksByType(data.Links, topologyOSPFAdjacencyLinkType))
	require.Equal(t, 1, topologyStatsToV1(data.Stats)["l3_subnet_emitted_links"])
	require.Equal(t, 1, topologyStatsToV1(data.Stats)["l3_subnet_visible_links"])
	require.Equal(t, 1, topologyStatsToV1(data.Stats)["ospf_adjacency_emitted_links"])
	require.Equal(t, 1, topologyStatsToV1(data.Stats)["ospf_adjacency_visible_links"])
}

func TestTopologyRegistry_BGPAdjacencyEmitsEstablishedManagedPeerLinkAndDetailRows(t *testing.T) {
	registry := newTopologyRegistry()

	cacheA := newTopologyCache()
	cacheA.updateTime = time.Now()
	cacheA.lastUpdate = cacheA.updateTime
	cacheA.agentID = "agent-test"
	cacheA.localDevice = topologyDevice{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		SysName:       "router-a",
		ManagementIP:  "10.0.0.1",
	}
	cacheA.bgpPeersByKey["a"] = topologyBGPPeer{
		RoutingInstance: "default",
		NeighborIP:      "198.51.100.2",
		RemoteAS:        "65002",
		LocalIP:         "198.51.100.1",
		LocalAS:         "65001",
		LocalIdentifier: "1.1.1.1",
		PeerIdentifier:  "2.2.2.2",
		State:           "established",
	}

	cacheB := newTopologyCache()
	cacheB.updateTime = time.Now().Add(time.Second)
	cacheB.lastUpdate = cacheB.updateTime
	cacheB.agentID = "agent-test"
	cacheB.localDevice = topologyDevice{
		ChassisID:     "aa:bb:cc:dd:ee:ff",
		ChassisIDType: "macAddress",
		SysName:       "router-b",
		ManagementIP:  "10.0.0.2",
	}
	cacheB.bgpPeersByKey["b"] = topologyBGPPeer{
		RoutingInstance: "default",
		NeighborIP:      "198.51.100.1",
		RemoteAS:        "65001",
		LocalIP:         "198.51.100.2",
		LocalAS:         "65002",
		LocalIdentifier: "2.2.2.2",
		PeerIdentifier:  "1.1.1.1",
		State:           "established",
	}

	registry.register(cacheA)
	registry.register(cacheB)

	data, ok := snapshotTopologyRegistryForTest(registry)

	require.True(t, ok)
	require.Len(t, data.Links, 1)
	link := data.Links[0]
	require.Equal(t, "3", link.Layer)
	require.Equal(t, topologyBGPAdjacencyLinkType, link.Protocol)
	require.Equal(t, topologyBGPAdjacencyLinkType, link.LinkType)
	require.Equal(t, "observed", link.Direction)
	require.Equal(t, "established", link.State)
	require.Equal(t, "bgp_established_adjacency", topologymodel.LinkInferenceValue(link))
	require.Equal(t, "logical_l3_bgp", topologymodel.LinkAttachmentModeValue(link))
	require.Equal(t, "default", topologyBGPLinkRoutingInstance(link))
	require.Equal(t, "65001", topologyBGPLocalAS(link))
	require.Equal(t, "65002", topologyBGPRemoteAS(link))
	require.Equal(t, 2, topologyStatsToV1(data.Stats)["bgp_peer_rows"])
	require.Equal(t, 2, topologyStatsToV1(data.Stats)["bgp_peer_detail_rows"])
	require.Equal(t, 1, topologyStatsToV1(data.Stats)["bgp_adjacency_emitted_links"])
	require.Equal(t, 1, topologyStatsToV1(data.Stats)["bgp_adjacency_suppressed_duplicate_link"])
	require.Equal(t, 1, topologyStatsToV1(data.Stats)["bgp_adjacency_visible_links"])

	routerA := findDeviceActorBySysName(data, "router-a")
	require.NotNil(t, routerA)
	routerB := findDeviceActorBySysName(data, "router-b")
	require.NotNil(t, routerB)
	require.Len(t, routerA.Detail.BGP, 1)
	require.Equal(t, routerB.ActorID, routerA.Detail.BGP[0].RemoteActorID)
}

func TestTopologyRegistry_BGPAdjacencyKeepsUnresolvedAndNonEstablishedPeersAsDetails(t *testing.T) {
	registry := newTopologyRegistry()

	cache := newTopologyCache()
	cache.updateTime = time.Now()
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent-test"
	cache.localDevice = topologyDevice{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		SysName:       "router-a",
		ManagementIP:  "10.0.0.1",
	}
	cache.bgpPeersByKey["unresolved"] = topologyBGPPeer{
		RoutingInstance: "default",
		NeighborIP:      "203.0.113.2",
		RemoteAS:        "65002",
		LocalIP:         "198.51.100.1",
		LocalAS:         "65001",
		LocalIdentifier: "1.1.1.1",
		PeerIdentifier:  "2.2.2.2",
		State:           "established",
	}
	cache.bgpPeersByKey["idle"] = topologyBGPPeer{
		RoutingInstance: "default",
		NeighborIP:      "203.0.113.3",
		RemoteAS:        "65003",
		LocalIP:         "198.51.100.1",
		LocalAS:         "65001",
		LocalIdentifier: "1.1.1.1",
		PeerIdentifier:  "3.3.3.3",
		State:           "idle",
	}

	registry.register(cache)

	data, ok := snapshotTopologyRegistryForTest(registry)

	require.True(t, ok)
	require.Empty(t, data.Links)
	require.Equal(t, 2, topologyStatsToV1(data.Stats)["bgp_peer_rows"])
	require.Equal(t, 2, topologyStatsToV1(data.Stats)["bgp_peer_detail_rows"])
	require.Equal(t, 1, topologyStatsToV1(data.Stats)["bgp_adjacency_suppressed_unresolved_neighbor"])
	require.Equal(t, 1, topologyStatsToV1(data.Stats)["bgp_adjacency_suppressed_non_established_state"])
	require.Equal(t, 0, topologyStatsToV1(data.Stats)["bgp_adjacency_visible_links"])

	routerA := findDeviceActorBySysName(data, "router-a")
	require.NotNil(t, routerA)
	require.Len(t, routerA.Detail.BGP, 2)
	for _, row := range routerA.Detail.BGP {
		require.Empty(t, row.RemoteActorID)
	}
}

func TestTopologyRegistry_SnapshotWithOptions_LLDPManagedKeepsRequestedMapType(t *testing.T) {
	registry := newTopologyRegistry()
	registry.register(newTestTopologyCacheLLDP(
		"agent-test",
		time.Now().UTC(),
		"00:11:22:33:44:55",
		"sw-a",
		"10.0.0.1",
		"Gi0/1",
		"aa:bb:cc:dd:ee:ff",
		"sw-b",
		"10.0.0.2",
		"Gi0/2",
	))

	data, ok := registry.snapshotWithOptions(topologyQueryOptions{
		CollapseActorsByIP:     true,
		EliminateNonIPInferred: true,
		MapType:                topologyMapTypeLLDPCDPManaged,
		ManagedDeviceFocus:     topologyManagedFocusAllDevices,
		Depth:                  topologyDepthAllInternal,
	})
	require.True(t, ok)
	require.Equal(t, topologyMapTypeLLDPCDPManaged, topologyStatsToV1(data.Stats)["map_type"])
	require.Equal(t, topologyInferenceStrategyFDBMinimumKnowledge, topologyStatsToV1(data.Stats)["inference_strategy"])
}

func TestTopologyRegistry_SnapshotWithOptions_CollapseByIPPreservesEngineManagedOverlapPruning(t *testing.T) {
	registry := newTopologyRegistry()

	cache := newTopologyCache()
	cache.updateTime = time.Now().UTC()
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent-test"
	cache.localDevice = topologyDevice{
		ChassisID:     "aa:aa:aa:aa:aa:aa",
		ChassisIDType: "macAddress",
		SysName:       "switch-a",
		ManagementIP:  "10.0.0.1",
	}
	cache.lldpLocPorts["1"] = &lldpLocPort{
		portNum:       "1",
		portID:        "Gi0/1",
		portIDSubtype: "interfaceName",
	}
	cache.lldpRemotes["1:1"] = &lldpRemote{
		localPortNum:     "1",
		remIndex:         "1",
		chassisID:        "9c:6b:00:7b:98:c6",
		chassisIDSubtype: "macAddress",
		portID:           "9c:6b:00:7b:98:c7",
		portIDSubtype:    "macAddress",
		sysName:          "nova",
		managementAddr:   "172.22.0.1",
	}
	cache.ifNamesByIndex["1"] = "Gi0/1"
	cache.ifNamesByIndex["2"] = "Gi0/2"
	cache.bridgePortToIf["2"] = "2"
	cache.fdbEntries["9c:6b:00:7b:98:c7|2||"] = &fdbEntry{
		mac:        "9c:6b:00:7b:98:c7",
		bridgePort: "2",
	}
	cache.arpEntries["2|10.20.4.22|9c:6b:00:7b:98:c7"] = &arpEntry{
		ifIndex: "2",
		ifName:  "Gi0/2",
		ip:      "10.20.4.22",
		mac:     "9c:6b:00:7b:98:c7",
	}
	registry.register(cache)

	withoutCollapse, ok := registry.snapshotWithOptions(topologyQueryOptions{
		MapType:            topologyMapTypeAllDevicesLowConfidence,
		ManagedDeviceFocus: topologyManagedFocusAllDevices,
		Depth:              topologyDepthAllInternal,
	})
	require.True(t, ok)
	require.NotNil(t, findActorByMAC(withoutCollapse, "9c:6b:00:7b:98:c7"))

	withCollapse, ok := registry.snapshotWithOptions(topologyQueryOptions{
		CollapseActorsByIP: true,
		MapType:            topologyMapTypeAllDevicesLowConfidence,
		ManagedDeviceFocus: topologyManagedFocusAllDevices,
		Depth:              topologyDepthAllInternal,
	})
	require.True(t, ok)
	require.NotNil(t, findActorByMAC(withCollapse, "9c:6b:00:7b:98:c6"))
	require.Nil(t, findActorByMAC(withCollapse, "9c:6b:00:7b:98:c7"))
	require.Equal(t, 1, topologyStatsToV1(withCollapse.Stats)["actors_unlinked_suppressed"])
}

func TestTopologyRegistry_ManagedDeviceFocusTargets_ReturnsPerDeviceIPTargets(t *testing.T) {
	registry := newTopologyRegistry()
	registry.register(newTestTopologyCacheLLDP(
		"agent-test",
		time.Now().UTC(),
		"00:11:22:33:44:55",
		"sw-a",
		"10.0.0.1",
		"Gi0/1",
		"aa:bb:cc:dd:ee:ff",
		"sw-b",
		"10.0.0.2",
		"Gi0/2",
	))

	targets := registry.managedDeviceFocusTargets()
	require.Len(t, targets, 1)
	require.Equal(t, "ip:10.0.0.1", targets[0].Value)
	require.Equal(t, "sw-a (10.0.0.1)", targets[0].Name)
}

func TestTopologyCache_SnapshotEngineObservationsUsesDirectLocalObservation(t *testing.T) {
	cache := newTopologyCache()
	cache.updateTime = time.Now()
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent-test"
	cache.localDevice = topologyDevice{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		SysName:       "sw-a",
		ManagementIP:  "10.0.0.1",
	}
	cache.lldpLocPorts["1"] = &lldpLocPort{
		portNum:       "1",
		portID:        "Gi0/1",
		portIDSubtype: "interfaceName",
	}
	cache.lldpRemotes["1:1"] = &lldpRemote{
		localPortNum:     "1",
		remIndex:         "1",
		chassisID:        "aa:bb:cc:dd:ee:ff",
		chassisIDSubtype: "macAddress",
		portID:           "Gi0/2",
		portIDSubtype:    "interfaceName",
		sysName:          "sw-b",
		managementAddr:   "10.0.0.2",
	}
	cache.cdpRemotes["1:1"] = &cdpRemote{
		ifIndex:    "1",
		ifName:     "Gi0/1",
		deviceID:   "sw-b",
		sysName:    "sw-b",
		devicePort: "Gi0/2",
		address:    "10.0.0.2",
	}

	snapshot, ok := cache.snapshotEngineObservations()
	require.True(t, ok)
	require.Len(t, snapshot.L2Observations, 1)
	require.Equal(t, snapshot.LocalDeviceID, snapshot.L2Observations[0].DeviceID)
	require.Len(t, snapshot.L2Observations[0].LLDPRemotes, 1)
	require.Len(t, snapshot.L2Observations[0].CDPRemotes, 1)
}

func TestTopologyCache_SnapshotEngineObservationsIncludesL3Interfaces(t *testing.T) {
	cache := newTopologyCache()
	cache.updateTime = time.Now()
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent-test"
	cache.localDevice = topologyDevice{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		SysName:       "router-a",
		ManagementIP:  "10.0.0.1",
	}
	cache.updateIfIndexByIP(map[string]string{
		tagTopoIfIndex: "2",
		tagTopoIPAddr:  "198.51.100.1",
		tagTopoIPMask:  "255.255.255.252",
	})
	cache.updateIfNameByIndex(map[string]string{
		tagTopoIfIndex: "2",
		tagTopoIfName:  "Gi0/2",
		tagTopoIfDescr: "Uplink",
	})
	cache.updateIfIndexByIP(map[string]string{
		tagTopoIfIndex: "3",
		tagTopoIPAddr:  "2001:db8::1",
	})

	snapshot, ok := cache.snapshotEngineObservations()

	require.True(t, ok)
	require.Len(t, snapshot.L3Interfaces, 1)
	require.Equal(t, topologyL3Interface{
		DeviceID: snapshot.LocalDeviceID,
		IP:       "198.51.100.1",
		Netmask:  "255.255.255.252",
		IfIndex:  "2",
		IfName:   "Gi0/2",
		IfDescr:  "Uplink",
	}, snapshot.L3Interfaces[0])
}

func TestAggregateTopologyObservationSnapshotsIncludesL3Interfaces(t *testing.T) {
	collectedAt := time.Now()
	snapshots := []topologyObservationSnapshot{
		{
			LocalDeviceID: "device-a",
			AgentID:       "agent-a",
			CollectedAt:   collectedAt,
			L2Observations: []topologyengine.L2Observation{{
				DeviceID: "device-a",
			}},
			L3Interfaces: []topologyL3Interface{{
				DeviceID: "device-a",
				IP:       "198.51.100.1",
				Netmask:  "255.255.255.252",
				IfIndex:  "2",
			}},
		},
	}

	aggregate, ok := aggregateTopologyObservationSnapshots(snapshots)

	require.True(t, ok)
	require.Len(t, aggregate.L3Interfaces, 1)
	require.Equal(t, snapshots[0].L3Interfaces[0], aggregate.L3Interfaces[0])
}

func TestTopologyRegistry_SnapshotReturnsFalseWithoutCollectedCaches(t *testing.T) {
	registry := newTopologyRegistry()
	cache := newTopologyCache()
	registry.register(cache)

	_, ok := snapshotTopologyRegistryForTest(registry)
	require.False(t, ok)
}

func TestTopologyRegistry_SnapshotDeterministicAcrossRepeatedCalls(t *testing.T) {
	registry := newTopologyRegistry()

	cacheA := newTopologyCache()
	cacheA.updateTime = time.Now()
	cacheA.lastUpdate = cacheA.updateTime
	cacheA.agentID = "agent-test"
	cacheA.localDevice = topologyDevice{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		SysName:       "sw-a",
		ManagementIP:  "10.0.0.1",
	}
	cacheA.lldpLocPorts["1"] = &lldpLocPort{
		portNum:       "1",
		portID:        "Gi0/1",
		portIDSubtype: "interfaceName",
	}
	cacheA.lldpRemotes["1:1"] = &lldpRemote{
		localPortNum:     "1",
		remIndex:         "1",
		chassisID:        "aa:bb:cc:dd:ee:ff",
		chassisIDSubtype: "macAddress",
		portID:           "Gi0/2",
		portIDSubtype:    "interfaceName",
		sysName:          "sw-b",
		managementAddr:   "10.0.0.2",
	}

	cacheB := newTopologyCache()
	cacheB.updateTime = time.Now().Add(time.Second)
	cacheB.lastUpdate = cacheB.updateTime
	cacheB.agentID = "agent-test"
	cacheB.localDevice = topologyDevice{
		ChassisID:     "aa:bb:cc:dd:ee:ff",
		ChassisIDType: "macAddress",
		SysName:       "sw-b",
		ManagementIP:  "10.0.0.2",
	}
	cacheB.lldpLocPorts["1"] = &lldpLocPort{
		portNum:       "1",
		portID:        "Gi0/2",
		portIDSubtype: "interfaceName",
	}
	cacheB.lldpRemotes["1:1"] = &lldpRemote{
		localPortNum:     "1",
		remIndex:         "1",
		chassisID:        "00:11:22:33:44:55",
		chassisIDSubtype: "macAddress",
		portID:           "Gi0/1",
		portIDSubtype:    "interfaceName",
		sysName:          "sw-a",
		managementAddr:   "10.0.0.1",
	}

	registry.register(cacheA)
	registry.register(cacheB)

	baseline, ok := snapshotTopologyRegistryForTest(registry)
	require.True(t, ok)
	require.NotEmpty(t, baseline.Actors)
	require.NotEmpty(t, baseline.Links)

	for range 10 {
		next, ok := snapshotTopologyRegistryForTest(registry)
		require.True(t, ok)
		require.Equal(t, baseline, next)
	}
}

func TestTopologyRegistry_SnapshotDeduplicatesDuplicateDeviceObservations(t *testing.T) {
	registry := newTopologyRegistry()

	cacheA := newTopologyCache()
	cacheA.updateTime = time.Now()
	cacheA.lastUpdate = cacheA.updateTime
	cacheA.agentID = "agent-test"
	cacheA.localDevice = topologyDevice{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		SysName:       "sw-a",
		ManagementIP:  "10.0.0.1",
	}
	cacheA.lldpLocPorts["1"] = &lldpLocPort{
		portNum:       "1",
		portID:        "Gi0/1",
		portIDSubtype: "interfaceName",
	}
	cacheA.lldpRemotes["1:1"] = &lldpRemote{
		localPortNum:     "1",
		remIndex:         "1",
		chassisID:        "aa:bb:cc:dd:ee:ff",
		chassisIDSubtype: "macAddress",
		portID:           "Gi0/2",
		portIDSubtype:    "interfaceName",
		sysName:          "sw-b",
		managementAddr:   "10.0.0.2",
	}

	cacheB := newTopologyCache()
	cacheB.updateTime = cacheA.updateTime
	cacheB.lastUpdate = cacheA.lastUpdate
	cacheB.agentID = cacheA.agentID
	cacheB.localDevice = cacheA.localDevice
	cacheB.lldpLocPorts["1"] = cacheA.lldpLocPorts["1"]
	cacheB.lldpRemotes["1:1"] = cacheA.lldpRemotes["1:1"]

	registry.register(cacheA)
	registry.register(cacheB)

	data, ok := snapshotTopologyRegistryForTest(registry)
	require.True(t, ok)

	require.Len(t, data.Links, 1)
	require.Equal(t, 1, topologyStatsToV1(data.Stats)["links_total"])
	require.Equal(t, 2, countActorsByType(data, "device"))
}

func TestCanonicalMatchKey_NormalizesEquivalentMACRepresentations(t *testing.T) {
	raw := topologyMatch{ChassisIDs: []string{"7049a26572cd"}}
	colon := topologyMatch{MacAddresses: []string{"70:49:A2:65:72:CD"}}
	require.Equal(t, "mac:70:49:a2:65:72:cd", topologymodel.CanonicalMatchKey(raw))
	require.Equal(t, "mac:70:49:a2:65:72:cd", topologymodel.CanonicalMatchKey(colon))
	require.Contains(t, topologymodel.MatchIdentityKeys(raw), "hw:70:49:a2:65:72:cd")
	require.Contains(t, topologymodel.MatchIdentityKeys(colon), "hw:70:49:a2:65:72:cd")
}

func TestApplySNMPTopologyShapePolicies_CollapsesActorsByIP(t *testing.T) {
	data := topologyData{
		Actors: []topologyActor{
			{
				ActorID:   "device:a",
				ActorType: "device",
				Match: topologyMatch{
					IPAddresses:  []string{"10.0.0.10"},
					MacAddresses: []string{"aa:aa:aa:aa:aa:aa"},
				},
			},
			{
				ActorID:   "endpoint:b",
				ActorType: "endpoint",
				Match: topologyMatch{
					IPAddresses:  []string{"10.0.0.10"},
					MacAddresses: []string{"bb:bb:bb:bb:bb:bb"},
				},
			},
		},
		Links: []topologyLink{
			{
				SrcActorID: "endpoint:b",
				DstActorID: "device:a",
				Protocol:   "fdb",
				Direction:  "bidirectional",
			},
		},
	}

	topologyshape.ApplyPolicies(&data, topologyQueryOptions{
		CollapseActorsByIP: true,
		MapType:            topologyMapTypeHighConfidenceInferred,
	})

	require.Len(t, data.Actors, 1)
	require.Len(t, data.Links, 0)
	require.Equal(t, 1, topologyStatsToV1(data.Stats)["actors_collapsed_by_ip"])
}

func TestApplySNMPTopologyShapePolicies_EliminatesNonIPInferredActorsAndSparseSegments(t *testing.T) {
	data := topologyData{
		Actors: []topologyActor{
			{
				ActorID:   "segment:s1",
				ActorType: "segment",
				Match: topologyMatch{
					Hostnames: []string{"segment:s1"},
				},
			},
			{
				ActorID:   "endpoint:e1",
				ActorType: "endpoint",
				Match: topologyMatch{
					MacAddresses: []string{"cc:cc:cc:cc:cc:cc"},
				},
			},
		},
		Links: []topologyLink{
			{
				SrcActorID: "segment:s1",
				DstActorID: "endpoint:e1",
				Protocol:   "fdb",
				Direction:  "bidirectional",
			},
		},
	}

	topologyshape.ApplyPolicies(&data, topologyQueryOptions{
		EliminateNonIPInferred: true,
		MapType:                topologyMapTypeHighConfidenceInferred,
	})

	require.Len(t, data.Actors, 0)
	require.Len(t, data.Links, 0)
	require.Equal(t, 1, topologyStatsToV1(data.Stats)["actors_non_ip_inferred_suppressed"])
	require.Equal(t, 1, topologyStatsToV1(data.Stats)["segments_sparse_suppressed"])
}

func TestApplySNMPTopologyShapePolicies_HighConfidenceSuppressesUnlinkedInferredEndpoints(t *testing.T) {
	data := topologyData{
		Actors: []topologyActor{
			{
				ActorID:   "device:d1",
				ActorType: "device",
				Source:    "snmp",
				Match:     topologyMatch{IPAddresses: []string{"10.0.0.1"}},
			},
			{
				ActorID:   "endpoint:linked",
				ActorType: "endpoint",
				Source:    "snmp",
				Match:     topologyMatch{IPAddresses: []string{"10.0.0.2"}},
			},
			{
				ActorID:   "endpoint:unlinked",
				ActorType: "endpoint",
				Source:    "snmp",
				Match:     topologyMatch{IPAddresses: []string{"10.0.0.3"}},
			},
		},
		Links: []topologyLink{
			{
				SrcActorID: "device:d1",
				DstActorID: "endpoint:linked",
				Protocol:   "fdb",
				Direction:  "bidirectional",
			},
		},
	}

	topologyshape.ApplyPolicies(&data, topologyQueryOptions{
		MapType: topologyMapTypeHighConfidenceInferred,
	})

	require.Len(t, data.Actors, 2)
	require.Equal(t, 1, topologyStatsToV1(data.Stats)["actors_map_type_suppressed"])
	for _, actor := range data.Actors {
		require.NotEqual(t, "endpoint:unlinked", actor.ActorID)
	}
}

func TestApplySNMPTopologyShapePolicies_LLDPManagedMapKeepsOnlyLLDPCDPAndManagedDevices(t *testing.T) {
	data := topologyData{
		Actors: []topologyActor{
			{
				ActorID:   "device:d1",
				ActorType: "device",
				Source:    "snmp",
				Match:     topologyMatch{IPAddresses: []string{"10.0.0.1"}},
			},
			{
				ActorID:   "device:d2",
				ActorType: "device",
				Source:    "snmp",
				Match:     topologyMatch{IPAddresses: []string{"10.0.0.2"}},
			},
			{
				ActorID:   "endpoint:e1",
				ActorType: "endpoint",
				Source:    "snmp",
				Match:     topologyMatch{IPAddresses: []string{"10.0.0.3"}},
			},
		},
		Links: []topologyLink{
			{
				SrcActorID: "device:d1",
				DstActorID: "device:d2",
				Protocol:   "lldp",
				Direction:  "bidirectional",
			},
			{
				SrcActorID: "device:d1",
				DstActorID: "endpoint:e1",
				Protocol:   "fdb",
				Direction:  "bidirectional",
			},
		},
	}

	topologyshape.ApplyPolicies(&data, topologyQueryOptions{
		MapType: topologyMapTypeLLDPCDPManaged,
	})

	require.Len(t, data.Actors, 2)
	require.Len(t, data.Links, 1)
	require.Equal(t, "lldp", data.Links[0].Protocol)
	require.Equal(t, 1, topologyStatsToV1(data.Stats)["actors_map_type_suppressed"])
}

func TestMarkProbableDeltaLinks_MarksAllAddedLinksAsProbable(t *testing.T) {
	strictData := topologyData{
		Links: []topologyLink{
			{
				SrcActorID: "device:d1",
				DstActorID: "device:d2",
				Protocol:   "lldp",
				Direction:  "bidirectional",
			},
		},
	}
	probableData := topologyData{
		Links: []topologyLink{
			{
				SrcActorID: "device:d1",
				DstActorID: "device:d2",
				Protocol:   "lldp",
				Direction:  "bidirectional",
			},
			{
				SrcActorID: "device:d1",
				DstActorID: "segment:s1",
				Protocol:   "bridge",
				Direction:  "bidirectional",
				L2: &graph.LinkL2{
					BridgeDomain: "bridge-domain:s1",
				},
			},
		},
	}

	topologyshape.MarkProbableDeltaLinks(&strictData, &probableData)

	require.Len(t, probableData.Links, 2)
	require.Equal(t, "", probableData.Links[0].State)
	require.Equal(t, "probable", probableData.Links[1].State)
	require.Equal(t, "probable", topologymodel.LinkInferenceValue(probableData.Links[1]))
	require.Equal(t, "probable_bridge_anchor", topologymodel.LinkAttachmentModeValue(probableData.Links[1]))
}

func TestApplyTopologyDepthFocusFilter_ManagedFocusDepthZero(t *testing.T) {
	data := topologyData{
		Actors: []topologyActor{
			{
				ActorID:   "device:managed-a",
				ActorType: "device",
				Source:    "snmp",
				Match:     topologyMatch{IPAddresses: []string{"10.0.0.1"}},
			},
			{
				ActorID:   "device:managed-b",
				ActorType: "device",
				Source:    "snmp",
				Match:     topologyMatch{IPAddresses: []string{"10.0.0.2"}},
			},
			{
				ActorID:   "endpoint:e1",
				ActorType: "endpoint",
				Source:    "snmp",
				Match:     topologyMatch{IPAddresses: []string{"10.0.0.3"}},
			},
			{
				ActorID:   "segment:s1",
				ActorType: "segment",
				Source:    "snmp",
				Match:     topologyMatch{Hostnames: []string{"segment:s1"}},
			},
		},
		Links: []topologyLink{
			{
				SrcActorID: "device:managed-a",
				DstActorID: "device:managed-b",
				Protocol:   "lldp",
				Direction:  "bidirectional",
			},
			{
				SrcActorID: "device:managed-a",
				DstActorID: "segment:s1",
				Protocol:   "bridge",
				Direction:  "bidirectional",
			},
			{
				SrcActorID: "segment:s1",
				DstActorID: "endpoint:e1",
				Protocol:   "fdb",
				Direction:  "bidirectional",
			},
		},
	}

	topologyshape.ApplyDepthFocusFilter(&data, topologyQueryOptions{
		ManagedDeviceFocus:     "ip:10.0.0.1",
		Depth:                  0,
		EliminateNonIPInferred: true,
	})

	require.Len(t, data.Actors, 1)
	require.Len(t, data.Links, 0)
	require.Equal(t, "ip:10.0.0.1", topologyStatsToV1(data.Stats)["managed_snmp_device_focus"])
	require.Equal(t, 0, topologyStatsToV1(data.Stats)["depth"])
}

func TestApplyTopologyDepthFocusFilter_ManagedFocusDepthOneIncludesDirectNeighbors(t *testing.T) {
	data := topologyData{
		Actors: []topologyActor{
			{
				ActorID:   "device:managed-a",
				ActorType: "device",
				Source:    "snmp",
				Match:     topologyMatch{IPAddresses: []string{"10.0.0.1"}},
			},
			{
				ActorID:   "device:managed-b",
				ActorType: "device",
				Source:    "snmp",
				Match:     topologyMatch{IPAddresses: []string{"10.0.0.2"}},
			},
			{
				ActorID:   "endpoint:e1",
				ActorType: "endpoint",
				Source:    "snmp",
				Match:     topologyMatch{IPAddresses: []string{"10.0.0.3"}},
			},
			{
				ActorID:   "segment:s1",
				ActorType: "segment",
				Source:    "snmp",
				Match:     topologyMatch{Hostnames: []string{"segment:s1"}},
			},
		},
		Links: []topologyLink{
			{
				SrcActorID: "device:managed-a",
				DstActorID: "device:managed-b",
				Protocol:   "lldp",
				Direction:  "bidirectional",
			},
			{
				SrcActorID: "device:managed-a",
				DstActorID: "segment:s1",
				Protocol:   "bridge",
				Direction:  "bidirectional",
			},
			{
				SrcActorID: "segment:s1",
				DstActorID: "endpoint:e1",
				Protocol:   "fdb",
				Direction:  "bidirectional",
			},
		},
	}

	topologyshape.ApplyDepthFocusFilter(&data, topologyQueryOptions{
		ManagedDeviceFocus:     "ip:10.0.0.1",
		Depth:                  1,
		EliminateNonIPInferred: true,
	})

	require.Len(t, data.Actors, 4)
	require.Len(t, data.Links, 3)
	require.Equal(t, "ip:10.0.0.1", topologyStatsToV1(data.Stats)["managed_snmp_device_focus"])
	require.Equal(t, 1, topologyStatsToV1(data.Stats)["depth"])
}

func TestApplyTopologyDepthFocusFilter_MultiFocusDepthZeroIncludesAllShortestPaths(t *testing.T) {
	data := topologyData{
		Actors: []topologyActor{
			{
				ActorID:   "device:managed-a",
				ActorType: "device",
				Source:    "snmp",
				Match:     topologyMatch{IPAddresses: []string{"10.0.0.1"}},
			},
			{
				ActorID:   "device:managed-b",
				ActorType: "device",
				Source:    "snmp",
				Match:     topologyMatch{IPAddresses: []string{"10.0.0.2"}},
			},
			{
				ActorID:   "device:managed-c",
				ActorType: "device",
				Source:    "snmp",
				Match:     topologyMatch{IPAddresses: []string{"10.0.0.3"}},
			},
			{
				ActorID:   "segment:s1",
				ActorType: "segment",
				Source:    "snmp",
				Match:     topologyMatch{Hostnames: []string{"segment:s1"}},
			},
		},
		Links: []topologyLink{
			{
				SrcActorID: "device:managed-a",
				DstActorID: "device:managed-b",
				Protocol:   "lldp",
				Direction:  "bidirectional",
			},
			{
				SrcActorID: "device:managed-b",
				DstActorID: "device:managed-c",
				Protocol:   "lldp",
				Direction:  "bidirectional",
			},
			{
				SrcActorID: "device:managed-a",
				DstActorID: "segment:s1",
				Protocol:   "bridge",
				Direction:  "bidirectional",
			},
			{
				SrcActorID: "segment:s1",
				DstActorID: "device:managed-c",
				Protocol:   "fdb",
				Direction:  "bidirectional",
			},
		},
	}

	topologyshape.ApplyDepthFocusFilter(&data, topologyQueryOptions{
		ManagedDeviceFocus:     "ip:10.0.0.3,ip:10.0.0.1",
		Depth:                  0,
		EliminateNonIPInferred: true,
	})

	actorIDs := make([]string, 0, len(data.Actors))
	for _, actor := range data.Actors {
		actorIDs = append(actorIDs, actor.ActorID)
	}
	assert.ElementsMatch(
		t,
		[]string{"device:managed-a", "device:managed-b", "device:managed-c", "segment:s1"},
		actorIDs,
	)
	require.Len(t, data.Links, 4)
	require.Equal(t, "ip:10.0.0.1,ip:10.0.0.3", topologyStatsToV1(data.Stats)["managed_snmp_device_focus"])
	require.Equal(t, 0, topologyStatsToV1(data.Stats)["depth"])
}

func TestApplyTopologyDepthFocusFilter_DepthExpandsFromSelectedRootsOnly(t *testing.T) {
	data := topologyData{
		Actors: []topologyActor{
			{
				ActorID:   "device:managed-a",
				ActorType: "device",
				Source:    "snmp",
				Match:     topologyMatch{IPAddresses: []string{"10.0.0.1"}},
			},
			{
				ActorID:   "device:managed-b",
				ActorType: "device",
				Source:    "snmp",
				Match:     topologyMatch{IPAddresses: []string{"10.0.0.2"}},
			},
			{
				ActorID:   "device:managed-c",
				ActorType: "device",
				Source:    "snmp",
				Match:     topologyMatch{IPAddresses: []string{"10.0.0.3"}},
			},
			{
				ActorID:   "endpoint:x",
				ActorType: "endpoint",
				Source:    "snmp",
				Match:     topologyMatch{IPAddresses: []string{"10.0.0.50"}},
			},
		},
		Links: []topologyLink{
			{
				SrcActorID: "device:managed-a",
				DstActorID: "device:managed-b",
				Protocol:   "lldp",
				Direction:  "bidirectional",
			},
			{
				SrcActorID: "device:managed-b",
				DstActorID: "device:managed-c",
				Protocol:   "lldp",
				Direction:  "bidirectional",
			},
			{
				SrcActorID: "device:managed-b",
				DstActorID: "endpoint:x",
				Protocol:   "fdb",
				Direction:  "bidirectional",
			},
		},
	}

	topologyshape.ApplyDepthFocusFilter(&data, topologyQueryOptions{
		ManagedDeviceFocus:     "ip:10.0.0.1,ip:10.0.0.3",
		Depth:                  1,
		EliminateNonIPInferred: true,
	})

	actorIDs := make([]string, 0, len(data.Actors))
	for _, actor := range data.Actors {
		actorIDs = append(actorIDs, actor.ActorID)
	}
	assert.ElementsMatch(
		t,
		[]string{"device:managed-a", "device:managed-b", "device:managed-c"},
		actorIDs,
	)
	for _, link := range data.Links {
		assert.False(t, link.SrcActorID == "endpoint:x" || link.DstActorID == "endpoint:x")
	}
}

func countActorsByType(data topologyData, actorType string) int {
	total := 0
	for _, actor := range data.Actors {
		if actor.ActorType == actorType {
			total++
		}
	}
	return total
}
