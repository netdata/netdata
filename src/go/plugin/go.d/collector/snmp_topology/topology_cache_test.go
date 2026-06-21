// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestTopologyCache(dev ddsnmp.DeviceConnectionInfo) *topologyCache {
	cache := newTopologyCache()
	cache.localDevice = buildLocalTopologyDevice(dev)
	cache.agentID = dev.Hostname
	cache.updateTime = time.Now()
	return cache
}

func TestTopologyMetricHandlersRegisteredForRowKinds(t *testing.T) {
	tests := map[string]struct {
		kind ddsnmp.TopologyKind
	}{
		"lldp_loc_port":            {kind: ddsnmp.KindLldpLocPort},
		"lldp_loc_man_addr":        {kind: ddsnmp.KindLldpLocManAddr},
		"lldp_rem":                 {kind: ddsnmp.KindLldpRem},
		"lldp_rem_man_addr":        {kind: ddsnmp.KindLldpRemManAddr},
		"lldp_rem_man_addr_compat": {kind: ddsnmp.KindLldpRemManAddrCompat},
		"cdp_cache":                {kind: ddsnmp.KindCdpCache},
		"if_name":                  {kind: ddsnmp.KindIfName},
		"if_status":                {kind: ddsnmp.KindIfStatus},
		"if_duplex":                {kind: ddsnmp.KindIfDuplex},
		"ip_if_index":              {kind: ddsnmp.KindIpIfIndex},
		"bridge_port_if_index":     {kind: ddsnmp.KindBridgePortIfIndex},
		"fdb_entry":                {kind: ddsnmp.KindFdbEntry},
		"qbridge_fdb_entry":        {kind: ddsnmp.KindQbridgeFdbEntry},
		"qbridge_vlan_entry":       {kind: ddsnmp.KindQbridgeVlanEntry},
		"stp_port":                 {kind: ddsnmp.KindStpPort},
		"vtp_vlan":                 {kind: ddsnmp.KindVtpVlan},
		"arp_entry":                {kind: ddsnmp.KindArpEntry},
		"arp_legacy_entry":         {kind: ddsnmp.KindArpLegacyEntry},
		"ospf_neighbor":            {kind: ddsnmp.KindOSPFNeighbor},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.NotNil(t, topologyMetricHandlers[tc.kind], "missing topology handler for %s", tc.kind)
		})
	}
}

func TestTopologyCache_LldpSnapshot(t *testing.T) {
	cache := newTestTopologyCache(ddsnmp.DeviceConnectionInfo{
		Hostname: "10.0.0.1", SysObjectID: "1.3.6.1.4.1.9.1.1", SysName: "sw1", SysDescr: "Switch 1", SysLocation: "dc1",
	})

	pms := []*ddsnmp.ProfileMetrics{{
		DeviceMetadata: map[string]ddsnmp.MetaTag{
			tagLldpLocChassisID:        {Value: "00:11:22:33:44:55"},
			tagLldpLocChassisIDSubtype: {Value: "4"},
		},
	}}
	cache.updateTopologyProfileTags(pms)

	cache.updateTopologyCacheEntry(ddsnmp.Metric{
		TopologyKind: ddsnmp.KindLldpLocPort,
		Tags: map[string]string{
			tagLldpLocPortNum:       "1",
			tagLldpLocPortID:        "Gi0/1",
			tagLldpLocPortIDSubtype: "5",
			tagLldpLocPortDesc:      "uplink",
		},
	})
	cache.updateTopologyCacheEntry(ddsnmp.Metric{
		TopologyKind: ddsnmp.KindLldpRem,
		Tags: map[string]string{
			tagLldpLocPortNum:          "1",
			tagLldpRemIndex:            "1",
			tagLldpRemChassisID:        "aa:bb:cc:dd:ee:ff",
			tagLldpRemChassisIDSubtype: "4",
			tagLldpRemPortID:           "Gi0/2",
			tagLldpRemPortIDSubtype:    "5",
			tagLldpRemPortDesc:         "downlink",
			tagLldpRemSysName:          "sw2",
			tagLldpRemMgmtAddr:         "10.0.0.2",
		},
	})

	cache.finalizeTopologyCache()

	data, ok := snapshotTopologyCacheForTest(cache)

	require.True(t, ok)
	require.Len(t, data.Actors, 2)
	require.Len(t, data.Links, 1)

	link := data.Links[0]
	assert.Equal(t, "lldp", link.Protocol)
	assert.Equal(t, "unidirectional", link.Direction)
	assert.Equal(t, "Gi0/1", link.Src.PortID)
	assert.Equal(t, "Gi0/2", link.Dst.PortID)
	assert.Equal(t, "sw2", link.Dst.SysName)
}

