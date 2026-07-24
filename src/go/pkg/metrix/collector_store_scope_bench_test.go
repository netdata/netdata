// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"fmt"
	"strconv"
	"sync"
	"testing"
)

var benchmarkHostScopesSink []HostScope

func BenchmarkCollectorStoreFlattenProjectionCold(b *testing.B) {
	for _, instances := range []int{32, 512} {
		b.Run(fmt.Sprintf("structured_instances_%d", instances), func(b *testing.B) {
			store := benchmarkCommittedMixedStore(b, instances)
			snapshot := store.(*storeView).core.snapshot.Load()

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				flat := flattenSnapshot(snapshot)
				if flat.collectMeta.LastSuccessSeq == 0 {
					b.Fatal("expected committed snapshot")
				}
				benchmarkReaderCountSink = len(flat.series)
			}
		})
	}
}

func BenchmarkCollectorStoreFlattenProjectionWarm(b *testing.B) {
	for _, instances := range []int{32, 512} {
		b.Run(fmt.Sprintf("structured_instances_%d", instances), func(b *testing.B) {
			store := benchmarkCommittedMixedStore(b, instances)
			if meta := store.Read(ReadFlatten()).CollectMeta(); meta.LastSuccessSeq == 0 {
				b.Fatal("expected committed snapshot")
			}

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				reader := store.Read(ReadFlatten())
				if meta := reader.CollectMeta(); meta.LastSuccessSeq == 0 {
					b.Fatal("expected committed snapshot")
				}
			}
		})
	}
}

func BenchmarkCollectorStoreHostScopes(b *testing.B) {
	const totalSeries = 4096

	for _, totalScopes := range []int{1, 8, 64, 512} {
		b.Run(fmt.Sprintf("series_%d/scopes_%d", totalSeries, totalScopes), func(b *testing.B) {
			store := benchmarkCommittedScopedScalarStore(b, totalSeries, totalScopes)
			reader := store.Read(ReadRaw())
			scopes := reader.HostScopes()
			if len(scopes) != totalScopes {
				b.Fatalf("expected %d scopes, got %d", totalScopes, len(scopes))
			}

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				scopes = reader.HostScopes()
				if len(scopes) != totalScopes {
					b.Fatalf("expected %d scopes, got %d", totalScopes, len(scopes))
				}
				benchmarkHostScopesSink = scopes
			}
		})
	}
}

func BenchmarkCollectorStoreFlattenFanoutWarm(b *testing.B) {
	const totalSeries = 4096

	for _, totalScopes := range []int{1, 8, 64, 512} {
		b.Run(fmt.Sprintf("series_%d/scopes_%d", totalSeries, totalScopes), func(b *testing.B) {
			benchmarkCollectorStoreFlattenFanoutWarm(b, totalSeries, totalScopes)
		})
	}
}

func BenchmarkCollectorStoreFlattenFanoutBySeriesWarm(b *testing.B) {
	const totalScopes = 32

	for _, totalSeries := range []int{32, 512, 8192} {
		b.Run(fmt.Sprintf("scopes_%d/series_%d", totalScopes, totalSeries), func(b *testing.B) {
			benchmarkCollectorStoreFlattenFanoutWarm(b, totalSeries, totalScopes)
		})
	}
}

func BenchmarkCollectorStoreFlattenProjectionVisibilityCold(b *testing.B) {
	const (
		totalSeries = 4096
		totalScopes = 64
	)

	for _, mode := range []string{"fresh", "stale", "committed", "failed_attempt"} {
		b.Run(mode, func(b *testing.B) {
			store, cycle, _ := benchmarkScopedScalarFixture(b, totalSeries, totalScopes, mode == "committed")
			switch mode {
			case "stale":
				cycle.BeginCycle()
				if err := cycle.CommitCycleSuccess(); err != nil {
					b.Fatalf("commit stale visibility fixture: %v", err)
				}
			case "failed_attempt":
				cycle.BeginCycle()
				cycle.AbortCycle()
			}
			snapshot := store.(*storeView).core.snapshot.Load()

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				flat := flattenSnapshot(snapshot)
				if len(flat.series) != totalSeries {
					b.Fatalf("expected %d series, got %d", totalSeries, len(flat.series))
				}
				benchmarkReaderCountSink = len(flat.series)
			}
		})
	}
}

func BenchmarkCollectorStoreFlattenProjectionConcurrentFirst(b *testing.B) {
	const (
		totalSeries = 4096
		totalScopes = 64
		readers     = 32
	)

	store, cycle, writes := benchmarkScopedScalarFixture(b, totalSeries, totalScopes, false)
	results := make([]Reader, readers)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		b.StopTimer()
		cycle.BeginCycle()
		for i, write := range writes {
			write(SampleValue(i))
		}
		if err := cycle.CommitCycleSuccess(); err != nil {
			b.Fatalf("commit concurrent projection fixture: %v", err)
		}

		start := make(chan struct{})
		var ready, done sync.WaitGroup
		ready.Add(readers)
		done.Add(readers)
		for i := range readers {
			go func() {
				defer done.Done()
				ready.Done()
				<-start
				results[i] = store.Read(ReadFlatten())
			}()
		}
		ready.Wait()

		b.StartTimer()
		close(start)
		done.Wait()
		b.StopTimer()

		for _, reader := range results {
			if reader.CollectMeta().LastSuccessSeq == 0 {
				b.Fatal("expected committed snapshot")
			}
		}
		b.StartTimer()
	}
}

