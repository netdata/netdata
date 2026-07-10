// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"fmt"
	"strconv"
	"testing"
)

var (
	benchmarkReaderValueSink SampleValue
	benchmarkReaderCountSink int
)

// Reader benchmarks guard lookup, flattening, and iteration paths before the
// package-layout refactor. Latest results are measured on a developer laptop,
// not CI, and should be treated as before/after trend indicators.
// Latest (developer laptop, -count=10): ValueLookup s1 76-79ns/2allocs, s1000
// 82-84ns/2; FlattenConstruction s10 52us/1135allocs, s100 586us/10706;
// ForEachSeriesIdentityRaw s100 371ns/2, s1000 4.7us/2; RuntimeOverlayValueLookup
// d1 76ns/2, d64 440-468ns/2.
func BenchmarkReaderValueLookup(b *testing.B) {
	tests := []int{1, 1000}

	for _, totalSeries := range tests {
		b.Run(fmt.Sprintf("series_%d", totalSeries), func(b *testing.B) {
			s := benchmarkCommittedScalarStore(b, totalSeries)
			r := s.Read()
			labels := Labels{"id": strconv.Itoa(totalSeries / 2)}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				v, ok := r.Value("reader.scalar.value", labels)
				if !ok {
					b.Fatal("expected value")
				}
				benchmarkReaderValueSink = v
			}
		})
	}
}

func BenchmarkReaderFlattenConstruction(b *testing.B) {
	tests := []int{10, 100}

	for _, totalSeries := range tests {
		b.Run(fmt.Sprintf("series_%d", totalSeries), func(b *testing.B) {
			s := benchmarkCommittedMixedStore(b, totalSeries)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				r := s.Read(ReadFlatten())
				meta := r.CollectMeta()
				if meta.LastSuccessSeq == 0 {
					b.Fatal("expected committed snapshot")
				}
			}
		})
	}
}

func BenchmarkReaderForEachSeriesIdentityRaw(b *testing.B) {
	tests := []int{100, 1000}

	for _, totalSeries := range tests {
		b.Run(fmt.Sprintf("series_%d", totalSeries), func(b *testing.B) {
			s := benchmarkCommittedScalarStore(b, totalSeries)
			r := s.Read()
			raw, ok := r.(SeriesIdentityRawIterator)
			if !ok {
				b.Fatal("reader does not expose raw identity iterator")
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				count := 0
				raw.ForEachSeriesIdentityRaw(func(_ SeriesIdentity, _ SeriesMeta, _ string, _ []Label, _ SampleValue) {
					count++
				})
				if count != totalSeries {
					b.Fatalf("expected %d series, got %d", totalSeries, count)
				}
				benchmarkReaderCountSink = count
			}
		})
	}
}

func BenchmarkRuntimeOverlayValueLookup(b *testing.B) {
	tests := []int{1, 64}

	for _, depth := range tests {
		b.Run(fmt.Sprintf("depth_%d", depth), func(b *testing.B) {
			s := benchmarkRuntimeOverlayStore(b, depth)
			r := s.Read()
			labels := Labels{"id": "0"}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				v, ok := r.Value("reader.runtime.value", labels)
				if !ok {
					b.Fatal("expected value")
				}
				benchmarkReaderValueSink = v
			}
		})
	}
}

func benchmarkCommittedScalarStore(b *testing.B, totalSeries int) CollectorStore {
	b.Helper()

	s := NewCollectorStore()
	cc := benchmarkCycleController(b, s)
	gv := s.Write().SnapshotMeter("reader.scalar").Vec("id").Gauge("value")
	handles := make([]SnapshotGauge, totalSeries)

	for i := range totalSeries {
		h, err := gv.GetWithLabelValues(strconv.Itoa(i))
		if err != nil {
			b.Fatalf("create gauge handle: %v", err)
		}
		handles[i] = h
	}

	cc.BeginCycle()
	for i, h := range handles {
		h.Observe(SampleValue(i))
	}
	if err := cc.CommitCycleSuccess(); err != nil {
		b.Fatalf("commit scalar store: %v", err)
	}

	return s
}

