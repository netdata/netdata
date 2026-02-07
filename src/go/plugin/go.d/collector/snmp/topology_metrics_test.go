// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

func TestCollectTopologyMetrics(t *testing.T) {
	c := New()
	c.Hostname = "192.0.2.1"
	c.sysInfo = &snmputils.SysInfo{
		Name:        "device-1",
		SysObjectID: "1.2.3.4.5",
	}

	c.topologyCache.lastUpdate = time.Now()
	c.topologyCache.localDevice = topologyDevice{
		ChassisID:     "aa:bb:cc:dd:ee:ff",
		ChassisIDType: "macAddress",
	}
	c.topologyCache.lldpLocPorts["1"] = &lldpLocPort{
		portNum:       "1",
		portID:        "Gi0/1",
		portIDSubtype: "interfaceName",
	}
	c.topologyCache.lldpRemotes["1:1"] = &lldpRemote{
		localPortNum:     "1",
		remIndex:         "1",
		chassisID:        "11:22:33:44:55:66",
		chassisIDSubtype: "macAddress",
		portID:           "Gi0/2",
		portIDSubtype:    "interfaceName",
	}
	c.topologyCache.cdpRemotes["2:1"] = &cdpRemote{
		ifIndex:     "2",
		deviceIndex: "1",
		deviceID:    "cdp-device-1",
		devicePort:  "GigabitEthernet0/3",
	}

	mx := make(map[string]int64)
	c.collectTopologyMetrics(mx)

	assert.Equal(t, int64(3), mx["snmp_topology_devices_total"])
	assert.Equal(t, int64(2), mx["snmp_topology_devices_discovered"])
	assert.Equal(t, int64(2), mx["snmp_topology_links_total"])
	assert.Equal(t, int64(1), mx["snmp_topology_links_lldp"])
	assert.Equal(t, int64(1), mx["snmp_topology_links_cdp"])
	assert.True(t, c.topologyChartsAdded)
}
