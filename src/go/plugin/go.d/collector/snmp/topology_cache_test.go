// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTopologyCache_LldpSnapshot(t *testing.T) {
	coll := &Collector{
		Config:        Config{Hostname: "10.0.0.1"},
		topologyCache: newTopologyCache(),
		sysInfo:       &snmputils.SysInfo{SysObjectID: "1.3.6.1.4.1.9.1.1", Name: "sw1", Descr: "Switch 1", Location: "dc1"},
	}

	coll.resetTopologyCache()

	pms := []*ddsnmp.ProfileMetrics{{
		Tags: map[string]string{
			tagLldpLocChassisID:        "00:11:22:33:44:55",
			tagLldpLocChassisIDSubtype: "4",
		},
	}}
	coll.updateTopologyProfileTags(pms)

	coll.updateTopologyCacheEntry(ddsnmp.Metric{
		Name: metricLldpLocPortEntry,
		Tags: map[string]string{
			tagLldpLocPortNum:       "1",
			tagLldpLocPortID:        "Gi0/1",
			tagLldpLocPortIDSubtype: "5",
			tagLldpLocPortDesc:      "uplink",
		},
	})
	coll.updateTopologyCacheEntry(ddsnmp.Metric{
		Name: metricLldpRemEntry,
		Tags: map[string]string{
			tagLldpLocPortNum:          "1",
			tagLldpRemIndex:            "1",
			tagLldpRemChassisID:        "aa:bb:cc:dd:ee:ff",
			tagLldpRemChassisIDSubtype: "4",
			tagLldpRemPortID:           "Gi0/2",
			tagLldpRemPortIDSubtype:    "5",
			tagLldpRemPortDesc:         "downlink",
			tagLldpRemSysName:          "sw2",
		},
	})

	coll.finalizeTopologyCache()

	coll.topologyCache.mu.RLock()
	data, ok := coll.topologyCache.snapshot()
	coll.topologyCache.mu.RUnlock()

	require.True(t, ok)
	require.Len(t, data.Actors, 2)
	require.Len(t, data.Links, 1)

	link := data.Links[0]
	assert.Equal(t, "lldp", link.Protocol)
	assert.Equal(t, "bidirectional", link.Direction)
	assert.Equal(t, "Gi0/1", link.Src.Attributes["port_id"])
	assert.Equal(t, "Gi0/2", link.Dst.Attributes["port_id"])
	assert.Equal(t, "sw2", link.Dst.Attributes["sys_name"])
}

func TestTopologyCache_CdpSnapshot(t *testing.T) {
	cache := newTopologyCache()
	cache.updateTime = time.Now()
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent1"
	cache.localDevice = topologyDevice{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		ManagementIP:  "10.0.0.1",
	}

	cache.cdpRemotes["2:1"] = &cdpRemote{
		ifIndex:    "2",
		ifName:     "Gi0/2",
		deviceID:   "sw3",
		devicePort: "Gi0/3",
		address:    "10.0.0.3",
	}

	cache.mu.RLock()
	data, ok := cache.snapshot()
	cache.mu.RUnlock()

	require.True(t, ok)
	require.Len(t, data.Actors, 2)
	require.Len(t, data.Links, 1)
	assert.Equal(t, "cdp", data.Links[0].Protocol)
	assert.Equal(t, "bidirectional", data.Links[0].Direction)
	assert.Equal(t, "Gi0/2", data.Links[0].Src.Attributes["if_name"])
	assert.Equal(t, "Gi0/3", data.Links[0].Dst.Attributes["port_id"])
}

func TestTopologyCache_CdpSnapshotHexAddress(t *testing.T) {
	cache := newTopologyCache()
	cache.updateTime = time.Now()
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent1"
	cache.localDevice = topologyDevice{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		SysName:       "sw1",
		ManagementIP:  "10.0.0.1",
	}

	cache.cdpRemotes["2:1"] = &cdpRemote{
		ifIndex:    "2",
		ifName:     "Gi0/2",
		deviceID:   "sw3",
		sysName:    "sw3",
		devicePort: "Gi0/3",
		address:    "0a000003",
	}

	cache.mu.RLock()
	data, ok := cache.snapshot()
	cache.mu.RUnlock()

	require.True(t, ok)
	require.Len(t, data.Links, 1)
	assert.Equal(t, "cdp", data.Links[0].Protocol)
	assert.Equal(t, "bidirectional", data.Links[0].Direction)
	assert.True(t, linkHasRawAddressMetric(data.Links[0], "0a000003"))

	remote := findDeviceActorBySysName(data, "sw3")
	require.NotNil(t, remote)
	assert.Contains(t, remote.Match.IPAddresses, "10.0.0.3")
}

