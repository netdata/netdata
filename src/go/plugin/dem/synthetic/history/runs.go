// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"slices"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

// QueryRuns groups phases before outcome filtering. A retained start
// without a retained completion is unknown. Selection uses immutable run start
// time; completion knowledge can be newer than the interval. Retired jobs remain
// queryable. Memory is O(selected runs), with bounded SDK traversal buffers.
func (s *Store) QueryRuns(ctx context.Context, f RunFilter) (RunPage, error) {
	runs := make(map[string]synthetic.Run)
	err := s.scanSynthetic(ctx, f, "", func(phase synthetic.HistoryPhase, r synthetic.Run) {
		old, ok := runs[r.ID]
		if !ok || (phase == synthetic.HistoryComplete && r.CompletedUS >= old.CompletedUS) {
			r.Events = nil // Summary queries retain no full diagnostic timelines.
			runs[r.ID] = r
		}
	})
	if err != nil {
		return RunPage{}, err
	}
	page := RunPage{
		Runs: make([]synthetic.Run, 0, len(runs)),
	}
	for _, r := range runs {
		if f.Outcome == "" || r.Outcome == f.Outcome {
			page.Runs = append(page.Runs, r)
		}
	}
	slices.SortFunc(page.Runs, func(a, b synthetic.Run) int {
		if c := cmp.Compare(b.StartedUS, a.StartedUS); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})
	if f.Limit > 0 && len(page.Runs) > f.Limit {
		page.Truncated = true
		page.Runs = page.Runs[:f.Limit]
	}
	return page, ctx.Err()
}
func (s *Store) GetRun(ctx context.Context, jobID, runID string) (synthetic.Run, error) {
	if runID == "" {
		return synthetic.Run{}, fmt.Errorf("run ID is required")
	}
	var found synthetic.Run
	ok := false
	err := s.scanSynthetic(ctx, RunFilter{
		JobID: jobID,
	}, runID, func(phase synthetic.HistoryPhase, r synthetic.Run) {
		if !ok || (phase == synthetic.HistoryComplete && r.CompletedUS >= found.CompletedUS) {
			found = r
			ok = true
		}
	})
	if err != nil {
		return synthetic.Run{}, err
	}
	if !ok {
		return synthetic.Run{}, os.ErrNotExist
	}
	return found, ctx.Err()
}
