// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

func BenchmarkSNMPTopologyOfflineInspectionScaling(b *testing.B) {
	tests := []struct {
		devices             int
		fdbEntriesPerDevice int
		sharedEndpoints     bool
	}{
		{devices: 8, fdbEntriesPerDevice: 128},
		{devices: 40, fdbEntriesPerDevice: 1600},
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
			scenario := benchmarkTopologyReplayScenario(tc.devices, tc.fdbEntriesPerDevice, tc.sharedEndpoints)
			_, diagnostics := newTopologyScenarioReplayFixture(b, scenario)
			options := topologyoptions.DefaultQueryOptions()
			replay := replayTopologyDiagnosticStages(diagnostics, options)
			if replay.graph.state != topologyInspectionPresent || len(replay.data.Links) == 0 {
				b.Fatalf("inspection probe graph=%d links=%d err=%v", replay.graph.state, len(replay.data.Links), replay.err)
			}
			linkSubject, ok := topologyInspectionSubjectFromLink(replay.data, 0)
			if !ok {
				b.Fatal("inspection probe could not resolve first link")
			}

			b.Run("subject=device", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					report, err := inspectTopologyDevice(diagnostics, options, ddsnmp.DeviceRegistrationID(1))
					if err != nil || report.graphIdentity.membership.state == topologyInspectionUndetermined {
						b.Fatalf("device inspection state=%d err=%v", report.graphIdentity.membership.state, err)
					}
					runtime.KeepAlive(report)
				}
			})

			b.Run("subject=link", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					report, err := inspectTopologyLink(diagnostics, options, linkSubject)
					if err != nil || report.graphLink.membership.state == topologyInspectionUndetermined {
						b.Fatalf("link inspection state=%d err=%v", report.graphLink.membership.state, err)
					}
					runtime.KeepAlive(report)
				}
			})
		})
	}
}
