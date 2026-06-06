// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"os"
	"testing"
)

// pkg/prometheus parses an exposition on every scrape across ~19 importing
// collectors, so it is a hot path. Keep these results updated before/after parser
// changes so regressions stay visible.
//
// The payload is the real exposition fixtures the parser tests use:
// testdata/testdata.txt (a go-process scrape of gauges/counters/summaries) plus
// testdata/histogram-meta.txt (the histogram path), so the benchmark exercises
// every assembler branch on representative data.
//
// Command:
//
//	go test ./pkg/prometheus -run '^$' -bench parseTo -benchmem
//
// Measured on a developer laptop (Apple M-series, 14 logical CPUs), not CI, so the
// absolute numbers are machine-specific — compare relative before/after deltas.
//
// Legacy fused parser (master, pre-rewrite — captured 2026-06-05):
//
//	parseToMetricFamilies   127868 ns/op    73404 B/op   1314 allocs/op
//	parseToSeries           111382 ns/op   100444 B/op   1584 allocs/op
//
// Unified driver+assembler stream parser (this rewrite — captured 2026-06-05):
//
//	parseToMetricFamilies   133911 ns/op    73408 B/op   1314 allocs/op
//	parseToSeries           112803 ns/op   100448 B/op   1584 allocs/op
//	parseToStream           113248 ns/op   102807 B/op   1644 allocs/op
//
// No allocation win and no allocation regression: the legacy parser already
// parsed once per call and reused buffers, and both Scrape and ScrapeSeries match
// it exactly (flat allocs). ScrapeSeries uses the raw-label path (identical to
// legacy), so it is essentially free. Scrape/stream are a few percent slower on
// CPU — the per-sample Sample struct + the shared iterate indirection + assembler
// dispatch on the families/stream path (profiled: makeSample + applySample,
// inherent to the single-driver model, not a fixable hotspot). The win is the
// sample stream that metric relabeling consumes, not raw speed.

func readBenchData(tb testing.TB) []byte {
	tb.Helper()
	var data []byte
	for _, f := range []string{"testdata/testdata.txt", "testdata/histogram-meta.txt"} {
		b, err := os.ReadFile(f)
		if err != nil {
			tb.Fatal(err)
		}
		data = append(data, b...)
		data = append(data, '\n') // keep files separated when concatenated
	}
	return data
}

func BenchmarkPromTextParser_parseToMetricFamilies(b *testing.B) {
	data := readBenchData(b)
	var p promTextParser
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for range b.N {
		if _, err := p.parseToMetricFamilies(data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPromTextParser_parseToSeries(b *testing.B) {
	data := readBenchData(b)
	var p promTextParser
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for range b.N {
		if _, err := p.parseToSeries(data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPromTextParser_parseToStream(b *testing.B) {
	data := readBenchData(b)
	var p promTextParser
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for range b.N {
		if err := p.parseToStream(data, nil, func(Sample) error { return nil }); err != nil {
			b.Fatal(err)
		}
	}
}
