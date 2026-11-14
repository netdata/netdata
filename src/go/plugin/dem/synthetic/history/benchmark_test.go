// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"context"
	"fmt"
	"testing"

	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/stretchr/testify/require"
)

// BenchmarkSyntheticRuns measures QueryRuns over 10k runs (20k phase records),
// including snapshot opening and the real JSON decoder and reducer; filesystem
// cache is uncontrolled. Row semantics are tested in store_test.go.
func BenchmarkSyntheticRuns(b *testing.B) {
	const runCount = 10000
	ctx := context.Background()
	journal, err := demjournal.Open(ctx, b.TempDir())
	require.NoError(b, err)
	b.Cleanup(func() { require.NoError(b, journal.Close()) })
	store := NewStore(journal)
	const start = int64(1700000000000000)
	for i := range runCount {
		run := synthetic.Run{
			VnodeGUID: "11111111-1111-4111-8111-111111111111",
			ID:        fmt.Sprintf("run-%08d", i), JobID: "journey:benchmark", Kind: synthetic.Journey,
			StartedUS: start + int64(i)*60000000, Outcome: synthetic.Unknown,
		}
		_, err := store.AppendRun(ctx, "start", run)
		require.NoError(b, err)
		run.CompletedUS = run.StartedUS + 90000000
		run.Outcome = synthetic.Success
		_, err = store.AppendRun(ctx, "complete", run)
		require.NoError(b, err)
	}
	require.NoError(b, store.Sync(ctx))
	widths := map[string]int64{"narrow": 899, "broad": runCount * 60}
	for name, seconds := range widths {
		after := start / 1000000
		before := after + seconds
		filter := RunFilter{After: after, Before: &before, Limit: 2000}
		expected, err := store.QueryRuns(ctx, filter)
		require.NoError(b, err)
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				page, err := store.QueryRuns(ctx, filter)
				if err != nil || len(page.Runs) != len(expected.Runs) || page.Truncated != expected.Truncated {
					b.Fatalf("query mismatch: len=%d truncated=%v err=%v", len(page.Runs), page.Truncated, err)
				}
			}
		})
	}
}
