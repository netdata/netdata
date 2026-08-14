// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"fmt"
	"strconv"
	"sync"
	"testing"
)

var benchmarkHostScopesSink []HostScope

// Cold flatten builds both projected series and their lookup index. Label/key
// construction is O(source series + projected label/key bytes); index construction
// adds sum(k log k) label-key comparisons across scope/name groups plus scope/name
// catalog sorting. It retains no global cache, introduces no quadratic scan, and
// CollectorStore owns one projection per exact snapshot. The benchmarks keep setup
// outside the timer and verify the exact output count before timing so skipped
// projection work cannot look like an optimization.
//
// Reproducible comparison (darwin/arm64, Apple M4 Pro, 2026-07-27):
//   - merge-base production: beede2920e18ec0e0efa3c522367ce26deec1281
//   - optimized production: 1f59fa84ba593a0560663d1ce185542485eddf7d
//   - identical benchmark harness: 759296896a6758c2d8919df169a3d14807d6650d
//
// The merge base predates most of this matrix. The harness versions of
// collector_store_scope_bench_test.go, reader_bench_test.go, and
// summary_bench_test.go were therefore overlaid unchanged onto clean trees at
// both production revisions. Results are medians of -count=10; ns/op is a
// developer-laptop trend indicator, not a CI gate.
//
//	go test -run '^$' -bench 'FlattenProjectionCold$' -benchmem -benchtime=100ms -count=10 ./pkg/metrix
//
// Representative merge-base -> optimized results (`ns/op` is the local trend):
//   - scalar/512: ns/op +1.9%; 37,400 -> 37,400 B/op; 21 -> 21 allocs/op.
//   - mixed/512: ns/op -18.1%; 7,709,253 -> 7,182,939 B/op;
//     54,445 -> 45,741 allocs/op.
//   - histogram labels_8/512: ns/op -47.5%; 17,267,578 -> 10,579,147 B/op;
//     109,683 -> 63,603 allocs/op.
//   - summary fanout_8/labels_8/512: ns/op -44.1%; 12,206,783 -> 7,987,898 B/op;
//     72,786 -> 45,650 allocs/op.
//   - stateset fanout_8/labels_8/512: ns/op -42.7%; 10,526,587 -> 6,954,869 B/op;
//     57,402 -> 37,434 allocs/op.
//   - measureset gauge/counter fanout_8/labels_8/512:
//     ns/op -39.8%/-41.7%; 10,578,865/10,578,856 ->
//     7,465,884/7,465,880 B/op; 61,579 -> 45,195 allocs/op.
func BenchmarkCollectorStoreFlattenProjectionCold(b *testing.B) {
	for _, instances := range []int{32, 512} {
		b.Run(fmt.Sprintf("structured_instances_%d", instances), func(b *testing.B) {
			store := benchmarkCommittedMixedStore(b, instances)
			snapshot := store.(*storeView).core.snapshot.Load()
			benchmarkFlattenProjectionCold(b, snapshot, 15*instances)
		})
	}
}

func BenchmarkCollectorStoreScalarFlattenProjectionCold(b *testing.B) {
	for _, instances := range []int{32, 512} {
		b.Run(fmt.Sprintf("instances_%d", instances), func(b *testing.B) {
			store := benchmarkCommittedScalarStore(b, instances)
			snapshot := store.(*storeView).core.snapshot.Load()
			benchmarkFlattenProjectionCold(b, snapshot, instances)
		})
	}
}

func BenchmarkCollectorStoreHistogramFlattenProjectionCold(b *testing.B) {
	for _, labels := range []int{1, 8} {
		for _, instances := range []int{32, 512} {
			b.Run(fmt.Sprintf("labels_%d/instances_%d", labels, instances), func(b *testing.B) {
				store := benchmarkCommittedHistogramStore(b, instances, labels)
				snapshot := store.(*storeView).core.snapshot.Load()
				benchmarkFlattenProjectionCold(b, snapshot, 15*instances)
			})
		}
	}
}

func BenchmarkCollectorStoreSummaryNoQuantilesFlattenProjectionCold(b *testing.B) {
	for _, instances := range []int{32, 512} {
		b.Run(fmt.Sprintf("instances_%d", instances), func(b *testing.B) {
			store := benchmarkCommittedSummaryNoQuantilesStore(b, instances)
			snapshot := store.(*storeView).core.snapshot.Load()
			benchmarkFlattenProjectionCold(b, snapshot, 2*instances)
		})
	}
}