func TestTopologyCache_CdpSnapshotRawAddressWithoutIP(t *testing.T) {
	cache := newTopologyCache()
	cache.updateTime = time.Now()
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent1"
	cache.localDevice = topologyDevice{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		SysName:       "sw1",
		ManagementIP:  "10.0.0.1",
	}

	cache.cdpRemotes["2:1"] = &cdpRemote{
		ifIndex:    "2",
		ifName:     "Gi0/2",
		deviceID:   "edge-sw3",
		sysName:    "edge-sw3",
		devicePort: "Gi0/3",
		address:    "edge-sw3.mgmt.local",
	}

	cache.mu.RLock()
	data, ok := cache.snapshot()
	cache.mu.RUnlock()

	require.True(t, ok)
	require.Len(t, data.Links, 1)
	assert.Equal(t, "cdp", data.Links[0].Protocol)
	assert.Equal(t, "bidirectional", data.Links[0].Direction)
	assert.True(t, linkHasRawAddressMetric(data.Links[0], "edge-sw3.mgmt.local"))
}

func TestTopologyCache_SnapshotBidirectionalPairMetadata(t *testing.T) {
	cache := newTopologyCache()
	cache.updateTime = time.Now()
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent1"
	cache.localDevice = topologyDevice{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		SysName:       "sw1",
		ManagementIP:  "10.0.0.1",
	}
	cache.lldpLocPorts["1"] = &lldpLocPort{
		portNum:       "1",
		portID:        "Gi0/1",
		portIDSubtype: "interfaceName",
		portDesc:      "uplink",
	}
	cache.lldpRemotes["1:1"] = &lldpRemote{
		localPortNum:     "1",
		remIndex:         "1",
		chassisID:        "aa:bb:cc:dd:ee:ff",
		chassisIDSubtype: "macAddress",
		portID:           "Gi0/2",
		portIDSubtype:    "interfaceName",
		portDesc:         "downlink",
		sysName:          "sw2",
		managementAddr:   "10.0.0.2",
	}

	cache.mu.RLock()
	data, ok := cache.snapshot()
	cache.mu.RUnlock()

	require.True(t, ok)
	require.Len(t, data.Links, 1)
	link := data.Links[0]
	require.Equal(t, "lldp", link.Protocol)
	require.Equal(t, "bidirectional", link.Direction)
	require.Equal(t, true, link.Metrics["pair_consistent"])
	require.Equal(t, 1, data.Stats["links_bidirectional"])
	require.Equal(t, 0, data.Stats["links_unidirectional"])
}

func TestTopologyCache_SnapshotMergesRemoteIdentityAcrossProtocols(t *testing.T) {
	cache := newTopologyCache()
	cache.updateTime = time.Now()
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent1"
	cache.localDevice = topologyDevice{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		SysName:       "sw1",
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
		sysName:          "sw2",
		managementAddr:   "10.0.0.2",
	}
	cache.cdpRemotes["1:1"] = &cdpRemote{
		ifIndex:    "1",
		ifName:     "Gi0/1",
		deviceID:   "sw2.domain.local",
		sysName:    "sw2",
		devicePort: "Gi0/2",
		address:    "10.0.0.2",
	}

	cache.mu.RLock()
	data, ok := cache.snapshot()
	cache.mu.RUnlock()

	require.True(t, ok)
	require.Equal(t, 2, countDeviceActors(data))
	require.NotNil(t, findLinkByProtocol(data, "lldp"))
	require.NotNil(t, findLinkByProtocol(data, "cdp"))

	remoteIdentityMatches := 0
	for _, actor := range data.Actors {
		if actor.ActorType != "device" {
			continue
		}
		if actor.Match.SysName == "sw2" ||
			containsString(actor.Match.ChassisIDs, "aa:bb:cc:dd:ee:ff") ||
			containsString(actor.Match.IPAddresses, "10.0.0.2") {
			remoteIdentityMatches++
		}
	}
	require.Equal(t, 1, remoteIdentityMatches)
}

