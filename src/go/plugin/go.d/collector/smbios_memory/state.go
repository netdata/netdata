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

type discrepancy struct {
	Locator string `json:"locator"`
	Missing bool   `json:"missing"`
	Deficit uint64 `json:"deficit_bytes"`
}

type persistentState struct {
	Version    int                `json:"version"`
	Owner      string             `json:"owner"`
	AcceptedAt time.Time          `json:"accepted_at"`
	Baseline   []inventory.Device `json:"baseline"`
	Loss       []discrepancy      `json:"loss,omitempty"`
	LossAt     time.Time          `json:"loss_at,omitempty"`
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
	if s.Version != 1 || owner == "" || s.Owner != owner || s.AcceptedAt.IsZero() || len(s.Baseline) == 0 {
		return nil, errors.New("memory baseline has unsupported version, owner or content")
	}
	seen := make(map[string]uint64)
	for _, d := range s.Baseline {
		if d.Locator == "" || seen[d.Locator] != 0 || d.Capacity == nil || *d.Capacity == 0 ||
			d.Population != "populated" {
			return nil, errors.New("memory baseline contains an invalid slot")
		}
		seen[d.Locator] = *d.Capacity
	}
	lost := make(map[string]bool)
	for _, loss := range s.Loss {
		if seen[loss.Locator] == 0 || lost[loss.Locator] || loss.Deficit == 0 || loss.Deficit > seen[loss.Locator] ||
			(loss.Missing && loss.Deficit != seen[loss.Locator]) || s.LossAt.IsZero() {
			return nil, errors.New("memory baseline contains invalid loss evidence")
		}
		lost[loss.Locator] = true
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

func proposedState(current *persistentState, table *inventory.Table, owner string, now time.Time) *persistentState {
	var loss []discrepancy
	if current != nil {
		devices := make(map[string]inventory.Device, len(table.Devices))
		for _, d := range table.Devices {
			devices[d.Locator] = d
		}
		for _, old := range current.Baseline {
			d, exists := devices[old.Locator]
			var capacity uint64
			if exists {
				capacity = *d.Capacity
			}
			if capacity < *old.Capacity {
				loss = append(
					loss,
					discrepancy{
						Locator: old.Locator,
						Missing: !exists || d.Population == "empty",
						Deficit: *old.Capacity - capacity,
					},
				)
			}
		}
		if len(loss) > 0 {
			next := *current
			if !reflect.DeepEqual(loss, current.Loss) {
				next.Loss = loss
				next.LossAt = now
			}
			return &next
		}
	}
	// Accept additions only after proving that no previously accepted slot shrank.
	next := &persistentState{
		Version:    1,
		Owner:      owner,
		AcceptedAt: now,
	}
	for _, d := range table.Devices {
		if d.Population == "populated" {
			next.Baseline = append(next.Baseline, d)
		}
	}
	if current != nil && sameCapacities(current.Baseline, next.Baseline) {
		// Descriptive metadata changes are not baseline changes or state writes.
		next.Baseline = current.Baseline
		next.AcceptedAt = current.AcceptedAt
	}
	return next
}

func sameCapacities(a, b []inventory.Device) bool {
	if len(a) != len(b) {
		return false
	}
	bySlot := make(map[string]uint64, len(a))
	for _, d := range a {
		bySlot[d.Locator] = *d.Capacity
	}
	for _, d := range b {
		if capacity, ok := bySlot[d.Locator]; !ok || capacity != *d.Capacity {
			return false
		}
	}
	return true
}
