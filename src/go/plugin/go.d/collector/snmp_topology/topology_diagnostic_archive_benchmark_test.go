// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"bytes"
	jsonv2 "encoding/json/v2"
	"fmt"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
)

func BenchmarkSNMPTopologyDiagnosticArchive(b *testing.B) {
	cases := map[string]func(testing.TB) topologyDiagnostics{
		"representative": func(tb testing.TB) topologyDiagnostics {
			_, diagnostics := newTopologyScenarioReplayFixture(tb, newMixedL2L3ControlScenario())
			completeTopologyDiagnosticArchiveFixture(&diagnostics)
			return diagnostics
		},
		"maximum_records": func(tb testing.TB) topologyDiagnostics {
			return benchmarkTopologyArchiveDiagnostics(tb, 3, 83_000, 8)
		},
		"maximum_combined": func(tb testing.TB) topologyDiagnostics {
			return benchmarkTopologyArchiveDiagnostics(tb, 3, 83_000, 215)
		},
		"maximum_logical_bytes": func(tb testing.TB) topologyDiagnostics {
			return benchmarkTopologyArchiveDiagnostics(tb, 2, 7_500, 4_000)
		},
		"maximum_escaping": func(tb testing.TB) topologyDiagnostics {
			return benchmarkTopologyArchiveDiagnosticsWithValue(tb, 2, 7_500, strings.Repeat("\x00", 4_000))
		},
	}
	for name, build := range cases {
		b.Run(name, func(b *testing.B) {
			diagnostics := build(b)
			var encoded bytes.Buffer
			if err := writeTopologyDiagnosticArchiveWithProducerVersion(
				&encoded,
				diagnostics,
				"v-benchmark",
			); err != nil {
				b.Fatal(err)
			}
			compressedBytes := encoded.Len()
			decodedBytes := decodedTopologyDiagnosticArchiveSize(b, encoded.Bytes())
			runtime.GC()
			b.Run("write", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					var target bytes.Buffer
					if err := writeTopologyDiagnosticArchiveWithProducerVersion(
						&target,
						diagnostics,
						"v-benchmark",
					); err != nil {
						b.Fatal(err)
					}
				}
				reportTopologyDiagnosticArchiveSize(b, decodedBytes, compressedBytes)
			})
			b.Run("read", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := readTopologyDiagnosticArchive(
						bytes.NewReader(encoded.Bytes()),
						defaultTopologyDiagnosticArchiveReadLimits(),
					); err != nil {
						b.Fatal(err)
					}
				}
				reportTopologyDiagnosticArchiveSize(b, decodedBytes, compressedBytes)
			})
		})
	}
}

func BenchmarkSNMPTopologyDiagnosticArchiveEncoderLevel(b *testing.B) {
	cases := map[string]topologyDiagnostics{}
	_, cases["representative"] = newTopologyScenarioReplayFixture(b, newMixedL2L3ControlScenario())
	cases["maximum_combined"] = benchmarkTopologyArchiveDiagnostics(b, 3, 83_000, 215)
	levels := map[string]zstd.EncoderLevel{
		"fastest": zstd.SpeedFastest,
		"default": zstd.SpeedDefault,
		"better":  zstd.SpeedBetterCompression,
	}
	for shape, diagnostics := range cases {
		document, err := newTopologyDiagnosticArchiveDocumentV1(diagnostics, "v-benchmark")
		if err != nil {
			b.Fatal(err)
		}
		for name, level := range levels {
			b.Run(shape+"/"+name, func(b *testing.B) {
				var compressedBytes int
				b.ReportAllocs()
				for b.Loop() {
					var target bytes.Buffer
					encoder, err := zstd.NewWriter(
						&target,
						zstd.WithEncoderLevel(level),
						zstd.WithEncoderConcurrency(1),
						zstd.WithEncoderCRC(true),
					)
					if err != nil {
						b.Fatal(err)
					}
					if err := jsonv2.MarshalWrite(encoder, document, topologyDiagnosticArchiveWriterJSONOptions); err != nil {
						b.Fatal(err)
					}
					if err := encoder.Close(); err != nil {
						b.Fatal(err)
					}
					compressedBytes = target.Len()
				}
				b.ReportMetric(float64(compressedBytes), "compressed_B")
			})
		}
	}
}

func decodedTopologyDiagnosticArchiveSize(tb testing.TB, encoded []byte) int {
	tb.Helper()
	decoder, err := zstd.NewReader(bytes.NewReader(encoded), zstd.WithDecoderConcurrency(1))
	if err != nil {
		tb.Fatal(err)
	}
	n, err := io.Copy(io.Discard, decoder)
	decoder.Close()
	if err != nil {
		tb.Fatal(err)
	}
	return int(n)
}

func reportTopologyDiagnosticArchiveSize(
	b *testing.B,
	decodedBytes int,
	compressedBytes int,
) {
	b.ReportMetric(float64(decodedBytes), "decoded_B")
	b.ReportMetric(float64(compressedBytes), "compressed_B")
	b.ReportMetric(float64(decodedBytes)/float64(compressedBytes), "compression_ratio")
}

func benchmarkTopologyArchiveDiagnostics(
	tb testing.TB,
	deviceCount int,
	metricsPerDevice int,
	tagValueBytes int,
) topologyDiagnostics {
	tb.Helper()
	return benchmarkTopologyArchiveDiagnosticsWithValue(tb, deviceCount, metricsPerDevice, strings.Repeat("x", tagValueBytes))
}

