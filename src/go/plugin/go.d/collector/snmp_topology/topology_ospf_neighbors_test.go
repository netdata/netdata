// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTopologyCache_OSPFNeighborDropsUnspecifiedOnlyNeighborIdentity(t *testing.T) {
	cache := newTopologyBuilder()

	cache.updateOSPFNeighbor(map[string]string{
		tagOSPFNeighborRouterID: "0.0.0.0",
		tagOSPFNeighborIP:       "0.0.0.0",
		tagOSPFNeighborState:    "full",
	})

	require.Empty(t, cache.ospfNeighborsByKey)
}

func TestTopologyCache_OSPFSnapshotAllocationsDoNotScaleWithPrefixNeighborCrossProduct(t *testing.T) {
	allocsForNeighbors := func(neighbors int) float64 {
		cache := newTopologyBuilder()
		cache.localDevice.ChassisID = "02:00:00:00:00:01"
		cache.localDevice.ChassisIDType = "macAddress"
		cache.localDevice.SysName = "allocation-router"
		for i := range 256 {
			ip := fmt.Sprintf("10.0.%d.%d", i>>8, i&255)
			ifIndex := fmt.Sprintf("%d", i+1)
			cache.updateIfIndexByIP(modernIPv4Tags(
				ip,
				ifIndex,
				fmt.Sprintf("%s.%s.1.4.%s.32", ipAddressPrefixOriginOID, ifIndex, ip),
			))
		}
		for i := range neighbors {
			cache.updateOSPFNeighbor(map[string]string{
				tagOSPFNeighborRouterID: fmt.Sprintf("192.0.2.%d", i+1),
				tagOSPFNeighborIP:       fmt.Sprintf("198.51.100.%d", i+1),
				tagOSPFNeighborState:    "full",
			})
		}
		cache.finalize()

		return testing.AllocsPerRun(10, func() {
			snapshot, ok := cache.buildObservationSnapshot()
			runtime.KeepAlive(snapshot)
			runtime.KeepAlive(ok)
		})
	}

	one := allocsForNeighbors(1)
	many := allocsForNeighbors(32)
	require.LessOrEqual(t, many, one+512, "prefix matching must not reparse every interface for every neighbor")
}

func TestTopologyCache_AddresslessOSPFSnapshotDoesNotBuildPrefixIndex(t *testing.T) {
	allocsForNeighbors := func(neighbors int) float64 {
		cache := newTopologyBuilder()
		cache.localDevice.ChassisID = "02:00:00:00:00:01"
		cache.localDevice.ChassisIDType = "macAddress"
		cache.localDevice.SysName = "addressless-router"
		for i := range 256 {
			ip := fmt.Sprintf("10.0.%d.%d", i>>8, i&255)
			ifIndex := fmt.Sprintf("%d", i+1)
			cache.updateIfIndexByIP(modernIPv4Tags(
				ip,
				ifIndex,
				fmt.Sprintf("%s.%s.1.4.%s.32", ipAddressPrefixOriginOID, ifIndex, ip),
			))
		}
		for i := range neighbors {
			cache.updateOSPFNeighbor(map[string]string{
				tagOSPFNeighborRouterID: fmt.Sprintf("192.0.2.%d", i+1),
				tagOSPFNeighborIP:       "0.0.0.0",
				tagOSPFNeighborState:    "full",
			})
		}
		cache.finalize()

		return testing.AllocsPerRun(10, func() {
			snapshot, ok := cache.buildObservationSnapshot()
			runtime.KeepAlive(snapshot)
			runtime.KeepAlive(ok)
		})
	}

	withoutNeighbors := allocsForNeighbors(0)
	addresslessNeighbors := allocsForNeighbors(32)
	require.LessOrEqual(t, addresslessNeighbors, withoutNeighbors+512,
		"addressless neighbors must not trigger prefix-index construction")
}

