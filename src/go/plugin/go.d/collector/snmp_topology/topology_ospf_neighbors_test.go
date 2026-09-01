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

func TestTopologyCache_OSPFMatchingReusesCompactL3Projection(t *testing.T) {
	cache := newTopologyBuilder()
	for i := range 256 {
		cache.updateIfIndexByIP(modernIPv4Tags(
			fmt.Sprintf("10.0.%d.%d", i/256, i%256),
			fmt.Sprintf("%d", i+1),
			"0.0",
		))
	}
	cache.finalize()
	l3Interfaces := cache.snapshotL3Interfaces("local")

	allocsForMatches := func(matches int) float64 {
		return testing.AllocsPerRun(10, func() {
			for range matches {
				match, ok := matchOSPFNeighborLocalInterface("192.0.2.1", l3Interfaces)
				runtime.KeepAlive(match)
				runtime.KeepAlive(ok)
			}
		})
	}

	one := allocsForMatches(1)
	many := allocsForMatches(32)
	require.LessOrEqual(t, many, one+4, "matching must not rebuild the full address projection per neighbor")
}

func TestTopologyCache_MatchOSPFNeighborLocalInterfaceUsesLongestPrefix(t *testing.T) {
	cache := newTopologyBuilder()
	cache.ipAddressesByIP = map[string]resolvedIPAddress{
		"10.0.0.1": {ifIndex: "1", netmask: "255.255.0.0"},
		"10.0.1.1": {ifIndex: "2", netmask: "255.255.255.252"},
	}

	match, ok := matchOSPFNeighborLocalInterface("10.0.1.2", cache.snapshotL3Interfaces("local"))

	require.True(t, ok)
	require.Equal(t, "10.0.1.1", match.IP)
	require.Equal(t, "10.0.1.0", match.Network)
	require.Equal(t, "255.255.255.252", match.Netmask)
	require.Equal(t, "10.0.1.0/30", match.Subnet)
	require.Equal(t, 30, match.Prefix)
}

func BenchmarkTopologyCacheOSPFSnapshotWithoutPrefixes(b *testing.B) {
	for _, addressRows := range []int{4096, 65536} {
		b.Run(fmt.Sprintf("addresses=%d/neighbors=100", addressRows), func(b *testing.B) {
			cache := newTopologyBuilder()
			cache.localDevice.ChassisID = "02:00:00:00:00:01"
			cache.localDevice.ChassisIDType = "macAddress"
			cache.localDevice.SysName = "benchmark-router"
			for i := range addressRows {
				cache.updateIfIndexByIP(modernIPv4Tags(
					fmt.Sprintf("169.254.%d.%d", (i>>8)&255, i&255),
					fmt.Sprintf("%d", i+1),
					"0.0",
				))
			}
			for i := range 100 {
				cache.updateOSPFNeighbor(map[string]string{
					tagOSPFNeighborRouterID: fmt.Sprintf("192.0.2.%d", i+1),
					tagOSPFNeighborIP:       fmt.Sprintf("198.51.100.%d", i+1),
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
		})
	}
}
