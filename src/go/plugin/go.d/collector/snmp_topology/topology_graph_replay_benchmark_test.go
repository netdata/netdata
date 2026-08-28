// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

func BenchmarkSNMPTopologyTypedGraphReplayScaling(b *testing.B) {
	tests := []struct {
		devices             int
		fdbEntriesPerDevice int
		sharedEndpoints     bool
	}{
		{devices: 8, fdbEntriesPerDevice: 128},
		{devices: 40, fdbEntriesPerDevice: 1600, sharedEndpoints: true},
	}

	for _, tc := range tests {
		name := fmt.Sprintf(
			"devices=%d/fdb_entries_per_device=%d/shared_endpoints=%t",
			tc.devices,
			tc.fdbEntriesPerDevice,
			tc.sharedEndpoints,
		)
		b.Run(name, func(b *testing.B) {
			scenario := benchmarkTopologyReplayScenario(
				tc.devices,
				tc.fdbEntriesPerDevice,
				tc.sharedEndpoints,
			)
			registry, diagnostics := newTopologyScenarioReplayFixture(b, scenario)
			options := topologyoptions.DefaultQueryOptions()

			b.Run("source=live", func(b *testing.B) {
				deps := funcDepsAdapter{registry: registry}
				probe, ok, err := deps.Snapshot(options)
				if err != nil || !ok || probe.Links.Rows == 0 {
					b.Fatalf("live probe links=%d ok=%t err=%v", probe.Links.Rows, ok, err)
				}
				b.ReportMetric(float64(probe.Actors.Rows), "actors/op")
				b.ReportMetric(float64(probe.Links.Rows), "links/op")
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					payload, ok, err := deps.Snapshot(options)
					if err != nil || !ok {
						b.Fatalf("live typed graph ok=%t err=%v", ok, err)
					}
					runtime.KeepAlive(payload)
				}
			})

			b.Run("source=offline_replay", func(b *testing.B) {
				probe, ok, err := replayTopologyDiagnostics(diagnostics, options)
				if err != nil || !ok || probe.Links.Rows == 0 {
					b.Fatalf("replay probe links=%d ok=%t err=%v", probe.Links.Rows, ok, err)
				}
				b.ReportMetric(float64(probe.Actors.Rows), "actors/op")
				b.ReportMetric(float64(probe.Links.Rows), "links/op")
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					payload, ok, err := replayTopologyDiagnostics(diagnostics, options)
					if err != nil || !ok {
						b.Fatalf("offline typed graph replay ok=%t err=%v", ok, err)
					}
					runtime.KeepAlive(payload)
				}
			})
		})
	}
}

func benchmarkTopologyReplayScenario(
	deviceCount int,
	fdbEntriesPerDevice int,
	sharedEndpoints bool,
) *topologyScenario {
	scenario := newTopologyScenario("benchmark-fdb-replay")
	devices := make([]*topologyScenarioDevice, 0, deviceCount)
	ports := make([]*topologyScenarioPort, 0, deviceCount)
	for deviceIndex := range deviceCount {
		device := scenario.Switch(
			fmt.Sprintf("benchmark-switch-%d", deviceIndex),
			fmt.Sprintf("192.0.2.%d", deviceIndex+1),
			benchmarkManagedFabricDeviceMAC(deviceIndex),
		)
		devices = append(devices, device)
		ports = append(ports, device.Port("uplink", 1))
	}
	for deviceIndex := range devices {
		if fdbEntriesPerDevice > 0 {
			scenario.FDB(ports[deviceIndex], devices[(deviceIndex+1)%deviceCount].chassisMAC)
		}
		for entryIndex := 1; entryIndex < fdbEntriesPerDevice; entryIndex++ {
			scenario.FDB(
				ports[deviceIndex],
				benchmarkManagedFabricEndpointMAC(deviceIndex, entryIndex, sharedEndpoints),
			)
		}
	}
	return scenario
}