func TestTopologyCache_MatchOSPFNeighborLocalInterfaceUsesLongestPrefix(t *testing.T) {
	cache := newTopologyBuilder()
	cache.ipAddressesByIP = map[string]resolvedIPAddress{
		"10.0.0.1": {ifIndex: "1", netmask: "255.255.0.0"},
		"10.0.1.1": {ifIndex: "2", netmask: "255.255.255.252"},
	}

	match, ok := newTopologyOSPFLocalInterfaceIndex(cache.snapshotL3Interfaces("local")).match("10.0.1.2")

	require.True(t, ok)
	require.Equal(t, "10.0.1.1", match.IP)
	require.Equal(t, "10.0.1.0", match.Network)
	require.Equal(t, "255.255.255.252", match.Netmask)
	require.Equal(t, "10.0.1.0/30", match.Subnet)
	require.Equal(t, 30, match.Prefix)
}

func TestTopologyCache_MatchOSPFNeighborLocalInterfaceKeepsFirstSortedEqualPrefix(t *testing.T) {
	cache := newTopologyBuilder()
	cache.ipAddressesByIP = map[string]resolvedIPAddress{
		"10.0.1.2": {ifIndex: "2", netmask: "255.255.255.0"},
		"10.0.1.1": {ifIndex: "1", netmask: "255.255.255.0"},
	}

	match, ok := newTopologyOSPFLocalInterfaceIndex(cache.snapshotL3Interfaces("local")).match("10.0.1.99")

	require.True(t, ok)
	require.Equal(t, "10.0.1.1", match.IP)
}

func BenchmarkTopologyCacheOSPFSnapshotWithoutPrefixes(b *testing.B) {
	for _, addressRows := range []int{4096, 65536} {
		b.Run(fmt.Sprintf("addresses=%d/neighbors=100", addressRows), func(b *testing.B) {
			benchmarkTopologyCacheOSPFSnapshot(b, addressRows, false, false)
		})
	}
}

func BenchmarkTopologyCacheOSPFSnapshotWithPrefixes(b *testing.B) {
	for _, addressRows := range []int{4096, 65536} {
		b.Run(fmt.Sprintf("addresses=%d/neighbors=100", addressRows), func(b *testing.B) {
			benchmarkTopologyCacheOSPFSnapshot(b, addressRows, true, false)
		})
	}
}

func BenchmarkTopologyCacheOSPFSnapshotAddresslessNeighbors(b *testing.B) {
	for _, addressRows := range []int{4096, 65536} {
		b.Run(fmt.Sprintf("addresses=%d/neighbors=100", addressRows), func(b *testing.B) {
			benchmarkTopologyCacheOSPFSnapshot(b, addressRows, true, true)
		})
	}
}

func benchmarkTopologyCacheOSPFSnapshot(b *testing.B, addressRows int, withPrefixes, addressless bool) {
	cache := newTopologyBuilder()
	cache.localDevice.ChassisID = "02:00:00:00:00:01"
	cache.localDevice.ChassisIDType = "macAddress"
	cache.localDevice.SysName = "benchmark-router"
	for i := range addressRows {
		ip := fmt.Sprintf("10.%d.%d.%d", (i>>16)&255, (i>>8)&255, i&255)
		if !withPrefixes {
			ip = fmt.Sprintf("169.254.%d.%d", (i>>8)&255, i&255)
		}
		ifIndex := fmt.Sprintf("%d", i+1)
		pointer := "0.0"
		if withPrefixes {
			pointer = fmt.Sprintf("%s.%s.1.4.%s.32", ipAddressPrefixOriginOID, ifIndex, ip)
		}
		cache.updateIfIndexByIP(modernIPv4Tags(ip, ifIndex, pointer))
	}
	for i := range 100 {
		neighborIP := fmt.Sprintf("198.51.100.%d", i+1)
		if addressless {
			neighborIP = "0.0.0.0"
		}
		cache.updateOSPFNeighbor(map[string]string{
			tagOSPFNeighborRouterID: fmt.Sprintf("192.0.2.%d", i+1),
			tagOSPFNeighborIP:       neighborIP,
			tagOSPFNeighborState:    "full",
		})
	}
	cache.finalize()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		snapshot, ok := cache.buildObservationSnapshot()
		runtime.KeepAlive(snapshot)
		runtime.KeepAlive(ok)
	}
}
