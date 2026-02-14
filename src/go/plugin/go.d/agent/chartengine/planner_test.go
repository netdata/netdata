// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

func TestInferDimensionLabelKeyScenarios(t *testing.T) {
	tests := map[string]struct {
		metricName string
		meta       metrix.SeriesMeta
		wantKey    string
		wantOK     bool
		wantErr    bool
	}{
		"histogram bucket uses le label": {
			meta: metrix.SeriesMeta{
				FlattenRole: metrix.FlattenRoleHistogramBucket,
			},
			wantKey: "le",
			wantOK:  true,
		},
		"summary quantile uses quantile label": {
			meta: metrix.SeriesMeta{
				FlattenRole: metrix.FlattenRoleSummaryQuantile,
			},
			wantKey: "quantile",
			wantOK:  true,
		},
		"stateset uses metric family name label": {
			metricName: "system_status",
			meta: metrix.SeriesMeta{
				FlattenRole: metrix.FlattenRoleStateSetState,
			},
			wantKey: "system_status",
			wantOK:  true,
		},
		"histogram count does not infer dynamic key": {
			meta: metrix.SeriesMeta{
				FlattenRole: metrix.FlattenRoleHistogramCount,
			},
			wantOK: false,
		},
		"non flattened role is an inference error": {
			meta: metrix.SeriesMeta{
				FlattenRole: metrix.FlattenRoleNone,
			},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			key, ok, err := inferDimensionLabelKey(tc.metricName, tc.meta)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantKey, key)
			assert.Equal(t, tc.wantOK, ok)
		})
	}
}

func TestBuildPlanResolvesInferDimensionNames(t *testing.T) {
	tests := map[string]struct {
		yaml      string
		setup     func(t *testing.T, s metrix.CollectorStore)
		wantNames []string
		wantKinds []ActionKind
	}{
		"histogram bucket inference resolves bucket names from le": {
			yaml: `
version: v1
groups:
  - family: Latency
    metrics:
      - svc.latency_seconds_bucket
    charts:
      - title: Latency buckets
        context: latency_bucket
        units: observations
        dimensions:
          - selector: svc.latency_seconds_bucket
`,
			setup: func(t *testing.T, s metrix.CollectorStore) {
				t.Helper()
				cc := mustCycleController(t, s)
				h := s.Write().SnapshotMeter("svc").Histogram("latency_seconds", metrix.WithHistogramBounds(1, 2))
				cc.BeginCycle()
				h.ObservePoint(metrix.HistogramPoint{
					Count: 2,
					Sum:   3,
					Buckets: []metrix.BucketPoint{
						{UpperBound: 1, CumulativeCount: 1},
						{UpperBound: 2, CumulativeCount: 2},
					},
				})
				cc.CommitCycleSuccess()
			},
			wantNames: []string{"+Inf", "1", "2"},
			wantKinds: []ActionKind{ActionCreateChart, ActionCreateDimension, ActionCreateDimension, ActionCreateDimension, ActionUpdateChart},
		},
		"summary quantile inference resolves quantile labels": {
			yaml: `
version: v1
groups:
  - family: Latency
    metrics:
      - svc.request_time
    charts:
      - title: Request time quantiles
        context: request_time_quantile
        units: seconds
        dimensions:
          - selector: svc.request_time{quantile=~".+"}
`,
			setup: func(t *testing.T, s metrix.CollectorStore) {
				t.Helper()
				cc := mustCycleController(t, s)
				sm := s.Write().SnapshotMeter("svc")
				sum := sm.Summary("request_time", metrix.WithSummaryQuantiles(0.5, 0.9))

				cc.BeginCycle()
				sum.ObservePoint(metrix.SummaryPoint{
					Count: 10,
					Sum:   8.8,
					Quantiles: []metrix.QuantilePoint{
						{Quantile: 0.5, Value: 0.4},
						{Quantile: 0.9, Value: 1.2},
					},
				})
				cc.CommitCycleSuccess()
			},
			wantNames: []string{"0.5", "0.9"},
			wantKinds: []ActionKind{ActionCreateChart, ActionCreateDimension, ActionCreateDimension, ActionUpdateChart},
		},
		"stateset inference resolves state names from metric-family label key": {
			yaml: `
version: v1
groups:
  - family: Service
    metrics:
      - system.status
    charts:
      - title: System status
        context: system_status
        units: state
        dimensions:
          - selector: system.status
`,
			setup: func(t *testing.T, s metrix.CollectorStore) {
				t.Helper()
				cc := mustCycleController(t, s)
				ss := s.Write().SnapshotMeter("system").StateSet(
					"status",
					metrix.WithStateSetStates("ok", "failed"),
					metrix.WithStateSetMode(metrix.ModeEnum),
				)
				cc.BeginCycle()
				ss.Enable("ok")
				cc.CommitCycleSuccess()
			},
			wantNames: []string{"failed", "ok"},
			wantKinds: []ActionKind{ActionCreateChart, ActionCreateDimension, ActionCreateDimension, ActionUpdateChart},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			e, err := New()
			require.NoError(t, err)
			require.NoError(t, e.LoadYAML([]byte(tc.yaml), 1))

			store := metrix.NewCollectorStore()
			tc.setup(t, store)

			plan, err := e.BuildPlan(store.Read())
			require.NoError(t, err)

			got := make([]string, 0, len(plan.InferredDimensions))
			for _, dim := range plan.InferredDimensions {
				got = append(got, dim.Name)
			}
			assert.Equal(t, tc.wantNames, got)
			assert.Equal(t, tc.wantKinds, actionKinds(plan.Actions))
		})
	}
}

