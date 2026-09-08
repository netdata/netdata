// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"math"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Separate real stores permit a descriptor to change while preserving the
// flattened identity. Aborting the first plan retains discovery, not lifecycle.
func TestAutogenCacheReaderTransitions(t *testing.T) {
	tests := map[string]struct {
		first, next func(metrix.SnapshotMeter)
		sharedName  string
	}{
		"metadata changes": {sharedName: "work", first: func(m metrix.SnapshotMeter) {
			m.Gauge("work", metrix.WithDescription("Before"), metrix.WithUnit("bytes")).Observe(7)
		}, next: func(m metrix.SnapshotMeter) {
			m.Gauge("work", metrix.WithDescription("After"), metrix.WithUnit("seconds"), metrix.WithFloat(true)).
				Observe(7)
		}},
		"metadata disappears": {sharedName: "work", first: func(m metrix.SnapshotMeter) {
			m.Gauge("work", metrix.WithDescription("Before"), metrix.WithUnit("bytes"), metrix.WithFloat(true)).
				Observe(7)
		}, next: func(m metrix.SnapshotMeter) { m.Gauge("work").Observe(7) }},
		"gauge becomes counter": {
			sharedName: "work",
			first:      func(m metrix.SnapshotMeter) { m.Gauge("work").Observe(7) },
			next:       func(m metrix.SnapshotMeter) { m.Counter("work").ObserveTotal(7) },
		},
		"counter becomes gauge": {
			sharedName: "work",
			first:      func(m metrix.SnapshotMeter) { m.Counter("work").ObserveTotal(7) },
			next:       func(m metrix.SnapshotMeter) { m.Gauge("work").Observe(7) },
		},
		"scalar becomes histogram bucket": {sharedName: "latency_bucket", first: func(m metrix.SnapshotMeter) {
			m.Counter("latency_bucket").ObserveTotal(7, m.LabelSet(metrix.Label{
				Key:   "le",
				Value: "1",
			}))
		}, next: observeAutogenCacheHistogram},
		"histogram count becomes summary count": {
			sharedName: "latency_count",
			first:      observeAutogenCacheHistogram,
			next: func(m metrix.SnapshotMeter) {
				m.Summary("latency").ObservePoint(metrix.SummaryPoint{
					Count: 8,
					Sum:   10,
				})
			},
		},
		"scalar becomes summary quantile": {sharedName: "latency", first: func(m metrix.SnapshotMeter) {
			m.Gauge("latency").Observe(7, m.LabelSet(metrix.Label{
				Key:   "quantile",
				Value: "0.5",
			}))
		}, next: func(m metrix.SnapshotMeter) {
			m.Summary("latency", metrix.WithSummaryQuantiles(0.5)).ObservePoint(metrix.SummaryPoint{
				Count:     8,
				Sum:       10,
				Quantiles: []metrix.QuantilePoint{{Quantile: 0.5, Value: 7}},
			})
		}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			e := newAutogenCacheTestEngine(t)
			before := autogenCacheTestReader(t, tc.first)
			after := autogenCacheTestReader(t, tc.next)
			identities := func(reader metrix.Reader) map[string]metrix.SeriesIdentity {
				out := make(map[string]metrix.SeriesIdentity)
				reader.ForEachSeriesIdentity(
					func(id metrix.SeriesIdentity, _ metrix.SeriesMeta, name string, _ metrix.LabelView, _ metrix.SampleValue) {
						out[name] = id
					},
				)
				return out
			}
			require.Contains(t, identities(before), tc.sharedName)
			require.Contains(t, identities(after), tc.sharedName)
			require.Equal(
				t,
				identities(before)[tc.sharedName],
				identities(after)[tc.sharedName],
				"transition must reach the same cache identity",
			)
			a, err := e.PreparePlan(before)
			require.NoError(t, err)
			a.Abort()
			actual, err := e.PreparePlan(after)
			require.NoError(t, err)
			defer actual.Abort()
			fresh := newAutogenCacheTestEngine(t, WithPlanRouteDiagnosticObserver(func(PlanRouteDiagnostic) {}))
			expected, err := fresh.PreparePlan(after)
			require.NoError(t, err)
			defer expected.Abort()
			require.NotEmpty(t, expected.Plan().Actions)
			assert.Equal(t, expected.Plan(), actual.Plan(), "cached discovery must preserve current reader semantics")
		})
	}
}

