// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
)

func TestBuildPlanAuthoredAlgorithmResolvesFromRuntimeKind(t *testing.T) {
	tests := map[string]struct {
		metricName string
		algorithm  string
		observe    func(metrix.CollectorStore)
		want       program.Algorithm
	}{
		"counter without conventional suffix defaults to incremental": {
			metricName: "jobs_processed",
			observe: func(store metrix.CollectorStore) {
				store.Write().SnapshotMeter("").Counter("jobs_processed").ObserveTotal(7)
			},
			want: program.AlgorithmIncremental,
		},
		"gauge with counter-like suffix defaults to absolute": {
			metricName: "job_execution_cpu_total",
			observe: func(store metrix.CollectorStore) {
				store.Write().SnapshotMeter("").Gauge("job_execution_cpu_total").Observe(0.5)
			},
			want: program.AlgorithmAbsolute,
		},
		"explicit absolute overrides counter kind": {
			metricName: "jobs_processed",
			algorithm:  "absolute",
			observe: func(store metrix.CollectorStore) {
				store.Write().SnapshotMeter("").Counter("jobs_processed").ObserveTotal(7)
			},
			want: program.AlgorithmAbsolute,
		},
		"explicit incremental overrides gauge kind": {
			metricName: "queue_depth",
			algorithm:  "incremental",
			observe: func(store metrix.CollectorStore) {
				store.Write().SnapshotMeter("").Gauge("queue_depth").Observe(3)
			},
			want: program.AlgorithmIncremental,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			algorithm := ""
			if tc.algorithm != "" {
				algorithm = "\n        algorithm: " + tc.algorithm
			}
			yaml := fmt.Sprintf(`
version: v1
groups:
  - family: Test
    metrics: [%s]
    charts:
      - title: Test metric
        context: test_metric
        units: value%s
        dimensions:
          - selector: %s
            name: value
`, tc.metricName, algorithm, tc.metricName)

			e, err := New()
			require.NoError(t, err)
			require.NoError(t, e.LoadYAML([]byte(yaml), 1))

			store := metrix.NewCollectorStore()
			cc := mustCycleController(t, store)
			cc.BeginCycle()
			tc.observe(store)
			require.NoError(t, cc.CommitCycleSuccess())

			plan, err := buildPlan(e, store.Read(metrix.ReadFlatten()))
			require.NoError(t, err)
			dim := findOnlyCreateDimension(t, plan)
			assert.Equal(t, tc.want, dim.Algorithm)
			wantPolicy := program.AlgorithmAuto
			if tc.algorithm != "" {
				wantPolicy = program.Algorithm(tc.algorithm)
			}
			assert.Equal(t, wantPolicy, dim.ChartMeta.Algorithm)
		})
	}
}

