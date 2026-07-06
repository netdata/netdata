// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func newTestSNMPTopologyCollector() *Collector {
	coll, _ := newTestSNMPTopologyCollectorWithStore()
	return coll
}

func newTestSNMPTopologyCollectorWithStore() (*Collector, *ddsnmp.DeviceStore) {
	store := ddsnmp.NewDeviceStore()
	return New(store, NewTrapEnrichmentHandle()), store
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
	return registry.snapshotWithOptions(options)
}

func snapshotTopologyCacheForTest(cache *topologyCache) (topologymodel.Data, bool) {
	return snapshotTopologyCacheForTestWithOptions(cache, defaultTopologyQueryOptionsForTest())
}

func snapshotTopologyCacheForTestWithOptions(cache *topologyCache, options topologyoptions.QueryOptions) (topologymodel.Data, bool) {
	registry := newTopologyRegistry()
	registry.register(cache)
	return snapshotTopologyRegistryForTestWithOptions(registry, options)
}

func defaultTopologyQueryOptionsForTest() topologyoptions.QueryOptions {
	return topologyoptions.QueryOptions{
		CollapseActorsByIP:     true,
		EliminateNonIPInferred: true,
		MapType:                topologyoptions.MapTypeLLDPCDPManaged,
		InferenceStrategy:      topologyoptions.InferenceStrategyFDBMinimumKnowledge,
		ManagedDeviceFocus:     topologyoptions.ManagedFocusAllDevices,
		Depth:                  topologyoptions.DepthAllInternal,
	}
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
