// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

// A rotating subset is replaced every cycle. Eight generations and three-cycle
// retention keep cardinality bounded while ensuring replaced identities expire
// before reuse. Writes/commit/indexing are excluded from the planner timer.
func BenchmarkAutogenMixedChurn(b *testing.B) {
	const population = 1000
	for _, percent := range []int{0, 10, 50, 100} {
		b.Run(fmt.Sprintf("replace_%d_percent", percent), func(b *testing.B) {
			store := metrix.NewCollectorStore(metrix.WithExpireAfterSuccessCycles(3))
			managed, _ := metrix.AsCycleManagedStore(store)
			cc := managed.CycleController()
			meter := store.Write().SnapshotMeter("")
			gauge := meter.Gauge("work")
			var labels [8][]metrix.LabelSet
			for generation := range labels {
				for i := range population {
					suffix := 0
					if i < population*percent/100 {
						suffix = generation
					}
					labels[generation] = append(labels[generation], meter.LabelSet(metrix.Label{
						Key:   "id",
						Value: fmt.Sprintf("%d_%d", i, suffix),
					}))
				}
			}
			engine := newAutogenCacheTestEngine(b, WithEnginePolicy(EnginePolicy{
				Autogen: &AutogenPolicy{
					Enabled:                  true,
					ExpireAfterSuccessCycles: 3,
				},
			}))
			cycle := func(i int) metrix.Reader {
				cc.BeginCycle()
				for n, l := range labels[i%len(labels)] {
					gauge.Observe(float64(n), l)
				}
				if err := cc.CommitCycleSuccess(); err != nil {
					b.Fatal(err)
				}
				reader := store.Read(metrix.ReadRaw(), metrix.ReadFlatten())
				reader.ForEachSeriesIdentity(
					func(metrix.SeriesIdentity, metrix.SeriesMeta, string, metrix.LabelView, metrix.SampleValue) {},
				)
				return reader
			}
			for i := range 16 {
				if _, err := buildPlan(engine, cycle(i)); err != nil {
					b.Fatal(err)
				}
			}
			var last Plan
			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				b.StopTimer()
				reader := cycle(i + 16)
				b.StartTimer()
				var err error
				last, err = buildPlan(engine, reader)
				if err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			updates, creates, removes := 0, 0, 0
			for _, a := range last.Actions {
				switch a.(type) {
				case UpdateChartAction:
					updates++
				case CreateChartAction:
					creates++
				case RemoveChartAction:
					removes++
				}
			}
			if updates != population || creates != population*percent/100 || removes != creates {
				b.Fatalf("updates/create/remove=%d/%d/%d", updates, creates, removes)
			}
			if retained := len(engine.state.materialized.charts); retained > population*3 {
				b.Fatalf("retained charts=%d exceed retention envelope", retained)
			}
		})
	}
}

// Construct lifecycle state through real collection and planning, then isolate
// expiry cost. Each destructive sample gets an untimed clone of that state.
func BenchmarkCollectExpiryRemovals(b *testing.B) {
	for _, shape := range []struct{ charts, dims int }{{1, 10000}, {100, 100}, {1000, 10}} {
		for _, mode := range []string{"none", "half_dimensions", "all_dimensions", "all_charts"} {
			b.Run(fmt.Sprintf("%s/charts_%d_dims_%d", mode, shape.charts, shape.dims), func(b *testing.B) {
				engine, err := New(WithRuntimeStore(nil))
				if err != nil {
					b.Fatal(err)
				}
				if err = engine.LoadYAML([]byte(`version: v1
groups:
 - family: Expiry
   metrics: [work]
   charts:
    - id: work
      title: Work
      context: work
      units: units
      instances:
       by_labels: [chart]
      lifecycle:
       expire_after_cycles: 3
       dimensions:
        expire_after_cycles: 2
      dimensions:
       - selector: work
         name_from_label: dim
`), 1); err != nil {
					b.Fatal(err)
				}
				store := metrix.NewCollectorStore()
				managed, _ := metrix.AsCycleManagedStore(store)
				cc := managed.CycleController()
				m := store.Write().SnapshotMeter("")
				g := m.Gauge("work")
				write := func(half bool) {
					cc.BeginCycle()
					for c := range shape.charts {
						for d := range shape.dims {
							if half && d%2 != 0 {
								continue
							}
							g.Observe(float64(d), m.LabelSet(metrix.Label{
								Key:   "chart",
								Value: fmt.Sprint(c),
							}, metrix.Label{
								Key:   "dim",
								Value: fmt.Sprint(d),
							}))
						}
					}
					if err := cc.CommitCycleSuccess(); err != nil {
						b.Fatal(err)
					}
					if _, err := buildPlan(engine, store.Read(metrix.ReadRaw(), metrix.ReadFlatten())); err != nil {
						b.Fatal(err)
					}
				}
				write(false)
				current := uint64(1)
				wantDims, wantCharts := 0, 0
				switch mode {
				case "half_dimensions":
					write(true)
					current = 3
					wantDims = shape.charts * (shape.dims / 2)
				case "all_dimensions":
					current = 3
					wantDims = shape.charts * shape.dims
				case "all_charts":
					current = 4
					wantCharts = shape.charts
				}
				fixture := engine.state.materialized
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					b.StopTimer()
					state := fixture.clone()
					b.StartTimer()
					dims, charts := collectExpiryRemovals(current, &state)
					if len(dims) != wantDims || len(charts) != wantCharts {
						b.Fatalf("removed dims/charts=%d/%d, want %d/%d", len(dims), len(charts), wantDims, wantCharts)
					}
				}
			})
		}
	}
}
