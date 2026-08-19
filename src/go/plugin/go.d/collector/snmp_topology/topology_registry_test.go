// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyenrich"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	topologyv1renderer "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyv1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyv1test"

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
	cacheA.localDevice = topologymodel.Device{
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
		managementAddrs: []topologymodel.ManagementAddress{
			{Address: "10.0.0.2", AddressType: "ipv4", Source: "lldp_remote"},
		},
	}

	cacheB := newTopologyCache()
	cacheB.updateTime = time.Now().Add(time.Second)
	cacheB.lastUpdate = cacheB.updateTime
	cacheB.agentID = "agent-test"
	cacheB.localDevice = topologymodel.Device{
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
		managementAddrs: []topologymodel.ManagementAddress{
			{Address: "10.0.0.1", AddressType: "ipv4", Source: "lldp_remote"},
		},
	}

	registry.register(cacheA)
	registry.register(cacheB)

	data, ok := snapshotTopologyRegistryForTest(registry)
	require.True(t, ok)
	require.Equal(t, "2", data.Layer)
	require.Equal(t, "snmp", data.Source)
	require.Equal(t, "summary", data.View)

	require.GreaterOrEqual(t, topologyStatsToV1ForTest(t, data.Stats)["devices_total"].(int), 2)
	require.GreaterOrEqual(t, topologyStatsToV1ForTest(t, data.Stats)["links_total"].(int), 1)
	require.GreaterOrEqual(t, topologyStatsToV1ForTest(t, data.Stats)["links_lldp"].(int), 1)
}

func TestBuildSNMPTopologySnapshotPreservesL2BuildError(t *testing.T) {
	_, ok, err := buildSNMPTopologySnapshot(topologymodel.ObservationAggregate{
		L2Observations: []topologyengine.L2Observation{{}},
	}, topologyoptions.DefaultQueryOptions())
	require.ErrorContains(t, err, "empty device id")
	require.False(t, ok)
}

func TestBuildProbableTopologySnapshotMatchesIndependentLegacyPath(t *testing.T) {
	collectedAt := time.Date(2026, time.August, 19, 12, 0, 0, 0, time.UTC)
	observations := []topologyengine.L2Observation{
		{
			DeviceID:          "switch-a",
			Hostname:          "switch-a",
			ManagementIP:      "192.0.2.1",
			ManagementAliases: []string{"10.0.0.1", "192.0.2.1"},
			ChassisID:         "00:11:22:33:44:55",
			Labels:            map[string]string{"device_category": "Switch", "site": "lab-a"},
			Interfaces: []topologyengine.ObservedInterface{{
				IfIndex: 1, IfName: "Gi0/1", IfDescr: "uplink", MAC: "00:11:22:33:44:56",
				SpeedBps: 1_000_000_000, AdminStatus: "up", OperStatus: "up",
			}},
			BridgePorts: []topologyengine.BridgePortObservation{{BasePort: "1", IfIndex: 1}},
			LLDPRemotes: []topologyengine.LLDPRemoteObservation{{
				LocalPortNum: "1", LocalPortID: "Gi0/1", ChassisID: "aa:bb:cc:dd:ee:ff",
				SysName: "switch-b", PortID: "Gi0/2", ManagementIP: "192.0.2.2",
			}},
			FDBEntries: []topologyengine.FDBObservation{{
				MAC: "02:00:00:00:00:10", BridgePort: "1", IfIndex: 1, Status: "learned", VLANID: "10",
			}},
			ARPNDEntries: []topologyengine.ARPNDObservation{{
				Protocol: "arp", IfIndex: 1, IfName: "Gi0/1", IP: "198.51.100.10",
				MAC: "02:00:00:00:00:10", State: "reachable", AddrType: "ipv4",
			}},
		},
		{
			DeviceID:          "switch-b",
			Hostname:          "switch-b",
			ManagementIP:      "192.0.2.2",
			ManagementAliases: []string{"10.0.0.2", "192.0.2.2"},
			ChassisID:         "aa:bb:cc:dd:ee:ff",
			Labels:            map[string]string{"device_category": "Switch", "site": "lab-b"},
			Interfaces: []topologyengine.ObservedInterface{{
				IfIndex: 2, IfName: "Gi0/2", IfDescr: "downlink", MAC: "aa:bb:cc:dd:ee:fe",
				SpeedBps: 1_000_000_000, AdminStatus: "up", OperStatus: "up",
			}},
			BridgePorts: []topologyengine.BridgePortObservation{{BasePort: "2", IfIndex: 2}},
			FDBEntries: []topologyengine.FDBObservation{{
				MAC: "02:00:00:00:00:10", BridgePort: "2", IfIndex: 2, Status: "learned", VLANID: "10",
			}},
		},
	}
	aggregate := topologymodel.ObservationAggregate{
		L2Observations: observations,
		L3Interfaces: []topologymodel.L3Interface{
			{DeviceID: "switch-a", IP: "203.0.113.1", Netmask: "255.255.255.0", IfIndex: "1", IfName: "Gi0/1"},
			{DeviceID: "switch-b", IP: "203.0.113.2", Netmask: "255.255.255.0", IfIndex: "2", IfName: "Gi0/2"},
		},
		Snapshots: []topologymodel.ObservationSnapshot{
			{
				LocalDeviceID: "switch-a",
				LocalDevice: topologymodel.Device{
					ChassisID: "00:11:22:33:44:55", ChassisIDType: "macAddress", SysName: "switch-a",
					ManagementIP: "192.0.2.1", Labels: map[string]string{"site": "lab-a"},
				},
			},
			{
				LocalDeviceID: "switch-b",
				LocalDevice: topologymodel.Device{
					ChassisID: "aa:bb:cc:dd:ee:ff", ChassisIDType: "macAddress", SysName: "switch-b",
					ManagementIP: "192.0.2.2", Labels: map[string]string{"site": "lab-b"},
				},
			},
		},
		LocalDeviceID: "switch-a",
		AgentID:       "agent-1",
		CollectedAt:   collectedAt,
	}
	options := topologyoptions.DefaultQueryOptions()
	options.MapType = topologyoptions.MapTypeAllDevicesLowConfidence

	got, ok, err := buildProbableTopologySnapshot(aggregate, options)
	require.NoError(t, err)
	require.True(t, ok)

	want := buildProbableTopologySnapshotIndependentLegacyForTest(t, aggregate, options)
	require.Equal(t, want, got)
	wantPayload, err := topologyv1renderer.Render(want)
	require.NoError(t, err)
	gotPayload, err := topologyv1renderer.Render(got)
	require.NoError(t, err)
	wantJSON := topologyv1test.CanonicalJSON(t, topologyv1test.NormalizeData(t, wantPayload))
	gotJSON := topologyv1test.CanonicalJSON(t, topologyv1test.NormalizeData(t, gotPayload))
	require.Equal(t, wantJSON, gotJSON)
}

