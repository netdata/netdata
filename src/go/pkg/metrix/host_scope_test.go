// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHostScopePartitionsSeriesIdentity(t *testing.T) {
	store := NewCollectorStore()
	cc := cycleController(t, store)

	scope := HostScope{
		ScopeKey: "workload/api",
		GUID:     "guid-api",
		Hostname: "api",
		Labels:   map[string]string{"_vnode_type": "azure_workload"},
	}

	meter := store.Write().SnapshotMeter("azure")
	defaultGauge := meter.Gauge("requests")
	scopedGauge := meter.WithHostScope(scope).Gauge("requests")
	labels := meter.LabelSet(Label{Key: "resource", Value: "vm1"})

	cc.BeginCycle()
	defaultGauge.Observe(1, labels)
	scopedGauge.Observe(2, labels)
	require.NoError(t, cc.CommitCycleSuccess())

	mustValue(t, store.Read(), "azure.requests", Labels{"resource": "vm1"}, 1)
	mustValue(t, store.Read(ReadHostScope(scope.ScopeKey)), "azure.requests", Labels{"resource": "vm1"}, 2)

	_, ok := store.Read().Value("azure.requests", Labels{"resource": "vm2"})
	require.False(t, ok)
	_, ok = store.Read(ReadHostScope(scope.ScopeKey)).Value("azure.requests", Labels{"resource": "vm2"})
	require.False(t, ok)

	scopes := store.Read().HostScopes()
	require.Len(t, scopes, 2)
	require.True(t, scopes[0].IsDefault())
	require.Equal(t, scope, scopes[1])

	filteredScopes := store.Read(ReadHostScope(scope.ScopeKey)).HostScopes()
	require.Equal(t, scopes, filteredScopes)

	var defaultID, scopedID SeriesID
	store.Read().ForEachSeriesIdentity(func(identity SeriesIdentity, _ SeriesMeta, name string, _ LabelView, _ SampleValue) {
		if name == "azure.requests" {
			defaultID = identity.ID
		}
	})
	store.Read(ReadHostScope(scope.ScopeKey)).ForEachSeriesIdentity(func(identity SeriesIdentity, _ SeriesMeta, name string, _ LabelView, _ SampleValue) {
		if name == "azure.requests" {
			scopedID = identity.ID
		}
	})
	require.NotEmpty(t, defaultID)
	require.NotEmpty(t, scopedID)
	require.NotEqual(t, defaultID, scopedID)
}

func TestHostScopeMetadataRefreshesRetainedSeries(t *testing.T) {
	store := NewCollectorStore()
	cc := cycleController(t, store)

	scopeV1 := HostScope{
		ScopeKey: "workload/api",
		GUID:     "guid-api",
		Hostname: "api",
		Labels:   map[string]string{"_vnode_type": "azure_workload"},
	}
	scopeV2 := HostScope{
		ScopeKey: "workload/api",
		GUID:     "guid-api",
		Hostname: "api-v2",
		Labels:   map[string]string{"_vnode_type": "azure_workload", "region": "eastus"},
	}
	meter := store.Write().SnapshotMeter("azure")
	labels := meter.LabelSet(Label{Key: "resource", Value: "vm1"})

	cc.BeginCycle()
	meter.WithHostScope(scopeV1).Gauge("requests").Observe(1, labels)
	meter.WithHostScope(scopeV1).Gauge("errors").Observe(2, labels)
	require.NoError(t, cc.CommitCycleSuccess())

	cc.BeginCycle()
	meter.WithHostScope(scopeV2).Gauge("requests").Observe(3, labels)
	require.NoError(t, cc.CommitCycleSuccess())

	require.Equal(t, []HostScope{scopeV2}, store.Read().HostScopes())

	snapshot := store.(*storeView).core.snapshot.Load()
	seen := 0
	for _, series := range snapshot.series {
		if series.hostScopeKey != scopeV2.ScopeKey {
			continue
		}
		seen++
		require.Equal(t, scopeV2, series.hostScope)
	}
	require.Equal(t, 2, seen)
}

