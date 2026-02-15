// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
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
				_, err = e.BuildPlan(store.Read())
				require.NoError(t, err)

				cc.BeginCycle()
				c.ObserveTotal(20)
				cc.CommitCycleSuccess()
				_, err = e.BuildPlan(store.Read())
				require.NoError(t, err)

				rs := e.RuntimeStore()
				require.NotNil(t, rs)
				r := rs.ReadRaw()

				assertMetricValueAtLeast(t, r, "netdata.go.plugin.chartengine.build_calls_total", nil, 2)
				assertMetricValueAtLeast(t, r, "netdata.go.plugin.chartengine.route_cache_misses_total", nil, 1)
				assertMetricValueAtLeast(t, r, "netdata.go.plugin.chartengine.route_cache_hits_total", nil, 1)
				assertMetricValueAtLeast(t, r, "netdata.go.plugin.chartengine.series_scanned_total", nil, 2)
				assertMetricValueAtLeast(t, r, "netdata.go.plugin.chartengine.actions_total", metrix.Labels{"kind": "update_chart"}, 2)
				assertMetricValueAtLeast(t, r, "netdata.go.plugin.chartengine.plan_chart_instances", nil, 1)
				assertSummaryCountAtLeast(t, r, "netdata.go.plugin.chartengine.build_duration_ms", nil, 2)
				assertMetricMeta(
					t,
					r,
					"netdata.go.plugin.chartengine.build_calls_total",
					metrix.MetricMeta{
						Description: "Build plan calls",
						ChartFamily: "Planner",
						Unit:        "calls",
					},
				)
				assertMetricMeta(
					t,
					r,
					"netdata.go.plugin.chartengine.actions_total",
					metrix.MetricMeta{
						Description: "Planner actions by kind",
						ChartFamily: "Actions",
						Unit:        "actions",
					},
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

				plan, err := observer.BuildPlan(rs.Read())
				require.NoError(t, err)
				assert.Equal(t, []ActionKind{ActionCreateChart, ActionCreateDimension, ActionUpdateChart}, actionKinds(plan.Actions))

				update := findUpdateAction(plan)
				require.NotNil(t, update)
				require.Equal(t, "netdata_go_plugin_component_component_jobs", update.ChartID)
				require.Len(t, update.Values, 1)
				assert.Equal(t, "total", update.Values[0].Name)
				assert.True(t, update.Values[0].IsFloat)
				assert.Equal(t, float64(7), update.Values[0].Float64)
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
				_, err = producer.BuildPlan(store.Read())
				require.NoError(t, err)

				observer, err := New(
					WithRuntimeStore(nil),
					WithAutogenPolicy(AutogenPolicy{Enabled: true}),
					WithSeriesSelectionAllVisible(),
				)
				require.NoError(t, err)
				require.NoError(t, observer.LoadYAML([]byte(runtimeDummyTemplateYAML()), 1))

				plan, err := observer.BuildPlan(producer.RuntimeStore().Read())
				require.NoError(t, err)
				create := findCreateChartByTitle(plan.Actions, "Build plan calls")
				require.NotNil(t, create)
				assert.Equal(t, "Build plan calls", create.Meta.Title)
				assert.Equal(t, "Planner", create.Meta.Family)
				assert.Equal(t, "calls/s", create.Meta.Units)
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
