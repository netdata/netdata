// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"strconv"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

const benchTemplateYAML = `
version: v1
groups:
  - family: "Bench"
    metrics: ["bench_metric"]
    charts:
      - title: "Bench metric"
        context: "bench.metric"
        units: "units"
        dimensions:
          - selector: bench_metric
            name_from_label: id
`

func BenchmarkBuildPlanBySeriesCardinality(b *testing.B) {
	tests := map[string]int{
		"series_100":   100,
		"series_1000":  1000,
		"series_10000": 10000,
	}

	for name, seriesCount := range tests {
		b.Run(name, func(b *testing.B) {
			reader := benchmarkCollectorReader(b, seriesCount)

			engine, err := New()
			if err != nil {
				b.Fatalf("new engine: %v", err)
			}
			if err := engine.LoadYAML([]byte(benchTemplateYAML), 1); err != nil {
				b.Fatalf("load template: %v", err)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := engine.BuildPlan(reader); err != nil {
					b.Fatalf("build plan: %v", err)
				}
			}
		})
	}
}

func benchmarkCollectorReader(b *testing.B, seriesCount int) metrix.Reader {
	b.Helper()

	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	if !ok {
		b.Fatalf("collector store is not cycle-managed")
	}
	cc := managed.CycleController()

	meter := store.Write().SnapshotMeter("")
	g := meter.Gauge("bench_metric")

	cc.BeginCycle()
	for i := 0; i < seriesCount; i++ {
		g.Observe(metrix.SampleValue(i), meter.LabelSet(
			metrix.Label{Key: "id", Value: strconv.Itoa(i)},
		))
	}
	cc.CommitCycleSuccess()
	return store.Read(metrix.ReadRaw())
}
