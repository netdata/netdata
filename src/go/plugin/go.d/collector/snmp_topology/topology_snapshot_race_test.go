// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sync"
	"testing"
	"time"
)

// TestTopologyRegistry_ConcurrentSnapshotsDoNotRaceOnDeviceLabels reproduces the
// concurrent-map-write crash where the snapshot read path mutated a cache-shared
// map. snapshotEngineObservations holds only an RLock and passes c.localDevice
// (by value, so Labels still aliases the cache map) to normalizeTopologyDevice,
// which writes Labels. Two concurrent snapshot readers then write the same map.
//
// Run with -race. Before the fix this reports a data race / fatal concurrent map
// writes; after the fix (normalizeTopologyDevice clones Labels) it passes.
func TestTopologyRegistry_ConcurrentSnapshotsDoNotRaceOnDeviceLabels(t *testing.T) {
	registry := newTopologyRegistry()

	cache := newTopologyCache()
	cache.lastUpdate = time.Now()
	cache.localDevice = topologyDevice{
		ManagementIP:  "192.0.2.1",
		ChassisID:     "aa:bb:cc:dd:ee:ff",
		ChassisIDType: "macAddress",
		// Capabilities + a non-nil Labels map make normalizeTopologyDevice write
		// Labels["type"], which is the mutation that raced on the shared map.
		Capabilities: []string{"bridge"},
		Labels:       map[string]string{"seed": "value"},
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
	registry.register(cache)

	const goroutines = 64
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		// Alternate the default and option-aware registry read paths.
		collectPath := i%2 == 0
		go func() {
			defer wg.Done()
			if collectPath {
				_, _ = snapshotTopologyRegistryForTest(registry)
			} else {
				_, _ = registry.snapshotWithOptions(topologyQueryOptions{})
			}
		}()
	}
	wg.Wait()
}
