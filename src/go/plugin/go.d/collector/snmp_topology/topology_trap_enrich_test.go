// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTopologyCacheTrapEnrichmentForIP(t *testing.T) {
	cache := newTopologyCache()
	cache.ifIndexByIP["192.0.2.10"] = "7"
	cache.ifNamesByIndex["7"] = "Gi0/7"
	cache.lldpRemotes["7:2"] = &lldpRemote{sysName: "dist-b"}
	cache.lldpRemotes["7:1"] = &lldpRemote{sysName: "dist-a"}
	cache.lldpRemotes["8:1"] = &lldpRemote{sysName: "dist-c"}
	cache.cdpRemotes["7:1"] = &cdpRemote{sysName: "dist-a"}
	cache.cdpRemotes["9:1"] = &cdpRemote{ifIndex: "9", sysName: "dist-d"}

	enrich := cache.trapEnrichmentForIP("192.0.2.10")
	require.NotNil(t, enrich)
	require.Equal(t, "Gi0/7", enrich.Interface)
	require.Equal(t, []string{"dist-a", "dist-b", "dist-c", "dist-d"}, enrich.Neighbors)
}

func TestTopologyCacheTrapEnrichmentForIPNoInterfaceMatch(t *testing.T) {
	cache := newTopologyCache()
	cache.lldpRemotes["7:1"] = &lldpRemote{sysName: "dist-a"}

	require.Nil(t, cache.trapEnrichmentForIP("192.0.2.10"))
}

func TestTopologyCacheTrapEnrichmentForIPManagementIPWithoutInterfaceMatch(t *testing.T) {
	cache := newTopologyCache()
	cache.localDevice.ManagementIP = "192.0.2.30"
	cache.lldpRemotes["7:1"] = &lldpRemote{sysName: "dist-a"}

	enrich := cache.trapEnrichmentForIP("192.0.2.30")
	require.NotNil(t, enrich)
	require.Empty(t, enrich.Interface)
	require.Equal(t, []string{"dist-a"}, enrich.Neighbors)
}

func TestTopologyCacheTrapEnrichmentForIPIncludesLocalDeviceIdentity(t *testing.T) {
	cache := newTopologyCache()
	cache.localDevice.ManagementIP = "192.0.2.30"
	cache.localDevice.SysName = "core-sw-01"
	cache.localDevice.Vendor = "cisco"
	cache.localDevice.AgentID = "agent-node-id"
	cache.localDevice.NetdataHostID = "vnode-node-id"

	enrich := cache.trapEnrichmentForIP("192.0.2.30")
	require.NotNil(t, enrich)
	require.Equal(t, "core-sw-01", enrich.DeviceHostname)
	require.Equal(t, "cisco", enrich.DeviceVendor)
	require.Equal(t, "vnode-node-id", enrich.SourceVnodeID)
}

func TestTrapEnrichmentForIPUsesGlobalRegistry(t *testing.T) {
	cache := newTopologyCache()
	cache.ifIndexByIP["192.0.2.20"] = "11"
	cache.ifNamesByIndex["11"] = "Gi0/11"
	cache.lldpRemotes["11:1"] = &lldpRemote{sysName: "dist-c"}

	snmpTopologyRegistry.register(cache)
	defer snmpTopologyRegistry.unregister(cache)

	enrich := TrapEnrichmentForIP("192.0.2.20")
	require.NotNil(t, enrich)
	require.Equal(t, "Gi0/11", enrich.Interface)
	require.Equal(t, []string{"dist-c"}, enrich.Neighbors)

	mapped := TrapEnrichmentForIP("::ffff:192.0.2.20")
	require.NotNil(t, mapped)
	require.Equal(t, "Gi0/11", mapped.Interface)
}
