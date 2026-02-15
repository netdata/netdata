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
