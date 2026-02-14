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

func actionKinds(actions []EngineAction) []ActionKind {
	out := make([]ActionKind, 0, len(actions))
	for _, action := range actions {
		out = append(out, action.Kind())
	}
	return out
}

func mustCycleController(t *testing.T, s metrix.CollectorStore) metrix.CycleController {
	t.Helper()
	managed, ok := metrix.AsCycleManagedStore(s)
	require.True(t, ok)
	return managed.CycleController()
}
