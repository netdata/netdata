// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"
	"time"

	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyshape"
	"github.com/stretchr/testify/require"
)

func TestSNMPTopologyToV1_RealPipelineStatsCensus(t *testing.T) {
	registry := newTopologyRegistry()

	cacheA := newTopologyStatsCensusRouterCache(t, "router-a", "00:11:22:33:44:55", "10.0.0.1", "1.1.1.1", "198.51.100.1", "2")
	addTopologyStatsCensusLLDP(cacheA, "xe-0/0/0", "aa:bb:cc:dd:ee:ff", "router-b", "10.0.0.2", "xe-0/0/1")
	cacheA.updateTopologyCacheEntry(ddsnmp.Metric{
		TopologyKind: ddsnmp.KindOSPFNeighbor,
		Tags: map[string]string{
			tagOSPFNeighborRouterID:         "2.2.2.2",
			tagOSPFNeighborIP:               "198.51.100.2",
			tagOSPFNeighborAddresslessIndex: "0",
			tagOSPFNeighborState:            "full",
		},
	})
	cacheA.bgpPeersByKey["peer-router-b"] = topologyBGPPeer{
		RoutingInstance: "default",
		NeighborIP:      "198.51.100.2",
		RemoteAS:        "65002",
		LocalIP:         "198.51.100.1",
		LocalAS:         "65001",
		LocalIdentifier: "1.1.1.1",
		PeerIdentifier:  "2.2.2.2",
		State:           "established",
	}

	cacheB := newTopologyStatsCensusRouterCache(t, "router-b", "aa:bb:cc:dd:ee:ff", "10.0.0.2", "2.2.2.2", "198.51.100.2", "7")
	cacheB.bgpPeersByKey["peer-router-a"] = topologyBGPPeer{
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

	options := defaultTopologyQueryOptionsForTest()
	options.MapType = topologyMapTypeAllDevicesLowConfidence
	data, ok := snapshotTopologyRegistryForTestWithOptions(registry, options)
	require.True(t, ok)

	payload, err := snmpTopologyToV1(data)
	require.NoError(t, err)
	require.NotNil(t, payload.Stats)

	require.ElementsMatch(t, []string{
		"devices_total",
		"devices_discovered",
		"links_total",
		"links_lldp",
		"links_cdp",
		"links_stp",
		"links_bidirectional",
		"links_unidirectional",
		"links_fdb",
		"links_fdb_endpoint_candidates",
		"links_fdb_endpoint_emitted",
		"links_fdb_endpoint_suppressed",
		"endpoints_ambiguous_segments",
		"links_arp",
		"links_probable",
		"segments_suppressed",
		"actors_total",
		"actors_unlinked_suppressed",
		"endpoints_total",
		"inference_strategy",
		"attachments_total",
		"attachments_fdb",
		"enrichments_total",
		"enrichments_arp_nd",
		"bridge_domains_total",
		"identity_alias_endpoints_mapped",
		"identity_alias_endpoints_ambiguous_mac",
		"identity_alias_ips_merged",
		"identity_alias_ips_conflict_skipped",
		"actors_collapsed_by_ip",
		"actors_non_ip_inferred_suppressed",
		"actors_map_type_suppressed",
		"segments_sparse_suppressed",
		"map_type",
		"managed_snmp_device_focus",
		"depth",
		"actors_focus_depth_filtered",
		"links_focus_depth_filtered",
		"l3_subnet_candidate_subnets",
		"l3_subnet_candidate_links",
		"l3_subnet_emitted_links",
		"l3_subnet_suppressed_invalid",
		"l3_subnet_suppressed_unsupported_prefix",
		"l3_subnet_suppressed_duplicate_ip",
		"l3_subnet_suppressed_self_link",
		"l3_subnet_suppressed_unmatched",
		"l3_subnet_suppressed_multi_access",
		"l3_subnet_suppressed_unresolved_actor",
		"l3_subnet_suppressed_self_actor",
		"l3_subnet_suppressed_duplicate_link",
		"l3_subnet_visible_links",
		"ospf_neighbor_rows",
		"ospf_neighbor_detail_rows",
		"ospf_adjacency_emitted_links",
		"ospf_adjacency_suppressed_non_full_state",
		"ospf_adjacency_suppressed_unresolved_local",
		"ospf_adjacency_suppressed_unresolved_neighbor",
		"ospf_adjacency_suppressed_self_actor",
		"ospf_adjacency_suppressed_duplicate_link",
		"ospf_adjacency_visible_links",
		"bgp_peer_rows",
		"bgp_peer_detail_rows",
		"bgp_adjacency_emitted_links",
		"bgp_adjacency_suppressed_non_established_state",
		"bgp_adjacency_suppressed_unresolved_local",
		"bgp_adjacency_suppressed_unresolved_neighbor",
		"bgp_adjacency_suppressed_self_actor",
		"bgp_adjacency_suppressed_duplicate_link",
		"bgp_adjacency_visible_links",
	}, topologyStatsKeysForTest(payload.Stats))

	require.Equal(t, topologyMapTypeAllDevicesLowConfidence, payload.Stats["map_type"])
	require.Equal(t, topologyInferenceStrategyFDBMinimumKnowledge, payload.Stats["inference_strategy"])
	require.Equal(t, topologyManagedFocusAllDevices, payload.Stats["managed_snmp_device_focus"])
	require.Equal(t, topologyDepthAll, payload.Stats["depth"])
	require.Equal(t, 1, payload.Stats["l3_subnet_emitted_links"])
	require.Equal(t, 1, payload.Stats["l3_subnet_visible_links"])
	require.Equal(t, 1, payload.Stats["ospf_adjacency_emitted_links"])
	require.Equal(t, 1, payload.Stats["ospf_adjacency_visible_links"])
	require.Equal(t, 1, payload.Stats["bgp_adjacency_emitted_links"])
	require.Equal(t, 1, payload.Stats["bgp_adjacency_visible_links"])
	requireRealPipelineLinkEvidenceCensus(t, payload)

	stringStats := map[string]struct{}{
		"map_type":                  {},
		"inference_strategy":        {},
		"managed_snmp_device_focus": {},
		"depth":                     {},
	}
	for key, value := range payload.Stats {
		if _, ok := stringStats[key]; ok {
			require.IsType(t, "", value, "stat %q", key)
			continue
		}
		require.IsType(t, 0, value, "stat %q", key)
	}
}

func TestSNMPTopologyToV1_RealPipelineStatsCensusNoProtocolDataEmitsZeroProtocolKeys(t *testing.T) {
	registry := newTopologyRegistry()
	cache := newTopologyCache()
	cache.updateTime = time.Date(2026, time.June, 20, 12, 0, 0, 0, time.UTC)
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent-test"
	cache.localDevice = topologyDevice{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		SysName:       "switch-a",
		ManagementIP:  "10.0.0.1",
	}
	registry.register(cache)

	options := defaultTopologyQueryOptionsForTest()
	options.MapType = topologyMapTypeAllDevicesLowConfidence
	data, ok := snapshotTopologyRegistryForTestWithOptions(registry, options)
	require.True(t, ok)

	payload, err := snmpTopologyToV1(data)
	require.NoError(t, err)
	require.Equal(t, 0, payload.Stats["l3_subnet_candidate_links"])
	require.Equal(t, 0, payload.Stats["l3_subnet_emitted_links"])
	require.Equal(t, 0, payload.Stats["l3_subnet_visible_links"])
	require.Equal(t, 0, payload.Stats["ospf_neighbor_rows"])
	require.Equal(t, 0, payload.Stats["ospf_adjacency_emitted_links"])
	require.Equal(t, 0, payload.Stats["ospf_adjacency_visible_links"])
	require.Equal(t, 0, payload.Stats["bgp_peer_rows"])
	require.Equal(t, 0, payload.Stats["bgp_adjacency_emitted_links"])
	require.Equal(t, 0, payload.Stats["bgp_adjacency_visible_links"])
}

func TestTopologyStatsToV1_OmitsFocusKeysWhenFocusFilterReturnsEarly(t *testing.T) {
	data := &topologyData{
		Actors: []topologyActor{
			{ActorID: "segment-a", ActorType: "segment", Match: topologyMatch{IPAddresses: []string{"10.0.0.1"}}},
		},
	}

	topologyshape.ApplyDepthFocusFilter(data, topologyQueryOptions{
		ManagedDeviceFocus: "ip:10.0.0.1",
		Depth:              1,
	})

	stats := topologyStatsToV1(data.Stats)
	require.Equal(t, 1, stats["actors_total"])
	require.Equal(t, 0, stats["links_total"])
	require.NotContains(t, stats, "managed_snmp_device_focus")
	require.NotContains(t, stats, "depth")
	require.NotContains(t, stats, "actors_focus_depth_filtered")
	require.NotContains(t, stats, "links_focus_depth_filtered")
}

func newTopologyStatsCensusRouterCache(t *testing.T, sysName, chassisID, managementIP, routerID, ifIP, ifIndex string) *topologyCache {
	t.Helper()

	cache := newTopologyCache()
	cache.updateTime = time.Date(2026, time.June, 20, 12, 0, 0, 0, time.UTC)
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent-test"
	cache.localDevice = topologyDevice{
		ChassisID:     chassisID,
		ChassisIDType: "macAddress",
		SysName:       sysName,
		ManagementIP:  managementIP,
	}
	cache.updateTopologyProfileTags([]*ddsnmp.ProfileMetrics{{
		DeviceMetadata: map[string]ddsnmp.MetaTag{
			tagOSPFRouterID: {Value: routerID},
		},
	}})
	cache.updateIfIndexByIP(map[string]string{
		tagTopoIfIndex: ifIndex,
		tagTopoIPAddr:  ifIP,
		tagTopoIPMask:  "255.255.255.252",
	})
	return cache
}

func addTopologyStatsCensusLLDP(cache *topologyCache, localPortID, remoteChassis, remoteSysName, remoteMgmtIP, remotePortID string) {
	cache.lldpLocPorts["1"] = &lldpLocPort{
		portNum:       "1",
		portID:        localPortID,
		portIDSubtype: "interfaceName",
	}
	cache.lldpRemotes["1:1"] = &lldpRemote{
		localPortNum:     "1",
		remIndex:         "1",
		chassisID:        remoteChassis,
		chassisIDSubtype: "macAddress",
		portID:           remotePortID,
		portIDSubtype:    "interfaceName",
		sysName:          remoteSysName,
		managementAddr:   remoteMgmtIP,
	}
}

func requireRealPipelineLinkEvidenceCensus(t *testing.T, payload topologyv1.Data) {
	t.Helper()

	require.Contains(t, payload.Tables.Actor, "actor_port_links")
	require.Greater(t, payload.Tables.Actor["actor_port_links"].Table.Rows, 0)

	for _, linkType := range []string{
		snmpTopologyV1LinkLLDP,
		snmpTopologyV1LinkL3Subnet,
		snmpTopologyV1LinkOSPF,
		snmpTopologyV1LinkBGP,
	} {
		require.Contains(t, payload.Evidence, linkType)
		table := payload.Evidence[linkType].Table
		require.Equal(t, 1, table.Rows, "evidence rows for %s", linkType)
		require.Equal(t, "link_ref", topologyV1ColumnType(table, "link"), "evidence link_ref for %s", linkType)
		require.Empty(t, topologyV1ColumnType(table, "src_endpoint"), "removed raw src endpoint column for %s", linkType)
		require.Empty(t, topologyV1ColumnType(table, "dst_endpoint"), "removed raw dst endpoint column for %s", linkType)
		require.Empty(t, topologyV1ColumnType(table, "metrics"), "removed raw metrics column for %s", linkType)
	}

	lldpTable := payload.Evidence[snmpTopologyV1LinkLLDP].Table
	require.Equal(t, []string{"xe-0/0/0"}, topologyV1StringColumnValues(t, payload, lldpTable, "src_port_id"))
	require.Equal(t, []string{"xe-0/0/1"}, topologyV1StringColumnValues(t, payload, lldpTable, "dst_port_id"))
}

func topologyStatsKeysForTest(stats map[string]any) []string {
	keys := make([]string, 0, len(stats))
	for key := range stats {
		keys = append(keys, key)
	}
	return keys
}