func BenchmarkCollectorStoreScopedIteration(b *testing.B) {
	const targetSeries = 16

	for _, foreignSeries := range []int{0, 1024, 16384} {
		b.Run(fmt.Sprintf("target_%d/foreign_%d", targetSeries, foreignSeries), func(b *testing.B) {
			store, targetScopeKey := benchmarkCommittedForeignScopeStore(b, targetSeries, foreignSeries)
			reader := store.Read(ReadRaw(), ReadHostScope(targetScopeKey))
			count := benchmarkCountReaderSeries(reader)
			if count != targetSeries {
				b.Fatalf("expected %d target series, got %d", targetSeries, count)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				count = benchmarkCountReaderSeries(reader)
				if count != targetSeries {
					b.Fatalf("expected %d target series, got %d", targetSeries, count)
				}
				benchmarkReaderCountSink = count
			}
		})
	}
}

func benchmarkCommittedScopedScalarStore(b *testing.B, totalSeries, totalScopes int) CollectorStore {
	b.Helper()
	store, _, _ := benchmarkScopedScalarFixture(b, totalSeries, totalScopes, false)
	return store
}

func benchmarkScopedScalarFixture(b *testing.B, totalSeries, totalScopes int, committed bool) (CollectorStore, CycleController, []func(SampleValue)) {
	b.Helper()
	if totalScopes < 1 || totalSeries < totalScopes {
		b.Fatalf("invalid scoped fixture: %d series across %d scopes", totalSeries, totalScopes)
	}

	store := NewCollectorStore()
	cycle := benchmarkCycleController(b, store)
	scopes := benchmarkHostScopes(totalScopes)
	writes := make([]func(SampleValue), totalSeries)

	if committed {
		gaugeVec := store.Write().StatefulMeter("reader.scoped").Vec("id").Gauge("value")
		for i := range totalSeries {
			scope := scopes[i%totalScopes]
			scopedGaugeVec := gaugeVec
			if !scope.IsDefault() {
				scopedGaugeVec = gaugeVec.WithHostScope(scope)
			}
			handle, err := scopedGaugeVec.GetWithLabelValues(strconv.Itoa(i))
			if err != nil {
				b.Fatalf("create scoped stateful gauge handle: %v", err)
			}
			writes[i] = func(value SampleValue) {
				handle.Set(value)
			}
		}
	} else {
		gaugeVec := store.Write().SnapshotMeter("reader.scoped").Vec("id").Gauge("value")
		for i := range totalSeries {
			scope := scopes[i%totalScopes]
			scopedGaugeVec := gaugeVec
			if !scope.IsDefault() {
				scopedGaugeVec = gaugeVec.WithHostScope(scope)
			}
			handle, err := scopedGaugeVec.GetWithLabelValues(strconv.Itoa(i))
			if err != nil {
				b.Fatalf("create scoped snapshot gauge handle: %v", err)
			}
			writes[i] = func(value SampleValue) {
				handle.Observe(value)
			}
		}
	}

	cycle.BeginCycle()
	for i, write := range writes {
		write(SampleValue(i))
	}
	if err := cycle.CommitCycleSuccess(); err != nil {
		b.Fatalf("commit scoped store: %v", err)
	}
	return store, cycle, writes
}

func benchmarkCollectorStoreFlattenFanoutWarm(b *testing.B, totalSeries, totalScopes int) {
	b.Helper()

	store := benchmarkCommittedScopedScalarStore(b, totalSeries, totalScopes)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		discovery := store.Read(ReadRaw(), ReadFlatten())
		scopes := discovery.HostScopes()
		if len(scopes) != totalScopes {
			b.Fatalf("expected %d scopes, got %d", totalScopes, len(scopes))
		}
		for _, scope := range scopes {
			fresh := store.Read(ReadFlatten(), ReadHostScope(scope.ScopeKey))
			raw := store.Read(ReadRaw(), ReadFlatten(), ReadHostScope(scope.ScopeKey))
			if fresh.CollectMeta().LastSuccessSeq == 0 || raw.CollectMeta().LastSuccessSeq == 0 {
				b.Fatal("expected committed snapshot")
			}
		}
	}
}

func benchmarkCommittedForeignScopeStore(b *testing.B, targetSeries, foreignSeries int) (CollectorStore, string) {
	b.Helper()

	store := NewCollectorStore()
	cycle := benchmarkCycleController(b, store)
	gaugeVec := store.Write().SnapshotMeter("reader.foreign").Vec("id").Gauge("value")
	targetScope := HostScope{ScopeKey: "target", GUID: "target-guid", Hostname: "target"}
	foreignScope := HostScope{ScopeKey: "foreign", GUID: "foreign-guid", Hostname: "foreign"}
	handles := make([]SnapshotGauge, 0, targetSeries+foreignSeries)

	for i := range targetSeries {
		handle, err := gaugeVec.WithHostScope(targetScope).GetWithLabelValues("target-" + strconv.Itoa(i))
		if err != nil {
			b.Fatalf("create target gauge handle: %v", err)
		}
		handles = append(handles, handle)
	}
	for i := range foreignSeries {
		handle, err := gaugeVec.WithHostScope(foreignScope).GetWithLabelValues("foreign-" + strconv.Itoa(i))
		if err != nil {
			b.Fatalf("create foreign gauge handle: %v", err)
		}
		handles = append(handles, handle)
	}

	cycle.BeginCycle()
	for i, handle := range handles {
		handle.Observe(SampleValue(i))
	}
	if err := cycle.CommitCycleSuccess(); err != nil {
		b.Fatalf("commit foreign-scope store: %v", err)
	}
	return store, targetScope.ScopeKey
}

func benchmarkHostScopes(totalScopes int) []HostScope {
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
	return scopes
}

func benchmarkCountReaderSeries(reader Reader) int {
	count := 0
	reader.ForEachSeries(func(string, LabelView, SampleValue) {
		count++
	})
	return count
}
