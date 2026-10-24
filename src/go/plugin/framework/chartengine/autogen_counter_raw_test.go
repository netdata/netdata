// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
)

// counterRawTestChart is the observable shape of one created chart: its meta,
// its single dimension, and the value of the first update.
type counterRawTestChart struct {
	Context   string
	Title     string
	Units     string
	Family    string
	Type      program.ChartType
	Priority  int
	Dimension string
	Algorithm program.Algorithm
	Value     int64
}

func TestBuildPlanAutogenCounterRawCharts(t *testing.T) {
	const rateOnlyChart = "app_requests_total-code=200"
	tests := map[string]struct {
		template string
		observe  func(metrix.SnapshotMeter)
		want     map[string]counterRawTestChart
	}{
		"selected counter gets rate and raw charts": {
			template: counterRawTemplate("counter_raw_charts: [app_*_total]", ""),
			observe:  observeCounterRawRequests,
			want: map[string]counterRawTestChart{
				rateOnlyChart: {
					Context:   "ns.app_requests_total",
					Title:     "Requests served",
					Units:     "requests/s",
					Family:    "app_requests",
					Type:      program.ChartTypeLine,
					Priority:  70000,
					Dimension: "app_requests_total",
					Algorithm: program.AlgorithmIncremental,
					Value:     7,
				},
				"app_requests_total.raw-code=200": {
					Context:   "ns.app_requests_total.raw",
					Title:     "Requests served (raw)",
					Units:     "requests",
					Family:    "app_requests",
					Type:      program.ChartTypeLine,
					Priority:  70000,
					Dimension: "app_requests_total",
					Algorithm: program.AlgorithmAbsolute,
					Value:     7,
				},
			},
		},
		"raw units fall back to the metric name without metadata units": {
			template: counterRawTemplate("counter_raw_charts: [app_*]", ""),
			observe: func(m metrix.SnapshotMeter) {
				m.Counter("app_sent_bytes_total").ObserveTotal(42)
			},
			want: map[string]counterRawTestChart{
				"app_sent_bytes_total": {
					Context:   "ns.app_sent_bytes_total",
					Title:     `Metric "app_sent_bytes_total"`,
					Units:     "bytes/s",
					Family:    "app_sent",
					Type:      program.ChartTypeArea,
					Priority:  70000,
					Dimension: "app_sent_bytes_total",
					Algorithm: program.AlgorithmIncremental,
					Value:     42,
				},
				"app_sent_bytes_total.raw": {
					Context:   "ns.app_sent_bytes_total.raw",
					Title:     `Metric "app_sent_bytes_total" (raw)`,
					Units:     "bytes",
					Family:    "app_sent",
					Type:      program.ChartTypeArea,
					Priority:  70000,
					Dimension: "app_sent_bytes_total",
					Algorithm: program.AlgorithmAbsolute,
					Value:     42,
				},
			},
		},
		"option unset keeps the rate chart only": {
			template: counterRawTemplate("", ""),
			observe:  observeCounterRawRequests,
			want:     map[string]counterRawTestChart{rateOnlyChart: counterRawRequestsRateChart()},
		},
		"non-matching counter keeps the rate chart only": {
			template: counterRawTemplate("counter_raw_charts: [db_*]", ""),
			observe:  observeCounterRawRequests,
			want:     map[string]counterRawTestChart{rateOnlyChart: counterRawRequestsRateChart()},
		},
		"negated counter keeps the rate chart only": {
			template: counterRawTemplate("counter_raw_charts: ['!app_requests_total', 'app_*']", ""),
			observe:  observeCounterRawRequests,
			want:     map[string]counterRawTestChart{rateOnlyChart: counterRawRequestsRateChart()},
		},
		"gauge is never selected": {
			template: counterRawTemplate("counter_raw_charts: ['*']", ""),
			observe: func(m metrix.SnapshotMeter) {
				m.Gauge("app_inflight").Observe(3)
			},
			want: map[string]counterRawTestChart{
				"app_inflight": {
					Context:   "ns.app_inflight",
					Title:     `Metric "app_inflight"`,
					Units:     "inflight",
					Family:    "app_inflight",
					Type:      program.ChartTypeLine,
					Priority:  70000,
					Dimension: "app_inflight",
					Algorithm: program.AlgorithmAbsolute,
					Value:     3,
				},
			},
		},
		"rule-rejected counter gets no chart at all": {
			template: counterRawTemplate("counter_raw_charts: ['*']", `
    rules:
      - scope: 'app_*'
        selector:
          deny: [app_requests_total]`),
			observe: observeCounterRawRequests,
			want:    map[string]counterRawTestChart{},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			e, err := New()
			require.NoError(t, err)
			require.NoError(t, e.LoadYAML([]byte(tc.template), 1))

			store := metrix.NewCollectorStore()
			cc := mustCycleController(t, store)
			cc.BeginCycle()
			tc.observe(store.Write().SnapshotMeter(""))
			require.NoError(t, cc.CommitCycleSuccess())

			plan, err := buildPlan(e, store.Read(metrix.ReadFlatten()))
			require.NoError(t, err)
			assert.Equal(t, tc.want, counterRawTestCharts(t, plan))
		})
	}
}

