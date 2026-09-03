// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/stretchr/testify/require"
)

func TestTopologyCacheTrapEnrichment(t *testing.T) {
	tests := map[string]struct {
		setup               func(*topologyBuilder)
		source              string
		ifIndex             string
		wantDeviceStatus    string
		wantDeviceMethod    string
		wantInterface       string
		wantInterfaceStatus string
		wantNeighborStatus  string
		wantNeighbors       []string
	}{
		"uses-trap-if-index": {
			setup: func(cache *topologyBuilder) {
				cache.localDevice.ManagementIP = "192.0.2.30"
				cache.ipAddressesByIP["192.0.2.10"] = resolvedIPAddress{ifIndex: "99"}
				cache.ifNamesByIndex["7"] = "Gi0/7"
				cache.ifNamesByIndex["99"] = "Gi0/99"
				cache.lldpRemotes["7:2"] = &lldpRemote{localPortNum: "7", sysName: "dist-b"}
				cache.lldpRemotes["7:1"] = &lldpRemote{localPortNum: "7", sysName: "dist-a"}
				cache.lldpRemotes["99:1"] = &lldpRemote{localPortNum: "99", sysName: "wrong-source-ip-interface"}
				cache.cdpRemotes["7:1"] = &cdpRemote{ifIndex: "7", sysName: "dist-a"}
				cache.cdpRemotes["9:1"] = &cdpRemote{ifIndex: "9", sysName: "dist-d"}
			},
			source:              "192.0.2.30",
			ifIndex:             "7",
			wantDeviceStatus:    "matched",
			wantDeviceMethod:    "management_ip",
			wantInterface:       "Gi0/7",
			wantInterfaceStatus: "matched",
			wantNeighbors:       []string{"dist-a", "dist-b"},
		},
		"remote-map-key-fallback": {
			setup: func(cache *topologyBuilder) {
				cache.localDevice.ManagementIP = "192.0.2.30"
				cache.lldpRemotes["7:2"] = &lldpRemote{sysName: "dist-b"}
				cache.lldpRemotes["8:1"] = &lldpRemote{sysName: "dist-c"}
				cache.cdpRemotes["7:1"] = &cdpRemote{sysName: "dist-a"}
				cache.cdpRemotes["9:1"] = &cdpRemote{sysName: "dist-d"}
			},
			source:        "192.0.2.30",
			ifIndex:       "7",
			wantNeighbors: []string{"dist-a", "dist-b"},
		},
		"does-not-infer-interface-from-source-ip": {
			setup: func(cache *topologyBuilder) {
				cache.ipAddressesByIP["192.0.2.10"] = resolvedIPAddress{ifIndex: "7"}
				cache.ifNamesByIndex["7"] = "Gi0/7"
				cache.lldpRemotes["7:1"] = &lldpRemote{sysName: "dist-a"}
			},
			source:              "192.0.2.10",
			wantDeviceStatus:    "matched",
			wantDeviceMethod:    "local_interface_ip",
			wantInterfaceStatus: "skipped",
			wantNeighborStatus:  "skipped",
		},
		"selected-management-ip-outranks-interface-address": {
			setup: func(cache *topologyBuilder) {
				cache.localDevice.ManagementAddresses = []topologymodel.ManagementAddress{{
					Address: "192.0.2.10", AddressType: "ipv4", Source: "lldp_local",
				}}
				cache.ipAddressesByIP["192.0.2.10"] = resolvedIPAddress{ifIndex: "7"}
			},
			source:              "192.0.2.10",
			wantDeviceStatus:    "matched",
			wantDeviceMethod:    "management_ip",
			wantInterfaceStatus: "skipped",
			wantNeighborStatus:  "skipped",
		},
		"no-interface-match": {
			setup: func(cache *topologyBuilder) {
				cache.localDevice.ManagementIP = "192.0.2.30"
				cache.lldpRemotes["7:1"] = &lldpRemote{sysName: "dist-a"}
			},
			source:              "192.0.2.30",
			ifIndex:             "9",
			wantInterfaceStatus: "no_match",
			wantNeighborStatus:  "no_match",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cache := newTopologyBuilder()
			tc.setup(cache)

			enrich := trapEnrichmentForTest(cache, tc.source, tc.ifIndex)
			require.NotNil(t, enrich)
			if tc.wantDeviceStatus != "" {
				require.Equal(t, tc.wantDeviceStatus, enrich.DeviceStatus)
			}
			if tc.wantDeviceMethod != "" {
				require.Equal(t, tc.wantDeviceMethod, enrich.DeviceMethod)
			}
			require.Equal(t, tc.wantInterface, enrich.Interface)
			if tc.wantInterfaceStatus != "" {
				require.Equal(t, tc.wantInterfaceStatus, enrich.InterfaceStatus)
			}
			if tc.wantNeighborStatus != "" {
				require.Equal(t, tc.wantNeighborStatus, enrich.NeighborStatus)
			}
			if tc.wantNeighbors == nil {
				require.Empty(t, enrich.Neighbors)
			} else {
				require.Equal(t, tc.wantNeighbors, enrich.Neighbors)
			}
		})
	}
}

func TestTopologyCacheTrapEnrichmentIncludesLocalDeviceIdentity(t *testing.T) {
	cache := newTopologyBuilder()
	cache.localDevice.ManagementIP = "192.0.2.30"
	cache.localDevice.SysName = "core-sw-01"
	cache.localDevice.Vendor = "cisco"
	cache.localDevice.AgentID = "agent-node-id"
	cache.localDevice.NetdataHostID = "vnode-node-id"
	cache.rebuildTrapSourceMatchMethods()

	enrich := trapEnrichmentForTest(cache, "192.0.2.30", "")
	require.NotNil(t, enrich)
	require.Equal(t, "core-sw-01", enrich.DeviceHostname)
	require.Equal(t, "cisco", enrich.DeviceVendor)
	require.Equal(t, "vnode-node-id", enrich.SourceVnodeID)
}

