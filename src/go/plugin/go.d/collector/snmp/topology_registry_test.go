// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"testing"
	"time"

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

	data, ok := registry.snapshot()
	require.True(t, ok)
	require.Equal(t, "2", data.Layer)
	require.Equal(t, "snmp", data.Source)
	require.Equal(t, "summary", data.View)

	require.GreaterOrEqual(t, data.Stats["devices_total"].(int), 2)
	require.GreaterOrEqual(t, data.Stats["links_total"].(int), 1)
	require.GreaterOrEqual(t, data.Stats["links_lldp"].(int), 1)
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

	data, ok := registry.snapshot()
	require.True(t, ok)
	require.Len(t, data.Links, 1)
	require.Equal(t, "lldp", data.Links[0].Protocol)
	require.Equal(t, "unidirectional", data.Links[0].Direction)
	_, hasPairConsistency := data.Links[0].Metrics["pair_consistent"]
	require.False(t, hasPairConsistency)
	require.Equal(t, 1, data.Stats["links_unidirectional"].(int))
	require.Equal(t, 0, data.Stats["links_bidirectional"].(int))
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
	require.Len(t, snapshot.l2Observations, 1)
	require.Equal(t, snapshot.localDeviceID, snapshot.l2Observations[0].DeviceID)
	require.Len(t, snapshot.l2Observations[0].LLDPRemotes, 1)
	require.Len(t, snapshot.l2Observations[0].CDPRemotes, 1)
}

func TestTopologyRegistry_SnapshotReturnsFalseWithoutCollectedCaches(t *testing.T) {
	registry := newTopologyRegistry()
	cache := newTopologyCache()
	registry.register(cache)

	_, ok := registry.snapshot()
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

	baseline, ok := registry.snapshot()
	require.True(t, ok)
	require.NotEmpty(t, baseline.Actors)
	require.NotEmpty(t, baseline.Links)

	for range 10 {
		next, ok := registry.snapshot()
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

	data, ok := registry.snapshot()
	require.True(t, ok)

	require.Len(t, data.Links, 1)
	require.Equal(t, 1, data.Stats["links_total"])
	require.Equal(t, 2, countActorsByType(data, "device"))
}

func TestMergeSNMPTopologyData_DeduplicatesActorsByOverlappingIdentity(t *testing.T) {
	collectedAt := time.Now().UTC()
	l2 := topologyData{
		SchemaVersion: topologySchemaVersion,
		Source:        "snmp",
		Layer:         "2",
		View:          "summary",
		AgentID:       "agent-test",
		CollectedAt:   collectedAt,
		Actors: []topologyActor{
			{
				ActorType: "device",
				Layer:     "2",
				Source:    "snmp",
				Match: topologyMatch{
					ChassisIDs:  []string{"10.20.4.2"},
					IPAddresses: []string{"10.20.4.2"},
					SysName:     "sw-a",
				},
			},
		},
	}
	l3 := topologyData{
		SchemaVersion: topologySchemaVersion,
		Source:        "snmp",
		Layer:         "3",
		View:          "summary",
		AgentID:       "agent-test",
		CollectedAt:   collectedAt,
		Actors: []topologyActor{
			{
				ActorType: "device",
				Layer:     "3",
				Source:    "snmp",
				Match: topologyMatch{
					ChassisIDs:   []string{"18:fd:74:33:1a:9c"},
					MacAddresses: []string{"18FD74331A9C"},
					IPAddresses:  []string{"10.20.4.2"},
					SysName:      "sw-a-routing",
				},
			},
		},
	}

	merged, ok := mergeSNMPTopologyData(l2, true, l3, true, "agent-test", collectedAt)
	require.True(t, ok)
	require.Len(t, merged.Actors, 1)
}

func TestCanonicalMatchKey_NormalizesEquivalentMACRepresentations(t *testing.T) {
	raw := topologyMatch{ChassisIDs: []string{"7049a26572cd"}}
	colon := topologyMatch{MacAddresses: []string{"70:49:A2:65:72:CD"}}
	require.Equal(t, "chassis:70:49:a2:65:72:cd", canonicalMatchKey(raw))
	require.Equal(t, "mac:70:49:a2:65:72:cd", canonicalMatchKey(colon))
	require.Contains(t, topologyMatchIdentityKeys(raw), "hw:70:49:a2:65:72:cd")
	require.Contains(t, topologyMatchIdentityKeys(colon), "hw:70:49:a2:65:72:cd")
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
