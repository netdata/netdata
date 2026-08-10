// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

func BenchmarkJobRecord(b *testing.B) {
	job := NewRegistry().Attach("listener", Options{DedupEnabled: true})
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		job.PipelineReceived()
		job.Error(ErrorDecodeFailed)
		job.DedupSuppressed()
	}
}

func BenchmarkJobCollect(b *testing.B) {
	for _, enabled := range []bool{false, true} {
		name := "dedup_disabled"
		if enabled {
			name = "dedup_enabled"
		}
		b.Run(name, func(b *testing.B) {
			job := NewRegistry().Attach("listener", Options{DedupEnabled: enabled})
			store := metrix.NewCollectorStore()
			managed, ok := metrix.AsCycleManagedStore(store)
			if !ok {
				b.Fatal("collector store does not expose cycle control")
			}

			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				managed.CycleController().BeginCycle()
				job.Collect(store)
				if err := managed.CycleController().CommitCycleSuccess(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
