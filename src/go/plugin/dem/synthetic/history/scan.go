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

func (s *Store) scanSynthetic(ctx context.Context, f RunFilter, runID string, visit func(string, synthetic.Run)) error {
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
		if f.Kind != "" && run.Kind != f.Kind {
			return nil
		}
		if phase == "start" {
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
func decodeSynthetic(entry *sdk.SnapshotEntry) (string, synthetic.Run, bool, error) {
	var envelope demjournal.Envelope
	values := make(map[string]string, 4)
	err := entry.VisitPayloads(func(payload []byte) error {
		key, value, ok := bytes.Cut(payload, []byte{'='})
		if !ok {
			return fmt.Errorf("invalid journal field payload")
		}
		if err := envelope.Observe(key, value); err != nil {
			return err
		}
		switch name := string(key); name {
		case "DEM_PHASE", "DEM_JOB_ID", "DEM_RUN_ID", "DEM_DATA":
			if _, exists := values[name]; exists {
				return fmt.Errorf("duplicate synthetic history field %s", name)
			}
			values[name] = string(value)
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
	for _, name := range []string{"DEM_PHASE", "DEM_JOB_ID", "DEM_RUN_ID", "DEM_DATA"} {
		if values[name] == "" {
			return "", synthetic.Run{}, false, fmt.Errorf("missing synthetic history field %s", name)
		}
	}
	var run synthetic.Run
	if err := json.Unmarshal([]byte(values["DEM_DATA"]), &run); err != nil {
		return "", run, false, fmt.Errorf("decode synthetic history: %w", err)
	}
	phase := values["DEM_PHASE"]
	if (phase != "start" && phase != "complete") || run.ID != values["DEM_RUN_ID"] || run.JobID != values["DEM_JOB_ID"] || run.StartedUS != started {
		return "", run, false, fmt.Errorf("invalid synthetic journal identity or start time")
	}
	if phase == "complete" && run.CompletedUS <= 0 {
		return "", run, false, fmt.Errorf("invalid synthetic completion time")
	}
	return phase, run, true, nil
}
