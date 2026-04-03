// SPDX-License-Identifier: GPL-3.0-or-later

package promscrapemodel

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// Baseline on 2026-03-26 before parser/assembler optimization work
// (Apple M4 Pro, go test -run '^$' -bench 'BenchmarkAssembler_' -benchmem):
// BenchmarkAssembler_ApplySample-14  17934     66919 ns/op   13459 B/op    274 allocs/op
//
// Current result after the 2026-03-26 fast-path and scratch-label work:
// BenchmarkAssembler_ApplySample-14  19768     61165 ns/op     498 B/op     62 allocs/op
//
// Parse + assemble comparison on 2026-03-26
// (Apple M4 Pro, go test -run '^$' -bench 'Benchmark(MetricFamilies_|Assembler_)' -benchmem):
// BenchmarkMetricFamilies_NewParseAndAssemble-14  8748   141640 ns/op    99051 B/op   1686 allocs/op
// BenchmarkMetricFamilies_FastParseAndAssemble-14 8972   131566 ns/op    67444 B/op   1276 allocs/op

func BenchmarkAssembler_ApplySample(b *testing.B) {
	samples := mustParseBenchSamples(b)
	a := NewAssembler()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		a.beginCycle()
		for _, sample := range samples {
			if err := a.ApplySample(sample); err != nil {
				b.Fatalf("ApplySample(): %v", err)
			}
		}
		_ = a.MetricFamilies()
	}
}

func BenchmarkMetricFamilies_NewParseAndAssemble(b *testing.B) {
	var (
		p Parser
		a Assembler
	)
	data := mustReadBenchFixture(b)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		a.beginCycle()
		err := p.ParseStreamWithMeta(
			data,
			func(name, help string) {
				a.applyHelp(name, help)
			},
			func(sample Sample) error {
				return a.ApplySample(sample)
			},
		)
		if err != nil {
			b.Fatalf("ParseStreamWithMeta(): %v", err)
		}
		_ = a.MetricFamilies()
	}
}

func BenchmarkMetricFamilies_FastParseAndAssemble(b *testing.B) {
	var (
		p scrapeFastParser
		a Assembler
	)
	data := mustReadBenchFixture(b)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		a.beginCycle()
		err := p.parseToAssembler(data, &a)
		if err != nil {
			b.Fatalf("parseToAssembler(): %v", err)
		}
		_ = a.MetricFamilies()
	}
}

func mustParseBenchSamples(b *testing.B) Samples {
	b.Helper()

	p := NewParser(nil)
	var samples Samples
	err := p.ParseStream(mustReadBenchFixture(b), func(sample Sample) error {
		samples.Add(sample)
		return nil
	})
	require.NoError(b, err)
	return samples
}

func mustReadBenchFixture(b *testing.B) []byte {
	b.Helper()

	data, err := os.ReadFile("../testdata/testdata.txt")
	require.NoError(b, err)
	return data
}
