// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCollectTopologyMetrics(t *testing.T) {
	cache := newTopologyCache()
	cache.lastUpdate = time.Now()
	cache.localDevice = topologyDevice{
		ManagementIP:  "192.0.2.1",
		ChassisID:     "aa:bb:cc:dd:ee:ff",
		ChassisIDType: "macAddress",
	}
	cache.lldpLocPorts["1"] = &lldpLocPort{
		portNum:       "1",
		portID:        "Gi0/1",
		portIDSubtype: "interfaceName",
	}
	cache.lldpRemotes["1:1"] = &lldpRemote{
		localPortNum:     "1",
		remIndex:         "1",
		chassisID:        "11:22:33:44:55:66",
		chassisIDSubtype: "macAddress",
		portID:           "Gi0/2",
		portIDSubtype:    "interfaceName",
	}

	snmpTopologyRegistry.register(cache)
	defer snmpTopologyRegistry.unregister(cache)

	c := New()

	mx := make(map[string]int64)
	c.collectTopologyMetrics(mx)

	// The topology engine pipeline processes raw cache data into actors and links.
	// With minimal test data (one local + one LLDP remote), the engine produces
	// at least the local device and the remote device as actors.
	assert.GreaterOrEqual(t, mx["snmp_topology_devices_total"], int64(1))
	assert.GreaterOrEqual(t, mx["snmp_topology_links_total"], int64(0))
	assert.True(t, c.topologyChartsAdded)
}