func TestTrapEnrichmentHandleForSourceUsesPublishedRegistry(t *testing.T) {
	registry := newTopologyRegistry()
	handle := publishTrapTopologyRegistryForTest(registry)

	cache := newTopologyBuilder()
	cache.localDevice.ManagementIP = "192.0.2.20"
	cache.ifNamesByIndex["11"] = "Gi0/11"
	cache.lldpRemotes["11:1"] = &lldpRemote{sysName: "dist-c"}
	cache.rebuildTrapSourceMatchMethods()

	publishTestTopologyBuilder(registry, cache)

	enrich := handle.EnrichmentForSource("192.0.2.20", "11")
	require.NotNil(t, enrich)
	require.Equal(t, "matched", enrich.DeviceStatus)
	require.Equal(t, "Gi0/11", enrich.Interface)
	require.Equal(t, []string{"dist-c"}, enrich.Neighbors)
	enrich.Neighbors[0] = "caller-owned"
	require.Equal(t, []string{"dist-c"}, handle.EnrichmentForSource("192.0.2.20", "11").Neighbors)

	mapped := handle.EnrichmentForSource("::ffff:192.0.2.20", "11")
	require.NotNil(t, mapped)
	require.Equal(t, "Gi0/11", mapped.Interface)
}

func TestTrapEnrichmentHandleRejectsTypedNonIPManagementEvidence(t *testing.T) {
	registry := newTopologyRegistry()
	handle := publishTrapTopologyRegistryForTest(registry)

	cache := newTopologyBuilder()
	cache.localDevice.ManagementIP = "192.0.2.20"
	cache.localDevice.ManagementAddresses = []topologymodel.ManagementAddress{
		{Address: "c0000263", AddressType: "16", Source: "lldp_local"},
	}
	cache.rebuildTrapSourceMatchMethods()
	publishTestTopologyBuilder(registry, cache)

	enrich := handle.EnrichmentForSource("192.0.2.99", "")
	require.NotNil(t, enrich)
	require.Equal(t, "no_match", enrich.DeviceStatus)
	require.Zero(t, enrich.DeviceMatches)
}

func TestTrapEnrichmentHandleForSourceAmbiguousRegistryMatchDoesNotEnrich(t *testing.T) {
	registry := newTopologyRegistry()
	handle := publishTrapTopologyRegistryForTest(registry)

	cacheA := newTopologyBuilder()
	cacheA.localDevice.ManagementIP = "192.0.2.20"
	cacheA.ifNamesByIndex["11"] = "Gi0/11"
	cacheA.rebuildTrapSourceMatchMethods()
	cacheB := newTopologyBuilder()
	cacheB.localDevice.ManagementIP = "192.0.2.20"
	cacheB.ifNamesByIndex["11"] = "Gi0/11"
	cacheB.rebuildTrapSourceMatchMethods()

	publishTestTopologyBuilder(registry, cacheA)
	publishTestTopologyBuilder(registry, cacheB)

	enrich := handle.EnrichmentForSource("192.0.2.20", "11")
	require.NotNil(t, enrich)
	require.Equal(t, "ambiguous", enrich.DeviceStatus)
	require.Equal(t, 2, enrich.DeviceMatches)
	require.Empty(t, enrich.Interface)
	require.Empty(t, enrich.Neighbors)
}

func BenchmarkTopologyGenerationTrapEnrichmentForSource(b *testing.B) {
	for _, deviceCount := range []int{1, 128, 1024} {
		b.Run(fmt.Sprintf("devices=%d", deviceCount), func(b *testing.B) {
			devices := make([]*topologyDeviceGeneration, deviceCount)
			for i := range devices {
				devices[i] = &topologyDeviceGeneration{trap: topologyTrapDeviceGeneration{
					matchMethodByIP: map[string]string{
						fmt.Sprintf("198.51.%d.%d", i/254, i%254+1): "management_ip",
					},
				}}
			}
			devices[len(devices)-1].trap.matchMethodByIP["192.0.2.200"] = "management_ip"
			registry := newTopologyRegistry()
			registry.publishGeneration(&topologyGeneration{devices: devices})

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if enrichment := registry.trapEnrichmentForSource("192.0.2.200", ""); enrichment == nil {
					b.Fatal("expected trap enrichment")
				}
			}
		})
	}
}

func TestCollectorRunPublishesAndClearsTrapTopologyRegistry(t *testing.T) {
	coll := newTestSNMPTopologyCollector()
	coll.UpdateEvery = 3600

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
		return coll.trapEnrichment.registry.Load() == coll.topologyRegistry
	}, time.Second, 10*time.Millisecond)

	require.NoError(t, stopRunner())
	require.Nil(t, coll.trapEnrichment.registry.Load())
}

func TestCollectorCleanupDoesNotClearNewerTrapTopologyRegistry(t *testing.T) {
	trapEnrichment := NewTrapEnrichmentHandle()
	oldColl := newTestSNMPTopologyCollector()
	newColl := newTestSNMPTopologyCollector()
	oldColl.trapEnrichment = trapEnrichment
	newColl.trapEnrichment = trapEnrichment
	trapEnrichment.registry.Store(newColl.topologyRegistry)

	oldColl.Cleanup(context.Background())

	require.Same(t, newColl.topologyRegistry, trapEnrichment.registry.Load())
}

func publishTrapTopologyRegistryForTest(registry *topologyRegistry) *TrapEnrichmentHandle {
	handle := NewTrapEnrichmentHandle()
	handle.registry.Store(registry)
	return handle
}
