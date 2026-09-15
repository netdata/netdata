// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextCardinalitySharedContextAndHistogram(t *testing.T) {
	const template = `version: v1
groups:
  - family: service
    metrics: [a, b, latency_bucket, latency_count, latency_sum]
    charts:
      - id: a
        title: A
        context: combined
        units: units
        instances: {by_labels: [model]}
        dimensions: [{selector: a, name: value}]
      - id: b
        title: B
        context: combined
        units: units
        instances: {by_labels: [model]}
        dimensions: [{selector: b, name: value}]
      - title: Total
        context: total
        units: units
        dimensions: [{selector: a, name: value}]
      - id: buckets
        title: Latency
        context: latency
        units: seconds
        dimensions: [{selector: latency_bucket, name_from_label: le}]
      - id: observations
        title: Observations
        context: latency
        units: requests
        dimensions: [{selector: latency_count, name: count}]
      - id: duration
        title: Duration
        context: latency
        units: seconds
        dimensions: [{selector: latency_sum, name: sum}]
`
	store := metrix.NewCollectorStore()
	cycle := mustCycleController(t, store)
	cycle.BeginCycle()
	for i := 1; i <= 2; i++ {
		meter := store.Write().SnapshotMeter("").WithLabels(metrix.Label{Key: "model", Value: fmt.Sprint(i)})
		meter.Gauge("a", metrix.WithFloat(true)).Observe(float64(i * 10))
		meter.Gauge("b", metrix.WithFloat(true)).Observe(float64(i * 20))
		meter.Histogram("latency", metrix.WithHistogramBounds(1, 2), metrix.WithFloat(true)).ObservePoint(metrix.HistogramPoint{
			Count: float64(i * 3), Sum: float64(i * 4),
			Buckets: []metrix.BucketPoint{
				{UpperBound: 1, CumulativeCount: float64(i)},
				{UpperBound: 2, CumulativeCount: float64(i * 2)},
			},
		})
	}
	require.NoError(t, cycle.CommitCycleSuccess())
	for _, tc := range []struct {
		name                string
		total, perMetric    int
		combined, histogram bool
	}{
		{"context_counts_both_charts", 3, 0, false, false},
		{"metric_counts_are_independent", 0, 3, true, false},
		{"histogram_components_share_family", 0, 4, true, false},
		{"exact_boundaries", 5, 5, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine, err := New(WithEnginePolicy(EnginePolicy{MaxTimeSeries: tc.total, MaxTimeSeriesPerMetric: tc.perMetric}))
			require.NoError(t, err)
			require.NoError(t, engine.LoadYAML([]byte(template), 1))
			plan, err := buildPlan(engine, store.Read(metrix.ReadRaw(), metrix.ReadFlatten()))
			require.NoError(t, err)
			contexts := make(map[string]string)
			values := make(map[string][]float64)
			for _, action := range plan.Actions {
				switch action := action.(type) {
				case CreateChartAction:
					contexts[action.ChartID] = action.Meta.Context
				case UpdateChartAction:
					for _, value := range action.Values {
						values[contexts[action.ChartID]] = append(values[contexts[action.ChartID]], value.Float64)
					}
				}
			}
			assert.Equal(t, []float64{30}, values["total"])
			if tc.combined {
				assert.ElementsMatch(t, []float64{10, 20, 20, 40}, values["combined"])
			} else {
				assert.NotContains(t, values, "combined")
			}
			if tc.histogram {
				assert.ElementsMatch(t, []float64{3, 3, 3, 9, 12}, values["latency"])
			} else {
				assert.NotContains(t, values, "latency")
			}
		})
	}
}

func BenchmarkRollupCardinality(b *testing.B) {
	reader := benchmarkAggregationReader(b, 10000, 2)
	for _, enabled := range []bool{false, true} {
		b.Run(fmt.Sprintf("limits_%t", enabled), func(b *testing.B) {
			policy := EnginePolicy{}
			if enabled {
				policy.MaxTimeSeries, policy.MaxTimeSeriesPerMetric = 2000, 200
			}
			engine, err := New(WithRuntimeStore(nil), WithRuntimePlannerMode(), WithEnginePolicy(policy))
			if err != nil {
				b.Fatal(err)
			}
			if err := engine.LoadYAML([]byte(strings.ReplaceAll(benchAggregationTemplateYAML, "AGGREGATION", "sum")), 1); err != nil {
				b.Fatal(err)
			}
			if _, err := buildPlan(engine, reader); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			var plan Plan
			for b.Loop() {
				plan, err = buildPlan(engine, reader)
				if err != nil {
					b.Fatal(err)
				}
			}
			values := 0
			for _, action := range plan.Actions {
				if update, ok := action.(UpdateChartAction); ok {
					values += len(update.Values)
				}
			}
			if values != 2 {
				b.Fatalf("expected two rollup values, got %d", values)
			}
		})
	}
}
