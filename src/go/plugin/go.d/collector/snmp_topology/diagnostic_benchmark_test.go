// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/diagnostic"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

type benchmarkDiagnosticSink struct {
	results chan diagnostic.CaptureResult
}

func (s *benchmarkDiagnosticSink) Store(result diagnostic.CaptureResult) {
	s.results <- result
}

type benchmarkTopologySNMPHandler struct {
	*topologySNMPRecHandler
}

func (h *benchmarkTopologySNMPHandler) Connect() error { return nil }
func (h *benchmarkTopologySNMPHandler) Close() error   { return nil }

func BenchmarkCollectorRefreshDueDeviceWithDiagnostics(b *testing.B) {
	for _, recordCount := range []int{256, 4_096} {
		b.Run(fmt.Sprintf("topology_records=%d", recordCount), func(b *testing.B) {
			profile := &ddsnmp.Profile{
				SourceFile: "benchmark-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Topology: []ddprofiledefinition.TopologyConfig{{
						Kind: ddsnmp.KindIfName,
						MetricsConfig: ddprofiledefinition.MetricsConfig{
							Table: ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.31.1.1.1", Name: "ifXTable"},
							Symbols: []ddprofiledefinition.SymbolConfig{{
								OID: "1.3.6.1.2.1.31.1.1.1.1", Name: "ifName",
							}},
						},
					}},
				},
			}
			metrics := &ddsnmp.ProfileMetrics{Source: profile.SourceFile}
			for i := range recordCount {
				metrics.TopologyMetrics = append(metrics.TopologyMetrics, ddsnmp.Metric{
					TopologyKind: ddsnmp.KindIfName,
					Tags: map[string]string{
						tagTopoIfIndex: fmt.Sprint(i + 1),
						tagTopoIfName:  fmt.Sprintf("Ethernet%d", i+1),
					},
				})
			}

			sink := &benchmarkDiagnosticSink{results: make(chan diagnostic.CaptureResult, 2)}
			recorder, err := diagnostic.NewRecorder(diagnostic.RecorderConfig{
				QueueCapacity:    2,
				MaxMembers:       uint64(recordCount/diagnosticSemanticShardRecords + 16),
				MaxRetainedBytes: 256 << 20,
				Sink:             sink,
			})
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(recorder.Close)

			coll, store := newTestSNMPTopologyCollectorWithStore()
			dev := ddsnmp.DeviceConnectionInfo{
				Hostname: "192.0.2.10", Port: 161, SysName: "benchmark-switch",
				SysObjectID: "1.3.6.1.4.1.9.1.1",
			}
			registerTestDeviceState(store, dev)
			coll.topologyProfiles = func(ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile {
				return []*ddsnmp.Profile{profile}
			}
			coll.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
				return ddCollectorFunc(func() ([]*ddsnmp.ProfileMetrics, error) {
					return []*ddsnmp.ProfileMetrics{metrics}, nil
				})
			}
			coll.newSnmpClient = func() gosnmp.Handler {
				return &benchmarkTopologySNMPHandler{newTopologySNMPHandler([]gosnmp.SnmpPDU{{
					Name: snmputils.OidSnmpEngineTime, Type: gosnmp.Integer, Value: 1234,
				}})}
			}
			coll.diagnosticRecorder = recorder
			now := time.Date(2026, time.August, 27, 14, 0, 0, 0, time.UTC)
			coll.now = func() time.Time { return now }

			run := func() uint64 {
				now = now.Add(defaultRefreshEvery)
				stats := coll.refreshTopology(context.Background())
				if stats.errors != 0 || stats.cachedDevices != 1 {
					b.Fatalf("refresh stats: cached=%d errors=%d", stats.cachedDevices, stats.errors)
				}
				var retainedBytes uint64
				for range 2 {
					result := <-sink.results
					if result.Err != nil {
						b.Fatalf("diagnostic capture: %v", result.Err)
					}
					retainedBytes += result.RetainedBytes
				}
				return retainedBytes
			}

			run()
			b.ReportAllocs()
			b.ResetTimer()
			var retainedBytes uint64
			for b.Loop() {
				retainedBytes = run()
			}
			b.StopTimer()
			b.ReportMetric(float64(recordCount), "topology_records/op")
			b.ReportMetric(float64(retainedBytes), "retained_B/op")
		})
	}
}
