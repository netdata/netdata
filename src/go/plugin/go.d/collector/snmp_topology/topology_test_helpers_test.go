// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/reversedns"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func newTestSNMPTopologyCollector() *Collector {
	coll, _ := newTestSNMPTopologyCollectorWithStore()
	return coll
}

func newTestSNMPTopologyCollectorWithStore() (*Collector, *ddsnmp.DeviceStore) {
	store := ddsnmp.NewDeviceStore()
	return New(store, NewTrapEnrichmentHandle(), newTestReverseDNSResolver()), store
}

func newTestReverseDNSResolver() *reversedns.Resolver { return reversedns.New(reversedns.Config{}) }

func newTopologyRegistry() *topologyRegistry {
	return newTopologyRegistryWithResolver(newTestReverseDNSResolver())
}

// publishTestTopologyBuilder freezes a build-only cache and republishes the
// complete immutable test generation. Production publishes through sweeps.
func publishTestTopologyBuilder(r *topologyRegistry, cache *topologyBuilder) {
	if r == nil || cache == nil {
		return
	}
	current := r.acquireGeneration()
	sequence := uint64(1)
	states := make(map[string]deviceRefreshState)
	if current != nil {
		sequence = current.sequence + 1
		for _, device := range current.devices {
			states[device.key] = deviceRefreshState{generation: device}
		}
	}
	key := fmt.Sprintf("test:%d", sequence)
	states[key] = deviceRefreshState{generation: newTopologyDeviceGeneration(key, cache)}
	r.publishGeneration(newTopologyGeneration(sequence, time.Now(), states))
}

func snapshotTestTopologyBuilder(c *topologyBuilder) (topologymodel.ObservationSnapshot, bool) {
	generation := newTopologyDeviceGeneration("test", c)
	if generation == nil || !generation.freshAt(time.Now()) || !generation.hasObservation {
		return topologymodel.ObservationSnapshot{}, false
	}
	return generation.observation, true
}

func trapEnrichmentForTest(c *topologyBuilder, ip, trapIfIndex string) *TrapTopologyEnrichment {
	addr, ok := topologyutil.ParseIPAddress(ip)
	if !ok {
		return nil
	}
	return newTopologyTrapDeviceGeneration(c).enrichmentForCanonicalSource(addr.String(), trapIfIndex)
}

func registerTestDeviceState(store *ddsnmp.DeviceStore, devices ...ddsnmp.DeviceConnectionInfo) {
	for i, dev := range devices {
		store.Register(fmt.Sprintf("test:%s:%d:%d", dev.Hostname, dev.Port, i), dev)
	}
}

func snapshotTopologyRegistryForTest(registry *topologyRegistry) (topologymodel.Data, bool) {
	return snapshotTopologyRegistryForTestWithOptions(registry, defaultTopologyQueryOptionsForTest())
}

func testCountTopologyLinksByType(links []topologymodel.Link, linkType string) int {
	count := 0
	for _, link := range links {
		if link.LinkType == linkType {
			count++
		}
	}
	return count
}

func snapshotTopologyRegistryForTestWithOptions(registry *topologyRegistry, options topologyoptions.QueryOptions) (topologymodel.Data, bool) {
	data, ok, err := registry.snapshotWithOptions(options)
	return data, ok && err == nil
}

func snapshotTopologyCacheForTest(cache *topologyBuilder) (topologymodel.Data, bool) {
	return snapshotTopologyCacheForTestWithOptions(cache, defaultTopologyQueryOptionsForTest())
}

func snapshotTopologyCacheForTestWithOptions(cache *topologyBuilder, options topologyoptions.QueryOptions) (topologymodel.Data, bool) {
	registry := newTopologyRegistry()
	publishTestTopologyBuilder(registry, cache)
	return snapshotTopologyRegistryForTestWithOptions(registry, options)
}

func defaultTopologyQueryOptionsForTest() topologyoptions.QueryOptions {
	return topologyoptions.DefaultQueryOptions()
}

func containsMgmtAddr(snapshot topologymodel.Data, addrs map[string]struct{}) bool {
	for _, actor := range snapshot.Actors {
		for _, ip := range actor.Match.IPAddresses {
			if _, ok := addrs[ip]; ok {
				return true
			}
		}
	}
	for _, link := range snapshot.Links {
		for _, ip := range link.Src.Match.IPAddresses {
			if _, ok := addrs[ip]; ok {
				return true
			}
		}
		for _, ip := range link.Dst.Match.IPAddresses {
			if _, ok := addrs[ip]; ok {
				return true
			}
		}
	}
	return false
}