func TestBuildPlanAuthoredAlgorithmResolvesAfterRouteCacheLookup(t *testing.T) {
	const template = `
version: v1
groups:
  - family: Test
    metrics: [work]
    charts:
      - title: Work
        context: work
        units: value
        dimensions:
          - selector: work
            name: value
`

	e, err := New()
	require.NoError(t, err)
	require.NoError(t, e.LoadYAML([]byte(template), 1))

	gaugeStore := metrix.NewCollectorStore()
	gaugeCC := mustCycleController(t, gaugeStore)
	gaugeCC.BeginCycle()
	gaugeStore.Write().SnapshotMeter("").Gauge("work").Observe(3)
	require.NoError(t, gaugeCC.CommitCycleSuccess())

	first, err := e.PreparePlan(gaugeStore.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Equal(t, program.AlgorithmAbsolute, findOnlyCreateDimension(t, first.Plan()).Algorithm)
	first.Abort()

	counterStore := metrix.NewCollectorStore()
	counterCC := mustCycleController(t, counterStore)
	counterCC.BeginCycle()
	counterStore.Write().SnapshotMeter("").Counter("work").ObserveTotal(7)
	require.NoError(t, counterCC.CommitCycleSuccess())

	second, err := e.PreparePlan(counterStore.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	defer second.Abort()
	assert.Equal(t, program.AlgorithmIncremental, findOnlyCreateDimension(t, second.Plan()).Algorithm)
	assert.Equal(t, float64(1), engineRuntimeMetricValue(t, e, routeCacheHitsMetricName))
}

func TestBuildPlanAuthoredAlgorithmResolvesPerDimension(t *testing.T) {
	const template = `
version: v1
groups:
  - family: Test
    metrics: [queue_depth, work_completed]
    charts:
      - title: Mixed kinds
        context: mixed_kinds
        units: value
        dimensions:
          - selector: queue_depth
            name: depth
          - selector: work_completed
            name: completed
`

	e, err := New()
	require.NoError(t, err)
	require.NoError(t, e.LoadYAML([]byte(template), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	meter := store.Write().SnapshotMeter("")
	cc.BeginCycle()
	meter.Gauge("queue_depth").Observe(3)
	meter.Counter("work_completed").ObserveTotal(7)
	require.NoError(t, cc.CommitCycleSuccess())

	plan, err := buildPlan(e, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	got := make(map[string]program.Algorithm)
	for _, action := range plan.Actions {
		if dim, ok := action.(CreateDimensionAction); ok {
			got[dim.Name] = dim.Algorithm
			assert.Equal(t, program.AlgorithmAuto, dim.ChartMeta.Algorithm)
		}
	}
	assert.Equal(t, map[string]program.Algorithm{
		"depth":     program.AlgorithmAbsolute,
		"completed": program.AlgorithmIncremental,
	}, got)
}

func TestBuildPlanAuthoredExplicitAlgorithmControlsMixedKindAggregation(t *testing.T) {
	const template = `
version: v1
groups:
  - family: Test
    metrics: [queue_depth, work_completed]
    charts:
      - title: Aggregated mixed kinds
        context: aggregated_mixed_kinds
        units: value
        algorithm: absolute
        dimensions:
          - selector: queue_depth
            name_from_label: dimension
          - selector: work_completed
            name_from_label: dimension
`

	e, err := New()
	require.NoError(t, err)
	require.NoError(t, e.LoadYAML([]byte(template), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	meter := store.Write().SnapshotMeter("")
	dimension := meter.LabelSet(metrix.Label{Key: "dimension", Value: "value"})
	cc.BeginCycle()
	meter.Gauge("queue_depth").Observe(3, dimension)
	meter.Counter("work_completed").ObserveTotal(7, dimension)
	require.NoError(t, cc.CommitCycleSuccess())

	plan, err := buildPlan(e, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	dim := findOnlyCreateDimension(t, plan)
	assert.Equal(t, program.AlgorithmAbsolute, dim.Algorithm)
	update := findUpdateAction(plan)
	require.NotNil(t, update)
	require.Len(t, update.Values, 1)
	assert.Equal(t, int64(10), update.Values[0].Int64)
}

func TestBuildPlanAuthoredAlgorithmUsesFlattenedSeriesKind(t *testing.T) {
	const template = `
version: v1
groups:
  - family: Structured
    metrics:
      - svc.latency_seconds_bucket
      - svc.latency_seconds_count
      - svc.latency_seconds_sum
      - svc.request_seconds
      - svc.request_seconds_count
      - svc.request_seconds_sum
      - svc.status
      - svc.capacity_used
      - svc.operations_ok
    charts:
      - id: histogram_bucket
        title: Histogram bucket
        context: histogram_bucket
        units: observations
        dimensions:
          - selector: svc.latency_seconds_bucket
            name: value
      - id: histogram_count
        title: Histogram count
        context: histogram_count
        units: observations
        dimensions:
          - selector: svc.latency_seconds_count
            name: value
      - id: histogram_sum
        title: Histogram sum
        context: histogram_sum
        units: seconds
        dimensions:
          - selector: svc.latency_seconds_sum
            name: value
      - id: summary_quantile
        title: Summary quantile
        context: summary_quantile
        units: seconds
        dimensions:
          - selector: svc.request_seconds
            name: value
      - id: summary_count
        title: Summary count
        context: summary_count
        units: observations
        dimensions:
          - selector: svc.request_seconds_count
            name: value
      - id: summary_sum
        title: Summary sum
        context: summary_sum
        units: seconds
        dimensions:
          - selector: svc.request_seconds_sum
            name: value
      - id: state_set
        title: State set
        context: state_set
        units: state
        dimensions:
          - selector: svc.status
            name: value
      - id: measure_set_gauge
        title: Measure set gauge
        context: measure_set_gauge
        units: bytes
        dimensions:
          - selector: svc.capacity_used
            name: value
      - id: measure_set_counter
        title: Measure set counter
        context: measure_set_counter
        units: operations
        dimensions:
          - selector: svc.operations_ok
            name: value
`

	e, err := New()
	require.NoError(t, err)
	require.NoError(t, e.LoadYAML([]byte(template), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	meter := store.Write().SnapshotMeter("svc")
	histogram := meter.Histogram("latency_seconds", metrix.WithHistogramBounds(1))
	summary := meter.Summary("request_seconds", metrix.WithSummaryQuantiles(0.5))
	stateSet := meter.StateSet("status", metrix.WithStateSetStates("ready", "stopped"), metrix.WithStateSetMode(metrix.ModeEnum))
	capacity := meter.MeasureSetGauge("capacity", metrix.WithMeasureSetFields(metrix.MeasureFieldSpec{Name: "used"}))
	operations := meter.MeasureSetCounter("operations", metrix.WithMeasureSetFields(metrix.MeasureFieldSpec{Name: "ok"}))

	cc.BeginCycle()
	histogram.ObservePoint(metrix.HistogramPoint{
		Count:   2,
		Sum:     1.5,
		Buckets: []metrix.BucketPoint{{UpperBound: 1, CumulativeCount: 1}},
	})
	summary.ObservePoint(metrix.SummaryPoint{
		Count:     2,
		Sum:       1.2,
		Quantiles: []metrix.QuantilePoint{{Quantile: 0.5, Value: 0.4}},
	})
	stateSet.Enable("ready")
	capacity.ObservePoint(metrix.MeasureSetPoint{Values: []metrix.SampleValue{10}})
	operations.ObserveTotalPoint(metrix.MeasureSetPoint{Values: []metrix.SampleValue{7}})
	require.NoError(t, cc.CommitCycleSuccess())

	plan, err := buildPlan(e, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)

	got := make(map[string]program.Algorithm)
	for _, action := range plan.Actions {
		if dim, ok := action.(CreateDimensionAction); ok {
			got[dim.ChartID] = dim.Algorithm
		}
	}
	assert.Equal(t, map[string]program.Algorithm{
		"histogram_bucket":    program.AlgorithmIncremental,
		"histogram_count":     program.AlgorithmIncremental,
		"histogram_sum":       program.AlgorithmIncremental,
		"summary_quantile":    program.AlgorithmAbsolute,
		"summary_count":       program.AlgorithmIncremental,
		"summary_sum":         program.AlgorithmIncremental,
		"state_set":           program.AlgorithmAbsolute,
		"measure_set_gauge":   program.AlgorithmAbsolute,
		"measure_set_counter": program.AlgorithmIncremental,
	}, got)
}

func findOnlyCreateDimension(t *testing.T, plan Plan) CreateDimensionAction {
	t.Helper()
	var got []CreateDimensionAction
	for _, action := range plan.Actions {
		if dim, ok := action.(CreateDimensionAction); ok {
			got = append(got, dim)
		}
	}
	require.Len(t, got, 1)
	return got[0]
}

func TestResolveRuntimeAlgorithm(t *testing.T) {
	tests := map[string]struct {
		configured program.Algorithm
		kind       metrix.MetricKind
		want       program.Algorithm
	}{
		"auto counter":          {kind: metrix.MetricKindCounter, want: program.AlgorithmIncremental},
		"auto gauge":            {kind: metrix.MetricKindGauge, want: program.AlgorithmAbsolute},
		"auto unknown":          {kind: metrix.MetricKindUnknown, want: program.AlgorithmAbsolute},
		"absolute counter":      {configured: program.AlgorithmAbsolute, kind: metrix.MetricKindCounter, want: program.AlgorithmAbsolute},
		"incremental gauge":     {configured: program.AlgorithmIncremental, kind: metrix.MetricKindGauge, want: program.AlgorithmIncremental},
		"incremental unknown":   {configured: program.AlgorithmIncremental, kind: metrix.MetricKindUnknown, want: program.AlgorithmIncremental},
		"absolute unknown kind": {configured: program.AlgorithmAbsolute, kind: metrix.MetricKindUnknown, want: program.AlgorithmAbsolute},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, resolveRuntimeAlgorithm(tc.configured, tc.kind))
		})
	}
}
