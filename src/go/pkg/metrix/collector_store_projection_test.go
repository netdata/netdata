// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	projectionReaderSink Reader
	projectionScopesSink []HostScope
	projectionCountSink  int
)

func TestCollectorStoreFlattenProjectionIsSnapshotOwned(t *testing.T) {
	store := NewCollectorStore()
	cycle := cycleController(t, store)
	gauge := store.Write().SnapshotMeter("svc").Gauge("load")

	cycle.BeginCycle()
	gauge.Observe(1)
	require.NoError(t, cycle.CommitCycleSuccess())

	first := store.Read(ReadFlatten()).(*collectorReader)
	second := store.Read(ReadFlatten()).(*collectorReader)
	require.Same(t, first.snap, second.snap)

	rawSeries := store.(*storeView).core.snapshot.Load().series["svc.load"]
	flatSeries := first.snap.series["svc.load"]
	require.Same(t, rawSeries, flatSeries, "scalar projection must share immutable committed series")

	old := first
	cycle.BeginCycle()
	gauge.Observe(2)
	require.NoError(t, cycle.CommitCycleSuccess())

	current := store.Read(ReadFlatten()).(*collectorReader)
	require.NotSame(t, old.snap, current.snap)
	mustValue(t, old, "svc.load", nil, 1)
	mustValue(t, current, "svc.load", nil, 2)
}

func TestCollectorStoreConcurrentFlattenProjectionIsShared(t *testing.T) {
	store := NewCollectorStore()
	cycle := cycleController(t, store)
	gauge := store.Write().SnapshotMeter("svc").Gauge("load")

	cycle.BeginCycle()
	gauge.Observe(1)
	require.NoError(t, cycle.CommitCycleSuccess())

	const totalReaders = 32
	snapshots := make([]*readSnapshot, totalReaders)
	start := make(chan struct{})
	var wait sync.WaitGroup
	for i := range totalReaders {
		wait.Go(func() {
			<-start
			snapshots[i] = store.Read(ReadFlatten()).(*collectorReader).snap
		})
	}
	close(start)
	wait.Wait()

	for _, snapshot := range snapshots[1:] {
		require.Same(t, snapshots[0], snapshot)
	}
}

func TestCollectorStoreFreshVisibleHostScopesFollowExactSnapshot(t *testing.T) {
	store := NewCollectorStore()
	cycle := cycleController(t, store)
	scopeA := HostScope{ScopeKey: "scope-a", GUID: "guid-a", Hostname: "host-a", Labels: map[string]string{"scope": "a"}}
	scopeB := HostScope{ScopeKey: "scope-b", GUID: "guid-b", Hostname: "host-b", Labels: map[string]string{"scope": "b"}}
	scopeC := HostScope{ScopeKey: "scope-c", GUID: "guid-c", Hostname: "host-c", Labels: map[string]string{"scope": "c"}}
	snapshotMeter := store.Write().SnapshotMeter("svc")
	freshA := snapshotMeter.WithHostScope(scopeA).Gauge("fresh")
	staleC := snapshotMeter.WithHostScope(scopeC).Gauge("stale")
	committedB := store.Write().StatefulMeter("svc").WithHostScope(scopeB).Gauge("committed")

	cycle.BeginCycle()
	freshA.Observe(1)
	staleC.Observe(1)
	committedB.Set(1)
	require.NoError(t, cycle.CommitCycleSuccess())

	cycle.BeginCycle()
	freshA.Observe(2)
	require.NoError(t, cycle.CommitCycleSuccess())

	successReader := store.Read(ReadFlatten())
	successScopes := freshVisibleHostScopes(t, successReader)
	require.Equal(t, []HostScope{scopeA, scopeB}, successScopes)
	successScopes[0].Hostname = "mutated"
	successScopes[0].Labels["scope"] = "mutated"
	require.Equal(t, []HostScope{scopeA, scopeB}, freshVisibleHostScopes(t, successReader))

	cycle.BeginCycle()
	freshA.Observe(3)
	cycle.AbortCycle()

	failedReader := store.Read(ReadFlatten())
	require.Equal(t, []HostScope{scopeB}, freshVisibleHostScopes(t, failedReader))
	require.Equal(t, []HostScope{scopeA, scopeB}, freshVisibleHostScopes(t, successReader))
}

func TestCollectorStoreCommitErrorPublishesNewProjection(t *testing.T) {
	store := NewCollectorStore()
	cycle := cycleController(t, store)
	meter := store.Write().SnapshotMeter("svc")

	cycle.BeginCycle()
	meter.Gauge("load").Observe(1)
	require.NoError(t, cycle.CommitCycleSuccess())

	success := store.Read(ReadFlatten()).(*collectorReader)
	require.Equal(t, []HostScope{{}}, freshVisibleHostScopes(t, success))

	cycle.BeginCycle()
	meter.Gauge("load").Observe(2)
	meter.Counter("load").ObserveTotal(3)
	require.ErrorContains(t, cycle.CommitCycleSuccess(), "conflicting instrument kinds")

	failed := store.Read(ReadFlatten()).(*collectorReader)
	require.NotSame(t, success.snap, failed.snap)
	require.Same(t, failed.snap, store.Read(ReadFlatten()).(*collectorReader).snap)
	require.Equal(t, CollectStatusFailed, failed.CollectMeta().LastAttemptStatus)
	require.Empty(t, freshVisibleHostScopes(t, failed))
	mustValue(t, success, "svc.load", nil, 1)
	mustValue(t, store.Read(ReadRaw(), ReadFlatten()), "svc.load", nil, 1)
}