func TestTopologyCache_LLDPManagementAddressesAndCaps(t *testing.T) {
	coll := &Collector{
		Config:        Config{Hostname: "10.0.0.1"},
		topologyCache: newTopologyCache(),
		sysInfo:       &snmputils.SysInfo{SysObjectID: "1.3.6.1.4.1.9.1.1", Name: "sw1", Descr: "Switch 1"},
	}

	coll.resetTopologyCache()
	coll.updateTopologyProfileTags([]*ddsnmp.ProfileMetrics{{
		Tags: map[string]string{
			tagLldpLocChassisID:        "00:11:22:33:44:55",
			tagLldpLocChassisIDSubtype: "4",
			tagLldpLocSysCapEnabled:    "80",
			tagLldpLocSysCapSupported:  "80",
		},
	}})

	coll.updateTopologyCacheEntry(ddsnmp.Metric{
		Name: metricLldpLocManAddrEntry,
		Tags: map[string]string{
			tagLldpLocMgmtAddrSubtype: "2",
			tagLldpLocMgmtAddr:        "0a000001",
			tagLldpLocMgmtAddrIfID:    "1",
		},
	})
	coll.updateTopologyCacheEntry(ddsnmp.Metric{
		Name: metricLldpRemManAddrEntry,
		Tags: map[string]string{
			tagLldpLocPortNum:         "1",
			tagLldpRemIndex:           "1",
			tagLldpRemMgmtAddrSubtype: "2",
			tagLldpRemMgmtAddr:        "0a000002",
		},
	})
	coll.updateTopologyCacheEntry(ddsnmp.Metric{
		Name: metricLldpRemManAddrEntry,
		Tags: map[string]string{
			tagLldpLocPortNum:         "1",
			tagLldpRemIndex:           "1",
			tagLldpRemMgmtAddrSubtype: "1",
			tagLldpRemMgmtAddr:        "31302e32302e342e3834", // "10.20.4.84" ASCII-hex
		},
	})
	coll.updateTopologyCacheEntry(ddsnmp.Metric{
		Name: metricLldpRemManAddrEntry,
		Tags: map[string]string{
			tagLldpLocPortNum:         "1",
			tagLldpRemIndex:           "1",
			tagLldpRemMgmtAddrSubtype: "1",
			tagLldpRemMgmtAddr:        "666330303a663835333a6363643a653739333a3a31", // "fc00:f853:ccd:e793::1" ASCII-hex
		},
	})
	coll.updateTopologyCacheEntry(ddsnmp.Metric{
		Name: metricLldpRemManAddrEntry,
		Tags: map[string]string{
			tagLldpLocPortNum:                 "1",
			tagLldpRemIndex:                   "1",
			tagLldpRemMgmtAddrSubtype:         "1",
			tagLldpRemMgmtAddrLen:             "4",
			tagLldpRemMgmtAddrOctetPref + "1": "10",
			tagLldpRemMgmtAddrOctetPref + "2": "20",
			tagLldpRemMgmtAddrOctetPref + "3": "4",
			tagLldpRemMgmtAddrOctetPref + "4": "21",
		},
	})
	coll.updateTopologyCacheEntry(ddsnmp.Metric{
		Name: metricLldpRemEntry,
		Tags: map[string]string{
			tagLldpLocPortNum:          "1",
			tagLldpRemIndex:            "1",
			tagLldpRemChassisID:        "aa:bb:cc:dd:ee:ff",
			tagLldpRemChassisIDSubtype: "4",
			tagLldpRemSysName:          "sw2",
			tagLldpRemSysCapEnabled:    "80",
		},
	})

	coll.finalizeTopologyCache()

	coll.topologyCache.mu.RLock()
	data, ok := coll.topologyCache.snapshot()
	coll.topologyCache.mu.RUnlock()

	require.True(t, ok)
	require.Greater(t, len(data.Actors), 1)
	require.True(t, actorHasAttributeList(data, "management_addresses"))
	require.True(t, actorHasAttributeList(data, "capabilities_enabled"))
	require.True(t, containsMgmtAddr(data, map[string]struct{}{
		"10.0.0.2":              {},
		"10.20.4.21":            {},
		"10.20.4.84":            {},
		"fc00:f853:ccd:e793::1": {},
	}))
}

