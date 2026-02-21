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
	require.Len(t, data.Links, 2)

	require.NotNil(t, findLinkByProtocol(data, "fdb"))
	require.NotNil(t, findLinkByProtocol(data, "bridge"))
	require.Nil(t, findLinkByProtocol(data, "arp"))

	ep := findActorByMAC(data, "70:49:a2:65:72:cd")
	require.NotNil(t, ep)
	assert.Equal(t, "endpoint", ep.ActorType)
	assert.Contains(t, ep.Match.IPAddresses, "10.20.4.84")
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