func benchmarkTopologyArchiveDiagnosticsWithValue(
	tb testing.TB,
	deviceCount int,
	metricsPerDevice int,
	value string,
) topologyDiagnostics {
	tb.Helper()
	return benchmarkTopologyArchiveDiagnosticsWithMetric(
		tb,
		deviceCount,
		metricsPerDevice,
		ddsnmp.KindIfName,
		func(index int) map[string]string {
			return map[string]string{
				tagTopoIfIndex: fmt.Sprintf("%d", index+1),
				tagTopoIfName:  value,
			}
		},
	)
}

func benchmarkTopologyArchiveDiagnosticsWithMetric(
	tb testing.TB,
	deviceCount int,
	metricsPerDevice int,
	kind ddsnmp.TopologyKind,
	metricTags func(int) map[string]string,
) topologyDiagnostics {
	tb.Helper()
	const sequence = uint64(1)
	publishedAt := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	entries := make([]ddsnmp.DeviceEntry, 0, deviceCount)
	states := make(map[ddsnmp.DeviceRegistrationID]deviceRefreshState, deviceCount)
	selected := make(map[ddsnmp.DeviceRegistrationID]bool, deviceCount)
	seen := make(map[ddsnmp.DeviceRegistrationID]bool, deviceCount)
	for deviceIndex := range deviceCount {
		registrationID := ddsnmp.DeviceRegistrationID(deviceIndex + 1)
		hostname := fmt.Sprintf("192.0.2.%d", registrationID)
		info := ddsnmp.DeviceConnectionInfo{Hostname: hostname, SysName: fmt.Sprintf("switch-%d", registrationID)}
		metrics := make([]ddsnmp.Metric, 0, metricsPerDevice)
		references := make([]ddsnmpcollector.AcquisitionValueReference, 0, metricsPerDevice)
		for metricIndex := range metricsPerDevice {
			metrics = append(metrics, ddsnmp.Metric{
				TopologyKind: kind,
				Tags:         metricTags(metricIndex),
			})
			references = append(references, ddsnmpcollector.AcquisitionValueReference{
				RouteOrdinal: 0,
				RowOrdinal:   uint32(metricIndex),
			})
		}
		profileMetrics := &ddsnmp.ProfileMetrics{TopologyMetrics: metrics}
		recorder := newTopologyAcquisitionRecorder(
			topologyAcquisitionAttemptID{registrationID: registrationID, ordinal: 1},
			topologySemanticDeviceInputFromConnection(info),
			topologyTargetResolutionEvidence{outcome: topologyTargetResolutionEmpty},
			defaultTopologyAcquisitionLimits,
		)
		observer := recorder.beginContext(0, "", "")
		observer.ObserveProfile(ddsnmpcollector.AcquisitionProfileReport{
			Identity: ddsnmpcollector.AcquisitionProfileIdentity{Ordinal: 0},
			Outcome:  ddsnmpcollector.AcquisitionProfileOutcomeSuccess,
			Routes: []ddsnmpcollector.AcquisitionRouteReport{{
				Ordinal: 0,
				Kind:    ddsnmpcollector.AcquisitionRouteKindTopologyTable,
				Source:  ddsnmpcollector.AcquisitionRouteSourceWalk,
				Outcome: ddsnmpcollector.AcquisitionRouteOutcomeValues,
				Rows:    uint64(metricsPerDevice),
				Values:  uint64(metricsPerDevice),
			}},
			TopologyValueReferences: references,
		}, profileMetrics)
		recorder.completeContext(0, successfulAcquisitionPhase())
		recorder.setCollectedShape(publishedAt, time.Hour, 3600)
		capture := recorder.finish()
		if capture.state != diagnosticCaptureAvailable {
			tb.Fatalf("benchmark capture %d is unavailable: state=%d reason=%d records=%d bytes=%d",
				registrationID, capture.state, capture.reason, capture.recordCount, capture.logicalBytes)
		}
		snapshot := &topologyDeviceSnapshot{
			collectedAt: publishedAt,
			freshFor:    time.Hour,
			acquisition: capture,
		}
		generation := activateTopologyDeviceSnapshot(registrationID, sequence, publishedAt, snapshot)
		states[registrationID] = deviceRefreshState{
			generation:    generation,
			latestAttempt: capture,
			lastAttempt:   publishedAt,
			lastSuccess:   publishedAt,
			outcome:       deviceRefreshOutcomeSuccess,
		}
		entries = append(entries, ddsnmp.DeviceEntry{RegistrationID: registrationID, Info: info})
		selected[registrationID] = true
		seen[registrationID] = true
	}
	cut, err := projectTopologyDiagnosticCut(topologyDiagnosticCutInput{
		sequence:    sequence,
		startedAt:   publishedAt.Add(-time.Minute),
		publishedAt: publishedAt,
		entries:     entries,
		selected:    selected,
		seen:        seen,
		states:      states,
		limits:      defaultTopologyDiagnosticGlobalLimits,
	})
	if err != nil {
		tb.Fatal(err)
	}
	if cut.captureState != diagnosticCaptureAvailable {
		tb.Fatalf("benchmark cut is unavailable: state=%d reason=%d records=%d bytes=%d",
			cut.captureState, cut.captureReason, cut.recordCount, cut.logicalBytes)
	}
	return topologyDiagnostics{
		lifecycle: topologyJobLifecycleDiagnosticCut{
			state:  diagnosticCaptureAvailable,
			reason: diagnosticCaptureReasonNone,
			cut: ddsnmp.DeviceLifecycleCut{
				Sequence:   1,
				CapturedAt: publishedAt,
			},
		},
		producerScopeID: "benchmark-scope",
		topology:        cut,
	}
}
