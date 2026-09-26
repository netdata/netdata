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

// stateVersion 2 identifies slots by bank and device locator; version 1 used
// the device locator alone and is upgraded on read.
const stateVersion = 2

// discrepancy is one accepted slot that is missing or smaller than accepted.
type discrepancy struct {
	Bank    string `json:"bank,omitempty"`
	Locator string `json:"locator"`
	Missing bool   `json:"missing"`
	Deficit uint64 `json:"deficit_bytes"`
}

func (l discrepancy) slot() slotKey {
	return slotKey{
		bank:    l.Bank,
		locator: l.Locator,
	}
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

	// locatorKeyed marks a state upgraded from version 1 until a comparable
	// table has been matched with it; see adoptBanks.
	locatorKeyed bool
}

func (s *persistentState) validate(owner string) error {
	if s.Version != stateVersion || owner == "" || s.Owner != owner || s.AcceptedAt.IsZero() || len(s.Baseline) == 0 {
		return errors.New("memory baseline has unsupported version, owner or content")
	}
	accepted := make(map[slotKey]uint64, len(s.Baseline))
	for _, d := range s.Baseline {
		slot := slotOf(d)
		if d.Locator == "" || accepted[slot] != 0 || d.Capacity == nil || *d.Capacity == 0 ||
			d.Population != inventory.PopulationPopulated {
			return errors.New("memory baseline contains an invalid slot")
		}
		accepted[slot] = *d.Capacity
	}
	lost := make(map[slotKey]bool, len(s.Loss))
	for _, l := range s.Loss {
		capacity := accepted[l.slot()]
		if capacity == 0 || lost[l.slot()] || l.Deficit == 0 || l.Deficit > capacity ||
			(l.Missing && l.Deficit != capacity) || s.LossAt.IsZero() {
			return errors.New("memory baseline contains invalid loss evidence")
		}
		lost[l.slot()] = true
	}
	return nil
}

// upgrade converts a version 1 envelope in memory. Version 1 required unique
// device locators, so each recorded loss names exactly one accepted slot, whose
// bank it takes. The file itself is rewritten only by the next state change.
func (s *persistentState) upgrade() {
	if s.Version != 1 {
		return
	}
	banks := make(map[string]string, len(s.Baseline))
	for _, d := range s.Baseline {
		if _, ok := banks[d.Locator]; ok {
			return // not a valid version 1 baseline; validation rejects it
		}
		banks[d.Locator] = d.Bank
	}
	for i := range s.Loss {
		s.Loss[i].Bank = banks[s.Loss[i].Locator]
	}
	s.Version = stateVersion
	s.locatorKeyed = true
}

// adoptBanks matches an upgraded version 1 state with the first comparable
// table the way version 1 did, by device locator, and takes the table's banks.
// Version 1 kept the bank it first saw as descriptive metadata, so firmware may
// have renamed it since. A table whose locators repeat cannot be matched that
// way, and the saved banks remain.
func (s *persistentState) adoptBanks(devices []inventory.Device) {
	if !s.locatorKeyed {
		return
	}
	s.locatorKeyed = false
	banks := make(map[string]string, len(devices))
	for _, d := range devices {
		if _, ok := banks[d.Locator]; ok {
			return
		}
		banks[d.Locator] = d.Bank
	}
	for i, d := range s.Baseline {
		if bank, ok := banks[d.Locator]; ok {
			s.Baseline[i].Bank = bank
		}
	}
	for i, l := range s.Loss {
		if bank, ok := banks[l.Locator]; ok {
			s.Loss[i].Bank = bank
		}
	}
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
	if b.current != nil {
		b.current.adoptBanks(table.Devices)
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
	s.upgrade()
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