func TestHostScopeVecDerivationPartitionsSeries(t *testing.T) {
	store := NewCollectorStore()
	cc := cycleController(t, store)

	scopeA := HostScope{ScopeKey: "workload/api", GUID: "guid-api", Hostname: "api"}
	scopeB := HostScope{ScopeKey: "workload/db", GUID: "guid-db", Hostname: "db"}
	vec := store.Write().SnapshotMeter("azure").Vec("resource").Gauge("cpu")

	cc.BeginCycle()
	vec.WithLabelValues("vm1").Observe(10)
	vec.WithHostScope(scopeA).WithLabelValues("vm1").Observe(20)
	vec.WithHostScope(scopeB).WithLabelValues("vm1").Observe(30)
	require.NoError(t, cc.CommitCycleSuccess())

	mustValue(t, store.Read(), "azure.cpu", Labels{"resource": "vm1"}, 10)
	mustValue(t, store.Read(ReadHostScope(scopeA.ScopeKey)), "azure.cpu", Labels{"resource": "vm1"}, 20)
	mustValue(t, store.Read(ReadHostScope(scopeB.ScopeKey)), "azure.cpu", Labels{"resource": "vm1"}, 30)

	missingScopeReader := store.Read(ReadHostScope("workload/missing"))
	_, ok := missingScopeReader.Value("azure.cpu", Labels{"resource": "vm1"})
	require.False(t, ok)
	_, ok = missingScopeReader.Family("azure.cpu")
	require.False(t, ok)
}

