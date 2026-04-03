// SPDX-License-Identifier: GPL-3.0-or-later

package promscrapemodel

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// Baseline on 2026-03-26 before parser/assembler optimization work
// (Apple M4 Pro, go test -run '^$' -bench 'BenchmarkParser_' -benchmem):
// BenchmarkParser_ParseStream-14     10000    117591 ns/op   93053 B/op   1506 allocs/op
//
// Current result after the first safe optimization pass on 2026-03-26:
// BenchmarkParser_ParseStream-14      9807    156266 ns/op   93054 B/op   1506 allocs/op

func BenchmarkParser_ParseStream(b *testing.B) {
	data := mustReadBenchData(b)
	var p Parser

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		err := p.ParseStream(data, func(Sample) error { return nil })
		require.NoError(b, err)
	}
}

func mustReadBenchData(b *testing.B) []byte {
	b.Helper()

	data, err := os.ReadFile("../testdata/testdata.txt")
	require.NoError(b, err)
	return data
}
