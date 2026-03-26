// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus/promscrapemodel"
)

// Baseline on 2026-03-26 before parser/assembler optimization work
// (Apple M4 Pro, go test -run '^$' -bench 'BenchmarkAssembler_' -benchmem):
// BenchmarkAssembler_ApplySample-14  17934     66919 ns/op   13459 B/op    274 allocs/op
//
// Current result after the first safe optimization pass on 2026-03-26:
// BenchmarkAssembler_ApplySample-14  18406     62760 ns/op    6674 B/op    249 allocs/op
//
// Legacy vs new assembled MetricFamilies comparison on 2026-03-26
// (Apple M4 Pro, go test -run '^$' -bench 'Benchmark(MetricFamilies_|Assembler_)' -benchmem):
// BenchmarkMetricFamilies_LegacyParse-14          9342   120182 ns/op    66914 B/op   1214 allocs/op
// BenchmarkMetricFamilies_NewParseAndAssemble-14  8155   145639 ns/op   105231 B/op   1873 allocs/op

func BenchmarkAssembler_ApplySample(b *testing.B) {
	samples := mustParseBenchSamples(b)
	a := NewAssembler()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		a.beginCycle()
		for _, sample := range samples {
			require.NoError(b, a.ApplySample(sample))
		}
		_ = a.MetricFamilies()
	}
}

func BenchmarkMetricFamilies_LegacyParse(b *testing.B) {
	var p promTextParser

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := p.parseToMetricFamilies(testData)
		require.NoError(b, err)
	}
}

func BenchmarkMetricFamilies_NewParseAndAssemble(b *testing.B) {
	var (
		p promscrapemodel.Parser
		a Assembler
	)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		a.beginCycle()
		err := p.ParseStreamWithMeta(
			testData,
			func(name, help string) {
				a.applyHelp(name, help)
			},
			func(sample promscrapemodel.Sample) error {
				return a.ApplySample(sample)
			},
		)
		require.NoError(b, err)
		_ = a.MetricFamilies()
	}
}

func mustParseBenchSamples(b *testing.B) promscrapemodel.Samples {
	b.Helper()

	p := promscrapemodel.NewParser(nil)
	samples, err := p.Parse(testData)
	require.NoError(b, err)
	return samples
}
