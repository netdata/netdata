// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
)

func BenchmarkTopologyAcquisitionIngest(b *testing.B) {
	for _, metricCount := range []int{100, 1000, 10_000} {
		pms := benchmarkTopologyAcquisitionMetrics(metricCount)
		report := acquisitionReportForMetrics(0, ddsnmpcollector.AcquisitionProfileOutcomeSuccess, pms[0])
		input := topologySemanticDeviceInput{hostname: "192.0.2.1"}
		collectedAt := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

		for _, acquisition := range []bool{false, true} {
			b.Run(fmt.Sprintf("metrics=%d/acquisition=%t", metricCount, acquisition), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					builder := newTopologyBuilderFromSemanticInput(input, nil, collectedAt, time.Hour)
					var recorder *topologyAcquisitionRecorder
					if acquisition {
						recorder = newTopologyAcquisitionRecorder(
							topologyAcquisitionAttemptID{registrationID: 1, ordinal: 1},
							input,
							topologyTargetResolutionEvidence{outcome: topologyTargetResolutionEmpty},
							defaultTopologyAcquisitionLimits,
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
					if recorder != nil && recorder.finish().state != diagnosticCaptureAvailable {
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
				snapshot, err := replayTopologyAcquisitionEvidence(evidence)
				if err != nil || snapshot == nil {
					b.Fatalf("replay: snapshot=%v err=%v", snapshot, err)
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

func benchmarkTopologyAcquisitionEvidence(b *testing.B, pms []*ddsnmp.ProfileMetrics) *topologyAcquisitionAttemptEvidence {
	b.Helper()
	input := topologySemanticDeviceInput{hostname: "192.0.2.1"}
	recorder := newTopologyAcquisitionRecorder(
		topologyAcquisitionAttemptID{registrationID: 1, ordinal: 1},
		input,
		topologyTargetResolutionEvidence{outcome: topologyTargetResolutionEmpty},
		defaultTopologyAcquisitionLimits,
	)
	observer := recorder.beginContext(0, "", "")
	observer.ObserveProfile(acquisitionReportForMetrics(
		0, ddsnmpcollector.AcquisitionProfileOutcomeSuccess, pms[0],
	), pms[0])
	recorder.completeContext(0, successfulAcquisitionPhase())
	recorder.setCollectedShape(time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC), time.Hour, 1)
	capture := recorder.finish()
	if capture.state != diagnosticCaptureAvailable || capture.evidence == nil {
		b.Fatal("acquisition capture unavailable")
	}
	return capture.evidence
}
