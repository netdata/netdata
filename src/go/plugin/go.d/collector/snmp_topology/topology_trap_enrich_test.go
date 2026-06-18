// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTopologyCacheTrapEnrichmentForSourceUsesTrapIfIndex(t *testing.T) {
	cache := newTopologyCache()
	cache.localDevice.ManagementIP = "192.0.2.30"
	cache.ifIndexByIP["192.0.2.10"] = "99"
	cache.ifNamesByIndex["7"] = "Gi0/7"
	cache.ifNamesByIndex["99"] = "Gi0/99"
	cache.lldpRemotes["7:2"] = &lldpRemote{localPortNum: "7", sysName: "dist-b"}
	cache.lldpRemotes["7:1"] = &lldpRemote{localPortNum: "7", sysName: "dist-a"}
	cache.lldpRemotes["99:1"] = &lldpRemote{localPortNum: "99", sysName: "wrong-source-ip-interface"}
	cache.cdpRemotes["7:1"] = &cdpRemote{ifIndex: "7", sysName: "dist-a"}
	cache.cdpRemotes["9:1"] = &cdpRemote{ifIndex: "9", sysName: "dist-d"}

	enrich := cache.trapEnrichmentForSource("192.0.2.30", "7")
	require.NotNil(t, enrich)
	require.Equal(t, "matched", enrich.DeviceStatus)
	require.Equal(t, "management_ip", enrich.DeviceMethod)
	require.Equal(t, "Gi0/7", enrich.Interface)
	require.Equal(t, "matched", enrich.InterfaceStatus)
	require.Equal(t, []string{"dist-a", "dist-b"}, enrich.Neighbors)
}

func TestTopologyCacheTrapEnrichmentForSourceFallsBackToRemoteMapKeys(t *testing.T) {
	cache := newTopologyCache()
	cache.localDevice.ManagementIP = "192.0.2.30"
	cache.lldpRemotes["7:2"] = &lldpRemote{sysName: "dist-b"}
	cache.lldpRemotes["8:1"] = &lldpRemote{sysName: "dist-c"}
	cache.cdpRemotes["7:1"] = &cdpRemote{sysName: "dist-a"}
	cache.cdpRemotes["9:1"] = &cdpRemote{sysName: "dist-d"}

	enrich := cache.trapEnrichmentForSource("192.0.2.30", "7")
	require.NotNil(t, enrich)
	require.Equal(t, []string{"dist-a", "dist-b"}, enrich.Neighbors)
}

func TestTopologyCacheTrapEnrichmentForSourceDoesNotInferInterfaceFromSourceIP(t *testing.T) {
	cache := newTopologyCache()
	cache.ifIndexByIP["192.0.2.10"] = "7"
	cache.ifNamesByIndex["7"] = "Gi0/7"
	cache.lldpRemotes["7:1"] = &lldpRemote{sysName: "dist-a"}

	enrich := cache.trapEnrichmentForSource("192.0.2.10", "")
	require.NotNil(t, enrich)
	require.Equal(t, "matched", enrich.DeviceStatus)
	require.Equal(t, "local_interface_ip", enrich.DeviceMethod)
	require.Empty(t, enrich.Interface)
	require.Empty(t, enrich.Neighbors)
	require.Equal(t, "skipped", enrich.InterfaceStatus)
	require.Equal(t, "skipped", enrich.NeighborStatus)
}

func TestTopologyCacheTrapEnrichmentForSourceNoInterfaceMatch(t *testing.T) {
	cache := newTopologyCache()
	cache.localDevice.ManagementIP = "192.0.2.30"
	cache.lldpRemotes["7:1"] = &lldpRemote{sysName: "dist-a"}

	enrich := cache.trapEnrichmentForSource("192.0.2.30", "9")
	require.NotNil(t, enrich)
	require.Equal(t, "no_match", enrich.InterfaceStatus)
	require.Empty(t, enrich.Interface)
	require.Equal(t, "no_match", enrich.NeighborStatus)
	require.Empty(t, enrich.Neighbors)
}

