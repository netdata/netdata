// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"strconv"
	"strings"
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

const benchAutogenTemplateYAML = `
version: v1
groups:
  - family: "Unused"
    metrics: ["unused_metric"]
    charts:
      - title: "Unused metric"
        context: "unused.metric"
        units: "units"
        dimensions:
          - selector: unused_metric
            name: unused
`

const benchAggregationTemplateYAML = `
version: v1
groups:
  - family: "Bench"
    metrics: ["bench_metric"]
    charts:
      - title: "Bench metric"
        context: "bench.metric"
        units: "units"
        aggregation: AGGREGATION
        instances:
          by_labels: [team]
        dimensions:
          - selector: bench_metric
            name: value
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

			benchmarkSteadySeriesPlan(b, engine, reader, 1, seriesCount)
		})
	}
}

func BenchmarkBuildPlanRouteDiagnostics(b *testing.B) {
	const seriesCount = 10_000
	reader := benchmarkCollectorReader(b, seriesCount)

	for _, diagnostics := range []bool{false, true} {
		name := "disabled"
		var (
			opts  []Option
			facts int
		)
		if diagnostics {
			name = "enabled"
			opts = append(opts, WithPlanRouteDiagnosticObserver(func(PlanRouteDiagnostic) {
				facts++
			}))
		}

		b.Run(name, func(b *testing.B) {
			engine, err := New(opts...)
			if err != nil {
				b.Fatalf("new engine: %v", err)
			}
			if err := engine.LoadYAML([]byte(benchTemplateYAML), 1); err != nil {
				b.Fatalf("load template: %v", err)
			}
			benchmarkSteadySeriesPlan(b, engine, reader, 1, seriesCount)
			if diagnostics && facts == 0 {
				b.Fatal("diagnostic observer received no facts")
			}
		})
	}
}

func BenchmarkBuildPlanAutogenUnmatchedBaseline(b *testing.B) {
	tests := map[string]int{
		"series_100":   100,
		"series_1000":  1000,
		"series_10000": 10000,
	}

	for name, seriesCount := range tests {
		b.Run(name, func(b *testing.B) {
			reader := benchmarkCollectorReader(b, seriesCount)

			engine, err := New(WithEnginePolicy(EnginePolicy{
				Autogen: &AutogenPolicy{
					Enabled: true,
				},
			}))
			if err != nil {
				b.Fatalf("new engine: %v", err)
			}
			if err := engine.LoadYAML([]byte(benchAutogenTemplateYAML), 1); err != nil {
				b.Fatalf("load template: %v", err)
			}

			benchmarkSteadySeriesPlan(b, engine, reader, seriesCount, seriesCount)
		})
	}
}

func BenchmarkBuildPlanAggregationFanIn(b *testing.B) {
	seriesCounts := []int{100, 1000, 10000}
	modes := []struct {
		name string
		warm bool
	}{
		{name: "warm", warm: true},
		{name: "cold"},
	}
	aggregations := []struct {
		name  string
		value string
	}{
		{name: "default_sum"},
		{name: "sum", value: "sum"},
		{name: "min", value: "min"},
		{name: "max", value: "max"},
		{name: "avg", value: "avg"},
	}
	fanIns := []struct {
		name       string
		chartCount func(seriesCount int) int
	}{
		{name: "full_fan_in", chartCount: func(_ int) int { return 1 }},
		{name: "mixed_10_per_chart", chartCount: func(seriesCount int) int {
			return max(1, seriesCount/10)
		}},
	}

	for _, mode := range modes {
		for _, aggregation := range aggregations {
			for _, fanIn := range fanIns {
				for _, seriesCount := range seriesCounts {
					name := fmt.Sprintf("%s/%s/%s/series_%d", mode.name, aggregation.name, fanIn.name, seriesCount)
					b.Run(name, func(b *testing.B) {
						b.StopTimer()
						reader := benchmarkAggregationReader(b, seriesCount, fanIn.chartCount(seriesCount))
						aggregationLine := ""
						if aggregation.value != "" {
							aggregationLine = "aggregation: " + aggregation.value
						}
						tmpl := strings.ReplaceAll(
							benchAggregationTemplateYAML,
							"aggregation: AGGREGATION",
							aggregationLine,
						)

						b.ReportAllocs()
						if mode.warm {
							engine := benchmarkAggregationEngine(b, tmpl)
							if _, err := buildPlan(engine, reader); err != nil {
								b.Fatalf("warm build plan: %v", err)
							}
							b.ResetTimer()
							b.StartTimer()
							for i := 0; i < b.N; i++ {
								if _, err := buildPlan(engine, reader); err != nil {
									b.Fatalf("build plan: %v", err)
								}
							}
							return
						}

						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							engine := benchmarkAggregationEngine(b, tmpl)
							b.StartTimer()
							if _, err := buildPlan(engine, reader); err != nil {
								b.Fatalf("build plan: %v", err)
							}
							b.StopTimer()
						}
					})
				}
			}
		}
	}
}

func benchmarkAggregationEngine(b *testing.B, tmpl string) *Engine {
	b.Helper()
	engine, err := New(WithRuntimePlannerMode())
	if err != nil {
		b.Fatalf("new engine: %v", err)
	}
	if err := engine.LoadYAML([]byte(tmpl), 1); err != nil {
		b.Fatalf("load template: %v", err)
	}
	return engine
}

func benchmarkCollectorReader(b *testing.B, seriesCount int) *benchmarkSequenceReader {
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
	for i := range seriesCount {
		g.Observe(metrix.SampleValue(i), meter.LabelSet(
			metrix.Label{
				Key:   "id",
				Value: strconv.Itoa(i),
			},
		))
	}
	if err := cc.CommitCycleSuccess(); err != nil {
		b.Fatalf("commit benchmark cycle: %v", err)
	}
	reader := store.Read(metrix.ReadRaw())
	return &benchmarkSequenceReader{
		Reader: reader,
		raw:    reader.(metrix.SeriesIdentityRawIterator),
		seq:    1,
	}
}

func benchmarkAggregationReader(b *testing.B, seriesCount, chartCount int) metrix.Reader {
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
	for i := range seriesCount {
		g.Observe(metrix.SampleValue(i), meter.LabelSet(
			metrix.Label{
				Key:   "team",
				Value: strconv.Itoa(i % chartCount),
			},
			metrix.Label{
				Key:   "api_key",
				Value: strconv.Itoa(i),
			},
		))
	}
	if err := cc.CommitCycleSuccess(); err != nil {
		b.Fatalf("commit benchmark cycle: %v", err)
	}
	return store.Read(metrix.ReadRaw())
}

// Each iteration represents a new successful collection. Repeating a committed
// sequence intentionally deduplicates updates and does not measure steady planning.
func benchmarkSteadySeriesPlan(b *testing.B, engine *Engine, reader *benchmarkSequenceReader, charts, series int) {
	b.Helper()
	reader.seq = 1
	if _, err := buildPlan(engine, reader); err != nil {
		b.Fatalf("warm plan: %v", err)
	}
	var plan Plan
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		reader.seq = uint64(i) + 2
		var err error
		plan, err = buildPlan(engine, reader)
		if err != nil {
			b.Fatalf("build plan: %v", err)
		}
	}
	b.StopTimer()
	updates, values := 0, 0
	var total int64
	for _, action := range plan.Actions {
		update, ok := action.(UpdateChartAction)
		if !ok {
			b.Fatalf("unexpected warmed action %T", action)
		}
		updates++
		for _, value := range update.Values {
			if value.IsEmpty {
				b.Fatal("unexpected empty value")
			}
			values++
			total += value.Int64
		}
	}
	if updates != charts || values != series || total != int64(series*(series-1)/2) {
		b.Fatalf(
			"updates=%d values=%d total=%d; want %d/%d/%d",
			updates,
			values,
			total,
			charts,
			series,
			series*(series-1)/2,
		)
	}
}