func TestBuildPlanUsesRouteCacheReuse(t *testing.T) {
	e, err := New()
	require.NoError(t, err)

	yaml := `
version: v1
groups:
  - family: Service
    metrics:
      - svc.requests_total
    charts:
      - title: Requests
        context: requests
        units: requests/s
        dimensions:
          - selector: svc.requests_total
            name: total
`
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	c := store.Write().SnapshotMeter("svc").Counter("requests_total")

	cc.BeginCycle()
	c.ObserveTotal(10)
	cc.CommitCycleSuccess()

	plan1, err := e.BuildPlan(store.Read())
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionCreateChart, ActionCreateDimension, ActionUpdateChart}, actionKinds(plan1.Actions))
	stats1 := e.Stats()
	assert.Equal(t, uint64(0), stats1.RouteCacheHits)
	assert.Equal(t, uint64(1), stats1.RouteCacheMisses)
	require.NotNil(t, findUpdateAction(plan1))
	assert.Equal(t, float64(10), findUpdateAction(plan1).Values[0].Float64)

	cc.BeginCycle()
	c.ObserveTotal(20)
	cc.CommitCycleSuccess()

	plan2, err := e.BuildPlan(store.Read())
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionUpdateChart}, actionKinds(plan2.Actions))
	stats2 := e.Stats()
	assert.Equal(t, uint64(1), stats2.RouteCacheHits)
	assert.Equal(t, uint64(1), stats2.RouteCacheMisses)
	require.NotNil(t, findUpdateAction(plan2))
	assert.Equal(t, float64(20), findUpdateAction(plan2).Values[0].Float64)
}

func TestBuildPlanLifecycleDimensionExpiry(t *testing.T) {
	e, err := New()
	require.NoError(t, err)

	yaml := `
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
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

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

	plan1, err := e.BuildPlan(store.Read())
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionCreateChart, ActionCreateDimension, ActionCreateDimension, ActionUpdateChart}, actionKinds(plan1.Actions))

	cc.BeginCycle()
	total.Observe(101)
	cc.CommitCycleSuccess()

	plan2, err := e.BuildPlan(store.Read())
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionUpdateChart, ActionRemoveDimension}, actionKinds(plan2.Actions))
	removeDim := findRemoveDimensionAction(plan2)
	require.NotNil(t, removeDim)
	assert.Equal(t, "ok", removeDim.Name)
}

func TestBuildPlanLifecycleChartExpiry(t *testing.T) {
	e, err := New()
	require.NoError(t, err)

	yaml := `
version: v1
groups:
  - family: Service
    metrics:
      - svc.requests_total
    charts:
      - title: Requests
        context: requests
        units: requests/s
        lifecycle:
          expire_after_cycles: 1
        dimensions:
          - selector: svc.requests_total
            name: total
`
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	c := store.Write().SnapshotMeter("svc").Counter("requests_total")

	cc.BeginCycle()
	c.ObserveTotal(10)
	cc.CommitCycleSuccess()

	plan1, err := e.BuildPlan(store.Read())
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionCreateChart, ActionCreateDimension, ActionUpdateChart}, actionKinds(plan1.Actions))

	cc.BeginCycle()
	cc.CommitCycleSuccess()

	plan2, err := e.BuildPlan(store.Read())
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionRemoveChart}, actionKinds(plan2.Actions))
}

func TestBuildPlanLifecycleNoRemovalOnFailedCycle(t *testing.T) {
	e, err := New()
	require.NoError(t, err)

	yaml := `
version: v1
groups:
  - family: Service
    metrics:
      - svc.requests_total
    charts:
      - title: Requests
        context: requests
        units: requests/s
        lifecycle:
          expire_after_cycles: 1
        dimensions:
          - selector: svc.requests_total
            name: total
`
	require.NoError(t, e.LoadYAML([]byte(yaml), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	c := store.Write().SnapshotMeter("svc").Counter("requests_total")

	cc.BeginCycle()
	c.ObserveTotal(10)
	cc.CommitCycleSuccess()

	plan1, err := e.BuildPlan(store.Read())
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionCreateChart, ActionCreateDimension, ActionUpdateChart}, actionKinds(plan1.Actions))

	cc.BeginCycle()
	cc.AbortCycle()

	plan2, err := e.BuildPlan(store.Read())
	require.NoError(t, err)
	assert.Empty(t, plan2.Actions)

	cc.BeginCycle()
	cc.CommitCycleSuccess()

	plan3, err := e.BuildPlan(store.Read())
	require.NoError(t, err)
	assert.Equal(t, []ActionKind{ActionRemoveChart}, actionKinds(plan3.Actions))
}

func actionKinds(actions []EngineAction) []ActionKind {
	out := make([]ActionKind, 0, len(actions))
	for _, action := range actions {
		out = append(out, action.Kind())
	}
	return out
}

func findUpdateAction(plan Plan) *UpdateChartAction {
	for _, action := range plan.Actions {
		if update, ok := action.(UpdateChartAction); ok {
			return &update
		}
	}
	return nil
}

func findRemoveDimensionAction(plan Plan) *RemoveDimensionAction {
	for _, action := range plan.Actions {
		if remove, ok := action.(RemoveDimensionAction); ok {
			return &remove
		}
	}
	return nil
}

func mustCycleController(t *testing.T, s metrix.CollectorStore) metrix.CycleController {
	t.Helper()
	managed, ok := metrix.AsCycleManagedStore(s)
	require.True(t, ok)
	return managed.CycleController()
}
