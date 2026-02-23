// SPDX-License-Identifier: GPL-3.0-or-later

package collecttest

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScalarKeyFromLabelsMap(t *testing.T) {
	tests := map[string]struct {
		name   string
		labels map[string]string
		want   string
	}{
		"no labels": {
			name: "metric",
			want: "metric",
		},
		"sorted labels": {
			name:   "metric",
			labels: map[string]string{"z": "2", "a": "1"},
			want:   "metric{a=\"1\",z=\"2\"}",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := scalarKeyFromLabelsMap(tc.name, tc.labels)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestMaterializedChartsFiltersByContextAndID(t *testing.T) {
	plan := chartengine.Plan{
		Actions: []chartengine.EngineAction{
			chartengine.CreateChartAction{ChartID: "keep", Meta: chartengine.ChartMeta{Context: "ctx.keep"}},
			chartengine.CreateDimensionAction{ChartID: "keep", ChartMeta: chartengine.ChartMeta{Context: "ctx.keep"}, Name: "dimA"},
			chartengine.CreateChartAction{ChartID: "drop-by-context", Meta: chartengine.ChartMeta{Context: "ctx.drop"}},
			chartengine.CreateDimensionAction{ChartID: "drop-by-context", ChartMeta: chartengine.ChartMeta{Context: "ctx.drop"}, Name: "dimB"},
			chartengine.CreateChartAction{ChartID: "drop-by-id", Meta: chartengine.ChartMeta{Context: "ctx.keep2"}},
			chartengine.CreateDimensionAction{ChartID: "drop-by-id", ChartMeta: chartengine.ChartMeta{Context: "ctx.keep2"}, Name: "dimC"},
		},
	}

	got := materializedCharts(plan, planFilter{
		ExcludeContexts: map[string]struct{}{"ctx.drop": {}},
		ExcludeChartIDs: map[string]struct{}{"drop-by-id": {}},
	})

	assert.Len(t, got, 1)
	chart, ok := got["keep"]
	assert.True(t, ok)
	assert.Equal(t, "ctx.keep", chart.Context)
	_, hasDim := chart.Dimensions["dimA"]
	assert.True(t, hasDim)
}

func TestBuildChartCoverage(t *testing.T) {
	tests := map[string]struct {
		excludePatterns []string
		wantExpected    map[string][]string
		wantActual      map[string][]string
		wantErr         bool
	}{
		"selector-aware expected coverage with glob exclude": {
			excludePatterns: []string{"test.c"},
			wantExpected: map[string][]string{
				"test.a": {"x"},
			},
			wantActual: map[string][]string{
				"test.a": {"x"},
			},
		},
		"invalid glob pattern": {
			excludePatterns: []string{"["},
			wantErr:         true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			store := newTestCollectorStore(t, func(m metrix.SnapshotMeter) {
				m.Gauge("metric_a").Observe(1)
				m.Gauge("metric_b").Observe(2)
			})

			templateYAML := `
version: v1
context_namespace: test
groups:
  - family: Root
    metrics: [metric_a, metric_b]
    charts:
      - title: A
        context: a
        units: "1"
        dimensions:
          - selector: metric_a
            name: x
      - title: B
        context: b
        units: "1"
        dimensions:
          - selector: metric_b{role="missing"}
            name: y
      - title: C
        context: c
        units: "1"
        dimensions:
          - selector: metric_b
            name: z
`

			coverage, err := buildChartCoverage(templateYAML, 1, store.Read(metrix.ReadRaw()), tc.excludePatterns)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, normalizeCoverageDimsList(tc.wantExpected), normalizeCoverageDimsList(coverage.ExpectedByContext))
			require.Equal(t, normalizeCoverageDimsList(tc.wantActual), normalizeCoverageDims(coverage.ActualByContext))
		})
	}
}

func TestValidateChartTemplateSchema(t *testing.T) {
	tests := map[string]struct {
		template string
		wantErr  bool
	}{
		"valid template": {
			template: `
version: v1
groups:
  - family: Root
    metrics: [metric_a]
    charts:
      - title: A
        context: a
        units: "1"
        dimensions:
          - selector: metric_a
            name: x
`,
		},
		"schema rejects missing version": {
			template: `
groups:
  - family: Root
    metrics: [metric_a]
    charts:
      - title: A
        context: a
        units: "1"
        dimensions:
          - selector: metric_a
            name: x
`,
			wantErr: true,
		},
		"schema rejects unknown field": {
			template: `
version: v1
groups:
  - family: Root
    metrics: [metric_a]
    charts:
      - title: A
        context: a
        units: "1"
        unknown_field: true
        dimensions:
          - selector: metric_a
            name: x
`,
			wantErr: true,
		},
		"schema rejects non-string YAML key": {
			template: `
version: v1
groups:
  - family: Root
    metrics: [metric_a]
    charts:
      - title: A
        context: a
        units: "1"
        1: invalid
        dimensions:
          - selector: metric_a
            name: x
`,
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := ValidateChartTemplateSchema(tc.template)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestCollectOnceAbortsCycleOnPanic(t *testing.T) {
	store := metrix.NewCollectorStore()

	require.Panics(t, func() {
		_ = collectOnce(store, func(context.Context) error {
			panic("boom")
		})
	})

	err := collectOnce(store, func(_ context.Context) error {
		store.Write().SnapshotMeter("").Gauge("metric").Observe(1)
		return nil
	})
	require.NoError(t, err)
}

func TestBuildChartCoverageDynamicDimensions(t *testing.T) {
	tests := map[string]struct {
		template string
		store    metrix.CollectorStore
		want     map[string][]string
	}{
		"name_from_label dimensions are asserted from selector matches": {
			template: `
version: v1
context_namespace: test
groups:
  - family: Root
    metrics: [metric_a]
    charts:
      - title: A
        context: a
        units: "1"
        dimensions:
          - selector: metric_a
            name_from_label: state
`,
			store: newTestCollectorStore(t, func(m metrix.SnapshotMeter) {
				g := m.Gauge("metric_a")
				g.Observe(1, m.LabelSet(metrix.Label{Key: "state", Value: "up"}))
				g.Observe(1, m.LabelSet(metrix.Label{Key: "state", Value: "down"}))
			}),
			want: map[string][]string{
				"test.a": {"down", "up"},
			},
		},
		"inferred stateset dimensions are asserted from flattened metadata": {
			template: `
version: v1
context_namespace: test
groups:
  - family: Root
    metrics: [system.status]
    charts:
      - title: System status
        context: status
        units: state
        dimensions:
          - selector: system.status
`,
			store: newTestCollectorStore(t, func(m metrix.SnapshotMeter) {
				ss := m.StateSet(
					"system.status",
					metrix.WithStateSetStates("ok", "failed"),
					metrix.WithStateSetMode(metrix.ModeEnum),
				)
				ss.Enable("ok")
			}),
			want: map[string][]string{
				"test.status": {"failed", "ok"},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			coverage, err := buildChartCoverage(
				tc.template,
				1,
				tc.store.Read(metrix.ReadRaw(), metrix.ReadFlatten()),
				nil,
			)
			require.NoError(t, err)
			require.Equal(t, normalizeCoverageDimsList(tc.want), normalizeCoverageDimsList(coverage.ExpectedByContext))
			require.Equal(t, normalizeCoverageDimsList(tc.want), normalizeCoverageDims(coverage.ActualByContext))
		})
	}
}

func TestCollectScalarSeries(t *testing.T) {
	tests := map[string]struct {
		collectFn func(ctx context.Context, store metrix.CollectorStore) error
		want      map[string]metrix.SampleValue
		wantErr   bool
	}{
		"nil collector": {
			wantErr: true,
		},
		"collects scalar series from one cycle": {
			collectFn: func(_ context.Context, store metrix.CollectorStore) error {
				m := store.Write().SnapshotMeter("")
				m.Gauge("metric").Observe(7)
				return nil
			},
			want: map[string]metrix.SampleValue{
				"metric": 7,
			},
		},
		"returns collect error": {
			collectFn: func(_ context.Context, _ metrix.CollectorStore) error {
				return errors.New("boom")
			},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var collector interface {
				MetricStore() metrix.CollectorStore
				Collect(context.Context) error
			}
			if tc.collectFn != nil {
				store := metrix.NewCollectorStore()
				collector = testScalarCollector{
					store: store,
					collectFn: func(ctx context.Context) error {
						return tc.collectFn(ctx, store)
					},
				}
			}

			got, err := CollectScalarSeries(collector, metrix.ReadRaw())
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

type testScalarCollector struct {
	store     metrix.CollectorStore
	collectFn func(context.Context) error
}

func (c testScalarCollector) MetricStore() metrix.CollectorStore {
	return c.store
}

func (c testScalarCollector) Collect(ctx context.Context) error {
	return c.collectFn(ctx)
}

func newTestCollectorStore(t *testing.T, writeFn func(m metrix.SnapshotMeter)) metrix.CollectorStore {
	t.Helper()

	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	require.True(t, ok)

	cc := managed.CycleController()
	cc.BeginCycle()
	writeFn(store.Write().SnapshotMeter(""))
	cc.CommitCycleSuccess()
	return store
}

func normalizeCoverageDims(in map[string]map[string]struct{}) map[string][]string {
	out := make(map[string][]string, len(in))
	for contextName, dimSet := range in {
		dims := make([]string, 0, len(dimSet))
		for dimName := range dimSet {
			dims = append(dims, dimName)
		}
		sort.Strings(dims)
		out[contextName] = dims
	}
	return out
}

func normalizeCoverageDimsList(in map[string][]string) map[string][]string {
	out := make(map[string][]string, len(in))
	for contextName, dims := range in {
		clone := append([]string(nil), dims...)
		sort.Strings(clone)
		out[contextName] = clone
	}
	return out
}