func TestTopologyCache_CDPManagementAddresses(t *testing.T) {
	cache := newTopologyCache()
	cache.updateTime = time.Now()
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent1"
	cache.localDevice = topologyDevice{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		ManagementIP:  "10.0.0.1",
	}

	cache.updateCdpRemote(map[string]string{
		tagCdpIfIndex:               "2",
		tagCdpDeviceIndex:           "1",
		tagCdpDeviceID:              "sw3",
		tagCdpPrimaryMgmtAddrType:   "1",
		tagCdpPrimaryMgmtAddr:       "0a000003",
		tagCdpSecondaryMgmtAddrType: "1",
		tagCdpSecondaryMgmtAddr:     "0a000004",
	})

	cache.mu.RLock()
	data, ok := cache.snapshot()
	cache.mu.RUnlock()

	require.True(t, ok)
	require.True(t, containsMgmtAddr(data, map[string]struct{}{"10.0.0.3": {}, "10.0.0.4": {}}))
}

func TestTopologyCache_FDBAndARPEnrichment(t *testing.T) {
	cache := newTopologyCache()
	cache.updateTime = time.Now()
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent1"
	cache.localDevice = topologyDevice{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		ManagementIP:  "10.0.0.1",
	}

	cache.updateIfNameByIndex(map[string]string{
		tagTopoIfIndex: "3",
		tagTopoIfName:  "Port3",
	})
	cache.updateBridgePortMap(map[string]string{
		tagBridgeBasePort: "3",
		tagBridgeIfIndex:  "3",
	})
	cache.updateFdbEntry(map[string]string{
		tagFdbMac:        "7049a26572cd",
		tagFdbBridgePort: "3",
		tagFdbStatus:     "learned",
	})
	cache.updateArpEntry(map[string]string{
		tagArpIfIndex: "3",
		tagArpIfName:  "Port3",
		tagArpIP:      "10.20.4.84",
		tagArpMac:     "70:49:a2:65:72:cd",
		tagArpState:   "reachable",
	})

	cache.mu.RLock()
	data, ok := cache.snapshot()
	cache.mu.RUnlock()

	require.True(t, ok)
	require.GreaterOrEqual(t, len(data.Actors), 2)
	require.Len(t, data.Links, 1)

	require.NotNil(t, findLinkByProtocol(data, "fdb"))
	require.Nil(t, findLinkByProtocol(data, "bridge"))
	require.Nil(t, findLinkByProtocol(data, "arp"))

	ep := findActorByMAC(data, "70:49:a2:65:72:cd")
	require.NotNil(t, ep)
	assert.Equal(t, "endpoint", ep.ActorType)
	assert.Contains(t, ep.Match.IPAddresses, "10.20.4.84")
	assert.Equal(t, "single_port_mac", ep.Attributes["attachment_source"])
	assert.Equal(t, "Port3", ep.Attributes["attached_port"])
}

func TestTopologyCache_Dot1qVLANEnrichment(t *testing.T) {
	cache := newTopologyCache()
	cache.updateTime = time.Now()
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent1"
	cache.localDevice = topologyDevice{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		ManagementIP:  "10.0.0.1",
	}

	cache.updateBridgePortMap(map[string]string{
		tagBridgeBasePort: "7",
		tagBridgeIfIndex:  "3",
	})
	cache.updateFdbEntry(map[string]string{
		tagDot1qFdbID:     "100",
		tagDot1qFdbMac:    "7049a26572cd",
		tagDot1qFdbPort:   "7",
		tagDot1qFdbStatus: "learned",
	})
	cache.updateDot1qVlanMap(map[string]string{
		tagDot1qVlanID:    "200",
		tagDot1qVlanFdbID: "100",
	})

	obs := cache.buildEngineObservation(cache.localDevice)
	require.Len(t, obs.FDBEntries, 1)
	require.Equal(t, "200", obs.FDBEntries[0].VLANID)
	require.Equal(t, "70:49:a2:65:72:cd", obs.FDBEntries[0].MAC)
}

func TestTopologyCache_VTPVLANNameEnrichment(t *testing.T) {
	cache := newTopologyCache()
	cache.updateTime = time.Now()
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent1"
	cache.localDevice = topologyDevice{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		ManagementIP:  "10.0.0.1",
	}

	cache.updateBridgePortMap(map[string]string{
		tagBridgeBasePort: "7",
		tagBridgeIfIndex:  "3",
	})
	cache.updateDot1qVlanMap(map[string]string{
		tagDot1qVlanID:    "200",
		tagDot1qVlanFdbID: "100",
	})
	cache.updateVtpVlanEntry(map[string]string{
		tagVtpVlanIndex: "200",
		tagVtpVlanState: "operational",
		tagVtpVlanType:  "1",
		tagVtpVlanName:  "servers",
	})
	cache.updateFdbEntry(map[string]string{
		tagDot1qFdbID:     "100",
		tagDot1qFdbMac:    "7049a26572cd",
		tagDot1qFdbPort:   "7",
		tagDot1qFdbStatus: "learned",
	})

	obs := cache.buildEngineObservation(cache.localDevice)
	require.Len(t, obs.FDBEntries, 1)
	require.Equal(t, "200", obs.FDBEntries[0].VLANID)
	require.Equal(t, "servers", obs.FDBEntries[0].VLANName)
}

