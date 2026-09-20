// SPDX-License-Identifier: GPL-3.0-or-later

package smbios_memory

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/smbios_memory/internal/inventory"
)

const stateVersion = 1

// discrepancy is one accepted slot that is missing or smaller than accepted.
type discrepancy struct {
	Locator string `json:"locator"`
	Missing bool   `json:"missing"`
	Deficit uint64 `json:"deficit_bytes"`
}

// persistentState is the private envelope saved under the Agent's varlib directory:
// the accepted populated slots and the loss last confirmed against them.
type persistentState struct {
	Version    int                `json:"version"`
	Owner      string             `json:"owner"`
	AcceptedAt time.Time          `json:"accepted_at"`
	Baseline   []inventory.Device `json:"baseline"`
	Loss       []discrepancy      `json:"loss,omitempty"`
	LossAt     time.Time          `json:"loss_at,omitempty"`
}

func (s *persistentState) validate(owner string) error {
	if s.Version != stateVersion || owner == "" || s.Owner != owner || s.AcceptedAt.IsZero() || len(s.Baseline) == 0 {
		return errors.New("memory baseline has unsupported version, owner or content")
	}
	accepted := make(map[string]uint64, len(s.Baseline))
	for _, d := range s.Baseline {
		if d.Locator == "" || accepted[d.Locator] != 0 || d.Capacity == nil || *d.Capacity == 0 ||
			d.Population != inventory.PopulationPopulated {
			return errors.New("memory baseline contains an invalid slot")
		}
		accepted[d.Locator] = *d.Capacity
	}
	lost := make(map[string]bool, len(s.Loss))
	for _, l := range s.Loss {
		capacity := accepted[l.Locator]
		if capacity == 0 || lost[l.Locator] || l.Deficit == 0 || l.Deficit > capacity ||
			(l.Missing && l.Deficit != capacity) || s.LossAt.IsZero() {
			return errors.New("memory baseline contains invalid loss evidence")
		}
		lost[l.Locator] = true
	}
	return nil
}

// baselineStore owns the state file of the running canonical job. A read-only
// store (a debug run from a terminal) still loads and compares, and applies
// transitions in memory, but never writes the file the running Agent owns.
type baselineStore struct {
	path     string
	owner    string
	readOnly bool

	loaded  bool
	current *persistentState // nil until a baseline has been accepted and saved
	err     error            // load failure or missing Agent identity; persists until restart
	dirty   bool             // observed loss that could not be saved yet
}

func (b *baselineStore) exists() bool {
	_, err := os.Stat(b.path)
	return !errors.Is(err, os.ErrNotExist)
}

// load reads the state file once, on the first active collection. Init and Check
// also run for a replacement candidate while the previous job may still own the
// file; the first active Collect runs after that owner has stopped.
func (b *baselineStore) load() {
	if b.loaded {
		return
	}
	b.loaded = true
	if b.owner == "" {
		b.err = errors.New("Agent registry identity is unavailable; baseline persistence disabled")
		return
	}
	b.current, b.err = readState(b.path, b.owner)
	if errors.Is(b.err, os.ErrNotExist) {
		b.err = nil
	}
}

// reconcile compares a comparable table with the accepted baseline and saves
// validated transitions. Unchanged observations do not rewrite the file.
func (b *baselineStore) reconcile(table *inventory.Table, now time.Time) error {
	if b.err != nil || (b.current == nil && table.Populated == 0) {
		return nil
	}
	next := proposedState(b.current, table, b.owner, now)
	if !b.dirty && reflect.DeepEqual(b.current, next) {
		return nil
	}
	if b.readOnly {
		b.current = next
		return nil
	}
	if err := saveState(b.path, next); err != nil {
		b.retainLoss(next)
		return err
	}
	b.current, b.dirty = next, false
	return nil
}

// retainLoss remembers observed loss even if storage is temporarily unavailable.
// Neither baseline increases nor restorations are accepted on failure.
func (b *baselineStore) retainLoss(next *persistentState) {
	if b.current == nil || len(next.Loss) == 0 {
		return
	}
	b.current.Loss, b.current.LossAt = next.Loss, next.LossAt
	b.dirty = true
}

// flush retries saving retained loss, including while the source stays unreadable.
func (b *baselineStore) flush() error {
	if !b.dirty {
		return nil
	}
	if err := saveState(b.path, b.current); err != nil {
		return err
	}
	b.dirty = false
	return nil
}

func readState(path, owner string) (*persistentState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s persistentState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("decode memory baseline: %w", err)
	}
	if err := s.validate(owner); err != nil {
		return nil, err
	}
	return &s, nil
}

// saveState commits a complete domain envelope. The Agent owns the existing
// directory; only this collector's running canonical job writes this private file.
func saveState(path string, state *persistentState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".smbios-memory-*")
	if err != nil {
		return fmt.Errorf("create baseline temporary file: %w", err)
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write baseline: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync baseline: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close baseline: %w", err)
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return fmt.Errorf("publish baseline: %w", err)
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("open baseline directory: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync baseline directory: %w", err)
	}
	return nil
}
