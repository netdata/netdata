// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"

	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	sdk "github.com/netdata/systemd-journal-sdk/go/journal"
)

func (s *Store) scanSynthetic(ctx context.Context, f RunFilter, runID string, visit func(synthetic.HistoryPhase, synthetic.Run)) error {
	before := int64(math.MaxInt64)
	if f.Before != nil {
		before = *f.Before
	}
	if f.After < 0 || before < f.After {
		return fmt.Errorf("invalid synthetic start-time range")
	}
	reader, closeReader, err := s.journal.OpenReader(ctx)
	if err != nil {
		return err
	}
	defer closeReader()
	if reader == nil {
		return ctx.Err()
	}
	candidate := func(entry *sdk.SnapshotEntry) error {
		phase, run, syntheticEntry, err := decodeSynthetic(entry)
		if err != nil {
			return err
		}
		if !syntheticEntry || (f.JobID != "" && run.JobID != f.JobID) || (runID != "" && run.ID != runID) {
			return nil
		}
		if runID == "" && (run.StartedUS/1000000 < f.After || run.StartedUS/1000000 > before) {
			return nil
		}
		if f.VnodeGUID != "" && run.VnodeGUID != f.VnodeGUID {
			return nil
		}
		if f.Kind != "" && run.Kind != f.Kind {
			return nil
		}
		if phase == synthetic.HistoryStart {
			run.Outcome = synthetic.Unknown
			run.CompletedUS = 0
			run.DurationMS = nil
		}
		visit(phase, run)
		return nil
	}
	if runID != "" {
		return reader.VisitMatch("DEM_RUN_ID", runID, candidate)
	}
	return reader.VisitRange("synthetic", f.After, before, candidate)
}

// Both phases carry their immutable start clock, so indexed membership includes
// a retained completion even when it was written after the selected interval.
func decodeSynthetic(entry *sdk.SnapshotEntry) (synthetic.HistoryPhase, synthetic.Run, bool, error) {
	var envelope demjournal.Envelope
	var phase, jobID, runID string
	var run synthetic.Run
	var seen uint8
	var hasData bool
	var dataErr error
	err := entry.VisitPayloads(func(payload []byte) error {
		key, value, ok := bytes.Cut(payload, []byte{'='})
		if !ok {
			return fmt.Errorf("invalid journal field payload")
		}
		if err := envelope.Observe(key, value); err != nil {
			return err
		}
		var bit uint8
		var target *string
		switch string(key) {
		case "DEM_PHASE":
			bit, target = 1, &phase
		case "DEM_JOB_ID":
			bit, target = 2, &jobID
		case "DEM_RUN_ID":
			bit, target = 4, &runID
		case "DEM_DATA":
			bit = 8
		default:
			return nil
		}
		if seen&bit != 0 {
			return fmt.Errorf("duplicate synthetic history field %s", key)
		}
		seen |= bit
		if target != nil {
			*target = string(value)
		} else {
			// Payload bytes are borrowed only for this callback. Decode directly;
			// the run owns the resulting strings, including its captured vnode GUID.
			hasData = len(value) != 0
			dataErr = json.Unmarshal(value, &run)
		}
		return nil
	})
	if err != nil {
		return "", synthetic.Run{}, false, err
	}
	kind, started, err := envelope.Validate()
	if err != nil || kind != "synthetic" {
		return "", synthetic.Run{}, false, err
	}
	for _, field := range []struct{ name, value string }{
		{"DEM_PHASE", phase}, {"DEM_JOB_ID", jobID}, {"DEM_RUN_ID", runID},
	} {
		if field.value == "" {
			return "", synthetic.Run{}, false, fmt.Errorf("missing synthetic history field %s", field.name)
		}
	}
	if !hasData {
		return "", synthetic.Run{}, false, fmt.Errorf("missing synthetic history field DEM_DATA")
	}
	if dataErr != nil {
		return "", run, false, fmt.Errorf("decode synthetic history: %w", dataErr)
	}
	if !validPhase(synthetic.HistoryPhase(phase)) || run.ID != runID || run.JobID != jobID || run.StartedUS != started {
		return "", run, false, fmt.Errorf("invalid synthetic journal identity or start time")
	}
	if synthetic.HistoryPhase(phase) == synthetic.HistoryComplete && run.CompletedUS <= 0 {
		return "", run, false, fmt.Errorf("invalid synthetic completion time")
	}
	return synthetic.HistoryPhase(phase), run, true, nil
}