func TestTopologyCache_CdpSnapshot(t *testing.T) {
	cache := newTopologyCache()
	cache.updateTime = time.Now()
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent1"
	cache.localDevice = topologymodel.Device{
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

	data, ok := snapshotTopologyCacheForTest(cache)

	require.True(t, ok)
	require.Len(t, data.Actors, 2)
	require.Len(t, data.Links, 1)
	assert.Equal(t, "cdp", data.Links[0].Protocol)
	assert.Equal(t, "unidirectional", data.Links[0].Direction)
	assert.Equal(t, "Gi0/2", data.Links[0].Src.IfName)
	assert.Equal(t, "Gi0/3", data.Links[0].Dst.PortID)
}

func TestTopologyCache_UpdateTopologyProfileTags_STPBridgeAddressSetsSNMPIdentity(t *testing.T) {
	cache := newTestTopologyCache(ddsnmp.DeviceConnectionInfo{Hostname: "10.20.4.2"})
	cache.localDevice.ChassisID = "10.20.4.2"
	cache.localDevice.ChassisIDType = "management_ip"

	cache.updateTopologyProfileTags([]*ddsnmp.ProfileMetrics{{
		DeviceMetadata: map[string]ddsnmp.MetaTag{
			tagBridgeBaseAddress: {Value: "\"18 FD 74 33 1A 9C \""},
		},
	}})

	require.Equal(t, "18:fd:74:33:1a:9c", cache.stpBaseBridgeAddress)
	require.Equal(t, "18:fd:74:33:1a:9c", cache.localDevice.ChassisID)
	require.Equal(t, "macAddress", cache.localDevice.ChassisIDType)
}

func TestTopologyCache_UpdateFdbEntry_STPBridgeAddressTagSetsSNMPIdentity(t *testing.T) {
	cache := newTopologyCache()
	cache.localDevice = topologymodel.Device{
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
	cache.localDevice = topologymodel.Device{
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
	require.Equal(t, "255.255.255.0", cache.ifNetmaskByIP["10.20.4.1"])
	require.Equal(t, "255.255.255.0", cache.ifNetmaskByIP["10.20.4.2"])
	require.Empty(t, cache.ifNetmaskByIP["2001:db8::1"])
	require.Equal(t, topologymodel.L3Interface{
		IP:      "10.20.4.1",
		Netmask: "255.255.255.0",
		IfIndex: "1",
	}, cache.l3InterfacesByIP["10.20.4.1"])
	require.Equal(t, topologymodel.L3Interface{
		IP:      "10.20.4.2",
		Netmask: "255.255.255.0",
		IfIndex: "2",
	}, cache.l3InterfacesByIP["10.20.4.2"])
	require.NotContains(t, cache.l3InterfacesByIP, "2001:db8::1")

	addrs := cache.localDevice.ManagementAddresses
	require.Len(t, addrs, 3)
	require.Contains(t, addrs, topologymodel.ManagementAddress{
		Address:     "10.20.4.1",
		AddressType: "ipv4",
		Source:      "ip_mib",
	})
	require.Contains(t, addrs, topologymodel.ManagementAddress{
		Address:     "10.20.4.2",
		AddressType: "ipv4",
		Source:      "ip_mib",
	})
	require.Contains(t, addrs, topologymodel.ManagementAddress{
		Address:     "2001:db8::1",
		AddressType: "ipv6",
		Source:      "ip_mib",
	})
}

func TestTopologyCache_UpdateTopologyProfileTags_LLDPDoesNotOverrideExistingSNMPIdentity(t *testing.T) {
	cache := newTestTopologyCache(ddsnmp.DeviceConnectionInfo{Hostname: "10.20.4.2"})
	cache.localDevice.ChassisID = "18:fd:74:33:1a:9c"
	cache.localDevice.ChassisIDType = "macAddress"
	cache.localDevice.SysName = "MikroTik-Switch"

	cache.updateTopologyProfileTags([]*ddsnmp.ProfileMetrics{{
		DeviceMetadata: map[string]ddsnmp.MetaTag{
			tagLldpLocChassisID:        {Value: "00:11:22:33:44:55"},
			tagLldpLocChassisIDSubtype: {Value: "4"},
			tagLldpLocSysName:          {Value: "lldp-name"},
		},
	}})

	require.Equal(t, "18:fd:74:33:1a:9c", cache.localDevice.ChassisID)
	require.Equal(t, "macAddress", cache.localDevice.ChassisIDType)
	require.Equal(t, "MikroTik-Switch", cache.localDevice.SysName)
}

func TestTopologyCache_CdpSnapshotHexAddress(t *testing.T) {
	cache := newTopologyCache()
	cache.updateTime = time.Now()
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent1"
	cache.localDevice = topologymodel.Device{
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

	data, ok := snapshotTopologyCacheForTest(cache)

	require.True(t, ok)
	require.Len(t, data.Links, 1)
	assert.Equal(t, "cdp", data.Links[0].Protocol)
	assert.Equal(t, "unidirectional", data.Links[0].Direction)
	assert.True(t, linkHasRawAddressHint(data.Links[0], "0a000003"))

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
	cache.localDevice = topologymodel.Device{
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

	options := defaultTopologyQueryOptionsForTest()
	options.EliminateNonIPInferred = false
	data, ok := snapshotTopologyCacheForTestWithOptions(cache, options)

	require.True(t, ok)
	require.Len(t, data.Links, 1)
	assert.Equal(t, "cdp", data.Links[0].Protocol)
	assert.Equal(t, "unidirectional", data.Links[0].Direction)
	assert.True(t, linkHasRawAddressHint(data.Links[0], "edge-sw3.mgmt.local"))
}

func TestTopologyCache_SnapshotBidirectionalPairMetadata(t *testing.T) {
	cache := newTopologyCache()
	cache.updateTime = time.Now()
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent1"
	cache.localDevice = topologymodel.Device{
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

	remoteCache := newTopologyCache()
	remoteCache.updateTime = cache.updateTime
	remoteCache.lastUpdate = cache.lastUpdate
	remoteCache.agentID = "agent2"
	remoteCache.localDevice = topologymodel.Device{
		ChassisID:     "aa:bb:cc:dd:ee:ff",
		ChassisIDType: "macAddress",
		SysName:       "sw2",
		ManagementIP:  "10.0.0.2",
	}
	remoteCache.lldpLocPorts["2"] = &lldpLocPort{
		portNum:       "2",
		portID:        "Gi0/2",
		portIDSubtype: "interfaceName",
		portDesc:      "downlink",
	}
	remoteCache.lldpRemotes["2:1"] = &lldpRemote{
		localPortNum:     "2",
		remIndex:         "1",
		chassisID:        "00:11:22:33:44:55",
		chassisIDSubtype: "macAddress",
		portID:           "Gi0/1",
		portIDSubtype:    "interfaceName",
		portDesc:         "uplink",
		sysName:          "sw1",
		managementAddr:   "10.0.0.1",
	}

	registry := newTopologyRegistry()
	registry.register(cache)
	registry.register(remoteCache)
	data, ok := snapshotTopologyRegistryForTest(registry)

	require.True(t, ok)
	require.Len(t, data.Links, 1)
	link := data.Links[0]
	require.Equal(t, "lldp", link.Protocol)
	require.Equal(t, "bidirectional", link.Direction)
	require.NotNil(t, link.L2)
	require.True(t, link.L2.PairConsistent)
	require.Equal(t, 1, topologyStatsToV1ForTest(t, data.Stats)["links_bidirectional"])
	require.Equal(t, 0, topologyStatsToV1ForTest(t, data.Stats)["links_unidirectional"])
}

func TestTopologyCache_SnapshotMergesRemoteIdentityAcrossProtocols(t *testing.T) {
	cache := newTopologyCache()
	cache.updateTime = time.Now()
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent1"
	cache.localDevice = topologymodel.Device{
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

	data, ok := snapshotTopologyCacheForTest(cache)

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
	cache := newTestTopologyCache(ddsnmp.DeviceConnectionInfo{
		Hostname: "10.0.0.1", SysObjectID: "1.3.6.1.4.1.9.1.1", SysName: "sw1", SysDescr: "Switch 1",
	})
	cache.updateTopologyProfileTags([]*ddsnmp.ProfileMetrics{{
		DeviceMetadata: map[string]ddsnmp.MetaTag{
			tagLldpLocChassisID:        {Value: "00:11:22:33:44:55"},
			tagLldpLocChassisIDSubtype: {Value: "4"},
			tagLldpLocSysCapEnabled:    {Value: "80"},
			tagLldpLocSysCapSupported:  {Value: "80"},
		},
	}})

	cache.updateTopologyCacheEntry(ddsnmp.Metric{
		TopologyKind: ddsnmp.KindLldpLocManAddr,
		Tags: map[string]string{
			tagLldpLocMgmtAddrSubtype: "2",
			tagLldpLocMgmtAddr:        "0a000001",
			tagLldpLocMgmtAddrIfID:    "1",
		},
	})
	cache.updateTopologyCacheEntry(ddsnmp.Metric{
		TopologyKind: ddsnmp.KindLldpRemManAddr,
		Tags: map[string]string{
			tagLldpLocPortNum:         "1",
			tagLldpRemIndex:           "1",
			tagLldpRemMgmtAddrSubtype: "2",
			tagLldpRemMgmtAddr:        "0a000002",
		},
	})
	cache.updateTopologyCacheEntry(ddsnmp.Metric{
		TopologyKind: ddsnmp.KindLldpRemManAddr,
		Tags: map[string]string{
			tagLldpLocPortNum:         "1",
			tagLldpRemIndex:           "1",
			tagLldpRemMgmtAddrSubtype: "1",
			tagLldpRemMgmtAddr:        "31302e32302e342e3834", // "10.20.4.84" ASCII-hex
		},
	})
	cache.updateTopologyCacheEntry(ddsnmp.Metric{
		TopologyKind: ddsnmp.KindLldpRemManAddr,
		Tags: map[string]string{
			tagLldpLocPortNum:         "1",
			tagLldpRemIndex:           "1",
			tagLldpRemMgmtAddrSubtype: "1",
			tagLldpRemMgmtAddr:        "666330303a663835333a6363643a653739333a3a31", // "fc00:f853:ccd:e793::1" ASCII-hex
		},
	})
	cache.updateTopologyCacheEntry(ddsnmp.Metric{
		TopologyKind: ddsnmp.KindLldpRemManAddr,
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
	cache.updateTopologyCacheEntry(ddsnmp.Metric{
		TopologyKind: ddsnmp.KindLldpRem,
		Tags: map[string]string{
			tagLldpLocPortNum:          "1",
			tagLldpRemIndex:            "1",
			tagLldpRemChassisID:        "aa:bb:cc:dd:ee:ff",
			tagLldpRemChassisIDSubtype: "4",
			tagLldpRemSysName:          "sw2",
			tagLldpRemSysCapEnabled:    "80",
		},
	})

	cache.finalizeTopologyCache()

	data, ok := snapshotTopologyCacheForTest(cache)

	require.True(t, ok)
	require.Greater(t, len(data.Actors), 1)
	require.True(t, actorHasManagementAddresses(data))
	require.True(t, actorHasCapabilitiesEnabled(data))
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
	cache.localDevice = topologymodel.Device{
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

	data, ok := snapshotTopologyCacheForTest(cache)

	require.True(t, ok)
	require.True(t, containsMgmtAddr(data, map[string]struct{}{"10.0.0.3": {}, "10.0.0.4": {}}))
}

func TestTopologyCache_FDBAndARPEnrichment(t *testing.T) {
	cache := newTopologyCache()
	cache.updateTime = time.Now()
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent1"
	cache.localDevice = topologymodel.Device{
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

	options := defaultTopologyQueryOptionsForTest()
	options.MapType = topologyoptions.MapTypeAllDevicesLowConfidence
	data, ok := snapshotTopologyCacheForTestWithOptions(cache, options)

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
	assert.Equal(t, "single_port_mac", ep.Detail.L2.Endpoint.AttachmentSource)
	assert.Equal(t, "Port3", ep.Detail.L2.Endpoint.AttachedPort)
}

func TestTopologyCache_Dot1qVLANEnrichment(t *testing.T) {
	cache := newTopologyCache()
	cache.updateTime = time.Now()
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent1"
	cache.localDevice = topologymodel.Device{
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
	cache.localDevice = topologymodel.Device{
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
	cache.localDevice = topologymodel.Device{
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
	cache.localDevice = topologymodel.Device{
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
	cache.localDevice = topologymodel.Device{
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
	cache.localDevice = topologymodel.Device{
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
	cache.localDevice = topologymodel.Device{
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
	tests := map[string]struct {
		in     string
		status stpBridgeIDStatus
		mac    string
	}{
		"bridge-id-hex": {
			in:     "800066778899aabb",
			status: stpBridgeIDValid,
			mac:    "66:77:88:99:aa:bb",
		},
		"priority-bridge-id": {
			in:     "32768-66.77.88.99.aa.bb",
			status: stpBridgeIDValid,
			mac:    "66:77:88:99:aa:bb",
		},
		"quoted-hex-string": {
			in:     "\"18 FD 74 33 1A 9C \"",
			status: stpBridgeIDValid,
			mac:    "18:fd:74:33:1a:9c",
		},
		"hex-string-prefix": {
			in:     "Hex-STRING: 18 FD 74 33 1A 9C",
			status: stpBridgeIDValid,
			mac:    "18:fd:74:33:1a:9c",
		},
		"sentinel-text-empty": {
			in:     "0-00.00.00.00.00.00",
			status: stpBridgeIDEmpty,
			mac:    "",
		},
		"sentinel-hex-empty": {
			in:     "302d30302e30302e30302e30302e30302e3030",
			status: stpBridgeIDEmpty,
			mac:    "",
		},
		"invalid": {
			in:     "not-a-bridge-id",
			status: stpBridgeIDInvalid,
			mac:    "",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
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
	addrsA := []topologymodel.ManagementAddress{
		{Address: "10.20.4.60", Source: "src-a"},
		{Address: "10.20.4.205", Source: "src-b"},
	}
	addrsB := []topologymodel.ManagementAddress{
		{Address: "10.20.4.205", Source: "src-b"},
		{Address: "10.20.4.60", Source: "src-a"},
	}

	require.Equal(t, "10.20.4.205", pickManagementIP(addrsA))
	require.Equal(t, pickManagementIP(addrsA), pickManagementIP(addrsB))

	rawA := []topologymodel.ManagementAddress{
		{Address: "zeta"},
		{Address: "alpha"},
	}
	rawB := []topologymodel.ManagementAddress{
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
	cache.localDevice = topologymodel.Device{
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
		options := defaultTopologyQueryOptionsForTest()
		options.MapType = topologyoptions.MapTypeAllDevicesLowConfidence
		data, ok := snapshotTopologyCacheForTestWithOptions(cache, options)

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
	cache.localDevice = topologymodel.Device{
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

	data, ok := snapshotTopologyCacheForTest(cache)

	require.True(t, ok)
	require.NotEmpty(t, data.Actors)
	require.NotEmpty(t, data.Links)

	actorOrder := make([]string, 0, len(data.Actors))
	for _, actor := range data.Actors {
		actorOrder = append(actorOrder, actor.ActorType+"|"+topologymodel.CanonicalMatchKey(actor.Match))
	}
	expectedActorOrder := append([]string(nil), actorOrder...)
	sort.Strings(expectedActorOrder)
	assert.Equal(t, expectedActorOrder, actorOrder)

	linkOrder := make([]string, 0, len(data.Links))
	for _, link := range data.Links {
		linkOrder = append(linkOrder, topologymodel.LinkSortKey(link))
	}
	expectedLinkOrder := append([]string(nil), linkOrder...)
	sort.Strings(expectedLinkOrder)
	assert.Equal(t, expectedLinkOrder, linkOrder)
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
	bs, err := topologyutil.DecodeHexString("766d7831")
	require.NoError(t, err)

	decoded := topologyutil.DecodePrintableASCII(bs)
	require.Equal(t, "vmx1", decoded)
}

func TestDecodePrintableASCII_HexValueIsNotNumeric(t *testing.T) {
	bs, err := topologyutil.DecodeHexString("766d7831")
	require.NoError(t, err)

	decoded := topologyutil.DecodePrintableASCII(bs)
	assert.NotRegexp(t, "^[0-9]+$", decoded)
}

func TestNormalizeInterfaceAdminStatusAcceptsEnumStrings(t *testing.T) {
	tests := map[string]struct {
		in   string
		want string
	}{
		"up":      {in: "up(1)", want: "up"},
		"down":    {in: "down(2)", want: "down"},
		"testing": {in: "testing(3)", want: "testing"},
		"case":    {in: "UP (1)", want: "up"},
		"invalid": {in: "invalid(9)", want: ""},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, normalizeInterfaceAdminStatus(tc.in), tc.in)
		})
	}
}

func TestNormalizeInterfaceOperStatusAcceptsEnumStrings(t *testing.T) {
	tests := map[string]struct {
		in   string
		want string
	}{
		"up":                 {in: "up(1)", want: "up"},
		"down":               {in: "down(2)", want: "down"},
		"testing":            {in: "testing(3)", want: "testing"},
		"unknown":            {in: "unknown(4)", want: "unknown"},
		"dormant":            {in: "dormant(5)", want: "dormant"},
		"not-present":        {in: "notPresent(6)", want: "notPresent"},
		"lower-layer-down":   {in: "lowerLayerDown(7)", want: "lowerLayerDown"},
		"case-normalization": {in: "LOWERLAYERDOWN (7)", want: "lowerLayerDown"},
		"invalid":            {in: "invalid(9)", want: ""},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, normalizeInterfaceOperStatus(tc.in), tc.in)
		})
	}
}

func TestNormalizeInterfaceTypeAcceptsEnumStrings(t *testing.T) {
	tests := map[string]struct {
		in   string
		want string
	}{
		"ethernet-enum": {in: "ethernetCsmacd(6)", want: "ethernetcsmacd"},
		"ethernet-id":   {in: "6", want: "ethernetcsmacd"},
		"lag-enum":      {in: "ieee8023adLag(161)", want: "ieee8023adlag"},
		"lag-id":        {in: "161", want: "ieee8023adlag"},
		"vlan-enum":     {in: "l2vlan(135)", want: "l2vlan"},
		"empty":         {in: "", want: ""},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, normalizeInterfaceType(tc.in), tc.in)
		})
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

func TestTopologyCache_UpdateTopologySysUptime_StoresSysUptime(t *testing.T) {
	cache := newTopologyCache()
	cache.localDevice = topologymodel.Device{}

	cache.updateTopologySysUptime(4321)

	require.EqualValues(t, 4321, cache.localDevice.SysUptime)
	require.Equal(t, "4321", cache.localDevice.Labels["sys_uptime"])
}

func TestTopologyCache_IngestTopologyProfileMetrics_IncludesTopologyMetrics(t *testing.T) {
	cache := newTopologyCache()

	cache.ingestTopologyProfileMetrics([]*ddsnmp.ProfileMetrics{
		{
			TopologyMetrics: []ddsnmp.Metric{
				{
					Name:         "lldp_loc_port",
					TopologyKind: ddsnmp.KindLldpLocPort,
					Tags: map[string]string{
						tagLldpLocPortNum:       "7",
						tagLldpLocPortID:        "Gi1/0/7",
						tagLldpLocPortIDSubtype: "5",
						tagLldpLocPortDesc:      "Gi1/0/7",
					},
				},
				{
					Name:         "lldp_rem",
					TopologyKind: ddsnmp.KindLldpRem,
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
		},
	})

	require.Contains(t, cache.lldpLocPorts, "7")
	require.Contains(t, cache.lldpRemotes, "7:1")
	require.Zero(t, cache.localDevice.SysUptime)
	require.Empty(t, cache.localDevice.Labels["sys_uptime"])
}

func TestTopologyCache_IngestTopologyBGPPeers_IncludesOnlyPeerRows(t *testing.T) {
	established := int64(300)
	cache := newTopologyCache()

	cache.ingestTopologyBGPPeers([]*ddsnmp.ProfileMetrics{
		{
			BGPRows: []ddsnmp.BGPRow{
				{
					Kind:         ddprofiledefinition.BGPRowKindPeer,
					StructuralID: "peer-1",
					Identity: ddsnmp.BGPIdentity{
						RoutingInstance: "blue",
						Neighbor:        "192.0.2.2",
						RemoteAS:        "65002",
					},
					Descriptors: ddsnmp.BGPDescriptors{
						LocalAddress:    "192.0.2.1",
						LocalAS:         "65001",
						LocalIdentifier: "1.1.1.1",
						PeerIdentifier:  "2.2.2.2",
						PeerType:        "external",
						BGPVersion:      "4",
						Description:     "edge-peer",
					},
					Admin: ddsnmp.BGPAdmin{
						Enabled: ddsnmp.BGPBool{Has: true, Value: true},
					},
					State: ddsnmp.BGPState{
						Has:   true,
						State: ddprofiledefinition.BGPPeerStateEstablished,
					},
					Connection: ddsnmp.BGPConnection{
						EstablishedUptime: ddsnmp.BGPInt64{Has: true, Value: established},
					},
				},
				{
					Kind: ddprofiledefinition.BGPRowKindPeerFamily,
					Identity: ddsnmp.BGPIdentity{
						Neighbor:                "192.0.2.2",
						RemoteAS:                "65002",
						AddressFamily:           ddprofiledefinition.BGPAddressFamilyIPv4,
						SubsequentAddressFamily: ddprofiledefinition.BGPSubsequentAddressFamilyUnicast,
					},
				},
			},
		},
	})

	require.Len(t, cache.bgpPeersByKey, 1)
	peer := cache.bgpPeersByKey["peer-1"]
	require.Equal(t, "blue", peer.RoutingInstance)
	require.Equal(t, "192.0.2.2", peer.NeighborIP)
	require.Equal(t, "65002", peer.RemoteAS)
	require.Equal(t, "192.0.2.1", peer.LocalIP)
	require.Equal(t, "65001", peer.LocalAS)
	require.Equal(t, "1.1.1.1", peer.LocalIdentifier)
	require.Equal(t, "2.2.2.2", peer.PeerIdentifier)
	require.Equal(t, "external", peer.PeerType)
	require.Equal(t, "4", peer.BGPVersion)
	require.Equal(t, "edge-peer", peer.Description)
	require.Equal(t, "enabled", peer.AdminStatus)
	require.Equal(t, "established", peer.State)
	require.NotNil(t, peer.EstablishedUptime)
	require.Equal(t, established, *peer.EstablishedUptime)
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
	data := topologymodel.Data{
		Actors: []topologymodel.Actor{
			{
				ActorType: "device",
				Match: topologymodel.Match{
					SysName:     "sw1",
					ChassisIDs:  []string{"00:11:22:33:44:55"},
					IPAddresses: []string{"10.0.0.1"},
				},
				Detail: topologymodel.ActorDetail{
					L2: topologyengine.ProjectionActorDetail{
						Device: topologyengine.ProjectionDeviceActorDetail{
							VendorDerived:            "Acme Derived",
							VendorDerivedSource:      "mac_oui",
							VendorDerivedConfidence:  "low",
							VendorDerivedMatchPrefix: "00:11:22",
							Ports: []topologyengine.ProjectionPortDetail{
								{
									IfIndex: topologyengine.OptionalValue[int]{Value: 1, Has: true},
									IfName:  "swp07",
								},
							},
						},
					},
				},
			},
		},
	}

	local := topologymodel.Device{
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
		InterfaceCharts: map[string]topologymodel.InterfaceChartRef{
			"swp07": {
				ChartIDSuffix:    "swp07",
				AvailableMetrics: []string{"ifErrors", "ifTraffic"},
			},
		},
	}

	augmentLocalActorFromCache(&data, local)

	actor := findDeviceActorBySysName(data, "sw1")
	require.NotNil(t, actor)
	require.Equal(t, "Switch 1", actor.Detail.SNMP.SysDescr)
	require.Equal(t, "ops@example.net", actor.Detail.SNMP.SysContact)
	require.Equal(t, "dc1", actor.Detail.SNMP.SysLocation)
	require.EqualValues(t, 987654, actor.Detail.SNMP.SysUptime)
	require.Equal(t, "Cisco", actor.Detail.SNMP.Vendor)
	require.Equal(t, "snmp", actor.Detail.SNMP.VendorSource)
	require.Equal(t, "high", actor.Detail.SNMP.VendorConfidence)
	require.Equal(t, "Acme Derived", actor.Detail.L2.Device.VendorDerived)
	require.Equal(t, "mac_oui", actor.Detail.L2.Device.VendorDerivedSource)
	require.Equal(t, "low", actor.Detail.L2.Device.VendorDerivedConfidence)
	require.Equal(t, "00:11:22", actor.Detail.L2.Device.VendorDerivedMatchPrefix)
	require.Equal(t, "C9300-24T", actor.Detail.SNMP.Model)
	require.Equal(t, "SN-12345", actor.Detail.SNMP.SerialNumber)
	require.Equal(t, "17.9.4", actor.Detail.SNMP.SoftwareVersion)
	require.Equal(t, "1.2.3", actor.Detail.SNMP.FirmwareVersion)
	require.Equal(t, "A1", actor.Detail.SNMP.HardwareVersion)
	require.Equal(t, "11111111-1111-1111-1111-111111111111", actor.Detail.SNMP.NetdataHostID)
	require.Equal(t, topologyProfileChartIDPrefix, actor.Detail.SNMP.ChartIDPrefix)
	require.Equal(t, topologyProfileChartContextPrefix, actor.Detail.SNMP.ChartContextPrefix)
	require.Equal(t, map[string]string{"ping_rtt": "ping_rtt"}, actor.Detail.SNMP.DeviceCharts)
	require.Len(t, actor.Detail.L2.Device.Ports, 1)
	require.Equal(t, "swp07", actor.Detail.L2.Device.Ports[0].ChartIDSuffix)
	require.Equal(t, []string{"ifErrors", "ifTraffic"}, actor.Detail.L2.Device.Ports[0].AvailableMetrics)
}

func actorHasManagementAddresses(snapshot topologymodel.Data) bool {
	for _, actor := range snapshot.Actors {
		if len(actor.Detail.SNMP.ManagementAddresses) > 0 {
			return true
		}
		if len(actor.Detail.L2.Device.ManagementAddresses) > 0 {
			return true
		}
	}
	return false
}

func actorHasCapabilitiesEnabled(snapshot topologymodel.Data) bool {
	for _, actor := range snapshot.Actors {
		if len(actor.Detail.SNMP.CapabilitiesEnabled) > 0 {
			return true
		}
		if len(actor.Detail.L2.Device.CapabilitiesEnabled) > 0 {
			return true
		}
	}
	return false
}

func findLinkByProtocol(snapshot topologymodel.Data, protocol string) *topologymodel.Link {
	for i := range snapshot.Links {
		if snapshot.Links[i].Protocol == protocol {
			return &snapshot.Links[i]
		}
	}
	return nil
}

func findActorByMAC(snapshot topologymodel.Data, mac string) *topologymodel.Actor {
	for i := range snapshot.Actors {
		if slices.Contains(snapshot.Actors[i].Match.MacAddresses, mac) {
			return &snapshot.Actors[i]
		}
	}
	return nil
}

func countDeviceActors(snapshot topologymodel.Data) int {
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

func linkHasRawAddressHint(link topologymodel.Link, raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	return containsString(link.Src.Match.IPAddresses, raw) || containsString(link.Dst.Match.IPAddresses, raw)
}

func findDeviceActorBySysName(snapshot topologymodel.Data, sysName string) *topologymodel.Actor {
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
