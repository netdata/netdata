// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEngineLoadScenarios(t *testing.T) {
	tests := map[string]struct {
		initialYAML string
		initialRev  uint64
		reloadYAML  string
		reloadRev   uint64
		reloadErr   bool
		assert      func(t *testing.T, e *Engine)
	}{
		"load from yaml populates program and revision": {
			initialYAML: validTemplateYAML(),
			initialRev:  100,
			assert: func(t *testing.T, e *Engine) {
				t.Helper()
				require.True(t, e.ready())
				p := e.program()
				require.NotNil(t, p)
				assert.Equal(t, uint64(100), p.Revision())
				assert.Equal(t, "v1", p.Version())
				assert.Equal(t, []string{"mysql_queries_total"}, p.MetricNames())
			},
		},
		"failed reload keeps previous compiled program": {
			initialYAML: validTemplateYAML(),
			initialRev:  200,
			reloadYAML: `
version: v1
groups:
  - family: Database
    charts:
      - title: Broken
        context: broken
        units: "1"
        dimensions:
          - selector: mysql_queries_total
`,
			reloadRev: 201,
			reloadErr: true,
			assert: func(t *testing.T, e *Engine) {
				t.Helper()
				p := e.program()
				require.NotNil(t, p)
				assert.Equal(t, uint64(200), p.Revision())
				assert.Equal(t, []string{"mysql_queries_total"}, p.MetricNames())
			},
		},
		"invalid selector syntax on reload is rejected and keeps previous program": {
			initialYAML: validTemplateYAML(),
			initialRev:  300,
			reloadYAML: `
version: v1
groups:
  - family: Database
    metrics:
      - mysql_queries_total
    charts:
      - title: Broken selector
        context: broken_selector
        units: queries/s
        dimensions:
          - selector: mysql_queries_total{method="GET",}
            name: total
`,
			reloadRev: 301,
			reloadErr: true,
			assert: func(t *testing.T, e *Engine) {
				t.Helper()
				p := e.program()
				require.NotNil(t, p)
				assert.Equal(t, uint64(300), p.Revision())
				assert.Equal(t, []string{"mysql_queries_total"}, p.MetricNames())
			},
		},
		"invalid engine selector on reload is rejected and keeps previous program": {
			initialYAML: validTemplateYAML(),
			initialRev:  400,
			reloadYAML: `
version: v1
engine:
  selector:
    allow:
      - mysql_queries_total{db="main",}
groups:
  - family: Database
    metrics:
      - mysql_queries_total
    charts:
      - title: Queries
        context: queries_total
        units: queries/s
        dimensions:
          - selector: mysql_queries_total
            name: total
`,
			reloadRev: 401,
			reloadErr: true,
			assert: func(t *testing.T, e *Engine) {
				t.Helper()
				p := e.program()
				require.NotNil(t, p)
				assert.Equal(t, uint64(400), p.Revision())
				assert.Equal(t, []string{"mysql_queries_total"}, p.MetricNames())
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			e, err := New()
			require.NoError(t, err)

			require.NoError(t, e.LoadYAML([]byte(tc.initialYAML), tc.initialRev))
			if tc.reloadYAML != "" {
				err = e.LoadYAML([]byte(tc.reloadYAML), tc.reloadRev)
				if tc.reloadErr {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
			}
			if tc.assert != nil {
				tc.assert(t, e)
			}
		})
	}
}

func validTemplateYAML() string {
	return `
version: v1
groups:
  - family: Database
    metrics:
      - mysql_queries_total
    charts:
      - title: Queries
        context: queries_total
        units: queries/s
        dimensions:
          - selector: mysql_queries_total
            name: total
`
}

func TestEngineResetMaterializedPreservesProgram(t *testing.T) {
	e, err := New()
	require.NoError(t, err)
	require.NoError(t, e.LoadYAML([]byte(validTemplateYAML()), 42))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	cc.BeginCycle()
	store.Write().SnapshotMeter("").Gauge("mysql_queries_total").Observe(7)
	cc.CommitCycleSuccess()

	plan1, err := buildPlan(e, store.Read())
	require.NoError(t, err)
	require.NotNil(t, findCreateChartActionInEngineTests(plan1))

	p := e.program()
	require.NotNil(t, p)
	require.Equal(t, uint64(42), p.Revision())

	e.ResetMaterialized()

	p = e.program()
	require.NotNil(t, p)
	require.Equal(t, uint64(42), p.Revision())

	plan2, err := buildPlan(e, store.Read())
	require.NoError(t, err)
	require.NotNil(t, findCreateChartActionInEngineTests(plan2))
}

func TestEnginePreparePlanLifecycleScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"outstanding attempt blocks prepare until abort": {
			run: func(t *testing.T) {
				e, err := New()
				require.NoError(t, err)
				require.NoError(t, e.LoadYAML([]byte(validTemplateYAML()), 1))

				store := metrix.NewCollectorStore()
				cc := mustCycleController(t, store)
				cc.BeginCycle()
				store.Write().SnapshotMeter("").Gauge("mysql_queries_total").Observe(7)
				cc.CommitCycleSuccess()

				attempt, err := e.PreparePlan(store.Read())
				require.NoError(t, err)

				_, err = e.PreparePlan(store.Read())
				require.ErrorIs(t, err, ErrOutstandingPlanAttempt)

				attempt.Abort()
				require.Empty(t, e.state.materialized.charts)

				cc.BeginCycle()
				store.Write().SnapshotMeter("").Gauge("mysql_queries_total").Observe(8)
				cc.CommitCycleSuccess()

				plan, err := buildPlan(e, store.Read())
				require.NoError(t, err)
				require.NotNil(t, findCreateChartActionInEngineTests(plan))
			},
		},
		"aborted attempt does not advance materialized lifecycle": {
			run: func(t *testing.T) {
				e, err := New()
				require.NoError(t, err)
				require.NoError(t, e.LoadYAML([]byte(validTemplateYAML()), 1))

				store := metrix.NewCollectorStore()
				cc := mustCycleController(t, store)
				cc.BeginCycle()
				store.Write().SnapshotMeter("").Gauge("mysql_queries_total").Observe(9)
				cc.CommitCycleSuccess()

				attempt, err := e.PreparePlan(store.Read())
				require.NoError(t, err)
				attempt.Abort()
				require.Empty(t, e.state.materialized.charts)

				cc.BeginCycle()
				store.Write().SnapshotMeter("").Gauge("mysql_queries_total").Observe(12)
				cc.CommitCycleSuccess()

				plan, err := buildPlan(e, store.Read())
				require.NoError(t, err)
				require.NotNil(t, findCreateChartActionInEngineTests(plan))
			},
		},
		"reset materialized makes prepared commit stale": {
			run: func(t *testing.T) {
				e, err := New()
				require.NoError(t, err)
				require.NoError(t, e.LoadYAML([]byte(validTemplateYAML()), 1))

				store := metrix.NewCollectorStore()
				cc := mustCycleController(t, store)
				cc.BeginCycle()
				store.Write().SnapshotMeter("").Gauge("mysql_queries_total").Observe(11)
				cc.CommitCycleSuccess()

				attempt, err := e.PreparePlan(store.Read())
				require.NoError(t, err)

				e.ResetMaterialized()

				require.ErrorIs(t, attempt.Commit(), ErrStalePlanAttempt)
			},
		},
		"repeated commit is rejected after successful commit": {
			run: func(t *testing.T) {
				e, err := New()
				require.NoError(t, err)
				require.NoError(t, e.LoadYAML([]byte(validTemplateYAML()), 1))

				store := metrix.NewCollectorStore()
				cc := mustCycleController(t, store)
				cc.BeginCycle()
				store.Write().SnapshotMeter("").Gauge("mysql_queries_total").Observe(13)
				cc.CommitCycleSuccess()

				attempt, err := e.PreparePlan(store.Read())
				require.NoError(t, err)
				require.NotNil(t, findCreateChartActionInEngineTests(attempt.Plan()))

				require.NoError(t, attempt.Commit())
				require.NotEmpty(t, e.state.materialized.charts)
				require.ErrorIs(t, attempt.Commit(), ErrFinishedPlanAttempt)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

func TestEngineResetMaterializedKeepsRouteCacheWarm(t *testing.T) {
	e, err := New()
	require.NoError(t, err)
	require.NoError(t, e.LoadYAML([]byte(dynamicDimensionTemplateYAML()), 42))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	vec := store.Write().StatefulMeter("component").Vec("id").Gauge("load")

	cc.BeginCycle()
	vec.WithLabelValues("a").Set(1)
	vec.WithLabelValues("b").Set(2)
	cc.CommitCycleSuccess()

	plan1, err := buildPlan(e, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	require.NotNil(t, findCreateChartActionInEngineTests(plan1))
	stats1 := e.stats()
	require.Greater(t, stats1.RouteCacheMisses, uint64(0))

	plan2, err := buildPlan(e, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	require.Nil(t, findCreateChartActionInEngineTests(plan2))
	stats2 := e.stats()
	require.Greater(t, stats2.RouteCacheHits, stats1.RouteCacheHits)

	e.ResetMaterialized()

	plan3, err := buildPlan(e, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	require.NotNil(t, findCreateChartActionInEngineTests(plan3))
	stats3 := e.stats()
	require.Greater(t, stats3.RouteCacheHits, stats2.RouteCacheHits)
}

func findCreateChartActionInEngineTests(plan Plan) *CreateChartAction {
	for _, action := range plan.Actions {
		create, ok := action.(CreateChartAction)
		if ok {
			return &create
		}
	}
	return nil
}

func dynamicDimensionTemplateYAML() string {
	return `
version: v1
groups:
  - family: Runtime
    metrics:
      - component.load
    charts:
      - id: component_load
        title: Component Load
        context: netdata.go.plugin.component.component_load
        units: load
        dimensions:
          - selector: component.load
            name_from_label: id
`
}
