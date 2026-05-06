// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/topology/engine"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestCollector(dev ddsnmp.DeviceConnectionInfo) *Collector {
	cache := newTopologyCache()
	cache.localDevice = buildLocalTopologyDevice(dev)
	cache.agentID = dev.Hostname
	cache.updateTime = time.Now()
	return &Collector{
		topologyCache: cache,
		deviceCaches:  map[string]*topologyCache{dev.Hostname: cache},
	}
}

func TestTopologyCache_LldpSnapshot(t *testing.T) {
	coll := newTestCollector(ddsnmp.DeviceConnectionInfo{
		Hostname: "10.0.0.1", SysObjectID: "1.3.6.1.4.1.9.1.1", SysName: "sw1", SysDescr: "Switch 1", SysLocation: "dc1",
	})

	pms := []*ddsnmp.ProfileMetrics{{
		DeviceMetadata: map[string]ddsnmp.MetaTag{
			tagLldpLocChassisID:        {Value: "00:11:22:33:44:55"},
			tagLldpLocChassisIDSubtype: {Value: "4"},
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

func TestTopologyCache_UpdateTopologyProfileTags_STPBridgeAddressSetsSNMPIdentity(t *testing.T) {
	coll := newTestCollector(ddsnmp.DeviceConnectionInfo{Hostname: "10.20.4.2"})
	coll.topologyCache.localDevice.ChassisID = "10.20.4.2"
	coll.topologyCache.localDevice.ChassisIDType = "management_ip"

	coll.updateTopologyProfileTags([]*ddsnmp.ProfileMetrics{{
		DeviceMetadata: map[string]ddsnmp.MetaTag{
			tagBridgeBaseAddress: {Value: "\"18 FD 74 33 1A 9C \""},
		},
	}})

	require.Equal(t, "18:fd:74:33:1a:9c", coll.topologyCache.stpBaseBridgeAddress)
	require.Equal(t, "18:fd:74:33:1a:9c", coll.topologyCache.localDevice.ChassisID)
	require.Equal(t, "macAddress", coll.topologyCache.localDevice.ChassisIDType)
}

func TestTopologyCache_UpdateFdbEntry_STPBridgeAddressTagSetsSNMPIdentity(t *testing.T) {
	cache := newTopologyCache()
	cache.localDevice = topologyDevice{
		ChassisID:     "10.20.4.2",
		ChassisIDType: "management_ip",
	}

	cache.updateFdbEntry(map[string]string{
		tagStpBaseBridgeAddress: "18 FD 74 33 1A 9C",
		tagFdbMac:               "70:49:a2:65:72:cd",
		tagFdbBridgePort:        "7",
		tagFdbStatus:            "learned",
	})

	require.Equal(t, "18:fd:74:33:1a:9c", cache.stpBaseBridgeAddress)
	require.Equal(t, "18:fd:74:33:1a:9c", cache.localDevice.ChassisID)
	require.Equal(t, "macAddress", cache.localDevice.ChassisIDType)
}

func TestTopologyCache_BuildEngineObservation_DerivesBaseBridgeMACFromInterfacePhysAddress(t *testing.T) {
	cache := newTopologyCache()
	cache.updateTime = time.Now()
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent1"
	cache.localDevice = topologyDevice{
		ChassisID:     "10.20.4.2",
		ChassisIDType: "management_ip",
		ManagementIP:  "10.20.4.2",
	}

	cache.updateIfIndexByIP(map[string]string{
		tagTopoIPAddr:  "10.20.4.2",
		tagTopoIfIndex: "1",
	})
	cache.updateIfNameByIndex(map[string]string{
		tagTopoIfIndex: "1",
		tagTopoIfName:  "Port1",
		tagTopoIfPhys:  "\"18 FD 74 33 1A 9C \"",
	})

	obs := cache.buildEngineObservation(cache.localDevice)
	require.Equal(t, "18:fd:74:33:1a:9c", obs.BaseBridgeAddress)
	require.Equal(t, "macAddress:18:fd:74:33:1a:9c", obs.DeviceID)
}

func TestTopologyCache_UpdateIfIndexByIP_CollectsAllSNMPDeviceIPs(t *testing.T) {
	cache := newTopologyCache()

	cache.updateIfIndexByIP(map[string]string{
		tagTopoIfIndex: "1",
		tagTopoIPAddr:  "10.20.4.1",
		tagTopoIPMask:  "255.255.255.0",
	})
	cache.updateIfIndexByIP(map[string]string{
		tagTopoIfIndex: "2",
		tagTopoIPAddr:  "10.20.4.2",
		tagTopoIPMask:  "255.255.255.0",
	})
	cache.updateIfIndexByIP(map[string]string{
		tagTopoIfIndex: "3",
		tagTopoIPAddr:  "2001:db8::1",
	})
	// Duplicate row should not duplicate management address entries.
	cache.updateIfIndexByIP(map[string]string{
		tagTopoIfIndex: "1",
		tagTopoIPAddr:  "10.20.4.1",
		tagTopoIPMask:  "255.255.255.0",
	})

	require.Equal(t, "1", cache.ifIndexByIP["10.20.4.1"])
	require.Equal(t, "2", cache.ifIndexByIP["10.20.4.2"])
	require.Equal(t, "3", cache.ifIndexByIP["2001:db8::1"])

	addrs := cache.localDevice.ManagementAddresses
	require.Len(t, addrs, 3)
	require.Contains(t, addrs, topologyManagementAddress{
		Address:     "10.20.4.1",
		AddressType: "ipv4",
		Source:      "ip_mib",
	})
	require.Contains(t, addrs, topologyManagementAddress{
		Address:     "10.20.4.2",
		AddressType: "ipv4",
		Source:      "ip_mib",
	})
	require.Contains(t, addrs, topologyManagementAddress{
		Address:     "2001:db8::1",
		AddressType: "ipv6",
		Source:      "ip_mib",
	})
}

func TestTopologyCache_UpdateTopologyProfileTags_LLDPDoesNotOverrideExistingSNMPIdentity(t *testing.T) {
	coll := newTestCollector(ddsnmp.DeviceConnectionInfo{Hostname: "10.20.4.2"})
	coll.topologyCache.localDevice.ChassisID = "18:fd:74:33:1a:9c"
	coll.topologyCache.localDevice.ChassisIDType = "macAddress"
	coll.topologyCache.localDevice.SysName = "MikroTik-Switch"

	coll.updateTopologyProfileTags([]*ddsnmp.ProfileMetrics{{
		DeviceMetadata: map[string]ddsnmp.MetaTag{
			tagLldpLocChassisID:        {Value: "00:11:22:33:44:55"},
			tagLldpLocChassisIDSubtype: {Value: "4"},
			tagLldpLocSysName:          {Value: "lldp-name"},
		},
	}})

	require.Equal(t, "18:fd:74:33:1a:9c", coll.topologyCache.localDevice.ChassisID)
	require.Equal(t, "macAddress", coll.topologyCache.localDevice.ChassisIDType)
	require.Equal(t, "MikroTik-Switch", coll.topologyCache.localDevice.SysName)
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

func TestTopologyCache_UpdateLldpRemote_IgnoresRowsWithoutRemoteIndex(t *testing.T) {
	cache := newTopologyCache()

	cache.updateLldpRemote(map[string]string{
		tagLldpLocPortNum: "7",
		tagLldpRemSysName: "sw-b",
	})

	require.Empty(t, cache.lldpRemotes)
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
	coll := newTestCollector(ddsnmp.DeviceConnectionInfo{
		Hostname: "10.0.0.1", SysObjectID: "1.3.6.1.4.1.9.1.1", SysName: "sw1", SysDescr: "Switch 1",
	})
	coll.updateTopologyProfileTags([]*ddsnmp.ProfileMetrics{{
		DeviceMetadata: map[string]ddsnmp.MetaTag{
			tagLldpLocChassisID:        {Value: "00:11:22:33:44:55"},
			tagLldpLocChassisIDSubtype: {Value: "4"},
			tagLldpLocSysCapEnabled:    {Value: "80"},
			tagLldpLocSysCapSupported:  {Value: "80"},
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

func TestTopologyCache_Dot1qVLANFallbackUsesFDBIDWhenMapMissing(t *testing.T) {
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

	obs := cache.buildEngineObservation(cache.localDevice)
	require.Len(t, obs.FDBEntries, 1)
	require.Equal(t, "100", obs.FDBEntries[0].VLANID)
}

func TestTopologyCache_FDBDiagnostics(t *testing.T) {
	cache := newTopologyCache()

	cache.updateFdbEntry(map[string]string{
		tagDot1qFdbID:     "100",
		tagDot1qFdbPort:   "7",
		tagDot1qFdbStatus: "learned",
	})
	require.Equal(t, 1, cache.fdbRowsDroppedNoMAC)

	cache.updateFdbEntry(map[string]string{
		tagDot1qFdbID:     "100",
		tagDot1qFdbMac:    "7049a26572cd",
		tagDot1qFdbPort:   "7",
		tagDot1qFdbStatus: "learned",
	})
	cache.updateFDBDiagnostics()
	require.Equal(t, 1, cache.fdbRowsUnmappedPort)

	cache.updateBridgePortMap(map[string]string{
		tagBridgeBasePort: "7",
		tagBridgeIfIndex:  "3",
	})
	cache.updateFDBDiagnostics()
	require.Equal(t, 0, cache.fdbRowsUnmappedPort)
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

func TestTopologyCache_BuildEngineObservation_DerivesBaseBridgeMACFromFDBSelfEntries(t *testing.T) {
	cache := newTopologyCache()
	cache.updateTime = time.Now()
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent1"
	cache.localDevice = topologyDevice{
		ChassisID:     "10.20.4.2",
		ChassisIDType: "management_ip",
		ManagementIP:  "10.20.4.2",
	}
	// Device reports FDB rows but no LLDP local chassis; derive identity from self FDB MAC.
	cache.updateFdbEntry(map[string]string{
		tagFdbMac:        "18:fd:74:33:1a:9c",
		tagFdbBridgePort: "1",
		tagFdbStatus:     "self",
	})

	obs := cache.buildEngineObservation(cache.localDevice)
	require.Equal(t, "18:fd:74:33:1a:9c", obs.BaseBridgeAddress)
	require.Equal(t, "macAddress:18:fd:74:33:1a:9c", obs.DeviceID)
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
			name:   "quoted-hex-string",
			in:     "\"18 FD 74 33 1A 9C \"",
			status: stpBridgeIDValid,
			mac:    "18:fd:74:33:1a:9c",
		},
		{
			name:   "hex-string-prefix",
			in:     "Hex-STRING: 18 FD 74 33 1A 9C",
			status: stpBridgeIDValid,
			mac:    "18:fd:74:33:1a:9c",
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
	for range 25 {
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

func TestTopologyCache_BuildEngineObservations_SeparatesProtocolSpecificRemoteObservations(t *testing.T) {
	cache := newTopologyCache()
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
		sysName:          "sw-b",
		managementAddr:   "10.0.0.2",
	}
	cache.cdpRemotes["1:1"] = &cdpRemote{
		ifIndex:    "1",
		ifName:     "Gi0/1",
		deviceID:   "sw-b",
		sysName:    "switch-b",
		devicePort: "Gi0/2",
		address:    "10.0.0.2",
	}

	observations, localDeviceID := cache.buildEngineObservations(cache.localDevice)
	require.Equal(t, "macAddress:00:11:22:33:44:55", localDeviceID)
	require.Len(t, observations, 3)
	require.Equal(t, localDeviceID, observations[0].DeviceID)

	var lldpObservation *topologyengine.L2Observation
	var cdpObservation *topologyengine.L2Observation
	for i := 1; i < len(observations); i++ {
		observation := &observations[i]
		switch {
		case len(observation.LLDPRemotes) > 0:
			lldpObservation = observation
		case len(observation.CDPRemotes) > 0:
			cdpObservation = observation
		}
	}

	require.NotNil(t, lldpObservation)
	require.NotNil(t, cdpObservation)
	require.Equal(t, lldpObservation.DeviceID, cdpObservation.DeviceID)
	require.Equal(t, "macAddress:aa:bb:cc:dd:ee:ff", lldpObservation.DeviceID)
	require.Equal(t, "10.0.0.2", lldpObservation.ManagementIP)
	require.Equal(t, "10.0.0.2", cdpObservation.ManagementIP)
	require.Equal(t, "sw-b", lldpObservation.Hostname)
	require.Equal(t, "switch-b", cdpObservation.Hostname)
	require.Len(t, lldpObservation.LLDPRemotes, 1)
	require.Len(t, cdpObservation.CDPRemotes, 1)
}

func TestTopologyObservationIdentityResolver_ReusesStableRemoteIdentityAcrossSignals(t *testing.T) {
	resolver := newTopologyObservationIdentityResolver(topologyengine.L2Observation{
		DeviceID:     "macAddress:00:11:22:33:44:55",
		Hostname:     "sw-a",
		ManagementIP: "10.0.0.1",
		ChassisID:    "00:11:22:33:44:55",
	})

	idFromLLDP := resolver.resolve([]string{"sw-b"}, "AA-BB-CC-DD-EE-FF", "macAddress", "10.0.0.2")
	idFromCDP := resolver.resolve([]string{"switch-b", "sw-b"}, "", "", "10.0.0.2")
	idFromMgmtIP := resolver.resolve([]string{"switch-b"}, "", "", "10.0.0.2")

	require.Equal(t, "macAddress:aa:bb:cc:dd:ee:ff", idFromLLDP)
	require.Equal(t, idFromLLDP, idFromCDP)
	require.Equal(t, idFromLLDP, idFromMgmtIP)
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

func TestTopologyCache_UpdateIfNameByIndex_StoresExtendedInterfaceFields(t *testing.T) {
	cache := newTopologyCache()

	cache.updateIfNameByIndex(map[string]string{
		tagTopoIfIndex:  "9",
		tagTopoIfName:   "swp09",
		tagTopoIfAlias:  "uplink-core",
		tagTopoIfDescr:  "Uplink Port 9",
		tagTopoIfPhys:   "AA BB CC DD EE FF",
		tagTopoIfHigh:   "1000",
		tagTopoIfLast:   "34567",
		tagTopoIfDuplex: "3",
	})

	status := cache.ifStatusByIndex["9"]
	require.Equal(t, "Uplink Port 9", status.ifDescr)
	require.Equal(t, "uplink-core", status.ifAlias)
	require.Equal(t, "aa:bb:cc:dd:ee:ff", status.mac)
	require.EqualValues(t, 1000_000_000, status.speedBps)
	require.EqualValues(t, 34567, status.lastChange)
	require.Equal(t, "full", status.duplex)
}

func TestBuildLocalTopologyDevice_IncludesSysContactVendorAndModel(t *testing.T) {
	dev := ddsnmp.DeviceConnectionInfo{
		Hostname:    "10.0.0.1",
		SysObjectID: "1.3.6.1.4.1.9.1.1",
		SysName:     "sw1",
		SysDescr:    "Switch 1",
		SysContact:  "ops@example.net",
		SysLocation: "dc1",
		Vendor:      "Cisco",
		Model:       "C9300-24T",
		VnodeGUID:   "11111111-1111-1111-1111-111111111111",
		VnodeLabels: map[string]string{
			"serial":       "SN-12345",
			"version":      "17.9.4",
			"firmware":     "1.2.3",
			"hardware_rev": "A1",
			"sys_uptime":   "123456",
		},
	}

	device := buildLocalTopologyDevice(dev)
	require.Equal(t, "1.3.6.1.4.1.9.1.1", device.SysObjectID)
	require.Equal(t, "sw1", device.SysName)
	require.Equal(t, "Switch 1", device.SysDescr)
	require.Equal(t, "ops@example.net", device.SysContact)
	require.Equal(t, "dc1", device.SysLocation)
	require.Equal(t, "Cisco", device.Vendor)
	require.Equal(t, "C9300-24T", device.Model)
	require.Equal(t, "SN-12345", device.SerialNumber)
	require.Equal(t, "17.9.4", device.SoftwareVersion)
	require.Equal(t, "1.2.3", device.FirmwareVersion)
	require.Equal(t, "A1", device.HardwareVersion)
	require.EqualValues(t, 123456, device.SysUptime)
	require.Equal(t, "11111111-1111-1111-1111-111111111111", device.NetdataHostID)
	require.Equal(t, topologyProfileChartIDPrefix, device.ChartIDPrefix)
	require.Equal(t, topologyProfileChartContextPrefix, device.ChartContextPrefix)
}

func TestCollector_UpdateTopologyScalarMetric_StoresSysUptime(t *testing.T) {
	for _, metricName := range []string{"sysUpTime", "systemUptime"} {
		t.Run(metricName, func(t *testing.T) {
			coll := &Collector{
				topologyCache: newTopologyCache(),
			}
			coll.topologyCache.localDevice = topologyDevice{}

			coll.updateTopologyScalarMetric(ddsnmp.Metric{
				Name:  metricName,
				Value: 4321,
			})

			require.EqualValues(t, 4321, coll.topologyCache.localDevice.SysUptime)
			require.Equal(t, "4321", coll.topologyCache.localDevice.Labels["sys_uptime"])
		})
	}
}

func TestCollector_IngestTopologyProfileMetrics_IncludesHiddenMetrics(t *testing.T) {
	coll := &Collector{
		topologyCache: newTopologyCache(),
	}

	coll.ingestTopologyProfileMetrics([]*ddsnmp.ProfileMetrics{
		{
			HiddenMetrics: []ddsnmp.Metric{
				{
					Name: metricLldpLocPortEntry,
					Tags: map[string]string{
						tagLldpLocPortNum:       "7",
						tagLldpLocPortID:        "Gi1/0/7",
						tagLldpLocPortIDSubtype: "5",
						tagLldpLocPortDesc:      "Gi1/0/7",
					},
				},
				{
					Name: metricLldpRemEntry,
					Tags: map[string]string{
						tagLldpLocPortNum:          "7",
						tagLldpRemIndex:            "1",
						tagLldpRemChassisID:        "001122334455",
						tagLldpRemChassisIDSubtype: "4",
						tagLldpRemPortID:           "Gi1/0/1",
						tagLldpRemPortIDSubtype:    "5",
						tagLldpRemSysName:          "edge-sw1",
						tagLldpRemSysCapEnabled:    "28",
					},
				},
			},
			Metrics: []ddsnmp.Metric{
				{
					Name:  "systemUptime",
					Value: 1234,
				},
			},
		},
	})

	require.Contains(t, coll.topologyCache.lldpLocPorts, "7")
	require.Contains(t, coll.topologyCache.lldpRemotes, "7:1")
	require.EqualValues(t, 1234, coll.topologyCache.localDevice.SysUptime)
	require.Equal(t, "1234", coll.topologyCache.localDevice.Labels["sys_uptime"])
}

func TestBuildLocalTopologyDevice_MapsVersionToSoftwareOnly(t *testing.T) {
	dev := ddsnmp.DeviceConnectionInfo{
		Hostname:  "10.0.0.2",
		VnodeGUID: "22222222-2222-2222-2222-222222222222",
		VnodeLabels: map[string]string{
			"version": "9.1.2",
		},
	}

	device := buildLocalTopologyDevice(dev)
	require.Equal(t, "9.1.2", device.SoftwareVersion)
	require.Empty(t, device.FirmwareVersion)
	require.Empty(t, device.HardwareVersion)
}

func TestAugmentLocalActorFromCache_InjectsIdentityFields(t *testing.T) {
	data := topologyData{
		Actors: []topologyActor{
			{
				ActorType: "device",
				Match: topologyMatch{
					SysName:     "sw1",
					ChassisIDs:  []string{"00:11:22:33:44:55"},
					IPAddresses: []string{"10.0.0.1"},
				},
				Attributes: map[string]any{
					"vendor_derived":              "Acme Derived",
					"vendor_derived_source":       "mac_oui",
					"vendor_derived_confidence":   "low",
					"vendor_derived_match_prefix": "00:11:22",
					"if_statuses": []map[string]any{
						{
							"if_index": 1,
							"if_name":  "swp07",
						},
					},
				},
			},
		},
	}

	local := topologyDevice{
		ChassisID:          "00:11:22:33:44:55",
		SysName:            "sw1",
		SysDescr:           "Switch 1",
		SysContact:         "ops@example.net",
		SysLocation:        "dc1",
		SysUptime:          987654,
		Vendor:             "Cisco",
		Model:              "C9300-24T",
		SerialNumber:       "SN-12345",
		SoftwareVersion:    "17.9.4",
		FirmwareVersion:    "1.2.3",
		HardwareVersion:    "A1",
		NetdataHostID:      "11111111-1111-1111-1111-111111111111",
		ChartIDPrefix:      topologyProfileChartIDPrefix,
		ChartContextPrefix: topologyProfileChartContextPrefix,
		DeviceCharts: map[string]string{
			"ping_rtt": "ping_rtt",
		},
		InterfaceCharts: map[string]topologyInterfaceChartRef{
			"swp07": {
				ChartIDSuffix:    "swp07",
				AvailableMetrics: []string{"ifErrors", "ifTraffic"},
			},
		},
	}

	augmentLocalActorFromCache(&data, local)

	actor := findDeviceActorBySysName(data, "sw1")
	require.NotNil(t, actor)
	require.Equal(t, "Switch 1", actor.Attributes["sys_descr"])
	require.Equal(t, "ops@example.net", actor.Attributes["sys_contact"])
	require.Equal(t, "dc1", actor.Attributes["sys_location"])
	require.EqualValues(t, 987654, actor.Attributes["sys_uptime"])
	require.Equal(t, "Cisco", actor.Attributes["vendor"])
	require.Equal(t, "snmp", actor.Attributes["vendor_source"])
	require.Equal(t, "high", actor.Attributes["vendor_confidence"])
	require.Equal(t, "Acme Derived", actor.Attributes["vendor_derived"])
	require.Equal(t, "mac_oui", actor.Attributes["vendor_derived_source"])
	require.Equal(t, "low", actor.Attributes["vendor_derived_confidence"])
	require.Equal(t, "00:11:22", actor.Attributes["vendor_derived_match_prefix"])
	require.Equal(t, "C9300-24T", actor.Attributes["model"])
	require.Equal(t, "SN-12345", actor.Attributes["serial_number"])
	require.Equal(t, "17.9.4", actor.Attributes["software_version"])
	require.Equal(t, "1.2.3", actor.Attributes["firmware_version"])
	require.Equal(t, "A1", actor.Attributes["hardware_version"])
	require.Equal(t, "11111111-1111-1111-1111-111111111111", actor.Attributes["netdata_host_id"])
	require.Equal(t, topologyProfileChartIDPrefix, actor.Attributes["chart_id_prefix"])
	require.Equal(t, topologyProfileChartContextPrefix, actor.Attributes["chart_context_prefix"])

	deviceCharts, ok := actor.Attributes["device_charts"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "ping_rtt", deviceCharts["ping_rtt"])

	statuses, ok := actor.Attributes["if_statuses"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, statuses, 1)
	require.Equal(t, "swp07", statuses[0]["chart_id_suffix"])
	require.Equal(t, []string{"ifErrors", "ifTraffic"}, statuses[0]["available_metrics"])
}

/* Chart cross-linking test removed — feature dropped during split.
func TestCollector_SyncTopologyChartReferences(t *testing.T) {
	charts := &collectorapi.Charts{}
	require.NoError(t, charts.Add(
		&collectorapi.Chart{
			ID:    "snmp_device_prof_sysUpTime",
			Title: "System Uptime",
			Units: "1",
			Fam:   "sys",
			Ctx:   "snmp.device_prof_sysUpTime",
			Dims: collectorapi.Dims{
				{ID: "snmp_device_prof_sysUpTime", Name: "sysUpTime"},
			},
		},
		&collectorapi.Chart{
			ID:    "snmp_device_prof_ifTraffic_swp07",
			Title: "Traffic swp07",
			Units: "bit/s",
			Fam:   "ifTraffic",
			Ctx:   "snmp.device_prof_ifTraffic",
			Dims: collectorapi.Dims{
				{ID: "snmp_device_prof_ifTraffic_swp07_in", Name: "in"},
			},
		},
		&collectorapi.Chart{
			ID:    "ping_rtt",
			Title: "Ping round-trip time",
			Units: "milliseconds",
			Fam:   "Ping/RTT",
			Ctx:   "snmp.device_ping_rtt",
			Dims: collectorapi.Dims{
				{ID: "ping_rtt_avg", Name: "avg"},
			},
		},
	))

	coll := &Collector{
		charts:            charts,
		seenScalarMetrics: map[string]bool{"sysUpTime": true},
		ifaceCache:        newIfaceCache(),
		topologyCache:     newTopologyCache(),
		vnode:             &vnodes.VirtualNode{GUID: "11111111-1111-1111-1111-111111111111"},
	}

	coll.ifaceCache.interfaces["swp07"] = &ifaceEntry{
		name: "swp07",
		availableMetrics: map[string]struct{}{
			"ifTraffic": {},
			"ifErrors":  {},
		},
		updated: true,
	}

	coll.syncTopologyChartReferences()

	local := coll.topologyCache.localDevice
	require.Equal(t, "11111111-1111-1111-1111-111111111111", local.NetdataHostID)
	require.Equal(t, topologyProfileChartIDPrefix, local.ChartIDPrefix)
	require.Equal(t, topologyProfileChartContextPrefix, local.ChartContextPrefix)
	require.Equal(t, "snmp_device_prof_sysUpTime", local.DeviceCharts["sysUpTime"])
	require.Equal(t, "ping_rtt", local.DeviceCharts["ping_rtt"])
	require.Contains(t, local.InterfaceCharts, "swp07")
	require.Equal(t, "swp07", local.InterfaceCharts["swp07"].ChartIDSuffix)
	require.Equal(t, []string{"ifTraffic"}, local.InterfaceCharts["swp07"].AvailableMetrics)
}
*/

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
		if slices.Contains(snapshot.Actors[i].Match.MacAddresses, mac) {
			return &snapshot.Actors[i]
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
	return slices.Contains(values, target)
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
