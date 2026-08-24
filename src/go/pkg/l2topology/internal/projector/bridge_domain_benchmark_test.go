// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"fmt"
	"runtime"
	"testing"
)

func BenchmarkBuildBridgeDomainModelHighSegmentDensity(b *testing.B) {
	for _, segmentCount := range []int{128, 1024, 8192} {
		b.Run(fmt.Sprintf("segments=%d", segmentCount), func(b *testing.B) {
			macLinks := make([]bridgeMacLinkRecord, 0, segmentCount)
			for index := range segmentCount {
				macLinks = append(macLinks, bridgeMacLinkRecord{
					port: bridgePortRef{
						deviceID:    fmt.Sprintf("switch-%d", index),
						ifIndex:     1,
						bridgePort:  "1",
						fdbDomainID: fmt.Sprintf("fdb:%d", index),
					},
					endpointID: fmt.Sprintf("mac:02:00:%02x:%02x:%02x:%02x", index>>24, index>>16, index>>8, index),
					method:     "fdb",
				})
			}

			probe := buildBridgeDomainModel(nil, macLinks)
			if len(probe.domains) != segmentCount {
				b.Fatalf("domains=%d, want %d", len(probe.domains), segmentCount)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				model := buildBridgeDomainModel(nil, macLinks)
				runtime.KeepAlive(model)
			}
		})
	}
}
