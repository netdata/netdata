// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
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
	assert.Equal(t, "Gi0/2", data.Links[0].Src.Attributes["if_name"])
	assert.Equal(t, "Gi0/3", data.Links[0].Dst.Attributes["port_id"])
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
	require.True(t, containsMgmtAddr(data, map[string]struct{}{"10.0.0.2": {}}))
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
