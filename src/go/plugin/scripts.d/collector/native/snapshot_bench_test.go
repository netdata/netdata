// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/stretchr/testify/require"
)

// Includes strict decoding, validation, registration, writing and real commit.
// Work scales with payload bytes, samples and retained family names, not their
// product. Allocations should follow the current frame, plus retained contracts.
// Timing is a development-machine trend, not a CI threshold.
func BenchmarkSnapshotPipeline(b *testing.B) {
	for _, size := range []struct{ families, samples int }{{1, 1}, {10, 100}, {1000, 1}} {
		b.Run(fmt.Sprintf("families=%d/samples=%d", size.families, size.samples), func(b *testing.B) {
			var families []any
			for f := range size.families {
				var samples []any
				for s := range size.samples {
					samples = append(
						samples,
						map[string]any{
							"value":  float64(s) + 0.5,
							"labels": map[string]string{"instance": fmt.Sprint(s)},
						},
					)
				}
				families = append(
					families,
					map[string]any{"name": fmt.Sprintf("metric_%d", f), "unit": "jobs", "samples": samples},
				)
			}
			data, err := json.Marshal(map[string]any{"version": "v1", "metrics": families})
			require.NoError(b, err)
			c := New()
			managed, ok := metrix.AsCycleManagedStore(c.store)
			require.True(b, ok)
			cycle := managed.CycleController()
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				cycle.BeginCycle()
				c.reconcileContracts()
				snap, err := decodeSnapshot(data)
				if err != nil {
					b.Fatal(err)
				}
				if err := c.validateContracts(snap.Metrics); err != nil {
					b.Fatal(err)
				}
				c.writeSnapshot(snap)
				c.stageContracts(snap.Metrics)
				if err := cycle.CommitCycleSuccess(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