func TestTopologyCache_STPObservation(t *testing.T) {
	cache := newTopologyCache()
	cache.updateTime = time.Now()
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent1"
	cache.localDevice = topologyDevice{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		ManagementIP:  "10.0.0.1",
	}
	cache.stpBaseBridgeAddress = "00:11:22:33:44:55"
	cache.updateBridgePortMap(map[string]string{
		tagBridgeBasePort: "3",
		tagBridgeIfIndex:  "3",
	})
	cache.updateIfNameByIndex(map[string]string{
		tagTopoIfIndex: "3",
		tagTopoIfName:  "Port3",
	})
	cache.updateStpPortEntry(map[string]string{
		tagStpPort:                 "3",
		tagStpPortState:            "forwarding",
		tagStpPortEnable:           "enabled",
		tagStpPortPathCost:         "4",
		tagStpPortDesignatedBridge: "800066778899aabb",
		tagStpPortDesignatedPort:   "8001",
	})

	obs := cache.buildEngineObservation(cache.localDevice)
	require.Equal(t, "00:11:22:33:44:55", obs.BaseBridgeAddress)
	require.Len(t, obs.STPPorts, 1)
	require.Equal(t, "3", obs.STPPorts[0].Port)
	require.Equal(t, 3, obs.STPPorts[0].IfIndex)
	require.Equal(t, "Port3", obs.STPPorts[0].IfName)
	require.Equal(t, "66:77:88:99:aa:bb", obs.STPPorts[0].DesignatedBridge)
}

func TestTopologyCache_InterfaceStatusObservation(t *testing.T) {
	cache := newTopologyCache()
	cache.updateTime = time.Now()
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent1"
	cache.localDevice = topologyDevice{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		ManagementIP:  "10.0.0.1",
	}

	cache.updateIfNameByIndex(map[string]string{
		tagTopoIfIndex: "3",
		tagTopoIfName:  "Port3",
		tagTopoIfAdmin: "up",
		tagTopoIfOper:  "lowerLayerDown",
	})

	obs := cache.buildEngineObservation(cache.localDevice)
	require.Len(t, obs.Interfaces, 1)
	require.Equal(t, "Port3", obs.Interfaces[0].IfName)
	require.Equal(t, "up", obs.Interfaces[0].AdminStatus)
	require.Equal(t, "lowerLayerDown", obs.Interfaces[0].OperStatus)
}

func TestTopologyCache_InterfaceStatusObservation_FallsBackToIfIndexWhenIfNameMissing(t *testing.T) {
	cache := newTopologyCache()
	cache.updateTime = time.Now()
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent1"
	cache.localDevice = topologyDevice{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		ManagementIP:  "10.0.0.1",
	}

	cache.updateIfNameByIndex(map[string]string{
		tagTopoIfIndex: "7",
		tagTopoIfType:  "ethernetCsmacd(6)",
		tagTopoIfAdmin: "up(1)",
		tagTopoIfOper:  "up(1)",
	})

	obs := cache.buildEngineObservation(cache.localDevice)
	require.Len(t, obs.Interfaces, 1)
	require.Equal(t, 7, obs.Interfaces[0].IfIndex)
	require.Equal(t, "7", obs.Interfaces[0].IfName)
	require.Equal(t, "7", obs.Interfaces[0].IfDescr)
	require.Equal(t, "ethernetcsmacd", obs.Interfaces[0].InterfaceType)
	require.Equal(t, "up", obs.Interfaces[0].AdminStatus)
	require.Equal(t, "up", obs.Interfaces[0].OperStatus)
}