func BenchmarkCollectorStoreSummaryFlattenProjectionCold(b *testing.B) {
	benchmarkStructuredFlattenShapes(b, 3, func(b *testing.B, shape flattenProjectionBenchmarkShape) {
		store := benchmarkCommittedSummaryStore(b, shape.instances, shape.labels, shape.fanout)
		snapshot := store.(*storeView).core.snapshot.Load()
		benchmarkFlattenProjectionCold(b, snapshot, (shape.fanout+2)*shape.instances)
	})
}

func BenchmarkCollectorStoreStateSetFlattenProjectionCold(b *testing.B) {
	benchmarkStructuredFlattenShapes(b, 2, func(b *testing.B, shape flattenProjectionBenchmarkShape) {
		store := benchmarkCommittedStateSetStore(b, shape.instances, shape.labels, shape.fanout)
		snapshot := store.(*storeView).core.snapshot.Load()
		benchmarkFlattenProjectionCold(b, snapshot, shape.fanout*shape.instances)
	})
}

func BenchmarkCollectorStoreMeasureSetGaugeFlattenProjectionCold(b *testing.B) {
	benchmarkStructuredFlattenShapes(b, 2, func(b *testing.B, shape flattenProjectionBenchmarkShape) {
		store := benchmarkCommittedMeasureSetStore(b, shape.instances, shape.labels, shape.fanout, MeasureSetSemanticsGauge)
		snapshot := store.(*storeView).core.snapshot.Load()
		benchmarkFlattenProjectionCold(b, snapshot, shape.fanout*shape.instances)
	})
}

func BenchmarkCollectorStoreMeasureSetCounterFlattenProjectionCold(b *testing.B) {
	benchmarkStructuredFlattenShapes(b, 2, func(b *testing.B, shape flattenProjectionBenchmarkShape) {
		store := benchmarkCommittedMeasureSetStore(b, shape.instances, shape.labels, shape.fanout, MeasureSetSemanticsCounter)
		snapshot := store.(*storeView).core.snapshot.Load()
		benchmarkFlattenProjectionCold(b, snapshot, shape.fanout*shape.instances)
	})
}

