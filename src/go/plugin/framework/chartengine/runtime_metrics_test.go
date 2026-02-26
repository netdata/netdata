// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
)

func TestEngineRuntimeObservabilityScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"build plan emits chartengine internal runtime metrics": {
			run: func(t *testing.T) {
				e, err := New()
				require.NoError(t, err)
				require.NoError(t, e.LoadYAML([]byte(runtimeObservabilityTemplateYAML()), 1))

				store := metrix.NewCollectorStore()
				cc := mustCycleController(t, store)
				c := store.Write().SnapshotMeter("mysql").Counter("queries_total")

				cc.BeginCycle()
				c.ObserveTotal(10)
				cc.CommitCycleSuccess()
				_, err = e.BuildPlan(store.Read(metrix.ReadFlatten()))
				require.NoError(t, err)

				cc.BeginCycle()
				c.ObserveTotal(20)
				cc.CommitCycleSuccess()
				_, err = e.BuildPlan(store.Read(metrix.ReadFlatten()))
				require.NoError(t, err)

				rs := e.RuntimeStore()
				require.NotNil(t, rs)
				r := rs.Read(metrix.ReadRaw())

				assertMetricValueAtLeast(t, r, "netdata.go.plugin.chartengine.build_success_total", nil, 2)
				assertSummaryCountAtLeast(t, r, "netdata.go.plugin.chartengine.build_duration_seconds", nil, 2)
				assertSummaryCountAtLeast(t, r, "netdata.go.plugin.chartengine.build_phase_duration_seconds", metrix.Labels{"phase": "scan"}, 2)
				assertMetricValueAtLeast(t, r, "netdata.go.plugin.chartengine.route_cache_misses_total", nil, 1)
				assertMetricValueAtLeast(t, r, "netdata.go.plugin.chartengine.route_cache_hits_total", nil, 1)
				assertMetricValueAtLeast(t, r, "netdata.go.plugin.chartengine.route_cache_entries", nil, 1)
				assertMetricValueAtLeast(t, r, "netdata.go.plugin.chartengine.route_cache_retained_total", nil, 1)
				assertMetricValueAtLeast(t, r, "netdata.go.plugin.chartengine.series_scanned_total", nil, 2)
				assertMetricValueAtLeast(t, r, "netdata.go.plugin.chartengine.planner_actions_total", metrix.Labels{"kind": "update_chart"}, 2)
				assertMetricValueAtLeast(t, r, "netdata.go.plugin.chartengine.plan_chart_instances", nil, 1)
				assertMetricMeta(
					t,
					r,
					"netdata.go.plugin.chartengine.build_success_total",
					metrix.MetricMeta{
						Description: "Successful BuildPlan calls",
						ChartFamily: "ChartEngine/Build",
						Unit:        "builds",
					},
				)
				assertMetricMeta(
					t,
					r,
					"netdata.go.plugin.chartengine.planner_actions_total",
					metrix.MetricMeta{
						Description: "Planner actions by kind",
						ChartFamily: "ChartEngine/Actions",
						Unit:        "actions",
					},
				)
			},
		},
		"plan-size gauges keep last successful values on skipped build": {
			run: func(t *testing.T) {
				e, err := New()
				require.NoError(t, err)
				require.NoError(t, e.LoadYAML([]byte(runtimeObservabilityTemplateYAML()), 1))

				store := metrix.NewCollectorStore()
				cc := mustCycleController(t, store)
				c := store.Write().SnapshotMeter("mysql").Counter("queries_total")

				cc.BeginCycle()
				c.ObserveTotal(10)
				cc.CommitCycleSuccess()
				_, err = e.BuildPlan(store.Read(metrix.ReadFlatten()))
				require.NoError(t, err)

				before := e.RuntimeStore().Read(metrix.ReadRaw())
				beforeCharts, ok := before.Value("netdata.go.plugin.chartengine.plan_chart_instances", nil)
				require.True(t, ok)
				require.GreaterOrEqual(t, beforeCharts, float64(1))

				cc.BeginCycle()
				cc.AbortCycle()
				_, err = e.BuildPlan(store.Read(metrix.ReadFlatten()))
				require.NoError(t, err)

				after := e.RuntimeStore().Read(metrix.ReadRaw())
				assertMetricValueAtLeast(t, after, "netdata.go.plugin.chartengine.build_skipped_failed_collect_total", nil, 1)
				assertMetricValueAtLeast(t, after, "netdata.go.plugin.chartengine.build_success_total", nil, 1)

				afterCharts, ok := after.Value("netdata.go.plugin.chartengine.plan_chart_instances", nil)
				require.True(t, ok)
				assert.Equal(t, beforeCharts, afterCharts)
			},
		},
		"selector-filtered series are counted by reason": {
			run: func(t *testing.T) {
				selectorExpr := metrixselector.Expr{
					Allow: []string{`svc.errors_total`},
				}
				e, err := New(WithEnginePolicy(EnginePolicy{Selector: &selectorExpr}))
				require.NoError(t, err)
				require.NoError(t, e.LoadYAML([]byte(runtimeObservabilityTemplateYAML()), 1))

				store := metrix.NewCollectorStore()
				cc := mustCycleController(t, store)
				c := store.Write().SnapshotMeter("mysql").Counter("queries_total")

				cc.BeginCycle()
				c.ObserveTotal(10)
				cc.CommitCycleSuccess()
				_, err = e.BuildPlan(store.Read(metrix.ReadFlatten()))
				require.NoError(t, err)

				r := e.RuntimeStore().Read(metrix.ReadRaw())
				assertMetricValueAtLeast(t, r, "netdata.go.plugin.chartengine.series_filtered_total", metrix.Labels{"reason": "by_selector"}, 1)
			},
		},
		"expiry removals are counted by scope and reason": {
			run: func(t *testing.T) {
				e, err := New()
				require.NoError(t, err)
				require.NoError(t, e.LoadYAML([]byte(runtimeExpiryTemplateYAML()), 1))

				store := metrix.NewCollectorStore()
				cc := mustCycleController(t, store)
				sm := store.Write().SnapshotMeter("svc")
				total := sm.Gauge("total")
				modeMetric := sm.Gauge("mode_metric")
				modeOK := sm.LabelSet(metrix.Label{Key: "mode", Value: "ok"})

				cc.BeginCycle()
				total.Observe(100)
				modeMetric.Observe(1, modeOK)
				cc.CommitCycleSuccess()
				_, err = e.BuildPlan(store.Read(metrix.ReadFlatten()))
				require.NoError(t, err)

				cc.BeginCycle()
				total.Observe(101)
				cc.CommitCycleSuccess()
				_, err = e.BuildPlan(store.Read(metrix.ReadFlatten()))
				require.NoError(t, err)

				r := e.RuntimeStore().Read(metrix.ReadRaw())
				assertMetricValueAtLeast(
					t,
					r,
					"netdata.go.plugin.chartengine.lifecycle_removed_total",
					metrix.Labels{"scope": "dimension", "reason": "expiry"},
					1,
				)
			},
		},
		"component runtime store metrics can be planned into charts": {
			run: func(t *testing.T) {
				producer, err := New()
				require.NoError(t, err)
				rs := producer.RuntimeStore()
				require.NotNil(t, rs)

				component := rs.Write().StatefulMeter("component").Counter("jobs_total")
				component.Add(7)

				observer, err := New(WithRuntimeStore(nil))
				require.NoError(t, err)
				require.NoError(t, observer.LoadYAML([]byte(runtimeComponentTemplateYAML()), 1))

				plan, err := observer.BuildPlan(rs.Read(metrix.ReadFlatten()))
				require.NoError(t, err)
				assert.Equal(t, []ActionKind{ActionCreateChart, ActionCreateDimension, ActionUpdateChart}, actionKinds(plan.Actions))

				update := findUpdateAction(plan)
				require.NotNil(t, update)
				require.Equal(t, "netdata_go_plugin_component_component_jobs", update.ChartID)
				require.Len(t, update.Values, 1)
				assert.Equal(t, "total", update.Values[0].Name)
				assert.False(t, update.Values[0].IsFloat)
				assert.Equal(t, int64(7), update.Values[0].Int64)
			},
		},
		"autogen planning uses runtime metric metadata": {
			run: func(t *testing.T) {
				producer, err := New()
				require.NoError(t, err)
				require.NoError(t, producer.LoadYAML([]byte(runtimeObservabilityTemplateYAML()), 1))

				store := metrix.NewCollectorStore()
				cc := mustCycleController(t, store)
				c := store.Write().SnapshotMeter("mysql").Counter("queries_total")
				cc.BeginCycle()
				c.ObserveTotal(10)
				cc.CommitCycleSuccess()
				_, err = producer.BuildPlan(store.Read(metrix.ReadFlatten()))
				require.NoError(t, err)

				observer, err := New(
					WithRuntimeStore(nil),
					WithAutogenPolicy(AutogenPolicy{Enabled: true}),
					WithSeriesSelectionAllVisible(),
				)
				require.NoError(t, err)
				require.NoError(t, observer.LoadYAML([]byte(runtimeDummyTemplateYAML()), 1))

				plan, err := observer.BuildPlan(producer.RuntimeStore().Read(metrix.ReadFlatten()))
				require.NoError(t, err)
				create := findCreateChartByTitle(plan.Actions, "Successful BuildPlan calls")
				require.NotNil(t, create)
				assert.Equal(t, "Successful BuildPlan calls", create.Meta.Title)
				assert.Equal(t, "ChartEngine/Build", create.Meta.Family)
				assert.Equal(t, "builds/s", create.Meta.Units)

				duration := findCreateChartByID(plan.Actions, "netdata.go.plugin.chartengine.build_duration_seconds")
				require.NotNil(t, duration)
				assert.Equal(t, "BuildPlan duration in seconds", duration.Meta.Title)
				assert.Equal(t, "ChartEngine/Build", duration.Meta.Family)
				assert.Equal(t, "seconds", duration.Meta.Units)

				durationSum := findCreateChartByID(plan.Actions, "netdata.go.plugin.chartengine.build_duration_seconds_sum")
				require.NotNil(t, durationSum)
				assert.Equal(t, "BuildPlan duration in seconds", durationSum.Meta.Title)
				assert.Equal(t, "ChartEngine/Build", durationSum.Meta.Family)
				assert.Equal(t, "seconds", durationSum.Meta.Units)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

func assertMetricValueAtLeast(t *testing.T, reader metrix.Reader, name string, labels metrix.Labels, min float64) {
	t.Helper()
	value, ok := reader.Value(name, labels)
	require.Truef(t, ok, "expected metric %s with labels %v", name, labels)
	assert.GreaterOrEqualf(t, value, min, "unexpected metric value for %s labels %v", name, labels)
}

func assertSummaryCountAtLeast(t *testing.T, reader metrix.Reader, name string, labels metrix.Labels, min float64) {
	t.Helper()
	point, ok := reader.Summary(name, labels)
	require.Truef(t, ok, "expected summary %s with labels %v", name, labels)
	assert.GreaterOrEqualf(t, point.Count, min, "unexpected summary count for %s labels %v", name, labels)
}

func assertMetricMeta(t *testing.T, reader metrix.Reader, name string, want metrix.MetricMeta) {
	t.Helper()
	got, ok := reader.MetricMeta(name)
	require.Truef(t, ok, "expected metric metadata for %s", name)
	assert.Equal(t, want, got)
}

func findCreateChartByID(actions []EngineAction, chartID string) *CreateChartAction {
	for _, action := range actions {
		create, ok := action.(CreateChartAction)
		if !ok || create.ChartID != chartID {
			continue
		}
		c := create
		return &c
	}
	return nil
}

func findCreateChartByTitle(actions []EngineAction, title string) *CreateChartAction {
	for _, action := range actions {
		create, ok := action.(CreateChartAction)
		if !ok || create.Meta.Title != title {
			continue
		}
		c := create
		return &c
	}
	return nil
}

func runtimeComponentTemplateYAML() string {
	return `
version: v1
groups:
  - family: Runtime
    metrics:
      - component.jobs_total
    charts:
      - title: Component jobs
        context: netdata.go.plugin.component.component_jobs
        units: jobs/s
        dimensions:
          - selector: component.jobs_total
            name: total
`
}

func runtimeObservabilityTemplateYAML() string {
	return `
version: v1
groups:
  - family: Database
    metrics:
      - mysql.queries_total
    charts:
      - title: Queries
        context: queries_total
        units: queries/s
        dimensions:
          - selector: mysql.queries_total
            name: total
`
}

func runtimeExpiryTemplateYAML() string {
	return `
version: v1
groups:
  - family: Service
    metrics:
      - svc.total
      - svc.mode_metric
    charts:
      - title: Service status
        context: service_status
        units: state
        lifecycle:
          dimensions:
            expire_after_cycles: 1
        dimensions:
          - selector: svc.total
            name: total
          - selector: svc.mode_metric
            name_from_label: mode
`
}

func runtimeDummyTemplateYAML() string {
	return `
version: v1
groups:
  - family: Runtime
    metrics:
      - __runtime_dummy_metric
    charts:
      - id: runtime_dummy
        title: Runtime dummy
        context: netdata.go.plugin.runtime_dummy
        units: "1"
        dimensions:
          - selector: __runtime_dummy_metric
            name: value
`
}