func buildProbableTopologySnapshotIndependentLegacyForTest(
	t *testing.T,
	aggregate topologymodel.ObservationAggregate,
	options topologyoptions.QueryOptions,
) topologymodel.Data {
	t.Helper()

	strictResult, ok, err := buildSNMPL2TopologyResult(aggregate.L2Observations)
	require.NoError(t, err)
	require.True(t, ok)
	strictOptions := options
	strictOptions.MapType = topologyoptions.MapTypeHighConfidenceInferred
	strictData, err := projectSNMPL2TopologyData(
		strictResult, aggregate.AgentID, aggregate.LocalDeviceID, aggregate.CollectedAt, strictOptions,
	)
	require.NoError(t, err)
	augmentTopologySnapshotLocals(&strictData, aggregate.Snapshots)
	topologyshape.ApplyPolicies(&strictData, strictOptions)

	probableResult, ok, err := buildSNMPL2TopologyResult(aggregate.L2Observations)
	require.NoError(t, err)
	require.True(t, ok)
	probableOptions := options
	probableOptions.MapType = topologyoptions.MapTypeAllDevicesLowConfidence
	probableData, err := projectSNMPL2TopologyData(
		probableResult, aggregate.AgentID, aggregate.LocalDeviceID, aggregate.CollectedAt, probableOptions,
	)
	require.NoError(t, err)
	augmentTopologySnapshotLocals(&probableData, aggregate.Snapshots)
	topologyshape.ApplyPolicies(&probableData, probableOptions)

	topologyshape.MarkProbableDeltaLinks(&strictData, &probableData)
	topologyenrich.ApplyLayer3(&probableData, aggregate)
	topologyshape.ApplyDepthFocusFilter(&probableData, options)
	return probableData
}

func TestTopologyRegistry_EnqueueReverseDNSWarmFromDefaultSnapshotUsesDisplayCandidates(t *testing.T) {
	clock := newReverseDNSTestClock()
	warmed := make(chan string, 4)
	registry := newTopologyRegistry()
	dns := newTestTopologyReverseDNSWarmer(testTopologyReverseDNSConfig{
		now:         clock.Now,
		timeout:     time.Second,
		positiveTTL: time.Hour,
		negativeTTL: time.Minute,
		concurrency: 1,
		lookup: func(_ context.Context, ip string) ([]string, error) {
			warmed <- ip
			return []string{ip + ".example.test"}, nil
		},
	})
	registry.reverseDNS = dns.resolver
	registry.reverseDNSWarmer = dns.topologyReverseDNSWarmer
	registry.setReverseDNSWarmContext(context.Background())

	cache := newTopologyCache()
	seedPublishedEndpointSnapshot(cache)
	registry.register(cache)

	require.True(t, registry.enqueueReverseDNSWarmFromDefaultSnapshot())
	require.Eventually(t, func() bool {
		select {
		case ip := <-warmed:
			return ip == "10.0.0.10" || ip == "10.0.0.20"
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)
}

func TestTopologyRegistry_ReverseDNSCandidatesExcludeDeviceAliases(t *testing.T) {
	clock := newReverseDNSTestClock()
	dns := newTestTopologyReverseDNSWarmer(testTopologyReverseDNSConfig{
		now: clock.Now,
		lookup: func(_ context.Context, ip string) ([]string, error) {
			if ip == "198.51.100.8" {
				return []string{"unrelated-alias.example"}, nil
			}
			return nil, nil
		},
	})
	dns.warm(context.Background(), []string{"198.51.100.8"})

	registry := newTopologyRegistry()
	registry.reverseDNS = dns.resolver
	cache := newTopologyCache()
	seedPublishedEndpointSnapshot(cache)
	cache.localDevice.ManagementAddresses = []topologymodel.ManagementAddress{
		{Address: "10.0.0.10", AddressType: "ipv4", Source: managementAddressSourceCollectorTarget},
		{Address: "198.51.100.8", AddressType: "ipv4", Source: "lldp_local"},
	}
	registry.register(cache)

	candidates := registry.reverseDNSCandidateCollector()
	options := defaultTopologyQueryOptionsForTest()
	options.ResolveDNSName = candidates.lookupCached
	data, ok, err := registry.snapshotWithOptions(options)
	require.NoError(t, err)

	require.True(t, ok)
	require.True(t, containsMgmtAddr(data, map[string]struct{}{"198.51.100.8": {}}))
	require.Condition(t, func() bool {
		for _, actor := range data.Actors {
			if actor.Match.SysName == "switch-a" {
				return topologymodel.ActorDetailDisplayName(actor) == "switch-a"
			}
		}
		return false
	})
	require.Equal(t, []netip.Addr{
		netip.MustParseAddr("10.0.0.10"),
		netip.MustParseAddr("10.0.0.20"),
	}, candidates.collectedCandidates())

	payload, err := topologyv1renderer.Render(data)
	require.NoError(t, err)
	require.Contains(t, topologyV1StringColumnValues(t, payload, payload.Actors, "display_name"), "switch-a")
	require.NotContains(t, topologyV1StringColumnValues(t, payload, payload.Actors, "display_name"), "unrelated-alias.example")
	require.Contains(t, topologyV1ColumnValues(t, payload.Actors, "ip_addresses"), []any{
		"10.0.0.10",
		"198.51.100.8",
	})
	require.Contains(t, topologyV1StringColumnValues(t, payload, payload.Actors, "management_ip"), "10.0.0.10")
}

func TestTopologyRegistry_HasRenderableObservations(t *testing.T) {
	var nilRegistry *topologyRegistry
	require.False(t, nilRegistry.hasRenderableObservations())

	tests := map[string]struct {
		setup func(*topologyRegistry)
		want  bool
	}{
		"empty-registry": {
			want: false,
		},
		"cache-not-yet-published": {
			setup: func(registry *topologyRegistry) {
				registry.register(newTopologyCache())
			},
			want: false,
		},
		"cache-stale": {
			setup: func(registry *topologyRegistry) {
				cache := newTopologyCache()
				seedPublishedEndpointSnapshot(cache)
				cache.lastUpdate = time.Now().Add(-2 * time.Hour)
				cache.staleAfter = time.Hour
				registry.register(cache)
			},
			want: false,
		},
		"fresh-local-observation": {
			setup: func(registry *topologyRegistry) {
				cache := newTopologyCache()
				now := time.Now()
				cache.updateTime = now
				cache.lastUpdate = now
				cache.staleAfter = time.Hour
				cache.localDevice = topologymodel.Device{ManagementIP: "10.0.0.1"}
				registry.register(cache)
			},
			want: true,
		},
		"fresh-published-endpoint-snapshot": {
			setup: func(registry *topologyRegistry) {
				cache := newTopologyCache()
				seedPublishedEndpointSnapshot(cache)
				registry.register(cache)
			},
			want: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			registry := newTopologyRegistry()
			if tc.setup != nil {
				tc.setup(registry)
			}

			require.Equal(t, tc.want, registry.hasRenderableObservations())
		})
	}
}

func TestTopologyRegistry_SnapshotSingleCacheKeepsLLDPUnidirectional(t *testing.T) {
	registry := newTopologyRegistry()

	cache := newTopologyCache()
	cache.updateTime = time.Now()
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent-test"
	cache.localDevice = topologymodel.Device{
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
		managementAddrs: []topologymodel.ManagementAddress{
			{Address: "10.0.0.2", AddressType: "ipv4", Source: "lldp_remote"},
		},
	}

	registry.register(cache)

	data, ok := snapshotTopologyRegistryForTest(registry)
	require.True(t, ok)
	require.Len(t, data.Links, 1)
	require.Equal(t, "lldp", data.Links[0].Protocol)
	require.Equal(t, "unidirectional", data.Links[0].Direction)
	require.Nil(t, data.Links[0].L2)
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["links_unidirectional"].(int))
	require.Equal(t, 0, topologyStatsToV1ForTest(t, data.Stats)["links_bidirectional"].(int))
}