func TestHostScopeFlattenPreservesScopeScenarios(t *testing.T) {
	cases := map[string]struct {
		run func(t *testing.T)
	}{
		"histogram": {
			run: func(t *testing.T) {
				store := NewCollectorStore()
				cc := cycleController(t, store)

				scope := HostScope{ScopeKey: "workload/api", GUID: "guid-api", Hostname: "api"}
				hist := store.Write().SnapshotMeter("svc").WithHostScope(scope).Histogram("latency")
				labels := store.Write().SnapshotMeter("").LabelSet(Label{Key: "route", Value: "/api"})

				cc.BeginCycle()
				hist.ObservePoint(HistogramPoint{
					Count: 3,
					Sum:   6,
					Buckets: []BucketPoint{
						{UpperBound: 1, CumulativeCount: 1},
						{UpperBound: 5, CumulativeCount: 3},
					},
				}, labels)
				require.NoError(t, cc.CommitCycleSuccess())

				defaultReader := store.Read(ReadFlatten())
				_, ok := defaultReader.Value("svc.latency_count", Labels{"route": "/api"})
				require.False(t, ok)

				scopedReader := store.Read(ReadFlatten(), ReadHostScope(scope.ScopeKey))
				mustValue(t, scopedReader, "svc.latency_count", Labels{"route": "/api"}, 3)
				mustValue(t, scopedReader, "svc.latency_sum", Labels{"route": "/api"}, 6)
				mustValue(t, scopedReader, "svc.latency_bucket", Labels{"route": "/api", HistogramBucketLabel: "1"}, 1)
				mustValue(t, scopedReader, "svc.latency_bucket", Labels{"route": "/api", HistogramBucketLabel: "5"}, 3)
			},
		},
		"composite instruments": {
			run: func(t *testing.T) {
				store := NewCollectorStore()
				cc := cycleController(t, store)

				scope := HostScope{ScopeKey: "workload/api", GUID: "guid-api", Hostname: "api"}
				meter := store.Write().SnapshotMeter("svc")
				labels := meter.LabelSet(Label{Key: "route", Value: "/api"})
				summary := meter.WithHostScope(scope).Summary("latency", WithSummaryQuantiles(0.5))
				stateSet := meter.WithHostScope(scope).StateSet("mode", WithStateSetStates("idle", "busy"))
				measureSet := meter.WithHostScope(scope).MeasureSetGauge(
					"usage",
					WithMeasureSetFields(
						MeasureFieldSpec{Name: "used"},
						MeasureFieldSpec{Name: "limit"},
					),
				)

				cc.BeginCycle()
				summary.ObservePoint(SummaryPoint{
					Count: 2,
					Sum:   1,
					Quantiles: []QuantilePoint{
						{Quantile: 0.5, Value: 0.4},
					},
				}, labels)
				stateSet.ObserveStateSet(StateSetPoint{States: map[string]bool{"busy": true}}, labels)
				measureSet.ObserveFields(map[string]SampleValue{"used": 7, "limit": 10}, labels)
				require.NoError(t, cc.CommitCycleSuccess())

				defaultReader := store.Read(ReadFlatten())
				_, ok := defaultReader.Value("svc.latency_count", Labels{"route": "/api"})
				require.False(t, ok)
				_, ok = defaultReader.Value("svc.mode", Labels{"route": "/api", "svc.mode": "busy"})
				require.False(t, ok)
				_, ok = defaultReader.Value("svc.usage_used", Labels{"route": "/api", MeasureSetFieldLabel: "used"})
				require.False(t, ok)

				scopedReader := store.Read(ReadFlatten(), ReadHostScope(scope.ScopeKey))
				mustValue(t, scopedReader, "svc.latency_count", Labels{"route": "/api"}, 2)
				mustValue(t, scopedReader, "svc.latency_sum", Labels{"route": "/api"}, 1)
				mustValue(t, scopedReader, "svc.latency", Labels{"route": "/api", SummaryQuantileLabel: "0.5"}, 0.4)
				mustValue(t, scopedReader, "svc.mode", Labels{"route": "/api", "svc.mode": "idle"}, 0)
				mustValue(t, scopedReader, "svc.mode", Labels{"route": "/api", "svc.mode": "busy"}, 1)
				mustValue(t, scopedReader, "svc.usage_used", Labels{"route": "/api", MeasureSetFieldLabel: "used"}, 7)
				mustValue(t, scopedReader, "svc.usage_limit", Labels{"route": "/api", MeasureSetFieldLabel: "limit"}, 10)
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, tc.run)
	}
}

func TestHostScopeStatefulBaselinesArePerScope(t *testing.T) {
	store := NewCollectorStore()
	cc := cycleController(t, store)

	scopeA := HostScope{ScopeKey: "workload/api", GUID: "guid-api", Hostname: "api"}
	scopeB := HostScope{ScopeKey: "workload/db", GUID: "guid-db", Hostname: "db"}
	meter := store.Write().StatefulMeter("svc")
	gaugeA := meter.WithHostScope(scopeA).Gauge("load")
	gaugeB := meter.WithHostScope(scopeB).Gauge("load")
	counterA := meter.WithHostScope(scopeA).Counter("requests")
	counterB := meter.WithHostScope(scopeB).Counter("requests")
	labels := meter.LabelSet(Label{Key: "resource", Value: "vm1"})

	cc.BeginCycle()
	gaugeA.Add(1, labels)
	gaugeB.Add(10, labels)
	counterA.Add(1, labels)
	counterB.Add(10, labels)
	require.NoError(t, cc.CommitCycleSuccess())

	cc.BeginCycle()
	gaugeA.Add(2, labels)
	gaugeB.Add(3, labels)
	counterA.Add(2, labels)
	counterB.Add(3, labels)
	require.NoError(t, cc.CommitCycleSuccess())

	mustValue(t, store.Read(ReadHostScope(scopeA.ScopeKey)), "svc.load", Labels{"resource": "vm1"}, 3)
	mustValue(t, store.Read(ReadHostScope(scopeB.ScopeKey)), "svc.load", Labels{"resource": "vm1"}, 13)
	mustValue(t, store.Read(ReadHostScope(scopeA.ScopeKey)), "svc.requests", Labels{"resource": "vm1"}, 3)
	mustValue(t, store.Read(ReadHostScope(scopeB.ScopeKey)), "svc.requests", Labels{"resource": "vm1"}, 13)
	mustDelta(t, store.Read(ReadHostScope(scopeA.ScopeKey)), "svc.requests", Labels{"resource": "vm1"}, 2)
	mustDelta(t, store.Read(ReadHostScope(scopeB.ScopeKey)), "svc.requests", Labels{"resource": "vm1"}, 3)

	_, ok := store.Read().Value("svc.load", Labels{"resource": "vm1"})
	require.False(t, ok)
}

func TestHostScopeStatefulHistogramCumulativeWindowIsPerScope(t *testing.T) {
	store := NewCollectorStore()
	cc := cycleController(t, store)

	scopeA := HostScope{ScopeKey: "workload/api", GUID: "guid-api", Hostname: "api"}
	scopeB := HostScope{ScopeKey: "workload/db", GUID: "guid-db", Hostname: "db"}
	meter := store.Write().StatefulMeter("svc")
	histA := meter.WithHostScope(scopeA).Histogram("latency", WithHistogramBounds(1, 2))
	histB := meter.WithHostScope(scopeB).Histogram("latency", WithHistogramBounds(1, 2))
	labels := meter.LabelSet(Label{Key: "resource", Value: "vm1"})

	cc.BeginCycle()
	histA.Observe(0.5, labels)
	histA.Observe(1.5, labels)
	histB.Observe(3, labels)
	require.NoError(t, cc.CommitCycleSuccess())

	cc.BeginCycle()
	histA.Observe(0.2, labels)
	histB.Observe(0.8, labels)
	require.NoError(t, cc.CommitCycleSuccess())

	mustHistogram(t, store.Read(ReadHostScope(scopeA.ScopeKey)), "svc.latency", Labels{"resource": "vm1"}, HistogramPoint{
		Count: 3,
		Sum:   2.2,
		Buckets: []BucketPoint{
			{UpperBound: 1, CumulativeCount: 2},
			{UpperBound: 2, CumulativeCount: 3},
		},
	})
	mustHistogram(t, store.Read(ReadHostScope(scopeB.ScopeKey)), "svc.latency", Labels{"resource": "vm1"}, HistogramPoint{
		Count: 2,
		Sum:   3.8,
		Buckets: []BucketPoint{
			{UpperBound: 1, CumulativeCount: 1},
			{UpperBound: 2, CumulativeCount: 1},
		},
	})
}

func TestHostScopeConflictScenarios(t *testing.T) {
	cases := map[string]struct {
		run func(t *testing.T)
	}{
		"commit fails without publishing": {
			run: func(t *testing.T) {
				store := NewCollectorStore()
				cc := cycleController(t, store)

				scopeA := HostScope{ScopeKey: "workload/api", GUID: "guid-api", Hostname: "api"}
				scopeB := HostScope{ScopeKey: "workload/api", GUID: "guid-api", Hostname: "api-v2"}
				meter := store.Write().SnapshotMeter("azure")
				a := meter.WithHostScope(scopeA).Gauge("requests")
				b := meter.WithHostScope(scopeB).Gauge("requests")

				cc.BeginCycle()
				a.Observe(1)
				b.Observe(2)
				err := cc.CommitCycleSuccess()
				require.Error(t, err)
				require.True(t, errors.Is(err, ErrHostScopeConflict), "unexpected error: %v", err)

				meta := store.Read().CollectMeta()
				require.Equal(t, CollectStatusFailed, meta.LastAttemptStatus)
				require.Equal(t, uint64(1), meta.LastAttemptSeq)
				require.Equal(t, uint64(0), meta.LastSuccessSeq)

				_, ok := store.Read(ReadRaw(), ReadHostScope(scopeA.ScopeKey)).Value("azure.requests", nil)
				require.False(t, ok)
			},
		},
		"multiple conflicts join errors": {
			run: func(t *testing.T) {
				store := NewCollectorStore()
				cc := cycleController(t, store)

				scopeA := HostScope{ScopeKey: "workload/api", GUID: "guid-api", Hostname: "api"}
				scopeAConflict := HostScope{ScopeKey: "workload/api", GUID: "guid-api", Hostname: "api-v2"}
				scopeB := HostScope{ScopeKey: "workload/db", GUID: "guid-db", Hostname: "db"}
				scopeBConflict := HostScope{ScopeKey: "workload/db", GUID: "guid-db-v2", Hostname: "db"}
				meter := store.Write().SnapshotMeter("azure")

				cc.BeginCycle()
				meter.WithHostScope(scopeA).Gauge("requests").Observe(1)
				meter.WithHostScope(scopeAConflict).Gauge("requests").Observe(2)
				meter.WithHostScope(scopeB).Gauge("requests").Observe(3)
				meter.WithHostScope(scopeBConflict).Gauge("requests").Observe(4)
				err := cc.CommitCycleSuccess()
				require.Error(t, err)
				require.True(t, errors.Is(err, ErrHostScopeConflict), "unexpected error: %v", err)
				require.Contains(t, err.Error(), "scope_key=\"workload/api\"")
				require.Contains(t, err.Error(), "scope_key=\"workload/db\"")
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, tc.run)
	}
}

func TestHostScopeConcurrentWrites(t *testing.T) {
	store := NewCollectorStore()
	cc := cycleController(t, store)

	scopes := []HostScope{
		{ScopeKey: "workload/api", GUID: "guid-api", Hostname: "api"},
		{ScopeKey: "workload/db", GUID: "guid-db", Hostname: "db"},
		{ScopeKey: "workload/cache", GUID: "guid-cache", Hostname: "cache"},
	}
	meter := store.Write().StatefulMeter("svc")
	counters := make([]StatefulCounter, len(scopes))
	for i, scope := range scopes {
		counters[i] = meter.WithHostScope(scope).Counter("requests")
	}

	const writersPerScope = 8
	const writesPerWriter = 100

	var wg sync.WaitGroup
	cc.BeginCycle()
	for i := range scopes {
		for range writersPerScope {
			wg.Go(func() {
				for range writesPerWriter {
					counters[i].Add(1)
				}
			})
		}
	}
	wg.Wait()
	require.NoError(t, cc.CommitCycleSuccess())

	for _, scope := range scopes {
		mustValue(t, store.Read(ReadHostScope(scope.ScopeKey)), "svc.requests", nil, writersPerScope*writesPerWriter)
	}
}
