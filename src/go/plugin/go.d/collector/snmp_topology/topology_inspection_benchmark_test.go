// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"runtime"
	"testing"
)

func BenchmarkSNMPTopologyOfflineInspectionScaling(b *testing.B) {
	tests := map[string]struct {
		devices             int
		fdbEntriesPerDevice int
		sharedEndpoints     bool
	}{
		"devices=8/fdb_entries_per_device=128/shared_endpoints=false":   {devices: 8, fdbEntriesPerDevice: 128},
		"devices=40/fdb_entries_per_device=1600/shared_endpoints=false": {devices: 40, fdbEntriesPerDevice: 1600},
		"devices=40/fdb_entries_per_device=1600/shared_endpoints=true":  {devices: 40, fdbEntriesPerDevice: 1600, sharedEndpoints: true},
	}

	for name, tc := range tests {
		b.Run(name, func(b *testing.B) {
			scenario := benchmarkTopologyReplayScenario(tc.devices, tc.fdbEntriesPerDevice, tc.sharedEndpoints)
			_, diagnostics := newTopologyScenarioReplayFixture(b, scenario)
			document, err := newTopologyDiagnosticArchiveDocumentV1(diagnostics, "benchmark")
			if err != nil {
				b.Fatal(err)
			}
			archive, err := InspectDiagnosticDocument(document)
			if err != nil {
				b.Fatal(err)
			}
			options := DefaultDiagnosticQueryOptions()
			exact, err := archive.InspectLinkAt(options, 0)
			if err != nil {
				b.Fatal(err)
			}
			linkSubject := exact.Subject

			b.Run("subject=device", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					report, err := archive.InspectDevice(options, 1)
					if err != nil || report.GraphIdentity.Membership.State == "undetermined" {
						b.Fatalf("device inspection state=%s err=%v", report.GraphIdentity.Membership.State, err)
					}
					runtime.KeepAlive(report)
				}
			})

			b.Run("subject=link", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					report, err := archive.InspectLink(options, linkSubject)
					if err != nil || report.GraphLink.Membership.State == "undetermined" {
						b.Fatalf("link inspection state=%s err=%v", report.GraphLink.Membership.State, err)
					}
					runtime.KeepAlive(report)
				}
			})
		})
	}
}
