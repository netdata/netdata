// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

func (s *Store) scanSynthetic(ctx context.Context, f RunFilter, runID string, visit func(string, synthetic.Run)) error {
	reader, closeReader, err := s.journal.OpenReader(ctx)
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
