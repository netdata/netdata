// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"

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

func snapshotTopologyRegistryForTest(registry *topologyRegistry) (topologyData, bool) {
	return registry.snapshotWithOptions(topologyQueryOptions{
		CollapseActorsByIP:     true,
		EliminateNonIPInferred: true,
		MapType:                topologyMapTypeLLDPCDPManaged,
		InferenceStrategy:      topologyInferenceStrategyFDBMinimumKnowledge,
		ManagedDeviceFocus:     topologyManagedFocusAllDevices,
		Depth:                  topologyDepthAllInternal,
		ResolveDNSName:         resolveTopologyReverseDNSName,
	})
}

func containsMgmtAddr(snapshot topologyData, addrs map[string]struct{}) bool {
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
