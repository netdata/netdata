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
// Unified driver+assembler parser (this rewrite — captured 2026-06-05):
//
//	parseToMetricFamilies   133911 ns/op    73408 B/op   1314 allocs/op
//	parseToSeries           112803 ns/op   100448 B/op   1584 allocs/op
//
// Scrape (parseToMetricFamilies) and ScrapeSeries keep the legacy allocation profile
// (flat allocs); ScrapeSeries is the raw-label path (identical to legacy, essentially
// free), Scrape a few percent slower on CPU (per-sample Sample + iterate indirection +
// assembler dispatch — makeSample/applySample, inherent to the single-driver model).
//
// parseToSamples is the streaming path metric relabeling uses: it buffers the flat,
// classified sample stream (ownLabels=true, so each sample owns a copy of its labels —
// the cost paid to mutate them safely) instead of folding in place; the collector then
// relabels and calls Assemble. It trades more allocation (the label copies) for skipping
// the in-parse fold (captured 2026-06-10, Apple M4 Pro):
//
//	parseToMetricFamilies   143348 ns/op    73412 B/op   1314 allocs/op
//	parseToSeries           118127 ns/op   100450 B/op   1584 allocs/op
//	parseToSamples          127885 ns/op   190735 B/op   1726 allocs/op

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

func BenchmarkPromTextParser_parseToSamples(b *testing.B) {
	data := readBenchData(b)
	var p promTextParser
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for range b.N {
		if _, err := p.parseToSamples(data); err != nil {
			b.Fatal(err)
		}
	}
}
