// SPDX-License-Identifier: GPL-3.0-or-later
package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/systemd-journal-sdk/go/journal"
)

// AppendSyntheticRun writes one immutable start/completion record. An attempted
// append error has an uncertain disk outcome and must never trigger replay.
func (s *Store) AppendSyntheticRun(ctx context.Context, phase string, run synthetic.Run) (bool, error) {
	if phase != "start" && phase != "complete" {
		return false, fmt.Errorf("invalid synthetic history phase")
	}
	if run.ID == "" || run.JobID == "" || run.StartedUS <= 0 {
		return false, fmt.Errorf("invalid synthetic history identity")
	}
	if phase == "complete" && run.CompletedUS < run.StartedUS {
		return false, fmt.Errorf("invalid synthetic completion time")
	}
	data, err := json.Marshal(run)
	if err != nil {
		return false, err
	}
	if err = s.acquire(ctx); err != nil {
		return false, err
	}
	defer s.release()
	if s.closed {
		return false, os.ErrClosed
	}
	if s.log == nil {
		return false, fmt.Errorf("history journal unavailable after reopen failure")
	}
	if err = ctx.Err(); err != nil {
		return false, err
	}
	return true, s.log.Append([]journal.Field{
		journal.StringField("DEM_KIND", "synthetic"), journal.StringField("DEM_PHASE", phase),
		journal.StringField("DEM_JOB_ID", run.JobID), journal.StringField("DEM_RUN_ID", run.ID),
		journal.StringField("DEM_DATA", string(data)), journal.StringField("MESSAGE", "synthetic "+phase),
	}, s.host.EntryOptions())
}

func (s *Store) scanSynthetic(ctx context.Context, f synthetic.RunFilter, runID string, visit func(string, synthetic.Run)) error {
	reader, closeReader, err := s.openReader(ctx)
	if err != nil {
		return err
	}
	defer closeReader()
	if reader == nil {
		return ctx.Err()
	}
	before := int64(^uint64(0) >> 1)
	if f.Before != nil {
		before = *f.Before
	}
	if f.After < 0 || before < f.After {
		return fmt.Errorf("invalid synthetic saved-time range")
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		more, err := reader.Step()
		if err != nil {
			return err
		}
		if !more {
			return nil
		}
		saved, err := reader.GetRealtimeUsec()
		if err != nil {
			return err
		}
		if saved/1000000 < uint64(f.After) || saved/1000000 > uint64(before) {
			continue
		}
		var kind, phase, job, id string
		var data []byte
		err = reader.VisitEntryPayloads(func(payload []byte) error {
			key, value, ok := bytes.Cut(payload, []byte{'='})
			if !ok {
				return fmt.Errorf("invalid journal field payload")
			}
			switch string(key) {
			case "DEM_KIND":
				kind = string(value)
			case "DEM_PHASE":
				phase = string(value)
			case "DEM_JOB_ID":
				job = string(value)
			case "DEM_RUN_ID":
				id = string(value)
			case "DEM_DATA":
				data = append(data[:0], value...)
			}
			return nil
		})
		if err != nil {
			return err
		}
		if kind != "synthetic" || (f.JobID != "" && job != f.JobID) || (runID != "" && id != runID) {
			continue
		}
		var run synthetic.Run
		if err = json.Unmarshal(data, &run); err != nil {
			return fmt.Errorf("decode synthetic history: %w", err)
		}
		if (phase != "start" && phase != "complete") || run.ID != id || run.JobID != job {
			return fmt.Errorf("invalid synthetic journal identity")
		}
		if f.Kind != "" && run.Kind != f.Kind {
			continue
		}
		if phase == "start" {
			run.Outcome = synthetic.Unknown
			run.CompletedUS = 0
			run.DurationMS = nil
		}
		visit(phase, run)
	}
}

// QuerySyntheticRuns groups phases before outcome filtering. A retained start
// without a selected completion is unknown. Queries include retired jobs.
// Memory is O(selected runs); the SDK snapshot additionally owns entry offsets.
func (s *Store) QuerySyntheticRuns(ctx context.Context, f synthetic.RunFilter) (synthetic.RunPage, error) {
	runs := make(map[string]synthetic.Run)
	err := s.scanSynthetic(ctx, f, "", func(phase string, r synthetic.Run) {
		old, ok := runs[r.ID]
		if !ok || (phase == "complete" && r.CompletedUS >= old.CompletedUS) {
			r.Events = nil // Summary queries retain no full diagnostic timelines.
			runs[r.ID] = r
		}
	})
	if err != nil {
		return synthetic.RunPage{}, err
	}
	page := synthetic.RunPage{Runs: make([]synthetic.Run, 0, len(runs))}
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
func (s *Store) GetSyntheticRun(ctx context.Context, jobID, runID string) (synthetic.Run, error) {
	if runID == "" {
		return synthetic.Run{}, fmt.Errorf("run ID is required")
	}
	var found synthetic.Run
	ok := false
	err := s.scanSynthetic(ctx, synthetic.RunFilter{JobID: jobID}, runID, func(phase string, r synthetic.Run) {
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
