// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"testing"

	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	sdk "github.com/netdata/systemd-journal-sdk/go/journal"
	"github.com/stretchr/testify/require"
)

// The default fixture has 20k phase records. DEM_HISTORY_BENCH_RUNS allows a
// larger run count without changing normal test cost. Results include snapshot
// opening and the real JSON decoder/reducer; filesystem cache is uncontrolled.
func BenchmarkSyntheticRuns(b *testing.B) {
	runCount := 10000
	if value := os.Getenv("DEM_HISTORY_BENCH_RUNS"); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || n < 1000 {
			b.Fatal("DEM_HISTORY_BENCH_RUNS must be an integer of at least 1000")
		}
		runCount = n
	}
	ctx := context.Background()
	journal, err := demjournal.Open(ctx, b.TempDir())
	require.NoError(b, err)
	b.Cleanup(func() { require.NoError(b, journal.Close()) })
	store := NewStore(journal)
	const start = int64(1700000000000000)
	for i := 0; i < runCount; i++ {
		run := synthetic.Run{
			ID: fmt.Sprintf("run-%08d", i), JobID: "journey:benchmark", Kind: synthetic.Journey,
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
	for _, width := range []struct {
		name    string
		seconds int64
	}{{"narrow", 899}, {"broad", int64(runCount) * 60}} {
		after := start / 1000000
		before := after + width.seconds
		filter := RunFilter{After: after, Before: &before, Limit: 2000}
		expected, err := store.QueryRuns(ctx, filter)
		require.NoError(b, err)
		baseline, err := scanRunsBaseline(ctx, journal, filter)
		require.NoError(b, err)
		require.Equal(b, expected, baseline)
		for _, path := range []string{"indexed", "scan"} {
			b.Run(width.name+"/"+path, func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					var page RunPage
					var err error
					if path == "indexed" {
						page, err = store.QueryRuns(ctx, filter)
					} else {
						page, err = scanRunsBaseline(ctx, journal, filter)
					}
					if err != nil || len(page.Runs) != len(expected.Runs) || page.Truncated != expected.Truncated {
						b.Fatalf("query mismatch: len=%d truncated=%v err=%v", len(page.Runs), page.Truncated, err)
					}
				}
			})
		}
	}
}

// This test-only exact scan is the cost baseline, not a production fallback.
// It deliberately shares decoding and mirrors phase reduction so timing isolates
// candidate selection. Independent expected-row semantics live in store_test.go.
func scanRunsBaseline(ctx context.Context, journal *demjournal.Store, filter RunFilter) (RunPage, error) {
	reader, closeReader, err := journal.OpenReader(ctx)
	if err != nil {
		return RunPage{}, err
	}
	defer closeReader()
	runs := make(map[string]synthetic.Run)
	err = reader.VisitEntries(func(entry *sdk.SnapshotEntry) error {
		phase, run, valid, err := decodeSynthetic(entry)
		if err != nil {
			return err
		}
		if !valid || run.StartedUS/1000000 < filter.After || run.StartedUS/1000000 > *filter.Before {
			return nil
		}
		if phase == "start" {
			run.CompletedUS = 0
			run.DurationMS = nil
			run.Outcome = synthetic.Unknown
		}
		old, found := runs[run.ID]
		if !found || (phase == "complete" && run.CompletedUS >= old.CompletedUS) {
			run.Events = nil
			runs[run.ID] = run
		}
		return nil
	})
	if err != nil {
		return RunPage{}, err
	}
	page := RunPage{Runs: make([]synthetic.Run, 0, len(runs))}
	for _, run := range runs {
		page.Runs = append(page.Runs, run)
	}
	sort.Slice(page.Runs, func(i, j int) bool {
		if page.Runs[i].StartedUS != page.Runs[j].StartedUS {
			return page.Runs[i].StartedUS > page.Runs[j].StartedUS
		}
		return page.Runs[i].ID < page.Runs[j].ID
	})
	if len(page.Runs) > filter.Limit {
		page.Truncated = true
		page.Runs = page.Runs[:filter.Limit]
	}
	return page, nil
}