func TestTopologyCacheTrapEnrichmentForSourceIncludesLocalDeviceIdentity(t *testing.T) {
	cache := newTopologyCache()
	cache.localDevice.ManagementIP = "192.0.2.30"
	cache.localDevice.SysName = "core-sw-01"
	cache.localDevice.Vendor = "cisco"
	cache.localDevice.AgentID = "agent-node-id"
	cache.localDevice.NetdataHostID = "vnode-node-id"

	enrich := cache.trapEnrichmentForSource("192.0.2.30", "")
	require.NotNil(t, enrich)
	require.Equal(t, "core-sw-01", enrich.DeviceHostname)
	require.Equal(t, "cisco", enrich.DeviceVendor)
	require.Equal(t, "vnode-node-id", enrich.SourceVnodeID)
}

func TestTrapEnrichmentForSourceUsesActiveRegistry(t *testing.T) {
	registry := newTopologyRegistry()
	publishTrapTopologyRegistryForTest(t, registry)

	cache := newTopologyCache()
	cache.localDevice.ManagementIP = "192.0.2.20"
	cache.ifNamesByIndex["11"] = "Gi0/11"
	cache.lldpRemotes["11:1"] = &lldpRemote{sysName: "dist-c"}

	registry.register(cache)

	enrich := TrapEnrichmentForSource("192.0.2.20", "11")
	require.NotNil(t, enrich)
	require.Equal(t, "matched", enrich.DeviceStatus)
	require.Equal(t, "Gi0/11", enrich.Interface)
	require.Equal(t, []string{"dist-c"}, enrich.Neighbors)

	mapped := TrapEnrichmentForSource("::ffff:192.0.2.20", "11")
	require.NotNil(t, mapped)
	require.Equal(t, "Gi0/11", mapped.Interface)
}

func TestTrapEnrichmentForSourceAmbiguousActiveRegistryMatchDoesNotEnrich(t *testing.T) {
	registry := newTopologyRegistry()
	publishTrapTopologyRegistryForTest(t, registry)

	cacheA := newTopologyCache()
	cacheA.localDevice.ManagementIP = "192.0.2.20"
	cacheA.ifNamesByIndex["11"] = "Gi0/11"
	cacheB := newTopologyCache()
	cacheB.localDevice.ManagementIP = "192.0.2.20"
	cacheB.ifNamesByIndex["11"] = "Gi0/11"

	registry.register(cacheA)
	registry.register(cacheB)

	enrich := TrapEnrichmentForSource("192.0.2.20", "11")
	require.NotNil(t, enrich)
	require.Equal(t, "ambiguous", enrich.DeviceStatus)
	require.Equal(t, 2, enrich.DeviceMatches)
	require.Empty(t, enrich.Interface)
	require.Empty(t, enrich.Neighbors)
}

func TestCollectorRunPublishesAndClearsTrapTopologyRegistry(t *testing.T) {
	previous := activeTrapTopologyRegistry.Swap(nil)
	t.Cleanup(func() { activeTrapTopologyRegistry.Store(previous) })

	coll := New()
	coll.UpdateEvery = 3600
	coll.registeredDevices = nil

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- coll.Run(ctx)
	}()

	stopped := false
	stopRunner := func() error {
		if stopped {
			return nil
		}
		stopped = true
		cancel()
		select {
		case err := <-errCh:
			return err
		case <-time.After(time.Second):
			return errors.New("runner did not stop")
		}
	}
	defer func() {
		require.NoError(t, stopRunner())
	}()

	require.Eventually(t, func() bool {
		return activeTrapTopologyRegistry.Load() == coll.topologyRegistry
	}, time.Second, 10*time.Millisecond)

	require.NoError(t, stopRunner())
	require.Nil(t, activeTrapTopologyRegistry.Load())
}

func TestCollectorCleanupDoesNotClearNewerTrapTopologyRegistry(t *testing.T) {
	previous := activeTrapTopologyRegistry.Swap(nil)
	t.Cleanup(func() { activeTrapTopologyRegistry.Store(previous) })

	oldColl := New()
	newColl := New()
	activeTrapTopologyRegistry.Store(newColl.topologyRegistry)

	oldColl.Cleanup(context.Background())

	require.Same(t, newColl.topologyRegistry, activeTrapTopologyRegistry.Load())
}

func publishTrapTopologyRegistryForTest(t *testing.T, registry *topologyRegistry) {
	t.Helper()

	previous := activeTrapTopologyRegistry.Swap(registry)
	t.Cleanup(func() {
		activeTrapTopologyRegistry.CompareAndSwap(registry, previous)
	})
}