func TestStpBridgeAddressToMAC_ParsesAndRejectsSentinels(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		status stpBridgeIDStatus
		mac    string
	}{
		{
			name:   "bridge-id-hex",
			in:     "800066778899aabb",
			status: stpBridgeIDValid,
			mac:    "66:77:88:99:aa:bb",
		},
		{
			name:   "priority-bridge-id",
			in:     "32768-66.77.88.99.aa.bb",
			status: stpBridgeIDValid,
			mac:    "66:77:88:99:aa:bb",
		},
		{
			name:   "sentinel-text-empty",
			in:     "0-00.00.00.00.00.00",
			status: stpBridgeIDEmpty,
			mac:    "",
		},
		{
			name:   "sentinel-hex-empty",
			in:     "302d30302e30302e30302e30302e30302e3030",
			status: stpBridgeIDEmpty,
			mac:    "",
		},
		{
			name:   "invalid",
			in:     "not-a-bridge-id",
			status: stpBridgeIDInvalid,
			mac:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mac, status := parseSTPBridgeID(tt.in, 0)
			require.Equal(t, tt.status, status)
			require.Equal(t, tt.mac, mac)
			require.Equal(t, tt.mac, stpBridgeAddressToMAC(tt.in))
		})
	}
}

func TestTopologyCache_VTPVLANContexts_SortedAndValidated(t *testing.T) {
	cache := newTopologyCache()
	cache.vlanIDToName["200"] = "servers"
	cache.vlanIDToName["10"] = "users"
	cache.vlanIDToName["abc"] = "invalid"
	cache.vlanIDToName[""] = "invalid-empty"

	contexts := cache.vtpVLANContexts()
	require.Len(t, contexts, 2)
	require.Equal(t, "10", contexts[0].vlanID)
	require.Equal(t, "users", contexts[0].vlanName)
	require.Equal(t, "200", contexts[1].vlanID)
	require.Equal(t, "servers", contexts[1].vlanName)
}

func TestTopologyCache_VLANContextFDBEntriesRemainDistinct(t *testing.T) {
	cache := newTopologyCache()
	cache.updateBridgePortMap(map[string]string{
		tagBridgeBasePort: "7",
		tagBridgeIfIndex:  "3",
	})
	cache.updateIfNameByIndex(map[string]string{
		tagTopoIfIndex: "3",
		tagTopoIfName:  "Port3",
	})

	cache.updateFdbEntry(map[string]string{
		tagFdbMac:                  "70:49:a2:65:72:cd",
		tagFdbBridgePort:           "7",
		tagFdbStatus:               "learned",
		tagTopologyContextVLANID:   "10",
		tagTopologyContextVLANName: "users",
	})
	cache.updateFdbEntry(map[string]string{
		tagFdbMac:                  "70:49:a2:65:72:cd",
		tagFdbBridgePort:           "7",
		tagFdbStatus:               "learned",
		tagTopologyContextVLANID:   "200",
		tagTopologyContextVLANName: "servers",
	})

	obs := cache.buildEngineObservation(cache.localDevice)
	require.Len(t, obs.FDBEntries, 2)
	require.Equal(t, "10", obs.FDBEntries[0].VLANID)
	require.Equal(t, "users", obs.FDBEntries[0].VLANName)
	require.Equal(t, "200", obs.FDBEntries[1].VLANID)
	require.Equal(t, "servers", obs.FDBEntries[1].VLANName)
}

func TestPickManagementIP_DeterministicAcrossInputOrder(t *testing.T) {
	addrsA := []topologyManagementAddress{
		{Address: "10.20.4.60", Source: "src-a"},
		{Address: "10.20.4.205", Source: "src-b"},
	}
	addrsB := []topologyManagementAddress{
		{Address: "10.20.4.205", Source: "src-b"},
		{Address: "10.20.4.60", Source: "src-a"},
	}

	require.Equal(t, "10.20.4.205", pickManagementIP(addrsA))
	require.Equal(t, pickManagementIP(addrsA), pickManagementIP(addrsB))

	rawA := []topologyManagementAddress{
		{Address: "zeta"},
		{Address: "alpha"},
	}
	rawB := []topologyManagementAddress{
		{Address: "alpha"},
		{Address: "zeta"},
	}
	require.Equal(t, "alpha", pickManagementIP(rawA))
	require.Equal(t, pickManagementIP(rawA), pickManagementIP(rawB))
}

