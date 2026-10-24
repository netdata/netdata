// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologydiag"
)

func BenchmarkTopologyAcquisitionIngest(b *testing.B) {
	for _, metricCount := range []int{100, 1000, 10_000} {
		pms := benchmarkTopologyAcquisitionMetrics(metricCount)
		report := acquisitionReportForMetrics(0, ddsnmpcollector.AcquisitionProfileOutcomeSuccess, pms[0])
		input := topologydiag.DeviceInput{Hostname: "192.0.2.1"}
		collectedAt := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

		for _, acquisition := range []bool{false, true} {
			b.Run(fmt.Sprintf("metrics=%d/acquisition=%t", metricCount, acquisition), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					builder := newTopologyBuilderFromSemanticInput(input, nil, collectedAt, time.Hour)
					var recorder *topologyAcquisitionRecorder
					if acquisition {
						recorder = newTopologyAcquisitionRecorder(
							topologydiag.AcquisitionAttemptID{RegistrationID: 1, Ordinal: 1},
							input,
							topologydiag.TargetResolutionEvidence{Outcome: topologydiag.TargetResolutionEmpty},
						)
						observer := recorder.beginContext(0, "", "")
						observer.ObserveProfile(report, pms[0])
						recorder.completeContext(0, successfulAcquisitionPhase())
						recorder.setCollectedShape(collectedAt, time.Hour, 1)
					}
					applyTopologySemanticEvent(builder, topologySemanticEvent{kind: topologySemanticEventSysUptime, sysUptime: 1})
					applyTopologySemanticEvent(builder, topologySemanticEvent{kind: topologySemanticEventProfileTags, profiles: pms})
					applyTopologySemanticEvent(builder, topologySemanticEvent{kind: topologySemanticEventTopologyMetrics, profiles: pms})
					applyTopologySemanticEvent(builder, topologySemanticEvent{kind: topologySemanticEventBGPPeers, profiles: pms})
					if recorder != nil && recorder.finish().State != topologydiag.CaptureAvailable {
						b.Fatal("acquisition capture unavailable")
					}
				}
			})
		}
	}
}

func BenchmarkTopologyAcquisitionReplay(b *testing.B) {
	for _, metricCount := range []int{100, 1000, 10_000} {
		pms := benchmarkTopologyAcquisitionMetrics(metricCount)
		evidence := benchmarkTopologyAcquisitionEvidence(b, pms)

		b.Run(fmt.Sprintf("metrics=%d", metricCount), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				snapshot, present := replayTopologyAcquisitionEvidence(evidence)
				if !present {
					b.Fatal("missing replay observation")
				}
				runtime.KeepAlive(snapshot)
			}
		})
	}
}

func benchmarkTopologyAcquisitionMetrics(metricCount int) []*ddsnmp.ProfileMetrics {
	pms := []*ddsnmp.ProfileMetrics{{TopologyMetrics: make([]ddsnmp.Metric, metricCount)}}
	for i := range metricCount {
		pms[0].TopologyMetrics[i] = ddsnmp.Metric{
			TopologyKind: ddsnmp.KindIfName,
			Tags: map[string]string{
				tagTopoIfIndex: fmt.Sprintf("%d", i+1),
				tagTopoIfName:  fmt.Sprintf("Ethernet%d", i+1),
			},
		}
	}
	return pms
}

func benchmarkTopologyAcquisitionEvidence(b *testing.B, pms []*ddsnmp.ProfileMetrics) *topologydiag.AcquisitionAttemptEvidence {
	b.Helper()
	input := topologydiag.DeviceInput{Hostname: "192.0.2.1"}
	recorder := newTopologyAcquisitionRecorder(
		topologydiag.AcquisitionAttemptID{RegistrationID: 1, Ordinal: 1},
		input,
		topologydiag.TargetResolutionEvidence{Outcome: topologydiag.TargetResolutionEmpty},
	)
	observer := recorder.beginContext(0, "", "")
	observer.ObserveProfile(acquisitionReportForMetrics(
		0, ddsnmpcollector.AcquisitionProfileOutcomeSuccess, pms[0],
	), pms[0])
	recorder.completeContext(0, successfulAcquisitionPhase())
	recorder.setCollectedShape(time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC), time.Hour, 1)
	capture := recorder.finish()
	if capture.State != topologydiag.CaptureAvailable || capture.Evidence == nil {
		b.Fatal("acquisition capture unavailable")
	}
	return capture.Evidence
}
