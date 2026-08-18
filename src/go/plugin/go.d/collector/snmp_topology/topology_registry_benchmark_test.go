// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

func BenchmarkSNMPTopologyFunctionAliasScaling(b *testing.B) {
	tests := []struct {
		devices       int
		aliases       int
		sharedPrimary bool
	}{
		{devices: 16, aliases: 1},
		{devices: 64, aliases: 1},
		{devices: 16, aliases: 64},
		{devices: 32, aliases: 64},
		{devices: 64, aliases: 64},
		{devices: 64, aliases: 256},
		{devices: 16, aliases: 64, sharedPrimary: true},
		{devices: 32, aliases: 64, sharedPrimary: true},
		{devices: 64, aliases: 64, sharedPrimary: true},
		{devices: 64, aliases: 256, sharedPrimary: true},
	}

	for _, tc := range tests {
		b.Run(fmt.Sprintf("devices=%d/aliases=%d/shared_primary=%t", tc.devices, tc.aliases, tc.sharedPrimary), func(b *testing.B) {
			registry := benchmarkAliasRichTopologyRegistry(tc.devices, tc.aliases, tc.sharedPrimary)
			deps := funcDepsAdapter{registry: registry}
			options := topologyoptions.DefaultQueryOptions()
			options.MapType = topologyoptions.MapTypeAllDevicesLowConfidence

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				payload, ok, err := deps.Snapshot(options)
				if err != nil {
					b.Fatal(err)
				}
				if !ok {
					b.Fatal("topology snapshot is unavailable")
				}
				runtime.KeepAlive(payload)
			}
		})
	}
}

func benchmarkAliasRichTopologyRegistry(deviceCount, aliasCount int, sharedPrimary bool) *topologyRegistry {
	registry := &topologyRegistry{
		caches:          make(map[*topologyCache]struct{}, deviceCount),
		producerScopeID: "benchmark-producer",
	}
	now := time.Now()
	selectedIPs := make([]string, deviceCount)
	caches := make([]*topologyCache, 0, deviceCount)

	for deviceIndex := 0; deviceIndex < deviceCount; deviceIndex++ {
		cache := newTopologyCache()
		cache.lastUpdate = now
		cache.updateTime = now
		cache.agentID = fmt.Sprintf("benchmark-agent-%d", deviceIndex)

		addresses := make([]topologymodel.ManagementAddress, 0, aliasCount)
		for aliasIndex := 0; aliasIndex < aliasCount; aliasIndex++ {
			ip := benchmarkAliasIPAddress(deviceIndex, aliasIndex)
			addresses = append(addresses, topologymodel.ManagementAddress{
				Address:     ip,
				AddressType: "ipv4",
				Source:      "ip_mib",
			})
			selectedIPs[deviceIndex] = ip
		}
		if sharedPrimary {
			selectedIPs[deviceIndex] = "172.16.0.1"
		}

		cache.localDevice = topologymodel.Device{
			ChassisID:           fmt.Sprintf("02:00:00:00:%02x:%02x", deviceIndex/256, deviceIndex%256),
			ChassisIDType:       "macAddress",
			SysName:             fmt.Sprintf("benchmark-router-%d", deviceIndex),
			ManagementIP:        selectedIPs[deviceIndex],
			ManagementAddresses: addresses,
			OSPFRouterID:        fmt.Sprintf("192.0.2.%d", deviceIndex+1),
		}
		registry.caches[cache] = struct{}{}
		caches = append(caches, cache)
	}

	if len(caches) >= 2 {
		caches[0].l3InterfacesByIP["198.51.100.1"] = topologymodel.L3Interface{
			IP:      "198.51.100.1",
			Netmask: "255.255.255.252",
			IfIndex: "1",
		}
		caches[1].l3InterfacesByIP["198.51.100.2"] = topologymodel.L3Interface{
			IP:      "198.51.100.2",
			Netmask: "255.255.255.252",
			IfIndex: "1",
		}
		caches[0].ospfNeighborsByKey["benchmark-neighbor"] = topologymodel.OSPFNeighbor{
			LocalRouterID:    caches[0].localDevice.OSPFRouterID,
			NeighborRouterID: caches[1].localDevice.OSPFRouterID,
			NeighborIP:       selectedIPs[1],
			LocalIP:          selectedIPs[0],
			State:            "full",
		}
		caches[0].bgpPeersByKey["benchmark-peer"] = topologymodel.BGPPeer{
			NeighborIP:      selectedIPs[1],
			LocalIP:         selectedIPs[0],
			LocalIdentifier: caches[0].localDevice.OSPFRouterID,
			PeerIdentifier:  caches[1].localDevice.OSPFRouterID,
			State:           "established",
		}
	}

	return registry
}

func benchmarkAliasIPAddress(deviceIndex, aliasIndex int) string {
	return fmt.Sprintf("10.%d.%d.%d", deviceIndex+1, aliasIndex/254, aliasIndex%254+1)
}