func TestTopologyRegistry_DefaultMapEmitsL3SubnetForManagedRoutersWithoutLLDP(t *testing.T) {
	registry := newTopologyRegistry()

	cacheA := newTopologyCache()
	cacheA.updateTime = time.Now()
	cacheA.lastUpdate = cacheA.updateTime
	cacheA.agentID = "agent-test"
	cacheA.localDevice = topologymodel.Device{
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
	cacheB.localDevice = topologymodel.Device{
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
	require.Equal(t, topologymodel.L3SubnetLinkType, link.Protocol)
	require.Equal(t, topologymodel.L3SubnetLinkType, link.LinkType)
	require.Equal(t, "observed", link.Direction)
	require.Equal(t, "shared_subnet", topologymodel.LinkInferenceValue(link))
	require.Equal(t, "logical_l3_subnet", topologymodel.LinkAttachmentModeValue(link))
	require.NotNil(t, link.Detail.L3Subnet)
	require.Equal(t, "198.51.100.0/30", link.Detail.L3Subnet.Subnet)
	require.Equal(t, "198.51.100.1", link.Detail.L3Subnet.SrcIP)
	require.Equal(t, "198.51.100.2", link.Detail.L3Subnet.DstIP)
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["l3_subnet_emitted_links"])
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["l3_subnet_visible_links"])
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["links_total"])
}

func TestTopologyRegistry_WeakDevicesUseSelectedManagementIPIdentity(t *testing.T) {
	registry := newTopologyRegistry()
	for i, ip := range []string{"192.0.2.10", "198.51.100.10"} {
		cache := newTopologyCache()
		cache.updateTime = time.Now().Add(time.Duration(i) * time.Millisecond)
		cache.updateIfIndexByIP(map[string]string{
			tagTopoIfIndex: "1",
			tagTopoIPAddr:  ip,
			tagTopoIPMask:  "255.255.255.0",
		})
		cache.finalizeTopologyCache()
		registry.register(cache)
	}

	data, ok := snapshotTopologyRegistryForTest(registry)
	require.True(t, ok)
	require.Len(t, data.Actors, 2)

	got := make(map[string]struct{}, 2)
	for _, actor := range data.Actors {
		for _, ip := range actor.Match.IPAddresses {
			got[ip] = struct{}{}
		}
	}
	require.Equal(t, map[string]struct{}{
		"192.0.2.10":    {},
		"198.51.100.10": {},
	}, got)
}

func TestTopologyRegistry_DefaultMapEmitsL3SubnetSegmentForManagedRouters(t *testing.T) {
	registry := newTopologyRegistry()
	registry.producerScopeID = "producer-a"

	cacheA := newTopologyCache()
	cacheA.updateTime = time.Now()
	cacheA.lastUpdate = cacheA.updateTime
	cacheA.agentID = "agent-test"
	cacheA.localDevice = topologymodel.Device{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		SysName:       "router-a",
		ManagementIP:  "10.0.0.1",
	}
	cacheA.updateIfIndexByIP(map[string]string{
		tagTopoIfIndex: "2",
		tagTopoIPAddr:  "203.0.113.1",
		tagTopoIPMask:  "255.255.255.0",
	})
	cacheA.updateIfNameByIndex(map[string]string{
		tagTopoIfIndex: "2",
		tagTopoIfName:  "wan0",
	})

	cacheB := newTopologyCache()
	cacheB.updateTime = time.Now().Add(time.Second)
	cacheB.lastUpdate = cacheB.updateTime
	cacheB.agentID = "agent-test"
	cacheB.localDevice = topologymodel.Device{
		ChassisID:     "aa:bb:cc:dd:ee:ff",
		ChassisIDType: "macAddress",
		SysName:       "router-b",
		ManagementIP:  "10.0.0.2",
	}
	cacheB.updateIfIndexByIP(map[string]string{
		tagTopoIfIndex: "7",
		tagTopoIPAddr:  "203.0.113.2",
		tagTopoIPMask:  "255.255.255.0",
	})
	cacheB.updateIfNameByIndex(map[string]string{
		tagTopoIfIndex: "7",
		tagTopoIfName:  "wan7",
	})

	cacheC := newTopologyCache()
	cacheC.updateTime = time.Now().Add(2 * time.Second)
	cacheC.lastUpdate = cacheC.updateTime
	cacheC.agentID = "agent-test"
	cacheC.localDevice = topologymodel.Device{
		ChassisID:     "cc:dd:ee:ff:00:11",
		ChassisIDType: "macAddress",
		SysName:       "router-c",
		ManagementIP:  "10.0.0.3",
	}
	cacheC.updateIfIndexByIP(map[string]string{
		tagTopoIfIndex: "9",
		tagTopoIPAddr:  "203.0.113.3",
		tagTopoIPMask:  "255.255.255.0",
	})
	cacheC.updateIfNameByIndex(map[string]string{
		tagTopoIfIndex: "9",
		tagTopoIfName:  "wan9",
	})

	registry.register(cacheA)
	registry.register(cacheB)
	registry.register(cacheC)

	data, ok := snapshotTopologyRegistryForTest(registry)

	require.True(t, ok)
	require.Len(t, data.Actors, 4)
	require.Equal(t, 3, testCountTopologyLinksByType(data.Links, topologymodel.L3SubnetMembershipLinkType))
	require.Equal(t, 0, testCountTopologyLinksByType(data.Links, topologymodel.L3SubnetLinkType))

	var segment *topologymodel.Actor
	for i := range data.Actors {
		if data.Actors[i].ActorType == topologymodel.L3SubnetSegmentActorType {
			segment = &data.Actors[i]
			break
		}
	}
	require.NotNil(t, segment)
	require.Equal(t, topologymodel.SegmentKindL3Subnet, segment.SegmentKind)
	require.Equal(t, "203.0.113.0/24", topologymodel.ActorDetailDisplayName(*segment))

	memberHandles := make([]topologymodel.ActorHandle, 0, 3)
	for _, link := range data.Links {
		if link.LinkType != topologymodel.L3SubnetMembershipLinkType {
			continue
		}
		require.Equal(t, segment.ActorHandle, link.DstActorHandle)
		require.NotNil(t, link.Detail.L3SubnetMembership)
		require.Equal(t, "203.0.113.0/24", link.Detail.L3SubnetMembership.Subnet)
		require.Len(t, link.Detail.L3SubnetMembership.Interfaces, 1)
		memberHandles = append(memberHandles, link.SrcActorHandle)
	}
	routerA := findDeviceActorBySysName(data, "router-a")
	require.NotNil(t, routerA)
	routerB := findDeviceActorBySysName(data, "router-b")
	require.NotNil(t, routerB)
	routerC := findDeviceActorBySysName(data, "router-c")
	require.NotNil(t, routerC)
	require.ElementsMatch(t, []topologymodel.ActorHandle{routerA.ActorHandle, routerB.ActorHandle, routerC.ActorHandle}, memberHandles)
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["l3_subnet_segment_emitted_segments"])
	require.Equal(t, 3, topologyStatsToV1ForTest(t, data.Stats)["l3_subnet_membership_emitted_links"])
	require.Equal(t, 3, topologyStatsToV1ForTest(t, data.Stats)["l3_subnet_membership_visible_links"])
	require.Equal(t, 3, topologyStatsToV1ForTest(t, data.Stats)["links_total"])
}