func benchmarkCommittedMixedStore(b *testing.B, totalSeries int) CollectorStore {
	b.Helper()

	s := NewCollectorStore()
	cc := benchmarkCycleController(b, s)
	m := s.Write().SnapshotMeter("reader.flatten")
	hv := m.Vec("id").Histogram("latency", WithHistogramBounds(1, 2, 5))
	sv := m.Vec("id").Summary("request_time", WithSummaryQuantiles(0.5, 0.9, 0.99))
	ssv := m.Vec("id").StateSet("mode", WithStateSetStates("maintenance", "operational"), WithStateSetMode(ModeEnum))
	msv := m.Vec("id").MeasureSetGauge("payload", WithMeasureSetFields(
		MeasureFieldSpec{Name: "bytes", Float: false},
		MeasureFieldSpec{Name: "rows", Float: false},
	))

	hists := make([]SnapshotHistogram, totalSeries)
	summaries := make([]SnapshotSummary, totalSeries)
	states := make([]StateSetInstrument, totalSeries)
	measureSets := make([]SnapshotMeasureSetGauge, totalSeries)

	for i := range totalSeries {
		id := strconv.Itoa(i)

		h, err := hv.GetWithLabelValues(id)
		if err != nil {
			b.Fatalf("create histogram handle: %v", err)
		}
		hists[i] = h

		sum, err := sv.GetWithLabelValues(id)
		if err != nil {
			b.Fatalf("create summary handle: %v", err)
		}
		summaries[i] = sum

		state, err := ssv.GetWithLabelValues(id)
		if err != nil {
			b.Fatalf("create stateset handle: %v", err)
		}
		states[i] = state

		measureSet, err := msv.GetWithLabelValues(id)
		if err != nil {
			b.Fatalf("create measureset handle: %v", err)
		}
		measureSets[i] = measureSet
	}

	cc.BeginCycle()
	for i := range totalSeries {
		v := SampleValue(i + 1)
		hists[i].ObservePoint(HistogramPoint{
			Count: v + 3,
			Sum:   v * 10,
			Buckets: []BucketPoint{
				{UpperBound: 1, CumulativeCount: v},
				{UpperBound: 2, CumulativeCount: v + 1},
				{UpperBound: 5, CumulativeCount: v + 3},
			},
		})
		summaries[i].ObservePoint(SummaryPoint{
			Count: v + 3,
			Sum:   v * 10,
			Quantiles: []QuantilePoint{
				{Quantile: 0.5, Value: v},
				{Quantile: 0.9, Value: v + 1},
				{Quantile: 0.99, Value: v + 2},
			},
		})
		states[i].ObserveStateSet(StateSetPoint{
			States: map[string]bool{
				"maintenance": false,
				"operational": true,
			},
		})
		measureSets[i].ObserveFields(map[string]SampleValue{
			"bytes": v * 100,
			"rows":  v,
		})
	}
	if err := cc.CommitCycleSuccess(); err != nil {
		b.Fatalf("commit mixed store: %v", err)
	}

	return s
}

func benchmarkRuntimeOverlayStore(b *testing.B, depth int) RuntimeStore {
	b.Helper()

	s := NewRuntimeStore()
	view, ok := s.(*runtimeStoreView)
	if !ok {
		b.Fatal("expected runtimeStoreView")
	}
	view.backend.compaction.maxOverlayDepth = 0
	view.backend.compaction.maxOverlayWrites = 0

	gv := s.Write().StatefulMeter("reader.runtime").Vec("id").Gauge("value")
	for i := range depth {
		h, err := gv.GetWithLabelValues(strconv.Itoa(i))
		if err != nil {
			b.Fatalf("create runtime gauge handle: %v", err)
		}
		h.Set(SampleValue(i))
	}

	return s
}
