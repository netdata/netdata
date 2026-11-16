// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/stretchr/testify/require"
)

// Includes strict decoding, validation, registration, writing and real commit.
// Work scales with payload bytes, samples and retained family names, not their
// product. Allocations should follow the current frame, plus retained contracts.
// Timing is a development-machine trend, not a CI threshold.
func BenchmarkSnapshotPipeline(b *testing.B) {
	benchmarkSnapshotPipeline(b, formatJSON)
}

func BenchmarkLinesSnapshotPipeline(b *testing.B) {
	benchmarkSnapshotPipeline(b, formatLines)
}

func benchmarkSnapshotPipeline(b *testing.B, format string) {
	for _, size := range []struct{ families, samples int }{{1, 1}, {10, 100}, {1000, 1}} {
		b.Run(fmt.Sprintf("families=%d/samples=%d", size.families, size.samples), func(b *testing.B) {
			var families []any
			var lines strings.Builder
			for f := range size.families {
				var samples []any
				for s := range size.samples {
					fmt.Fprintf(&lines, "metric_%d:%g|gauge|#instance:%d|unit:jobs\n", f, float64(s)+0.5, s)
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
			decode := decodeSnapshot
			if format == formatLines {
				data, decode = []byte(lines.String()), decodeLines
			}
			c := New()
			managed, ok := metrix.AsCycleManagedStore(c.store)
			require.True(b, ok)
			cycle := managed.CycleController()
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				cycle.BeginCycle()
				c.reconcileContracts()
				snap, err := decode(data)
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

// Domain validation scans names without allocating normalized copies. This
// measures that path with decoding; time is a development-host trend only.
func BenchmarkStateSetSnapshot(b *testing.B) {
	for _, count := range []int{32, 512} {
		b.Run(fmt.Sprintf("states=%d", count), func(b *testing.B) {
			states := make([]string, count)
			for i := range states {
				states[i] = fmt.Sprintf("state %d", i)
			}
			data, err := json.Marshal(snapshot{
				Version: "v1",
				Checks:  []checkFamily{},
				Metrics: []metricFamily{{Name: "state", Type: metricStateSet, States: states,
					Samples: []metricSample{{Active: []string{states[0]}}}}},
			})
			require.NoError(b, err)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if _, err := decodeSnapshot(data); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