func BenchmarkCollectorStoreFlattenProjectionWarm(b *testing.B) {
	for _, instances := range []int{32, 512} {
		b.Run(fmt.Sprintf("structured_instances_%d", instances), func(b *testing.B) {
			store := benchmarkCommittedMixedStore(b, instances)
			reader := store.Read(ReadFlatten())
			if meta := reader.CollectMeta(); meta.LastSuccessSeq == 0 {
				b.Fatal("expected committed snapshot")
			}
			if got := benchmarkCountReaderSeries(reader); got != 15*instances {
				b.Fatalf("expected %d flattened series, got %d", 15*instances, got)
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

type flattenProjectionBenchmarkShape struct {
	instances int
	labels    int
	fanout    int
}

var flattenProjectionBenchmarkLabelNames = []string{
	"instance",
	"job",
	"method",
	"namespace",
	"region",
	"service",
	"status",
	"zone",
}

func benchmarkStructuredFlattenShapes(
	b *testing.B,
	lowFanout int,
	run func(b *testing.B, shape flattenProjectionBenchmarkShape),
) {
	b.Helper()

	shapes := []flattenProjectionBenchmarkShape{
		{instances: 32, labels: 1, fanout: lowFanout},
		{instances: 512, labels: 1, fanout: lowFanout},
		{instances: 512, labels: 8, fanout: lowFanout},
		{instances: 512, labels: 8, fanout: 8},
	}
	for _, shape := range shapes {
		name := fmt.Sprintf("fanout_%d/labels_%d/instances_%d", shape.fanout, shape.labels, shape.instances)
		b.Run(name, func(b *testing.B) {
			run(b, shape)
		})
	}
}

func benchmarkFlattenProjectionCold(b *testing.B, snapshot *readSnapshot, wantSeries int) {
	b.Helper()
	requireFlattenProjectionSeries(b, snapshot, wantSeries)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchmarkReaderCountSink = len(flattenSnapshot(snapshot).series)
	}
}

func requireFlattenProjectionSeries(tb testing.TB, snapshot *readSnapshot, wantSeries int) {
	tb.Helper()

	flat := flattenSnapshot(snapshot)
	if flat.collectMeta.LastSuccessSeq == 0 {
		tb.Fatal("expected committed snapshot")
	}
	if got := len(flat.series); got != wantSeries {
		tb.Fatalf("expected %d flattened series, got %d", wantSeries, got)
	}
}

func benchmarkFlattenLabelNames(tb testing.TB, totalLabels int) []string {
	tb.Helper()

	if totalLabels < 1 || totalLabels > len(flattenProjectionBenchmarkLabelNames) {
		tb.Fatalf("invalid flatten fixture label count: %d", totalLabels)
	}
	return flattenProjectionBenchmarkLabelNames[:totalLabels]
}

func benchmarkFlattenLabelValues(totalLabels, series int) []string {
	values := make([]string, totalLabels)
	values[0] = strconv.Itoa(series)
	for i := 1; i < totalLabels; i++ {
		values[i] = fmt.Sprintf("value_%d", i)
	}
	return values
}

func benchmarkCommittedHistogramStore(tb testing.TB, totalSeries, totalLabels int) CollectorStore {
	tb.Helper()

	store := NewCollectorStore()
	cycle := benchmarkCycleController(tb, store)
	meter := store.Write().SnapshotMeter("reader.flatten")
	labelNames := benchmarkFlattenLabelNames(tb, totalLabels)
	vector := meter.Vec(labelNames...).Histogram(
		"latency",
		WithHistogramBounds(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30),
	)
	handles := make([]SnapshotHistogram, totalSeries)
	for i := range handles {
		labelValues := benchmarkFlattenLabelValues(totalLabels, i)
		handle, err := vector.GetWithLabelValues(labelValues...)
		if err != nil {
			tb.Fatalf("create histogram handle: %v", err)
		}
		handles[i] = handle
	}

	buckets := []BucketPoint{
		{UpperBound: 0.005, CumulativeCount: 1},
		{UpperBound: 0.01, CumulativeCount: 2},
		{UpperBound: 0.025, CumulativeCount: 3},
		{UpperBound: 0.05, CumulativeCount: 4},
		{UpperBound: 0.1, CumulativeCount: 5},
		{UpperBound: 0.25, CumulativeCount: 6},
		{UpperBound: 0.5, CumulativeCount: 7},
		{UpperBound: 1, CumulativeCount: 8},
		{UpperBound: 2.5, CumulativeCount: 9},
		{UpperBound: 5, CumulativeCount: 10},
		{UpperBound: 10, CumulativeCount: 11},
		{UpperBound: 30, CumulativeCount: 12},
	}
	cycle.BeginCycle()
	for _, handle := range handles {
		handle.ObservePoint(HistogramPoint{
			Count:   13,
			Sum:     42,
			Buckets: buckets,
		})
	}
	if err := cycle.CommitCycleSuccess(); err != nil {
		tb.Fatalf("commit histogram store: %v", err)
	}
	return store
}

func benchmarkCommittedSummaryNoQuantilesStore(tb testing.TB, totalSeries int) CollectorStore {
	tb.Helper()
	return benchmarkCommittedSummaryStoreWithLabels(tb, totalSeries, []string{"id"}, 0)
}

func benchmarkCommittedSummaryStore(tb testing.TB, totalSeries, totalLabels, totalQuantiles int) CollectorStore {
	tb.Helper()
	return benchmarkCommittedSummaryStoreWithLabels(
		tb,
		totalSeries,
		benchmarkFlattenLabelNames(tb, totalLabels),
		totalQuantiles,
	)
}

func benchmarkCommittedSummaryStoreWithLabels(
	tb testing.TB,
	totalSeries int,
	labelNames []string,
	totalQuantiles int,
) CollectorStore {
	tb.Helper()

	store := NewCollectorStore()
	cycle := benchmarkCycleController(tb, store)
	quantiles := make([]float64, totalQuantiles)
	for i := range quantiles {
		quantiles[i] = float64(i+1) / float64(totalQuantiles+1)
	}
	meter := store.Write().SnapshotMeter("reader.flatten").Vec(labelNames...)
	var vector SnapshotSummaryVec
	if len(quantiles) == 0 {
		vector = meter.Summary("duration")
	} else {
		vector = meter.Summary("duration", WithSummaryQuantiles(quantiles...))
	}
	handles := make([]SnapshotSummary, totalSeries)
	for i := range handles {
		handle, err := vector.GetWithLabelValues(benchmarkFlattenLabelValues(len(labelNames), i)...)
		if err != nil {
			tb.Fatalf("create summary handle: %v", err)
		}
		handles[i] = handle
	}

	cycle.BeginCycle()
	for i, handle := range handles {
		value := SampleValue(i + 1)
		point := SummaryPoint{Count: value, Sum: value * 10}
		for j, quantile := range quantiles {
			point.Quantiles = append(point.Quantiles, QuantilePoint{
				Quantile: quantile,
				Value:    value + SampleValue(j),
			})
		}
		handle.ObservePoint(point)
	}
	if err := cycle.CommitCycleSuccess(); err != nil {
		tb.Fatalf("commit summary store: %v", err)
	}
	return store
}

func benchmarkCommittedStateSetStore(tb testing.TB, totalSeries, totalLabels, totalStates int) CollectorStore {
	tb.Helper()

	store := NewCollectorStore()
	cycle := benchmarkCycleController(tb, store)
	states := make([]string, totalStates)
	for i := range states {
		states[i] = fmt.Sprintf("state_%d", i)
	}
	vector := store.Write().SnapshotMeter("reader.flatten").
		Vec(benchmarkFlattenLabelNames(tb, totalLabels)...).
		StateSet("mode", WithStateSetStates(states...), WithStateSetMode(ModeEnum))
	handles := make([]StateSetInstrument, totalSeries)
	for i := range handles {
		handle, err := vector.GetWithLabelValues(benchmarkFlattenLabelValues(totalLabels, i)...)
		if err != nil {
			tb.Fatalf("create stateset handle: %v", err)
		}
		handles[i] = handle
	}

	cycle.BeginCycle()
	for i, handle := range handles {
		handle.Enable(states[i%len(states)])
	}
	if err := cycle.CommitCycleSuccess(); err != nil {
		tb.Fatalf("commit stateset store: %v", err)
	}
	return store
}

func benchmarkCommittedMeasureSetStore(
	tb testing.TB,
	totalSeries, totalLabels, totalFields int,
	semantics MeasureSetSemantics,
) CollectorStore {
	tb.Helper()

	store := NewCollectorStore()
	cycle := benchmarkCycleController(tb, store)
	fields := make([]MeasureFieldSpec, totalFields)
	values := make([]SampleValue, totalFields)
	for i := range fields {
		fields[i] = MeasureFieldSpec{Name: fmt.Sprintf("field_%d", i)}
		values[i] = SampleValue(i + 1)
	}
	meter := store.Write().SnapshotMeter("reader.flatten").Vec(benchmarkFlattenLabelNames(tb, totalLabels)...)

	cycle.BeginCycle()
	switch semantics {
	case MeasureSetSemanticsGauge:
		vector := meter.MeasureSetGauge("payload", WithMeasureSetFields(fields...))
		for i := range totalSeries {
			handle, err := vector.GetWithLabelValues(benchmarkFlattenLabelValues(totalLabels, i)...)
			if err != nil {
				tb.Fatalf("create measureset gauge handle: %v", err)
			}
			handle.ObservePoint(MeasureSetPoint{Values: values})
		}
	case MeasureSetSemanticsCounter:
		vector := meter.MeasureSetCounter("payload", WithMeasureSetFields(fields...))
		for i := range totalSeries {
			handle, err := vector.GetWithLabelValues(benchmarkFlattenLabelValues(totalLabels, i)...)
			if err != nil {
				tb.Fatalf("create measureset counter handle: %v", err)
			}
			handle.ObserveTotalPoint(MeasureSetPoint{Values: values})
		}
	default:
		tb.Fatalf("unsupported measureset semantics: %d", semantics)
	}
	if err := cycle.CommitCycleSuccess(); err != nil {
		tb.Fatalf("commit measureset store: %v", err)
	}
	return store
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
			requireFlattenProjectionSeries(b, snapshot, totalSeries)

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				benchmarkReaderCountSink = len(flattenSnapshot(snapshot).series)
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
		if got := benchmarkCountReaderSeries(results[0]); got != totalSeries {
			b.Fatalf("expected %d flattened series, got %d", totalSeries, got)
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