func TestTopologyRegistry_OSPFSnapshotEnrichesSubnetAfterNeighborIngest(t *testing.T) {
	registry := newTopologyRegistry()

	cacheA := newTopologyCache()
	cacheA.updateTime = time.Now()
	cacheA.lastUpdate = cacheA.updateTime
	cacheA.agentID = "agent-test"
	cacheA.localDevice = topologymodel.Device{
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
	cacheB.localDevice = topologymodel.Device{
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
	require.Equal(t, 1, testCountTopologyLinksByType(data.Links, topologymodel.L3SubnetLinkType))
	require.Equal(t, 1, testCountTopologyLinksByType(data.Links, topologymodel.OSPFAdjacencyLinkType))
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["l3_subnet_emitted_links"])
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["l3_subnet_visible_links"])
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["ospf_adjacency_emitted_links"])
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["ospf_adjacency_visible_links"])
}

func TestTopologyRegistry_BGPAdjacencyEmitsEstablishedManagedPeerLinkAndDetailRows(t *testing.T) {
	registry := newTopologyRegistry()

	cacheA := newTopologyCache()
	cacheA.updateTime = time.Now()
	cacheA.lastUpdate = cacheA.updateTime
	cacheA.agentID = "agent-test"
	cacheA.localDevice = topologymodel.Device{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		SysName:       "router-a",
		ManagementIP:  "10.0.0.1",
	}
	cacheA.bgpPeersByKey["a"] = topologymodel.BGPPeer{
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
	cacheB.localDevice = topologymodel.Device{
		ChassisID:     "aa:bb:cc:dd:ee:ff",
		ChassisIDType: "macAddress",
		SysName:       "router-b",
		ManagementIP:  "10.0.0.2",
	}
	cacheB.bgpPeersByKey["b"] = topologymodel.BGPPeer{
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
	require.Equal(t, topologymodel.BGPAdjacencyLinkType, link.Protocol)
	require.Equal(t, topologymodel.BGPAdjacencyLinkType, link.LinkType)
	require.Equal(t, "observed", link.Direction)
	require.Equal(t, "established", link.State)
	require.Equal(t, "bgp_established_adjacency", topologymodel.LinkInferenceValue(link))
	require.Equal(t, "logical_l3_bgp", topologymodel.LinkAttachmentModeValue(link))
	require.NotNil(t, link.Detail.BGP)
	require.Equal(t, "default", link.Detail.BGP.RoutingInstance)
	require.Equal(t, "65001", link.Detail.BGP.LocalAS)
	require.Equal(t, "65002", link.Detail.BGP.RemoteAS)
	require.Equal(t, 2, topologyStatsToV1ForTest(t, data.Stats)["bgp_peer_rows"])
	require.Equal(t, 2, topologyStatsToV1ForTest(t, data.Stats)["bgp_peer_detail_rows"])
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["bgp_adjacency_emitted_links"])
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["bgp_adjacency_suppressed_duplicate_link"])
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["bgp_adjacency_visible_links"])

	routerA := findDeviceActorBySysName(data, "router-a")
	require.NotNil(t, routerA)
	routerB := findDeviceActorBySysName(data, "router-b")
	require.NotNil(t, routerB)
	require.Len(t, routerA.Detail.BGP, 1)
	require.Equal(t, routerB.ActorHandle, routerA.Detail.BGP[0].RemoteActorHandle)
}

func TestTopologyRegistry_BGPAdjacencyKeepsUnresolvedAndNonEstablishedPeersAsDetails(t *testing.T) {
	registry := newTopologyRegistry()

	cache := newTopologyCache()
	cache.updateTime = time.Now()
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent-test"
	cache.localDevice = topologymodel.Device{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		SysName:       "router-a",
		ManagementIP:  "10.0.0.1",
	}
	cache.bgpPeersByKey["unresolved"] = topologymodel.BGPPeer{
		RoutingInstance: "default",
		NeighborIP:      "203.0.113.2",
		RemoteAS:        "65002",
		LocalIP:         "198.51.100.1",
		LocalAS:         "65001",
		LocalIdentifier: "1.1.1.1",
		PeerIdentifier:  "2.2.2.2",
		State:           "established",
	}
	cache.bgpPeersByKey["idle"] = topologymodel.BGPPeer{
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
	require.Equal(t, 2, topologyStatsToV1ForTest(t, data.Stats)["bgp_peer_rows"])
	require.Equal(t, 2, topologyStatsToV1ForTest(t, data.Stats)["bgp_peer_detail_rows"])
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["bgp_adjacency_suppressed_unresolved_neighbor"])
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["bgp_adjacency_suppressed_non_established_state"])
	require.Equal(t, 0, topologyStatsToV1ForTest(t, data.Stats)["bgp_adjacency_visible_links"])

	routerA := findDeviceActorBySysName(data, "router-a")
	require.NotNil(t, routerA)
	require.Len(t, routerA.Detail.BGP, 2)
	for _, row := range routerA.Detail.BGP {
		require.True(t, row.RemoteActorHandle.IsZero())
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

	data, ok, err := registry.snapshotWithOptions(topologyoptions.QueryOptions{
		CollapseActorsByIP:     true,
		EliminateNonIPInferred: true,
		MapType:                topologyoptions.MapTypeLLDPCDPManaged,
		ManagedDeviceFocus:     topologyoptions.ManagedFocusAllDevices,
		Depth:                  topologyoptions.DepthAllInternal,
	})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, topologyoptions.MapTypeLLDPCDPManaged, topologyStatsToV1ForTest(t, data.Stats)["map_type"])
	require.Equal(t, topologyoptions.InferenceStrategyFDBMinimumKnowledge, topologyStatsToV1ForTest(t, data.Stats)["inference_strategy"])
}

func TestTopologyRegistry_DefaultSnapshotSuppressesInferredNeighborWithOnlyIneligibleManagementAddress(t *testing.T) {
	registry := newTopologyRegistry()
	cache := newTopologyCache()
	cache.updateTime = time.Now().UTC()
	cache.agentID = "agent-test"
	cache.localDevice = topologymodel.Device{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		SysName:       "switch-a",
		ManagementIP:  "192.0.2.1",
	}
	cache.lldpLocPorts["1"] = &lldpLocPort{
		portNum:       "1",
		portID:        "Gi0/1",
		portIDSubtype: "interfaceName",
	}
	cache.updateLldpRemote(map[string]string{
		tagLldpLocPortNum:          "1",
		tagLldpRemIndex:            "1",
		tagLldpRemChassisID:        "aa:bb:cc:dd:ee:ff",
		tagLldpRemChassisIDSubtype: "macAddress",
		tagLldpRemPortID:           "Gi0/2",
		tagLldpRemPortIDSubtype:    "interfaceName",
		tagLldpRemSysName:          "switch-b",
		tagLldpRemMgmtAddr:         "169.254.0.1",
		tagLldpRemMgmtAddrSubtype:  "1",
	})
	cache.finalizeTopologyCache()
	registry.register(cache)

	data, ok := snapshotTopologyRegistryForTest(registry)
	require.True(t, ok)
	require.Len(t, data.Actors, 1)
	require.NotNil(t, findDeviceActorBySysName(data, "switch-a"))
	require.Nil(t, findDeviceActorBySysName(data, "switch-b"))
	require.Empty(t, data.Links)
	require.Empty(t, cache.lldpRemotes["1:1"].managementAddrs)
}

