// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

// QueryRuns groups phases before outcome filtering. A retained start
// without a selected completion is unknown. Queries include retired jobs.
// Memory is O(selected runs); the SDK snapshot additionally owns entry offsets.
func (s *Store) QueryRuns(ctx context.Context, f RunFilter) (RunPage, error) {
	runs := make(map[string]synthetic.Run)
	err := s.scanSynthetic(ctx, f, "", func(phase string, r synthetic.Run) {
		old, ok := runs[r.ID]
		if !ok || (phase == "complete" && r.CompletedUS >= old.CompletedUS) {
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
	sort.Slice(page.Runs, func(i, j int) bool {
		a, b := page.Runs[i], page.Runs[j]
		if a.StartedUS != b.StartedUS {
			return a.StartedUS > b.StartedUS
		}
		return a.ID < b.ID
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
	}, runID, func(phase string, r synthetic.Run) {
		if !ok || (phase == "complete" && r.CompletedUS >= found.CompletedUS) {
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
