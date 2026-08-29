// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"bytes"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"fmt"
	"io"
	"math"
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
		"maximum_array_allocation": func(tb testing.TB) topologyDiagnostics {
			return benchmarkTopologyArchiveArrayAllocation(tb, 249_999)
		},
		"maximum_combined": func(tb testing.TB) topologyDiagnostics {
			return benchmarkTopologyArchiveDiagnostics(tb, 3, 83_000, 215)
		},
		"maximum_map_entries": func(tb testing.TB) topologyDiagnostics {
			return benchmarkTopologyArchiveMapCardinality(tb)
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
			collectionCounts := topologyDiagnosticArchiveCollectionCountsForBenchmark(b, encoded.Bytes())
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
				reportTopologyDiagnosticArchiveSize(b, decodedBytes, compressedBytes, collectionCounts)
			})
			b.Run("read", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := readTopologyDiagnosticArchive(bytes.NewReader(encoded.Bytes())); err != nil {
						b.Fatal(err)
					}
				}
				reportTopologyDiagnosticArchiveSize(b, decodedBytes, compressedBytes, collectionCounts)
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
					if err := jsonv2.MarshalWrite(encoder, document, topologyDiagnosticArchiveJSONOptions); err != nil {
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

func topologyDiagnosticArchiveCollectionCountsForBenchmark(
	tb testing.TB,
	encoded []byte,
) topologyDiagnosticArchiveCollectionCounts {
	tb.Helper()
	var counts topologyDiagnosticArchiveCollectionCounts
	limits := defaultTopologyDiagnosticArchiveLimits
	limits.maxArrayAllocationBytes = math.MaxUint64
	limits.maxMapEntries = math.MaxUint64
	err := consumeTopologyDiagnosticArchiveJSON(encoded, limits, func(decoder *jsontext.Decoder) error {
		var err error
		counts, err = preflightTopologyDiagnosticArchiveJSON(decoder, limits)
		return err
	})
	if err != nil {
		tb.Fatal(err)
	}
	return counts
}

func reportTopologyDiagnosticArchiveSize(
	b *testing.B,
	decodedBytes int,
	compressedBytes int,
	counts topologyDiagnosticArchiveCollectionCounts,
) {
	b.ReportMetric(float64(decodedBytes), "decoded_B")
	b.ReportMetric(float64(compressedBytes), "compressed_B")
	b.ReportMetric(float64(decodedBytes)/float64(compressedBytes), "compression_ratio")
	b.ReportMetric(float64(counts.arrayAllocationBytes), "array_allocation_B")
	b.ReportMetric(float64(counts.mapEntries), "map_entries")
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

func benchmarkTopologyArchiveMapCardinality(tb testing.TB) topologyDiagnostics {
	tb.Helper()
	tags := map[string]string{
		tagCdpIfIndex:               "",
		tagCdpIfName:                "",
		tagCdpDeviceIndex:           "",
		tagCdpDeviceID:              "",
		tagCdpAddressType:           "",
		tagCdpDevicePort:            "",
		tagCdpVersion:               "",
		tagCdpPlatform:              "",
		tagCdpCaps:                  "",
		tagCdpAddress:               "",
		tagCdpVTPDomain:             "",
		tagCdpNativeVLAN:            "",
		tagCdpDuplex:                "",
		tagCdpPower:                 "",
		tagCdpMTU:                   "",
		tagCdpSysName:               "",
		tagCdpSysObjectID:           "",
		tagCdpPrimaryMgmtAddrType:   "",
		tagCdpPrimaryMgmtAddr:       "",
		tagCdpSecondaryMgmtAddrType: "",
		tagCdpSecondaryMgmtAddr:     "",
		tagCdpPhysicalLocation:      "",
		tagCdpLastChange:            "",
	}
	logicalBytesPerMetric := uint64(len(ddsnmp.KindCdpCache)) + topologySemanticFilteredStringMapBytes(
		tags,
		func(key string) bool { return topologySemanticMetricTagAllowed(ddsnmp.KindCdpCache, key) },
	)
	perDeviceBudget := (defaultTopologyDiagnosticGlobalLimits.maxLogicalBytes-topologyDiagnosticCutLogicalBytes-
		2*topologyDiagnosticRowLogicalBytes)/2 - 1_024
	metricsPerDevice := min(
		int(perDeviceBudget/logicalBytesPerMetric),
		int(defaultTopologyAcquisitionLimits.maxRecords-4),
	)
	return benchmarkTopologyArchiveDiagnosticsWithMetric(
		tb,
		2,
		metricsPerDevice,
		ddsnmp.KindCdpCache,
		func(int) map[string]string { return tags },
	)
}

func benchmarkTopologyArchiveArrayAllocation(tb testing.TB, deviceCount int) topologyDiagnostics {
	tb.Helper()
	const sequence = uint64(1)
	devices := make([]topologySweepDeviceDiagnostic, 0, deviceCount)
	for index := range deviceCount {
		registrationID := ddsnmp.DeviceRegistrationID(index + 1)
		retained := &topologyAcquisitionCapture{
			attemptID: topologyAcquisitionAttemptID{registrationID: registrationID, ordinal: 1},
			state:     diagnosticCaptureUnavailable,
			reason:    diagnosticCaptureReasonProjectionError,
		}
		latest := &topologyAcquisitionCapture{
			attemptID: topologyAcquisitionAttemptID{registrationID: registrationID, ordinal: 2},
			state:     diagnosticCaptureUnavailable,
			reason:    diagnosticCaptureReasonProjectionError,
		}
		devices = append(devices, topologySweepDeviceDiagnostic{
			registrationID: registrationID,
			outcome:        deviceRefreshOutcomeFailed,
			retainedSuccess: topologyEvidenceRef{
				registrationID: registrationID,
				generation:     sequence,
			},
			hasRetainedSuccess: true,
			acquisition:        retained,
			latestAttempt:      latest,
		})
	}
	return topologyDiagnostics{
		lifecycle: topologyJobLifecycleDiagnosticCut{
			state:  diagnosticCaptureLimitExceeded,
			reason: diagnosticCaptureReasonGlobalRecordLimit,
		},
		producerScopeID: "benchmark-scope",
		topology: &topologySweepDiagnosticCut{
			sequence:      sequence,
			captureState:  diagnosticCaptureAvailable,
			captureReason: diagnosticCaptureReasonNone,
			recordCount:   uint64(deviceCount + 1),
			logicalBytes:  topologyDiagnosticCutLogicalBytes + uint64(deviceCount)*topologyDiagnosticRowLogicalBytes,
			devices:       devices,
		},
	}
}