func TestTopologyRegistry_SnapshotWithOptions_CollapseByIPPreservesEngineManagedOverlapPruning(t *testing.T) {
	registry := newTopologyRegistry()

	cache := newTopologyCache()
	cache.updateTime = time.Now().UTC()
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent-test"
	cache.localDevice = topologymodel.Device{
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
		managementAddrs: []topologymodel.ManagementAddress{
			{Address: "172.22.0.1", AddressType: "ipv4", Source: "lldp_remote"},
		},
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

	withoutCollapse, ok, err := registry.snapshotWithOptions(topologyoptions.QueryOptions{
		MapType:            topologyoptions.MapTypeAllDevicesLowConfidence,
		ManagedDeviceFocus: topologyoptions.ManagedFocusAllDevices,
		Depth:              topologyoptions.DepthAllInternal,
	})
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, findActorByMAC(withoutCollapse, "9c:6b:00:7b:98:c7"))

	withCollapse, ok, err := registry.snapshotWithOptions(topologyoptions.QueryOptions{
		CollapseActorsByIP: true,
		MapType:            topologyoptions.MapTypeAllDevicesLowConfidence,
		ManagedDeviceFocus: topologyoptions.ManagedFocusAllDevices,
		Depth:              topologyoptions.DepthAllInternal,
	})
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, findActorByMAC(withCollapse, "9c:6b:00:7b:98:c6"))
	require.Nil(t, findActorByMAC(withCollapse, "9c:6b:00:7b:98:c7"))
	require.Equal(t, 1, topologyStatsToV1ForTest(t, withCollapse.Stats)["actors_unlinked_suppressed"])
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

func TestTopologyRegistry_ManagedFocusRejectsTypedNonIPManagementEvidence(t *testing.T) {
	registry := newTopologyRegistry()
	now := time.Now()
	for _, device := range []topologymodel.Device{
		{
			ChassisID:     "00:11:22:33:44:55",
			ChassisIDType: "macAddress",
			SysName:       "sw-a",
			ManagementIP:  "192.0.2.10",
			ManagementAddresses: []topologymodel.ManagementAddress{
				{Address: "c0000263", AddressType: "16", Source: "lldp_local"},
			},
		},
		{
			ChassisID:     "aa:bb:cc:dd:ee:ff",
			ChassisIDType: "macAddress",
			SysName:       "sw-b",
			ManagementIP:  "192.0.2.20",
		},
	} {
		cache := newTopologyCache()
		cache.updateTime = now
		cache.lastUpdate = now
		cache.agentID = device.SysName
		cache.localDevice = device
		registry.register(cache)
	}

	options := defaultTopologyQueryOptionsForTest()
	options.ManagedDeviceFocus = "ip:192.0.2.99"
	options.Depth = 0
	data, ok := snapshotTopologyRegistryForTestWithOptions(registry, options)

	require.True(t, ok)
	require.Len(t, data.Actors, 2)
	require.NotNil(t, findDeviceActorBySysName(data, "sw-a"))
	require.NotNil(t, findDeviceActorBySysName(data, "sw-b"))
}

func TestTopologyRegistry_ManagedFocusUsesOnlyReconciledManagementAddresses(t *testing.T) {
	registry := newTopologyRegistry()
	now := time.Now().UTC()

	cacheA := newTopologyCache()
	cacheA.updateTime = now
	cacheA.lastUpdate = now
	cacheA.agentID = "sw-a"
	cacheA.localDevice = topologymodel.Device{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		SysName:       "sw-a",
		ManagementIP:  "192.0.2.10",
	}
	cacheA.updateIfIndexByIP(map[string]string{
		tagTopoIfIndex: "1",
		tagTopoIPAddr:  "192.0.2.20",
		tagTopoIPMask:  "255.255.255.0",
	})

	cacheB := newTopologyCache()
	cacheB.updateTime = now
	cacheB.lastUpdate = now
	cacheB.agentID = "sw-b"
	cacheB.localDevice = topologymodel.Device{
		ChassisID:     "aa:bb:cc:dd:ee:ff",
		ChassisIDType: "macAddress",
		SysName:       "sw-b",
		ManagementIP:  "192.0.2.20",
	}

	registry.register(cacheA)
	registry.register(cacheB)
	baseline, ok := snapshotTopologyRegistryForTest(registry)
	require.True(t, ok)
	baselineA := findDeviceActorBySysName(baseline, "sw-a")
	require.NotNil(t, baselineA)
	require.NotContains(t, baselineA.Match.IPAddresses, "192.0.2.20")
	require.NotContains(t, baselineA.Detail.L2.Device.ManagementAddresses, "192.0.2.20")
	require.NotEmpty(t, baselineA.Detail.SNMP.ManagementAddresses)
	require.Equal(t, "192.0.2.20", baselineA.Detail.SNMP.ManagementAddresses[0].Address)

	options := defaultTopologyQueryOptionsForTest()
	options.ManagedDeviceFocus = "ip:192.0.2.20"
	options.Depth = 0
	data, ok := snapshotTopologyRegistryForTestWithOptions(registry, options)
	require.True(t, ok)
	require.Len(t, data.Actors, 1)
	require.Nil(t, findDeviceActorBySysName(data, "sw-a"))
	require.NotNil(t, findDeviceActorBySysName(data, "sw-b"))
}

func TestTopologyRegistry_ManagedFocusRetainsUniqueReconciledLocalAlias(t *testing.T) {
	registry := newTopologyRegistry()
	now := time.Now().UTC()

	cacheA := newTopologyCache()
	cacheA.updateTime = now
	cacheA.lastUpdate = now
	cacheA.agentID = "sw-a"
	cacheA.localDevice = topologymodel.Device{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		SysName:       "sw-a",
		ManagementIP:  "192.0.2.10",
	}
	cacheA.updateIfIndexByIP(map[string]string{
		tagTopoIfIndex: "1",
		tagTopoIPAddr:  "192.0.2.11",
		tagTopoIPMask:  "255.255.255.0",
	})

	cacheB := newTopologyCache()
	cacheB.updateTime = now
	cacheB.lastUpdate = now
	cacheB.agentID = "sw-b"
	cacheB.localDevice = topologymodel.Device{
		ChassisID:     "aa:bb:cc:dd:ee:ff",
		ChassisIDType: "macAddress",
		SysName:       "sw-b",
		ManagementIP:  "192.0.2.20",
	}

	registry.register(cacheA)
	registry.register(cacheB)

	options := defaultTopologyQueryOptionsForTest()
	options.ManagedDeviceFocus = "ip:192.0.2.11"
	options.Depth = 0
	data, ok := snapshotTopologyRegistryForTestWithOptions(registry, options)
	require.True(t, ok)
	require.Len(t, data.Actors, 1)
	actor := findDeviceActorBySysName(data, "sw-a")
	require.NotNil(t, actor)
	require.Contains(t, actor.Match.IPAddresses, "192.0.2.11")
	require.Contains(t, actor.Detail.L2.Device.ManagementAddresses, "192.0.2.11")
}