func TestRuntimeStoreFlattenProjectionRemainsPerRead(t *testing.T) {
	store := NewRuntimeStore()
	gauge := store.Write().StatefulMeter("runtime").Gauge("load")
	gauge.Set(1)

	first := store.Read(ReadFlatten()).(*storeReader)
	second := store.Read(ReadFlatten()).(*storeReader)

	assert.NotSame(t, first.snap, second.snap)
	_, ok := any(first).(FreshVisibleHostScopesReader)
	assert.False(t, ok)

	gauge.Set(2)
	current := store.Read(ReadFlatten())
	mustValue(t, first, "runtime.load", nil, 1)
	mustValue(t, current, "runtime.load", nil, 2)
}

func TestRetryableLazyPointerRetriesAfterPanic(t *testing.T) {
	var lazy retryableLazyPointer[int]
	builds := 0

	require.Panics(t, func() {
		lazy.get(func() *int {
			builds++
			panic("build failed")
		})
	})

	value := lazy.get(func() *int {
		builds++
		result := 42
		return &result
	})
	require.Equal(t, 42, *value)
	require.Equal(t, 2, builds)

	again := lazy.get(func() *int {
		t.Fatal("published lazy value was rebuilt")
		return nil
	})
	require.Same(t, value, again)
}

func TestCollectorStoreSnapshotIndexShape(t *testing.T) {
	const (
		totalSeries = 256
		totalScopes = 8
	)
	store := projectionScopedStore(t, totalSeries, totalScopes)
	reader := store.Read(ReadFlatten()).(*collectorReader)
	index := reader.snapshotIndex()

	require.Len(t, index.hostScopes, totalScopes)
	require.Len(t, index.freshVisibleHostScopes, totalScopes)
	require.True(t, index.hasDefaultScope)
	require.Len(t, index.byScope, totalScopes-1)

	indexedSeries := len(index.defaultScope.byName["projection.value"])
	require.Equal(t, []string{"projection.value"}, index.defaultScope.names)
	require.Empty(t, index.defaultScope.structuredMetaByName)
	for _, scope := range index.byScope {
		require.Equal(t, []string{"projection.value"}, scope.names)
		require.Empty(t, scope.structuredMetaByName)
		indexedSeries += len(scope.byName["projection.value"])
	}
	require.Equal(t, totalSeries, indexedSeries)
}

func TestCollectorStoreSnapshotIndexSeparatesStructuredMetadataFromScalarIteration(t *testing.T) {
	store := NewCollectorStore()
	cycle := cycleController(t, store)
	histogram := store.Write().SnapshotMeter("svc").Histogram("latency", WithHistogramBounds(1))

	cycle.BeginCycle()
	histogram.ObservePoint(HistogramPoint{
		Count:   1,
		Sum:     0.5,
		Buckets: []BucketPoint{{UpperBound: 1, CumulativeCount: 1}},
	})
	require.NoError(t, cycle.CommitCycleSuccess())

	scope := store.Read().(*collectorReader).scopeIndex()
	require.Empty(t, scope.byName)
	require.Empty(t, scope.names)
	require.Contains(t, scope.structuredMetaByName, "svc.latency")
}

func TestCollectorStoreWarmPathAllocationShape(t *testing.T) {
	small := projectionScopedStore(t, 32, 32)
	large := projectionScopedStore(t, 8192, 32)
	small.Read(ReadFlatten())
	large.Read(ReadFlatten())

	smallReadAllocs := testing.AllocsPerRun(100, func() {
		projectionReaderSink = small.Read(ReadFlatten())
	})
	largeReadAllocs := testing.AllocsPerRun(100, func() {
		projectionReaderSink = large.Read(ReadFlatten())
	})
	require.LessOrEqual(t, smallReadAllocs, float64(3))
	require.Equal(t, smallReadAllocs, largeReadAllocs)

	smallReader := small.Read(ReadRaw())
	largeReader := large.Read(ReadRaw())
	smallReader.HostScopes()
	largeReader.HostScopes()
	smallScopeAllocs := testing.AllocsPerRun(20, func() {
		projectionScopesSink = smallReader.HostScopes()
	})
	largeScopeAllocs := testing.AllocsPerRun(20, func() {
		projectionScopesSink = largeReader.HostScopes()
	})
	require.Equal(t, smallScopeAllocs, largeScopeAllocs)
}