func TestTopologyCache_SnapshotDeterministicEndpointIPSelection(t *testing.T) {
	cache := newTopologyCache()
	cache.updateTime = time.Now()
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent1"
	cache.localDevice = topologyDevice{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		ManagementIP:  "10.0.0.1",
	}

	cache.updateIfNameByIndex(map[string]string{
		tagTopoIfIndex: "3",
		tagTopoIfName:  "Port3",
	})
	cache.updateBridgePortMap(map[string]string{
		tagBridgeBasePort: "3",
		tagBridgeIfIndex:  "3",
	})
	cache.updateFdbEntry(map[string]string{
		tagFdbMac:        "d8:5e:d3:0e:c5:e6",
		tagFdbBridgePort: "3",
		tagFdbStatus:     "learned",
	})
	cache.updateArpEntry(map[string]string{
		tagArpIfIndex: "3",
		tagArpIfName:  "Port3",
		tagArpIP:      "10.20.4.60",
		tagArpMac:     "d8:5e:d3:0e:c5:e6",
		tagArpState:   "reachable",
	})
	cache.updateArpEntry(map[string]string{
		tagArpIfIndex: "3",
		tagArpIfName:  "Port3",
		tagArpIP:      "10.20.4.205",
		tagArpMac:     "d8:5e:d3:0e:c5:e6",
		tagArpState:   "reachable",
	})

	expectedIPs := []string{"10.20.4.205", "10.20.4.60"}
	for i := 0; i < 25; i++ {
		cache.mu.RLock()
		data, ok := cache.snapshot()
		cache.mu.RUnlock()

		require.True(t, ok)
		ep := findActorByMAC(data, "d8:5e:d3:0e:c5:e6")
		require.NotNil(t, ep)
		require.Equal(t, expectedIPs, ep.Match.IPAddresses)
	}
}

func TestTopologyCache_SnapshotDeterministicOrdering(t *testing.T) {
	cache := newTopologyCache()
	cache.updateTime = time.Now()
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent1"
	cache.localDevice = topologyDevice{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		ManagementIP:  "10.0.0.1",
	}
	cache.lldpLocPorts["2"] = &lldpLocPort{portNum: "2", portID: "Gi0/2", portIDSubtype: "5"}
	cache.lldpLocPorts["1"] = &lldpLocPort{portNum: "1", portID: "Gi0/1", portIDSubtype: "5"}
	cache.lldpRemotes["2:1"] = &lldpRemote{
		localPortNum:     "2",
		remIndex:         "1",
		chassisID:        "00:00:00:00:00:22",
		chassisIDSubtype: "macAddress",
		sysName:          "sw2",
	}
	cache.lldpRemotes["1:1"] = &lldpRemote{
		localPortNum:     "1",
		remIndex:         "1",
		chassisID:        "00:00:00:00:00:11",
		chassisIDSubtype: "macAddress",
		sysName:          "sw1",
	}
	cache.cdpRemotes["3:1"] = &cdpRemote{
		ifIndex:    "3",
		ifName:     "Gi0/3",
		deviceID:   "sw3",
		devicePort: "Gi0/4",
		address:    "10.0.0.3",
	}

	cache.mu.RLock()
	data, ok := cache.snapshot()
	cache.mu.RUnlock()

	require.True(t, ok)
	require.NotEmpty(t, data.Actors)
	require.NotEmpty(t, data.Links)

	actorOrder := make([]string, 0, len(data.Actors))
	for _, actor := range data.Actors {
		actorOrder = append(actorOrder, actor.ActorType+"|"+canonicalMatchKey(actor.Match))
	}
	expectedActorOrder := append([]string(nil), actorOrder...)
	sort.Strings(expectedActorOrder)
	assert.Equal(t, expectedActorOrder, actorOrder)

	linkOrder := make([]string, 0, len(data.Links))
	for _, link := range data.Links {
		linkOrder = append(linkOrder, topologyLinkSortKey(link))
	}
	expectedLinkOrder := append([]string(nil), linkOrder...)
	sort.Strings(expectedLinkOrder)
	assert.Equal(t, expectedLinkOrder, linkOrder)
}

func TestDecodePrintableASCII_HumanReadableHex(t *testing.T) {
	bs, err := decodeHexString("766d7831")
	require.NoError(t, err)

	decoded := decodePrintableASCII(bs)
	require.Equal(t, "vmx1", decoded)
}

func TestDecodePrintableASCII_HexValueIsNotNumeric(t *testing.T) {
	bs, err := decodeHexString("766d7831")
	require.NoError(t, err)

	decoded := decodePrintableASCII(bs)
	assert.NotRegexp(t, "^[0-9]+$", decoded)
}

