// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricMetaScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"reader returns declared metric metadata": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				g := s.Write().SnapshotMeter("apache").Gauge(
					"workers_busy",
					WithDescription("Busy workers"),
					WithChartFamily("Workers"),
					WithChartPriority(70000),
					WithUnit("workers"),
					WithFloat(true),
				)

				cc.BeginCycle()
				g.Observe(5)
				cc.CommitCycleSuccess()

				meta, ok := s.Read().MetricMeta("apache.workers_busy")
				require.True(t, ok)
				assert.Equal(t, "Busy workers", meta.Description)
				assert.Equal(t, "Workers", meta.ChartFamily)
				assert.Equal(t, 70000, meta.ChartPriority)
				assert.Equal(t, "workers", meta.Unit)
				assert.True(t, meta.Float)
			},
		},
		"unknown metric metadata is unavailable": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				meta, ok := s.Read().MetricMeta("missing.metric")
				assert.False(t, ok)
				assert.Equal(t, MetricMeta{}, meta)
			},
		},
		"flattened reader resolves histogram metadata by flattened names": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				h := s.Write().SnapshotMeter("svc").Histogram(
					"latency",
					WithHistogramBounds(1, 2),
					WithDescription("Latency"),
					WithChartFamily("Service"),
					WithUnit("ms"),
					WithFloat(true),
				)

				cc.BeginCycle()
				h.ObservePoint(HistogramPoint{
					Count: 2,
					Sum:   3,
					Buckets: []BucketPoint{
						{UpperBound: 1, CumulativeCount: 1},
						{UpperBound: 2, CumulativeCount: 2},
					},
				})
				cc.CommitCycleSuccess()

				flat := s.Read(ReadFlatten())

				_, ok := flat.MetricMeta("svc.latency")
				require.False(t, ok, "expected canonical histogram family name to be absent in flattened view")

				meta, ok := flat.MetricMeta("svc.latency_bucket")
				require.True(t, ok, "expected flattened histogram bucket metric metadata")
				assert.Equal(t, "Latency", meta.Description)
				assert.Equal(t, "Service", meta.ChartFamily)
				assert.Zero(t, meta.ChartPriority)
				assert.Equal(t, "ms", meta.Unit)
				assert.True(t, meta.Float)

				meta, ok = flat.MetricMeta("svc.latency_count")
				require.True(t, ok, "expected flattened histogram count metric metadata")
				assert.Equal(t, "Latency", meta.Description)
				assert.Equal(t, "Service", meta.ChartFamily)
				assert.Zero(t, meta.ChartPriority)
				assert.Equal(t, "ms", meta.Unit)
				assert.True(t, meta.Float)

				meta, ok = flat.MetricMeta("svc.latency_sum")
				require.True(t, ok, "expected flattened histogram sum metric metadata")
				assert.Equal(t, "Latency", meta.Description)
				assert.Equal(t, "Service", meta.ChartFamily)
				assert.Zero(t, meta.ChartPriority)
				assert.Equal(t, "ms", meta.Unit)
				assert.True(t, meta.Float)
			},
		},
		"metadata redeclaration conflict panics": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				w := s.Write().SnapshotMeter("apache")
				_ = w.Gauge("workers_busy", WithDescription("Busy workers"))
				expectPanic(t, func() {
					_ = w.Gauge("workers_busy", WithDescription("Workers currently busy"))
				})
			},
		},
		"chart priority redeclaration conflict panics": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				w := s.Write().SnapshotMeter("apache")
				_ = w.Gauge("workers_busy", WithChartPriority(70000))
				expectPanic(t, func() {
					_ = w.Gauge("workers_busy", WithChartPriority(70001))
				})
			},
		},
		"float metadata redeclaration conflict panics": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				w := s.Write().SnapshotMeter("apache")
				_ = w.Gauge("workers_busy", WithFloat(true))
				expectPanic(t, func() {
					_ = w.Gauge("workers_busy", WithFloat(false))
				})
			},
		},
		"redeclare without metadata options keeps first metadata": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				w := s.Write().SnapshotMeter("apache")
				g := w.Gauge(
					"workers_busy",
					WithDescription("Busy workers"),
					WithChartFamily("Workers"),
					WithChartPriority(70000),
					WithUnit("workers"),
					WithFloat(true),
				)
				_ = w.Gauge("workers_busy")

				cc := cycleController(t, s)
				cc.BeginCycle()
				g.Observe(1)
				cc.CommitCycleSuccess()

				meta, ok := s.Read().MetricMeta("apache.workers_busy")
				require.True(t, ok)
				assert.Equal(t, "Busy workers", meta.Description)
				assert.Equal(t, "Workers", meta.ChartFamily)
				assert.Equal(t, 70000, meta.ChartPriority)
				assert.Equal(t, "workers", meta.Unit)
				assert.True(t, meta.Float)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

func TestMetricMetaHostScopeIsolation(t *testing.T) {
	store := NewCollectorStore()
	cycle := cycleController(t, store)
	scopeA := HostScope{ScopeKey: "scope-a", GUID: "guid-a", Hostname: "host-a"}
	scopeB := HostScope{ScopeKey: "scope-b", GUID: "guid-b", Hostname: "host-b"}
	meter := store.Write().SnapshotMeter("svc")
	gaugeA := meter.WithHostScope(scopeA).Gauge("load", WithDescription("Load"))
	gaugeB := meter.WithHostScope(scopeB).Gauge("load", WithDescription("Load"))

	cycle.BeginCycle()
	gaugeA.Observe(1)
	gaugeB.Observe(2)
	require.NoError(t, cycle.CommitCycleSuccess())

	cycle.BeginCycle()
	gaugeB.Observe(3)
	require.NoError(t, cycle.CommitCycleSuccess())

	for _, flattened := range []bool{false, true} {
		name := "canonical"
		opts := []ReadOption{ReadHostScope(scopeA.ScopeKey)}
		if flattened {
			name = "flattened"
			opts = append(opts, ReadFlatten())
		}

		t.Run(name+"/stale_metadata_falls_back_within_scope", func(t *testing.T) {
			_, valueOK := store.Read(opts...).Value("svc.load", nil)
			require.False(t, valueOK, "stale cycle-fresh value must remain hidden")

			meta, metaOK := store.Read(opts...).MetricMeta("svc.load")
			require.True(t, metaOK)
			assert.Equal(t, "Load", meta.Description)

			rawOpts := append(append([]ReadOption(nil), opts...), ReadRaw())
			rawMeta, rawOK := store.Read(rawOpts...).MetricMeta("svc.load")
			require.True(t, rawOK)
			assert.Equal(t, "Load", rawMeta.Description)
		})

		t.Run(name+"/missing_scope_never_falls_back_to_peer", func(t *testing.T) {
			missingOpts := []ReadOption{ReadHostScope("scope-missing")}
			if flattened {
				missingOpts = append(missingOpts, ReadFlatten())
			}
			_, freshOK := store.Read(missingOpts...).MetricMeta("svc.load")
			require.False(t, freshOK)

			missingOpts = append(missingOpts, ReadRaw())
			_, rawOK := store.Read(missingOpts...).MetricMeta("svc.load")
			require.False(t, rawOK)
		})
	}
}