func TestCollectorStoreScopedIterationDoesNotVisitForeignPartition(t *testing.T) {
	withoutForeign := projectionForeignScopeStore(t, 0)
	withForeign := projectionForeignScopeStore(t, 4096)
	withoutForeignReader := withoutForeign.Read(ReadRaw(), ReadHostScope("target"))
	withForeignReader := withForeign.Read(ReadRaw(), ReadHostScope("target"))
	require.Equal(t, 16, projectionCountSeries(withoutForeignReader))
	require.Equal(t, 16, projectionCountSeries(withForeignReader))

	indexed := withForeignReader.(*collectorReader)
	selected := indexed.scopeIndex()
	require.Same(t, indexed.snapshotIndex().byScope["target"], selected)
	require.Equal(t, 16, projectionCountIndexedSeries(selected))
	foreign := indexed.snapshotIndex().byScope["foreign"]
	require.Equal(t, 4096, projectionCountIndexedSeries(foreign))

	// Poison the foreign records after immutable partition publication. A late
	// global scan would now pass them through the target-scope visibility check
	// and invoke the callback; selected-partition traversal never visits them.
	for _, series := range foreign.byName {
		for _, item := range series {
			item.hostScopeKey = "target"
		}
	}
	globalVisible := 0
	for _, item := range indexed.snap.series {
		if item.desc != nil && isScalarKind(item.desc.kind) && indexed.visible(item) {
			globalVisible++
		}
	}
	require.Equal(t, 16+4096, globalVisible, "poison must make late global filtering observable")
	require.Equal(t, 16, projectionCountSeries(withForeignReader))

	withoutForeignAllocs := testing.AllocsPerRun(100, func() {
		projectionCountSink = projectionCountSeries(withoutForeignReader)
	})
	withForeignAllocs := testing.AllocsPerRun(100, func() {
		projectionCountSink = projectionCountSeries(withForeignReader)
	})
	require.Equal(t, withoutForeignAllocs, withForeignAllocs)
}

func freshVisibleHostScopes(t *testing.T, reader Reader) []HostScope {
	t.Helper()
	scopes, ok := reader.(FreshVisibleHostScopesReader)
	require.True(t, ok, "reader does not expose fresh-visible host scopes")
	return scopes.FreshVisibleHostScopes()
}

func projectionScopedStore(t *testing.T, totalSeries, totalScopes int) CollectorStore {
	t.Helper()
	require.GreaterOrEqual(t, totalSeries, totalScopes)

	store := NewCollectorStore()
	cycle := cycleController(t, store)
	gaugeVec := store.Write().SnapshotMeter("projection").Vec("id").Gauge("value")
	scopes := make([]HostScope, totalScopes)
	for i := 1; i < totalScopes; i++ {
		value := strconv.Itoa(i)
		scopes[i] = HostScope{
			ScopeKey: "scope-" + value,
			GUID:     "guid-" + value,
			Hostname: "host-" + value,
			Labels:   map[string]string{"scope": value},
		}
	}
	handles := make([]SnapshotGauge, totalSeries)
	for i := range totalSeries {
		scope := scopes[i%totalScopes]
		var err error
		if scope.IsDefault() {
			handles[i], err = gaugeVec.GetWithLabelValues(strconv.Itoa(i))
		} else {
			handles[i], err = gaugeVec.WithHostScope(scope).GetWithLabelValues(strconv.Itoa(i))
		}
		require.NoError(t, err)
	}

	cycle.BeginCycle()
	for i, handle := range handles {
		handle.Observe(SampleValue(i))
	}
	require.NoError(t, cycle.CommitCycleSuccess())
	return store
}

func projectionForeignScopeStore(t *testing.T, foreignSeries int) CollectorStore {
	t.Helper()

	store := NewCollectorStore()
	cycle := cycleController(t, store)
	gaugeVec := store.Write().SnapshotMeter("projection").Vec("id").Gauge("value")
	target := HostScope{ScopeKey: "target", GUID: "target-guid", Hostname: "target"}
	foreign := HostScope{ScopeKey: "foreign", GUID: "foreign-guid", Hostname: "foreign"}
	handles := make([]SnapshotGauge, 0, 16+foreignSeries)
	for i := range 16 {
		handle, err := gaugeVec.WithHostScope(target).GetWithLabelValues("target-" + strconv.Itoa(i))
		require.NoError(t, err)
		handles = append(handles, handle)
	}
	for i := range foreignSeries {
		handle, err := gaugeVec.WithHostScope(foreign).GetWithLabelValues("foreign-" + strconv.Itoa(i))
		require.NoError(t, err)
		handles = append(handles, handle)
	}

	cycle.BeginCycle()
	for i, handle := range handles {
		handle.Observe(SampleValue(i))
	}
	require.NoError(t, cycle.CommitCycleSuccess())
	return store
}

func projectionCountSeries(reader Reader) int {
	count := 0
	reader.ForEachSeries(func(string, LabelView, SampleValue) {
		count++
	})
	return count
}

func projectionCountIndexedSeries(index *snapshotScopeIndex) int {
	count := 0
	for _, series := range index.byName {
		count += len(series)
	}
	return count
}
