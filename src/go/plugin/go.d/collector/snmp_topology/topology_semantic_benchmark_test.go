// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func BenchmarkTopologySemanticIngest(b *testing.B) {
	for _, metricCount := range []int{100, 1000, 10_000} {
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
		input := topologySemanticDeviceInput{hostname: "192.0.2.1"}
		collectedAt := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

		for _, capture := range []bool{false, true} {
			b.Run(fmt.Sprintf("metrics=%d/capture=%t", metricCount, capture), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					builder := newTopologyBuilderFromSemanticInput(input, nil, collectedAt, time.Hour)
					var recorder *topologySemanticRecorder
					if capture {
						recorder = newTopologySemanticRecorder(
							input,
							nil,
							collectedAt,
							time.Hour,
							defaultTopologySemanticLimits,
						)
					}
					consumeTopologySemanticEvent(builder, recorder, topologySemanticEvent{kind: topologySemanticEventSysUptime, sysUptime: 1})
					consumeTopologySemanticEvent(builder, recorder, topologySemanticEvent{kind: topologySemanticEventProfileTags, profiles: pms})
					consumeTopologySemanticEvent(builder, recorder, topologySemanticEvent{kind: topologySemanticEventTopologyMetrics, profiles: pms})
					consumeTopologySemanticEvent(builder, recorder, topologySemanticEvent{kind: topologySemanticEventBGPPeers, profiles: pms})
					if recorder != nil && recorder.finish().state != diagnosticCaptureAvailable {
						b.Fatal("semantic capture unavailable")
					}
				}
			})
		}
	}
}