func TestAutogenCacheReuseAndReload(t *testing.T) {
	e := newAutogenCacheTestEngine(t)
	store := metrix.NewCollectorStore(metrix.WithExpireAfterSuccessCycles(1))
	cc := mustCycleController(t, store)
	meter := store.Write().SnapshotMeter("")
	g := meter.Gauge("work")
	cycle := func(observe bool) metrix.Reader {
		cc.BeginCycle()
		if observe {
			g.Observe(7)
		}
		require.NoError(t, cc.CommitCycleSuccess())
		return store.Read(metrix.ReadRaw(), metrix.ReadFlatten())
	}
	first := cycle(true)
	_, err := buildPlan(e, first)
	require.NoError(t, err)
	var identity metrix.SeriesIdentity
	first.ForEachSeriesIdentity(
		func(id metrix.SeriesIdentity, _ metrix.SeriesMeta, _ string, _ metrix.LabelView, _ metrix.SampleValue) {
			identity = id
		},
	)
	routes, ok := e.state.routeCache.Lookup(identity, 1, 1)
	require.True(t, ok)
	require.Len(t, routes, 1)
	original := &routes[0]
	next := cycle(true)
	_, err = buildPlan(e, next)
	require.NoError(t, err)
	routes, ok = e.state.routeCache.Lookup(identity, 1, 2)
	require.True(t, ok)
	require.Len(t, routes, 1)
	require.Same(t, original, &routes[0], "unchanged routes should reuse their immutable cached binding")
	// Source eviction must prune the cached autogen route too.
	_, err = buildPlan(e, cycle(false))
	require.NoError(t, err)
	_, ok = e.state.routeCache.Lookup(identity, 1, 3)
	require.False(t, ok)
	_, err = buildPlan(e, cycle(true))
	require.NoError(t, err)
	require.NoError(t, e.LoadYAML([]byte(`version: v1
context_namespace: changed
groups:
 - family: Authored
   metrics: [work]
   charts:
    - id: authored
      title: Authored
      context: work
      units: requests
      dimensions:
       - selector: work
         name: requests
`), 2))
	_, ok = e.state.routeCache.Lookup(identity, 1, 4)
	require.False(t, ok)
	plan, err := buildPlan(e, cycle(true))
	require.NoError(t, err)
	var created []CreateChartAction
	for _, a := range plan.Actions {
		if c, ok := a.(CreateChartAction); ok {
			created = append(created, c)
		}
	}
	require.Len(t, created, 1)
	assert.Equal(t, "Authored", created[0].Meta.Title)
	assert.Equal(t, "changed.work", created[0].Meta.Context)
}

func TestAutogenWarmPlanAllocationEnvelope(t *testing.T) {
	for _, n := range []int{128, 512} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			e := newAutogenCacheTestEngine(t, WithRuntimePlannerMode())
			reader := autogenCacheTestReader(t, func(m metrix.SnapshotMeter) {
				g := m.Gauge("work")
				for i := range n {
					g.Observe(float64(i), m.LabelSet(metrix.Label{
						Key:   "id",
						Value: fmt.Sprint(i),
					}))
				}
			})
			_, err := buildPlan(e, reader)
			require.NoError(t, err)
			allocs := testing.AllocsPerRun(
				10,
				func() { plan, err := buildPlan(e, reader); require.NoError(t, err); require.Len(t, plan.Actions, n) },
			)
			// Plans still allocate O(output charts/dimensions) staged state. This generous
			// ceiling excludes rebuilding O(series) autogen strings/routes on every hit.
			assert.LessOrEqual(t, allocs, float64(15*n+128))
		})
	}
}

func newAutogenCacheTestEngine(t testing.TB, opts ...Option) *Engine {
	t.Helper()
	opts = append([]Option{WithRuntimeStore(nil), WithEnginePolicy(EnginePolicy{
		Autogen: &AutogenPolicy{
			Enabled: true,
		},
	})}, opts...)
	e, err := New(opts...)
	require.NoError(t, err)
	require.NoError(t, e.LoadYAML([]byte(benchAutogenTemplateYAML), 1))
	return e
}
func autogenCacheTestReader(t testing.TB, observe func(metrix.SnapshotMeter)) metrix.Reader {
	t.Helper()
	s := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(s)
	require.True(t, ok)
	cc := managed.CycleController()
	cc.BeginCycle()
	observe(s.Write().SnapshotMeter(""))
	require.NoError(t, cc.CommitCycleSuccess())
	return s.Read(metrix.ReadRaw(), metrix.ReadFlatten())
}
func observeAutogenCacheHistogram(m metrix.SnapshotMeter) {
	m.Histogram("latency", metrix.WithHistogramBounds(1)).ObservePoint(metrix.HistogramPoint{
		Count: 8,
		Sum:   10,
		Buckets: []metrix.BucketPoint{
			{UpperBound: 1, CumulativeCount: 7},
			{UpperBound: math.Inf(1), CumulativeCount: 8},
		},
	})
}