func TestTopologyCache_SnapshotEngineObservationsUsesDirectLocalObservation(t *testing.T) {
	cache := newTopologyCache()
	cache.updateTime = time.Now()
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent-test"
	cache.localDevice = topologymodel.Device{
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
		managementAddrs: []topologymodel.ManagementAddress{
			{Address: "10.0.0.2", AddressType: "ipv4", Source: "lldp_remote"},
		},
	}
	cache.cdpRemotes["1:1"] = &cdpRemote{
		ifIndex:    "1",
		ifName:     "Gi0/1",
		deviceID:   "sw-b",
		sysName:    "sw-b",
		devicePort: "Gi0/2",
		managementAddrs: []topologymodel.ManagementAddress{
			{Address: "10.0.0.2", AddressType: "ipv4", Source: "cdp_cache_address"},
		},
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
	cache.localDevice = topologymodel.Device{
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
	require.Equal(t, topologymodel.L3Interface{
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
	snapshots := []topologymodel.ObservationSnapshot{
		{
			LocalDeviceID: "device-a",
			AgentID:       "agent-a",
			CollectedAt:   collectedAt,
			L2Observations: []topologyengine.L2Observation{{
				DeviceID: "device-a",
			}},
			L3Interfaces: []topologymodel.L3Interface{{
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
	cacheA.localDevice = topologymodel.Device{
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
		managementAddrs: []topologymodel.ManagementAddress{
			{Address: "10.0.0.2", AddressType: "ipv4", Source: "lldp_remote"},
		},
	}

	cacheB := newTopologyCache()
	cacheB.updateTime = time.Now().Add(time.Second)
	cacheB.lastUpdate = cacheB.updateTime
	cacheB.agentID = "agent-test"
	cacheB.localDevice = topologymodel.Device{
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
		managementAddrs: []topologymodel.ManagementAddress{
			{Address: "10.0.0.1", AddressType: "ipv4", Source: "lldp_remote"},
		},
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
	cacheA.localDevice = topologymodel.Device{
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
		managementAddrs: []topologymodel.ManagementAddress{
			{Address: "10.0.0.2", AddressType: "ipv4", Source: "lldp_remote"},
		},
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
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["links_total"])
	require.Equal(t, 2, countActorsByType(data, "device"))
}

func TestTopologyRegistry_DuplicateCachesPreserveReconciledManagementPrimary(t *testing.T) {
	registry := newTopologyRegistry()
	now := time.Now().UTC()

	for _, managementIP := range []string{"192.0.2.30", "192.0.2.20"} {
		cache := newTopologyCache()
		cache.updateTime = now
		cache.lastUpdate = now
		cache.agentID = managementIP
		cache.localDevice = topologymodel.Device{
			ChassisID:     "00:11:22:33:44:55",
			ChassisIDType: "macAddress",
			SysName:       "sw-a",
			ManagementIP:  managementIP,
		}
		registry.register(cache)
	}

	data, ok := snapshotTopologyRegistryForTest(registry)
	require.True(t, ok)
	require.Len(t, data.Actors, 1)
	actor := findDeviceActorBySysName(data, "sw-a")
	require.NotNil(t, actor)
	require.Equal(t, "192.0.2.20", actor.Detail.L2.Device.ManagementIP)
	require.Equal(t, "192.0.2.20", topologymodel.ActorDetailManagementIP(*actor))

	payload, err := topologyv1renderer.Render(data)
	require.NoError(t, err)
	require.Equal(t, []string{"192.0.2.20"}, topologyV1StringColumnValues(t, payload, payload.Actors, "management_ip"))
}

func TestCanonicalMatchKey_NormalizesEquivalentMACRepresentations(t *testing.T) {
	raw := topologymodel.Match{ChassisIDs: []string{"7049a26572cd"}}
	colon := topologymodel.Match{MacAddresses: []string{"70:49:A2:65:72:CD"}}
	require.Equal(t, "mac:70:49:a2:65:72:cd", topologymodel.CanonicalMatchKey(raw))
	require.Equal(t, "mac:70:49:a2:65:72:cd", topologymodel.CanonicalMatchKey(colon))
	require.Contains(t, topologymodel.MatchIdentityKeys(raw), "hw:70:49:a2:65:72:cd")
	require.Contains(t, topologymodel.MatchIdentityKeys(colon), "hw:70:49:a2:65:72:cd")
}

func TestApplySNMPTopologyShapePolicies_CollapsesActorsByIP(t *testing.T) {
	data := topologymodel.Data{
		Actors: []topologymodel.Actor{
			{
				ActorID:   "device:a",
				ActorType: "device",
				Match: topologymodel.Match{
					IPAddresses:  []string{"10.0.0.10"},
					MacAddresses: []string{"aa:aa:aa:aa:aa:aa"},
				},
			},
			{
				ActorID:   "endpoint:b",
				ActorType: "endpoint",
				Match: topologymodel.Match{
					IPAddresses:  []string{"10.0.0.10"},
					MacAddresses: []string{"bb:bb:bb:bb:bb:bb"},
				},
			},
		},
		Links: []topologymodel.Link{
			{
				SrcActorHandle: snmpTopologyTestActorHandle("endpoint:b"),
				DstActorHandle: snmpTopologyTestActorHandle("device:a"),
				Protocol:       "fdb",
				Direction:      "bidirectional",
			},
		},
	}

	assignSNMPTopologyTestHandles(t, &data)
	topologyshape.ApplyPolicies(&data, topologyoptions.QueryOptions{
		CollapseActorsByIP: true,
		MapType:            topologyoptions.MapTypeHighConfidenceInferred,
	})

	require.Len(t, data.Actors, 1)
	require.Len(t, data.Links, 0)
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["actors_collapsed_by_ip"])
}

func TestApplySNMPTopologyShapePolicies_EliminatesNonIPInferredActorsAndSparseSegments(t *testing.T) {
	data := topologymodel.Data{
		Actors: []topologymodel.Actor{
			{
				ActorID:     "segment:s1",
				ActorType:   "segment",
				SegmentKind: topologymodel.SegmentKindBroadcastDomain,
				Match: topologymodel.Match{
					Hostnames: []string{"segment:s1"},
				},
			},
			{
				ActorID:   "endpoint:e1",
				ActorType: "endpoint",
				Match: topologymodel.Match{
					MacAddresses: []string{"cc:cc:cc:cc:cc:cc"},
				},
			},
		},
		Links: []topologymodel.Link{
			{
				SrcActorHandle: snmpTopologyTestActorHandle("segment:s1"),
				DstActorHandle: snmpTopologyTestActorHandle("endpoint:e1"),
				Protocol:       "fdb",
				Direction:      "bidirectional",
			},
		},
	}

	assignSNMPTopologyTestHandles(t, &data)
	topologyshape.ApplyPolicies(&data, topologyoptions.QueryOptions{
		EliminateNonIPInferred: true,
		MapType:                topologyoptions.MapTypeHighConfidenceInferred,
	})

	require.Len(t, data.Actors, 0)
	require.Len(t, data.Links, 0)
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["actors_non_ip_inferred_suppressed"])
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["segments_sparse_suppressed"])
}

func TestApplySNMPTopologyShapePolicies_HighConfidenceSuppressesUnlinkedInferredEndpoints(t *testing.T) {
	data := topologymodel.Data{
		Actors: []topologymodel.Actor{
			{
				ActorID:   "device:d1",
				ActorType: "device",
				Source:    "snmp",
				Match:     topologymodel.Match{IPAddresses: []string{"10.0.0.1"}},
			},
			{
				ActorID:   "endpoint:linked",
				ActorType: "endpoint",
				Source:    "snmp",
				Match:     topologymodel.Match{IPAddresses: []string{"10.0.0.2"}},
			},
			{
				ActorID:   "endpoint:unlinked",
				ActorType: "endpoint",
				Source:    "snmp",
				Match:     topologymodel.Match{IPAddresses: []string{"10.0.0.3"}},
			},
		},
		Links: []topologymodel.Link{
			{
				SrcActorHandle: snmpTopologyTestActorHandle("device:d1"),
				DstActorHandle: snmpTopologyTestActorHandle("endpoint:linked"),
				Protocol:       "fdb",
				Direction:      "bidirectional",
			},
		},
	}

	assignSNMPTopologyTestHandles(t, &data)
	topologyshape.ApplyPolicies(&data, topologyoptions.QueryOptions{
		MapType: topologyoptions.MapTypeHighConfidenceInferred,
	})

	require.Len(t, data.Actors, 2)
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["actors_map_type_suppressed"])
	for _, actor := range data.Actors {
		require.NotEqual(t, "endpoint:unlinked", actor.ActorID)
	}
}

func TestApplySNMPTopologyShapePolicies_LLDPManagedMapKeepsOnlyLLDPCDPAndManagedDevices(t *testing.T) {
	data := topologymodel.Data{
		Actors: []topologymodel.Actor{
			{
				ActorID:   "device:d1",
				ActorType: "device",
				Source:    "snmp",
				Match:     topologymodel.Match{IPAddresses: []string{"10.0.0.1"}},
			},
			{
				ActorID:   "device:d2",
				ActorType: "device",
				Source:    "snmp",
				Match:     topologymodel.Match{IPAddresses: []string{"10.0.0.2"}},
			},
			{
				ActorID:   "endpoint:e1",
				ActorType: "endpoint",
				Source:    "snmp",
				Match:     topologymodel.Match{IPAddresses: []string{"10.0.0.3"}},
			},
		},
		Links: []topologymodel.Link{
			{
				SrcActorHandle: snmpTopologyTestActorHandle("device:d1"),
				DstActorHandle: snmpTopologyTestActorHandle("device:d2"),
				Protocol:       "lldp",
				Direction:      "bidirectional",
			},
			{
				SrcActorHandle: snmpTopologyTestActorHandle("device:d1"),
				DstActorHandle: snmpTopologyTestActorHandle("endpoint:e1"),
				Protocol:       "fdb",
				Direction:      "bidirectional",
			},
		},
	}

	assignSNMPTopologyTestHandles(t, &data)
	topologyshape.ApplyPolicies(&data, topologyoptions.QueryOptions{
		MapType: topologyoptions.MapTypeLLDPCDPManaged,
	})

	require.Len(t, data.Actors, 2)
	require.Len(t, data.Links, 1)
	require.Equal(t, "lldp", data.Links[0].Protocol)
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["actors_map_type_suppressed"])
}

func TestMarkProbableDeltaLinks_MarksAllAddedLinksAsProbable(t *testing.T) {
	strictData := topologymodel.Data{
		Links: []topologymodel.Link{
			{
				SrcActorHandle: snmpTopologyTestActorHandle("device:d1"),
				DstActorHandle: snmpTopologyTestActorHandle("device:d2"),
				Protocol:       "lldp",
				Direction:      "bidirectional",
			},
		},
	}
	probableData := topologymodel.Data{
		Links: []topologymodel.Link{
			{
				SrcActorHandle: snmpTopologyTestActorHandle("device:d1"),
				DstActorHandle: snmpTopologyTestActorHandle("device:d2"),
				Protocol:       "lldp",
				Direction:      "bidirectional",
			},
			{
				SrcActorHandle: snmpTopologyTestActorHandle("device:d1"),
				DstActorHandle: snmpTopologyTestActorHandle("segment:s1"),
				Protocol:       "bridge",
				Direction:      "bidirectional",
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
	data := topologymodel.Data{
		Actors: []topologymodel.Actor{
			{
				ActorID:   "device:managed-a",
				ActorType: "device",
				Source:    "snmp",
				Match:     topologymodel.Match{IPAddresses: []string{"10.0.0.1"}},
			},
			{
				ActorID:   "device:managed-b",
				ActorType: "device",
				Source:    "snmp",
				Match:     topologymodel.Match{IPAddresses: []string{"10.0.0.2"}},
			},
			{
				ActorID:   "endpoint:e1",
				ActorType: "endpoint",
				Source:    "snmp",
				Match:     topologymodel.Match{IPAddresses: []string{"10.0.0.3"}},
			},
			{
				ActorID:     "segment:s1",
				ActorType:   "segment",
				SegmentKind: topologymodel.SegmentKindBroadcastDomain,
				Source:      "snmp",
				Match:       topologymodel.Match{Hostnames: []string{"segment:s1"}},
			},
		},
		Links: []topologymodel.Link{
			{
				SrcActorHandle: snmpTopologyTestActorHandle("device:managed-a"),
				DstActorHandle: snmpTopologyTestActorHandle("device:managed-b"),
				Protocol:       "lldp",
				Direction:      "bidirectional",
			},
			{
				SrcActorHandle: snmpTopologyTestActorHandle("device:managed-a"),
				DstActorHandle: snmpTopologyTestActorHandle("segment:s1"),
				Protocol:       "bridge",
				Direction:      "bidirectional",
			},
			{
				SrcActorHandle: snmpTopologyTestActorHandle("segment:s1"),
				DstActorHandle: snmpTopologyTestActorHandle("endpoint:e1"),
				Protocol:       "fdb",
				Direction:      "bidirectional",
			},
		},
	}

	assignSNMPTopologyTestHandles(t, &data)
	topologyshape.ApplyDepthFocusFilter(&data, topologyoptions.QueryOptions{
		ManagedDeviceFocus:     "ip:10.0.0.1",
		Depth:                  0,
		EliminateNonIPInferred: true,
	})

	require.Len(t, data.Actors, 1)
	require.Len(t, data.Links, 0)
	require.Equal(t, "ip:10.0.0.1", topologyStatsToV1ForTest(t, data.Stats)["managed_snmp_device_focus"])
	require.Equal(t, 0, topologyStatsToV1ForTest(t, data.Stats)["depth"])
}

func TestApplyTopologyDepthFocusFilter_ManagedFocusDepthOneIncludesDirectNeighbors(t *testing.T) {
	data := topologymodel.Data{
		Actors: []topologymodel.Actor{
			{
				ActorID:   "device:managed-a",
				ActorType: "device",
				Source:    "snmp",
				Match:     topologymodel.Match{IPAddresses: []string{"10.0.0.1"}},
			},
			{
				ActorID:   "device:managed-b",
				ActorType: "device",
				Source:    "snmp",
				Match:     topologymodel.Match{IPAddresses: []string{"10.0.0.2"}},
			},
			{
				ActorID:   "endpoint:e1",
				ActorType: "endpoint",
				Source:    "snmp",
				Match:     topologymodel.Match{IPAddresses: []string{"10.0.0.3"}},
			},
			{
				ActorID:     "segment:s1",
				ActorType:   "segment",
				SegmentKind: topologymodel.SegmentKindBroadcastDomain,
				Source:      "snmp",
				Match:       topologymodel.Match{Hostnames: []string{"segment:s1"}},
			},
		},
		Links: []topologymodel.Link{
			{
				SrcActorHandle: snmpTopologyTestActorHandle("device:managed-a"),
				DstActorHandle: snmpTopologyTestActorHandle("device:managed-b"),
				Protocol:       "lldp",
				Direction:      "bidirectional",
			},
			{
				SrcActorHandle: snmpTopologyTestActorHandle("device:managed-a"),
				DstActorHandle: snmpTopologyTestActorHandle("segment:s1"),
				Protocol:       "bridge",
				Direction:      "bidirectional",
			},
			{
				SrcActorHandle: snmpTopologyTestActorHandle("segment:s1"),
				DstActorHandle: snmpTopologyTestActorHandle("endpoint:e1"),
				Protocol:       "fdb",
				Direction:      "bidirectional",
			},
		},
	}

	assignSNMPTopologyTestHandles(t, &data)
	topologyshape.ApplyDepthFocusFilter(&data, topologyoptions.QueryOptions{
		ManagedDeviceFocus:     "ip:10.0.0.1",
		Depth:                  1,
		EliminateNonIPInferred: true,
	})

	require.Len(t, data.Actors, 4)
	require.Len(t, data.Links, 3)
	require.Equal(t, "ip:10.0.0.1", topologyStatsToV1ForTest(t, data.Stats)["managed_snmp_device_focus"])
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["depth"])
}

func TestApplyTopologyDepthFocusFilter_MultiFocusDepthZeroIncludesAllShortestPaths(t *testing.T) {
	data := topologymodel.Data{
		Actors: []topologymodel.Actor{
			{
				ActorID:   "device:managed-a",
				ActorType: "device",
				Source:    "snmp",
				Match:     topologymodel.Match{IPAddresses: []string{"10.0.0.1"}},
			},
			{
				ActorID:   "device:managed-b",
				ActorType: "device",
				Source:    "snmp",
				Match:     topologymodel.Match{IPAddresses: []string{"10.0.0.2"}},
			},
			{
				ActorID:   "device:managed-c",
				ActorType: "device",
				Source:    "snmp",
				Match:     topologymodel.Match{IPAddresses: []string{"10.0.0.3"}},
			},
			{
				ActorID:     "segment:s1",
				ActorType:   "segment",
				SegmentKind: topologymodel.SegmentKindBroadcastDomain,
				Source:      "snmp",
				Match:       topologymodel.Match{Hostnames: []string{"segment:s1"}},
			},
		},
		Links: []topologymodel.Link{
			{
				SrcActorHandle: snmpTopologyTestActorHandle("device:managed-a"),
				DstActorHandle: snmpTopologyTestActorHandle("device:managed-b"),
				Protocol:       "lldp",
				Direction:      "bidirectional",
			},
			{
				SrcActorHandle: snmpTopologyTestActorHandle("device:managed-b"),
				DstActorHandle: snmpTopologyTestActorHandle("device:managed-c"),
				Protocol:       "lldp",
				Direction:      "bidirectional",
			},
			{
				SrcActorHandle: snmpTopologyTestActorHandle("device:managed-a"),
				DstActorHandle: snmpTopologyTestActorHandle("segment:s1"),
				Protocol:       "bridge",
				Direction:      "bidirectional",
			},
			{
				SrcActorHandle: snmpTopologyTestActorHandle("segment:s1"),
				DstActorHandle: snmpTopologyTestActorHandle("device:managed-c"),
				Protocol:       "fdb",
				Direction:      "bidirectional",
			},
		},
	}

	assignSNMPTopologyTestHandles(t, &data)
	topologyshape.ApplyDepthFocusFilter(&data, topologyoptions.QueryOptions{
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
	require.Equal(t, "ip:10.0.0.1,ip:10.0.0.3", topologyStatsToV1ForTest(t, data.Stats)["managed_snmp_device_focus"])
	require.Equal(t, 0, topologyStatsToV1ForTest(t, data.Stats)["depth"])
}

func TestApplyTopologyDepthFocusFilter_DepthExpandsFromSelectedRootsOnly(t *testing.T) {
	data := topologymodel.Data{
		Actors: []topologymodel.Actor{
			{
				ActorID:   "device:managed-a",
				ActorType: "device",
				Source:    "snmp",
				Match:     topologymodel.Match{IPAddresses: []string{"10.0.0.1"}},
			},
			{
				ActorID:   "device:managed-b",
				ActorType: "device",
				Source:    "snmp",
				Match:     topologymodel.Match{IPAddresses: []string{"10.0.0.2"}},
			},
			{
				ActorID:   "device:managed-c",
				ActorType: "device",
				Source:    "snmp",
				Match:     topologymodel.Match{IPAddresses: []string{"10.0.0.3"}},
			},
			{
				ActorID:   "endpoint:x",
				ActorType: "endpoint",
				Source:    "snmp",
				Match:     topologymodel.Match{IPAddresses: []string{"10.0.0.50"}},
			},
		},
		Links: []topologymodel.Link{
			{
				SrcActorHandle: snmpTopologyTestActorHandle("device:managed-a"),
				DstActorHandle: snmpTopologyTestActorHandle("device:managed-b"),
				Protocol:       "lldp",
				Direction:      "bidirectional",
			},
			{
				SrcActorHandle: snmpTopologyTestActorHandle("device:managed-b"),
				DstActorHandle: snmpTopologyTestActorHandle("device:managed-c"),
				Protocol:       "lldp",
				Direction:      "bidirectional",
			},
			{
				SrcActorHandle: snmpTopologyTestActorHandle("device:managed-b"),
				DstActorHandle: snmpTopologyTestActorHandle("endpoint:x"),
				Protocol:       "fdb",
				Direction:      "bidirectional",
			},
		},
	}

	assignSNMPTopologyTestHandles(t, &data)
	topologyshape.ApplyDepthFocusFilter(&data, topologyoptions.QueryOptions{
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
		endpointHandle := snmpTopologyTestActorHandle("endpoint:x")
		assert.False(t, link.SrcActorHandle == endpointHandle || link.DstActorHandle == endpointHandle)
	}
}

func countActorsByType(data topologymodel.Data, actorType string) int {
	total := 0
	for _, actor := range data.Actors {
		if actor.ActorType == actorType {
			total++
		}
	}
	return total
}