func TestNormalizeInterfaceAdminStatusAcceptsEnumStrings(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "up(1)", want: "up"},
		{in: "down(2)", want: "down"},
		{in: "testing(3)", want: "testing"},
		{in: "UP (1)", want: "up"},
		{in: "invalid(9)", want: ""},
	}

	for _, tc := range tests {
		assert.Equal(t, tc.want, normalizeInterfaceAdminStatus(tc.in), tc.in)
	}
}

func TestNormalizeInterfaceOperStatusAcceptsEnumStrings(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "up(1)", want: "up"},
		{in: "down(2)", want: "down"},
		{in: "testing(3)", want: "testing"},
		{in: "unknown(4)", want: "unknown"},
		{in: "dormant(5)", want: "dormant"},
		{in: "notPresent(6)", want: "notPresent"},
		{in: "lowerLayerDown(7)", want: "lowerLayerDown"},
		{in: "LOWERLAYERDOWN (7)", want: "lowerLayerDown"},
		{in: "invalid(9)", want: ""},
	}

	for _, tc := range tests {
		assert.Equal(t, tc.want, normalizeInterfaceOperStatus(tc.in), tc.in)
	}
}

func TestNormalizeInterfaceTypeAcceptsEnumStrings(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "ethernetCsmacd(6)", want: "ethernetcsmacd"},
		{in: "6", want: "ethernetcsmacd"},
		{in: "ieee8023adLag(161)", want: "ieee8023adlag"},
		{in: "161", want: "ieee8023adlag"},
		{in: "l2vlan(135)", want: "l2vlan"},
		{in: "", want: ""},
	}

	for _, tc := range tests {
		assert.Equal(t, tc.want, normalizeInterfaceType(tc.in), tc.in)
	}
}

func TestTopologyCache_UpdateIfNameByIndex_StoresStatusWithoutIfName(t *testing.T) {
	cache := newTopologyCache()

	cache.updateIfNameByIndex(map[string]string{
		tagTopoIfIndex: "7",
		tagTopoIfName:  "swp07",
	})

	cache.updateIfNameByIndex(map[string]string{
		tagTopoIfIndex: "7",
		tagTopoIfAdmin: "up(1)",
		tagTopoIfOper:  "down(2)",
	})

	require.Equal(t, "swp07", cache.ifNamesByIndex["7"])
	require.Equal(t, "up", cache.ifStatusByIndex["7"].admin)
	require.Equal(t, "down", cache.ifStatusByIndex["7"].oper)
}

func actorHasAttributeList(snapshot topologyData, key string) bool {
	for _, actor := range snapshot.Actors {
		if actor.Attributes == nil {
			continue
		}
		value, ok := actor.Attributes[key]
		if !ok || value == nil {
			continue
		}
		switch v := value.(type) {
		case []string:
			if len(v) > 0 {
				return true
			}
		case []topologyManagementAddress:
			if len(v) > 0 {
				return true
			}
		case []any:
			if len(v) > 0 {
				return true
			}
		default:
			return true
		}
	}
	return false
}

func findLinkByProtocol(snapshot topologyData, protocol string) *topologyLink {
	for i := range snapshot.Links {
		if snapshot.Links[i].Protocol == protocol {
			return &snapshot.Links[i]
		}
	}
	return nil
}

func findActorByMAC(snapshot topologyData, mac string) *topologyActor {
	for i := range snapshot.Actors {
		for _, m := range snapshot.Actors[i].Match.MacAddresses {
			if m == mac {
				return &snapshot.Actors[i]
			}
		}
	}
	return nil
}

func countDeviceActors(snapshot topologyData) int {
	total := 0
	for _, actor := range snapshot.Actors {
		if actor.ActorType == "device" {
			total++
		}
	}
	return total
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func linkHasRawAddressMetric(link topologyLink, raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(link.Metrics) == 0 {
		return false
	}
	srcRaw, srcOK := link.Metrics["src_remote_address_raw"].(string)
	dstRaw, dstOK := link.Metrics["dst_remote_address_raw"].(string)
	return (srcOK && srcRaw == raw) || (dstOK && dstRaw == raw)
}

func findDeviceActorBySysName(snapshot topologyData, sysName string) *topologyActor {
	for i := range snapshot.Actors {
		actor := &snapshot.Actors[i]
		if actor.ActorType != "device" {
			continue
		}
		if actor.Match.SysName == sysName {
			return actor
		}
	}
	return nil
}