// Reader permits missing family metadata even when scalar samples are visible.
// Only that optional input is hidden; real metrix identity/value iteration is used.
type autogenMetadataAbsentReader struct{ metrix.Reader }

func (autogenMetadataAbsentReader) MetricMeta(string) (metrix.MetricMeta, bool) {
	return metrix.MetricMeta{}, false
}

func TestAutogenCacheMetadataPresence(t *testing.T) {
	reader := autogenCacheTestReader(t, func(m metrix.SnapshotMeter) {
		m.Gauge("work", metrix.WithUnit("bytes"), metrix.WithFloat(true)).Observe(1.5)
	})
	e := newAutogenCacheTestEngine(t)
	for _, current := range []metrix.Reader{reader, autogenMetadataAbsentReader{
		Reader: reader,
	}, reader} {
		actual, err := e.PreparePlan(current)
		require.NoError(t, err)
		oracle := newAutogenCacheTestEngine(t, WithPlanRouteDiagnosticObserver(func(PlanRouteDiagnostic) {}))
		expected, err := oracle.PreparePlan(current)
		require.NoError(t, err)
		assert.Equal(t, expected.Plan(), actual.Plan())
		actual.Abort()
		expected.Abort()
	}
}

func TestCollectExpiryNoRemovalAllocationEnvelope(t *testing.T) {
	for _, n := range []int{128, 512} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			e := newAutogenCacheTestEngine(t, WithEnginePolicy(EnginePolicy{
				Autogen: &AutogenPolicy{
					Enabled:                  true,
					ExpireAfterSuccessCycles: 3,
				},
			}))
			reader := autogenCacheTestReader(t, func(m metrix.SnapshotMeter) {
				g := m.Gauge("work")
				for i := range n {
					g.Observe(1, m.LabelSet(metrix.Label{
						Key:   "id",
						Value: fmt.Sprint(i),
					}))
				}
			})
			_, err := buildPlan(e, reader)
			require.NoError(t, err)
			// Stable expiry still scans retained state, but allocates only for removals.
			removed := 0
			allocs := testing.AllocsPerRun(10, func() {
				dims, charts := collectExpiryRemovals(1, &e.state.materialized)
				removed += len(dims) + len(charts)
			})
			require.Zero(t, removed)
			assert.Zero(t, allocs)
		})
	}
}

// A kind transition can change the family selected by autogen rules without
// changing flattened identity. Rejection must release the previous discovery.
func TestAutogenCacheRejectionClearsBindingAndRecovers(t *testing.T) {
	e, err := New(WithRuntimeStore(nil))
	require.NoError(t, err)
	require.NoError(t, e.LoadYAML(benchmarkAutogenRulesTemplate([]string{"latency"}), 1))
	before := autogenCacheTestReader(t, func(m metrix.SnapshotMeter) { m.Counter("latency_count").ObserveTotal(8) })
	after := autogenCacheTestReader(t, observeAutogenCacheHistogram)
	var oldID, newID metrix.SeriesIdentity
	before.ForEachSeriesIdentity(func(id metrix.SeriesIdentity, _ metrix.SeriesMeta, name string, _ metrix.LabelView, _ metrix.SampleValue) {
		if name == "latency_count" {
			oldID = id
		}
	})
	after.ForEachSeriesIdentity(func(id metrix.SeriesIdentity, _ metrix.SeriesMeta, name string, _ metrix.LabelView, _ metrix.SampleValue) {
		if name == "latency_count" {
			newID = id
		}
	})
	require.NotEmpty(t, oldID.ID)
	require.Equal(t, oldID, newID)
	initial, err := e.PreparePlan(before)
	require.NoError(t, err)
	require.NotEmpty(t, initial.Plan().Actions)
	initialPlan := initial.Plan()
	initial.Abort()
	original, hit := e.state.routeCache.Lookup(oldID, 1, 1)
	require.True(t, hit)
	require.Len(t, original, 1)
	for range 3 {
		rejected, err := e.PreparePlan(after)
		require.NoError(t, err)
		require.Empty(t, rejected.Plan().Actions, "histogram family is denied")
		rejected.Abort()
		cached, hit := e.state.routeCache.Lookup(oldID, 1, 1)
		require.True(t, hit)
		require.Empty(t, cached, "obsolete scalar binding must be cleared after histogram family rejection")
	}
	recovered, err := e.PreparePlan(before)
	require.NoError(t, err)
	defer recovered.Abort()
	require.Equal(t, initialPlan, recovered.Plan(), "empty discovery must allow an eligible source to recover")
	cached, hit := e.state.routeCache.Lookup(oldID, 1, 1)
	require.True(t, hit)
	require.Len(t, cached, 1)
}
