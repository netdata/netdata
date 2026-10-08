// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

// Run with -race. The builder is frozen once before publication; every reader
// then traverses the same immutable generation without a cache lock.
func TestTopologyRegistry_ConcurrentSnapshotsReadImmutableGeneration(t *testing.T) {
	registry := newTopologyRegistry()

	cache := newTopologyBuilder()
	cache.lastUpdate = time.Now()
	cache.localDevice = topologymodel.Device{
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
	publishTestTopologyBuilder(registry, cache)

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
				_, _, _ = registry.snapshotWithOptions(topologyoptions.QueryOptions{})
			}
		}()
	}
	wg.Wait()
}