func TestBuildPlanAutogenCounterRawRoutesSurviveRouteCache(t *testing.T) {
	e, err := New()
	require.NoError(t, err)
	require.NoError(t, e.LoadYAML([]byte(counterRawTemplate("counter_raw_charts: [app_*]", "")), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	meter := store.Write().SnapshotMeter("")
	for _, total := range []metrix.SampleValue{7, 9} {
		cc.BeginCycle()
		meter.Counter("app_requests_total").ObserveTotal(total)
		require.NoError(t, cc.CommitCycleSuccess())
		_, err = buildPlan(e, store.Read(metrix.ReadFlatten()))
		require.NoError(t, err)
	}

	cc.BeginCycle()
	meter.Counter("app_requests_total").ObserveTotal(12)
	require.NoError(t, cc.CommitCycleSuccess())
	plan, err := buildPlan(e, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)

	updates := make(map[string]int64)
	for _, action := range plan.Actions {
		switch action := action.(type) {
		case CreateChartAction:
			t.Fatalf("unexpected chart creation on a warm cycle: %s", action.ChartID)
		case UpdateChartAction:
			require.Len(t, action.Values, 1)
			updates[action.ChartID] = action.Values[0].Int64
		}
	}
	assert.Equal(t, map[string]int64{"app_requests_total": 12, "app_requests_total.raw": 12}, updates)
}

func TestBuildPlanAutogenCounterRawSkipsStructuredComponents(t *testing.T) {
	e, err := New()
	require.NoError(t, err)
	require.NoError(t, e.LoadYAML([]byte(counterRawTemplate("counter_raw_charts: ['*']", "")), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	meter := store.Write().SnapshotMeter("")
	cc.BeginCycle()
	meter.Histogram("app_latency_seconds", metrix.WithHistogramBounds(1)).ObservePoint(metrix.HistogramPoint{
		Count:   2,
		Sum:     1.5,
		Buckets: []metrix.BucketPoint{{UpperBound: 1, CumulativeCount: 1}},
	})
	meter.Summary("app_request_seconds", metrix.WithSummaryQuantiles(0.5)).ObservePoint(metrix.SummaryPoint{
		Count:     2,
		Sum:       1.2,
		Quantiles: []metrix.QuantilePoint{{Quantile: 0.5, Value: 0.4}},
	})
	meter.MeasureSetCounter("app_operations", metrix.WithMeasureSetFields(metrix.MeasureFieldSpec{Name: "ok"})).
		ObserveTotalPoint(metrix.MeasureSetPoint{Values: []metrix.SampleValue{5}})
	require.NoError(t, cc.CommitCycleSuccess())

	plan, err := buildPlan(e, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	var created int
	for _, action := range plan.Actions {
		if create, ok := action.(CreateChartAction); ok {
			created++
			assert.NotContains(t, create.ChartID, counterRawSuffix)
			assert.NotContains(t, create.Meta.Context, counterRawSuffix)
		}
	}
	assert.NotZero(t, created)
}

func TestBuildPlanAutogenCounterRawSkipsTemplateChartedCounter(t *testing.T) {
	const template = `
version: v1
context_namespace: ns
engine:
  autogen:
    enabled: true
    counter_raw_charts: ['*']
groups:
  - family: App
    metrics: [app_requests_total]
    charts:
      - title: Requests
        context: requests
        units: requests/s
        dimensions:
          - selector: app_requests_total
            name: requests
`
	e, err := New()
	require.NoError(t, err)
	require.NoError(t, e.LoadYAML([]byte(template), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	cc.BeginCycle()
	store.Write().SnapshotMeter("").Counter("app_requests_total").ObserveTotal(7)
	require.NoError(t, cc.CommitCycleSuccess())

	plan, err := buildPlan(e, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	charts := counterRawTestCharts(t, plan)
	require.Len(t, charts, 1)
	for chartID, chart := range charts {
		assert.NotContains(t, chartID, counterRawSuffix)
		assert.Equal(t, "ns.requests", chart.Context)
	}
}

func TestTemplateSetCounterRawChartsPolicy(t *testing.T) {
	newSet := func(t *testing.T, patterns []string) *TemplateSet {
		t.Helper()
		set, err := NewTemplateSet(TemplateSetSpec{
			Policy: EnginePolicy{Autogen: &AutogenPolicy{Enabled: true, CounterRawCharts: patterns}},
		})
		require.NoError(t, err)
		return set
	}

	assert.True(t, newSet(t, nil).SameGlobalPolicy(newSet(t, []string{})),
		"absent and empty pattern lists are the same policy")
	assert.False(t, newSet(t, nil).SameGlobalPolicy(newSet(t, []string{"app_*"})))
	assert.Equal(t, []string{"app_*"}, newSet(t, []string{"app_*"}).GlobalPolicy().Autogen.CounterRawCharts)

	_, err := NewTemplateSet(TemplateSetSpec{
		Policy: EnginePolicy{Autogen: &AutogenPolicy{Enabled: true, CounterRawCharts: []string{" app_*"}}},
	})
	require.ErrorContains(t, err, "autogen counter_raw_charts")

	_, err = PrepareTemplateSet(newSet(t, nil),
		WithEnginePolicy(EnginePolicy{Autogen: &AutogenPolicy{Enabled: true, CounterRawCharts: []string{"!app_*"}}}))
	require.ErrorContains(t, err, "autogen counter_raw_charts")
}

func TestTemplateSetPathsPlanCounterRawCharts(t *testing.T) {
	tests := map[string]struct {
		newSet func(t *testing.T) *TemplateSet
	}{
		"native set policy": {
			newSet: func(t *testing.T) *TemplateSet {
				set, err := NewTemplateSet(TemplateSetSpec{
					Policy: EnginePolicy{
						Autogen: &AutogenPolicy{Enabled: true, CounterRawCharts: []string{"app_*"}},
					},
					FallbackContextNamespace: "ns",
				})
				require.NoError(t, err)
				return set
			},
		},
		"YAML document policy": {
			newSet: func(t *testing.T) *TemplateSet {
				set, err := NewTemplateSetYAML([]byte(counterRawTemplate("counter_raw_charts: [app_*]", "")))
				require.NoError(t, err)
				return set
			},
		},
		"job policy override": {
			newSet: func(t *testing.T) *TemplateSet {
				set, err := NewTemplateSetYAML([]byte(counterRawTemplate("", "")))
				require.NoError(t, err)
				prepared, err := PrepareTemplateSet(set, WithEnginePolicy(EnginePolicy{
					Autogen: &AutogenPolicy{Enabled: true, CounterRawCharts: []string{"app_*"}},
				}))
				require.NoError(t, err)
				return prepared
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			e, err := New()
			require.NoError(t, err)
			store := metrix.NewCollectorStore()
			cc := mustCycleController(t, store)
			cc.BeginCycle()
			observeCounterRawRequests(store.Write().SnapshotMeter(""))
			require.NoError(t, cc.CommitCycleSuccess())

			attempt, err := e.PreparePlanWithOptions(store.Read(metrix.ReadFlatten()), PlanOptions{TemplateSet: tc.newSet(t)})
			require.NoError(t, err)
			charts := counterRawTestCharts(t, attempt.Plan())
			require.NoError(t, attempt.Commit())

			raw, ok := charts["app_requests_total.raw-code=200"]
			require.True(t, ok, "raw chart missing; got %v", charts)
			assert.Equal(t, "ns.app_requests_total.raw", raw.Context)
			assert.Equal(t, program.AlgorithmAbsolute, raw.Algorithm)
		})
	}
}

func TestBuildPlanAutogenCounterRawSkipsOverBudgetChartID(t *testing.T) {
	// With the "job" type ID, "job.app_requests_total" (22) fits a 24-byte budget
	// and "job.app_requests_total.raw" (26) does not: only the raw chart is skipped.
	e, err := New(WithEmitTypeIDBudgetPrefix("job"))
	require.NoError(t, err)
	require.NoError(t, e.LoadYAML([]byte(counterRawTemplate("counter_raw_charts: [app_*]", `
    max_type_id_len: 24`)), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	cc.BeginCycle()
	store.Write().SnapshotMeter("").Counter("app_requests_total").ObserveTotal(7)
	require.NoError(t, cc.CommitCycleSuccess())

	plan, err := buildPlan(e, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	charts := counterRawTestCharts(t, plan)
	assert.Contains(t, charts, "app_requests_total")
	assert.NotContains(t, charts, "app_requests_total.raw")
}

func TestBuildPlanAutogenCounterRawRejectsCollidingSeries(t *testing.T) {
	e, err := New()
	require.NoError(t, err)
	require.NoError(t, e.LoadYAML([]byte(counterRawTemplate("counter_raw_charts: [app_requests_total]", "")), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	cc.BeginCycle()
	store.Write().SnapshotMeter("").Counter("app_requests_total").ObserveTotal(7)
	// A series literally named "app_requests_total.raw" owns the same chart ID.
	store.Write().SnapshotMeter("app_requests_total").Counter("raw").ObserveTotal(3)
	require.NoError(t, cc.CommitCycleSuccess())

	plan, err := buildPlan(e, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)

	var dims []CreateDimensionAction
	for _, action := range plan.Actions {
		if dim, ok := action.(CreateDimensionAction); ok && dim.ChartID == "app_requests_total.raw" {
			dims = append(dims, dim)
		}
	}
	require.Len(t, dims, 1, "colliding routes must not merge into one chart")
}

func counterRawTemplate(counterRawCharts, extraAutogen string) string {
	return `
version: v1
context_namespace: ns
engine:
  autogen:
    enabled: true
    ` + counterRawCharts + extraAutogen + `
groups:
  - family: Unused
    metrics: [unused_metric]
    charts:
      - title: Unused
        context: unused
        units: value
        dimensions:
          - selector: unused_metric
            name: value
`
}

func observeCounterRawRequests(m metrix.SnapshotMeter) {
	m.Counter("app_requests_total", metrix.WithDescription("Requests served"), metrix.WithUnit("requests")).
		ObserveTotal(7, m.LabelSet(metrix.Label{Key: "code", Value: "200"}))
}

func counterRawRequestsRateChart() counterRawTestChart {
	return counterRawTestChart{
		Context:   "ns.app_requests_total",
		Title:     "Requests served",
		Units:     "requests/s",
		Family:    "app_requests",
		Type:      program.ChartTypeLine,
		Priority:  70000,
		Dimension: "app_requests_total",
		Algorithm: program.AlgorithmIncremental,
		Value:     7,
	}
}

func counterRawTestCharts(t *testing.T, plan Plan) map[string]counterRawTestChart {
	t.Helper()
	charts := make(map[string]counterRawTestChart)
	for _, action := range plan.Actions {
		if create, ok := action.(CreateChartAction); ok {
			chart := charts[create.ChartID]
			chart.Context = create.Meta.Context
			chart.Title = create.Meta.Title
			chart.Units = create.Meta.Units
			chart.Family = create.Meta.Family
			chart.Type = create.Meta.Type
			chart.Priority = create.Meta.Priority
			charts[create.ChartID] = chart
		}
	}
	for _, action := range plan.Actions {
		switch action := action.(type) {
		case CreateDimensionAction:
			chart, ok := charts[action.ChartID]
			require.True(t, ok, "dimension for an uncreated chart %q", action.ChartID)
			chart.Dimension = action.Name
			chart.Algorithm = action.Algorithm
			charts[action.ChartID] = chart
		case UpdateChartAction:
			chart, ok := charts[action.ChartID]
			require.True(t, ok, "update for an uncreated chart %q", action.ChartID)
			require.Len(t, action.Values, 1)
			chart.Value = action.Values[0].Int64
			charts[action.ChartID] = chart
		}
	}
	return charts
}
