// SPDX-License-Identifier: GPL-3.0-or-later

package smartctl

import (
	"testing"

	"github.com/tidwall/gjson"
)

var benchmarkSmartctlMetrics map[string]int64

func BenchmarkCollectorCollectSmartDevice(b *testing.B) {
	result := gjson.ParseBytes(dataTypeSataDeviceHDDSda)
	dev := newSmartDevice(&result)
	id := newDeviceIdentity(dev.deviceName(), dev.deviceType())
	smartAttrs := newSmartAttributeIdentities(dev)
	collr := New()

	b.ReportAllocs()
	for b.Loop() {
		mx := make(map[string]int64)
		collr.collectSmartDevice(mx, dev, id, smartAttrs)
		benchmarkSmartctlMetrics = mx
	}
}

func TestCollectorCollectSmartDeviceAllocationEnvelope(t *testing.T) {
	result := gjson.ParseBytes(dataTypeSataDeviceHDDSda)
	dev := newSmartDevice(&result)
	id := newDeviceIdentity(dev.deviceName(), dev.deviceType())
	smartAttrs := newSmartAttributeIdentities(dev)
	collr := New()

	const maxAllocationsPerDevicePoll = 171
	allocs := testing.AllocsPerRun(100, func() {
		mx := make(map[string]int64)
		collr.collectSmartDevice(mx, dev, id, smartAttrs)
		benchmarkSmartctlMetrics = mx
	})
	if allocs > maxAllocationsPerDevicePoll {
		t.Fatalf("device poll allocated %.0f times, want at most %d", allocs, maxAllocationsPerDevicePoll)
	}
}
