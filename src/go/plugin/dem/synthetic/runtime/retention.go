// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type reservation struct {
	Generation string `json:"generation"`
	Bytes      int64  `json:"bytes"`
}

type inventory struct {
	ownershipInventory
	selected         *Manifest
	release          *Release
	automaticUpdates bool
	selectionError   error
	generationErrors map[string]error
	generations      map[string]generation
	preparing        map[string]int
	reservations     map[string]reservation
}

// Status describes logical managed files and protection. ReservedBytes is the
// unwritten remainder of active reservations, not additional used disk bytes.
// ProtectedBytes and ReclaimableBytes count each owned file once. CandidateBytes,
// ProcessBytes,
// ExecutionBytes and UncertainBytes describe overlapping generation protection;
// uncertain bytes are a subset of execution bytes. PreparingBytes and TrashBytes
// describe written workspace, not extra disk usage.
type Status struct {
	SelectionError        string            `json:"selection_error,omitempty"`
	GenerationErrors      map[string]string `json:"generation_errors,omitempty"`
	InventoryError        string            `json:"inventory_error,omitempty"`
	Selected              *Manifest         `json:"selected,omitempty"`
	Release               *Release          `json:"release,omitempty"`
	AutomaticUpdates      bool              `json:"automatic_updates"`
	SelectedBytes         int64             `json:"selected_bytes"`
	ProcessClaims         int               `json:"process_claims"`
	UsedBytes             int64             `json:"used_bytes"`
	ReservedBytes         int64             `json:"reserved_bytes"`
	LimitBytes            int64             `json:"limit_bytes"`
	ProtectedBytes        int64             `json:"protected_bytes"`
	ReclaimableBytes      int64             `json:"reclaimable_bytes"`
	CandidateBytes        int64             `json:"candidate_bytes"`
	CandidateClaims       int               `json:"candidate_claims"`
	ProcessBytes          int64             `json:"process_bytes"`
	ExecutionBytes        int64             `json:"execution_bytes"`
	UncertainBytes        int64             `json:"uncertain_bytes"`
	PreparingBytes        int64             `json:"preparing_bytes"`
	TrashBytes            int64             `json:"trash_bytes"`
	ProcessGenerations    int               `json:"process_generations"`
	ExecutionPins         int               `json:"execution_pins"`
	UncertainPins         int               `json:"uncertain_pins"`
	BootRecoveryAvailable bool              `json:"boot_recovery_available"`
	CleanupError          string            `json:"cleanup_error,omitempty"`
}

func (s *Store) ownershipInventoryLocked(reconcile bool) (inventory, error) {
	owned, err := s.ownershipLedger.inventoryLocked(reconcile)
	inv := inventory{
		ownershipInventory: owned,
		generations:        make(map[string]generation),
		preparing:          make(map[string]int),
		reservations:       make(map[string]reservation),
	}
	if err != nil {
		return inv, err
	}
	staging, err := s.childDirectories("staging")
	if err != nil {
		return inv, err
	}
	for _, name := range staging {
		alive, err := s.ownerAlive(name)
		if err != nil {
			return inv, err
		}
		if !alive {
			inv.reclaimableRecords[name] = true
			if !reconcile {
				continue
			}
			if err := s.removeOwned(name); err != nil {
				return inv, err
			}
			continue
		}
		var r reservation
		if err := readJSON(s.root, filepath.Join(name, "owner.json"), &r); err != nil {
			return inv, err
		}
		if !validID(r.Generation) || r.Bytes <= 0 {
			return inv, errors.New("invalid runtime reservation")
		}
		inv.reservations[name] = r
		inv.protectedRecords[name] = true
		inv.preparing[r.Generation]++
	}
	return inv, nil
}

func (s *Store) statusLocked(inv inventory) (Status, error) {
	status := Status{
		Selected:              inv.selected,
		Release:               inv.release,
		AutomaticUpdates:      inv.automaticUpdates,
		LimitBytes:            s.config.MaxBytes,
		ProcessGenerations:    len(inv.processes),
		BootRecoveryAvailable: s.boot.Verified,
	}
	if inv.selectionError != nil {
		status.SelectionError = inv.selectionError.Error()
	}
	if len(inv.generationErrors) > 0 {
		status.GenerationErrors = make(map[string]string, len(inv.generationErrors))
		for id, err := range inv.generationErrors {
			status.GenerationErrors[id] = err.Error()
		}
	}
	if err := errors.Join(inv.cleanupError, s.cleanupError); err != nil {
		status.CleanupError = err.Error()
	}
	var err error
	var bytesByRecord map[string]int64
	status.UsedBytes, bytesByRecord, err = s.diskByteCounts(".")
	if err != nil {
		return status, err
	}
	for name, r := range inv.reservations {
		// Use the SAME observation for used and reserved bytes: writers run
		// outside exclusion, so two scans could undercount a growing operation.
		written := bytesByRecord[name]
		status.PreparingBytes += written
		if r.Bytes > written {
			status.ReservedBytes += r.Bytes - written
		}
	}
	for name, size := range bytesByRecord {
		if inv.protectedRecords[name] {
			status.ProtectedBytes += size
		}
		if inv.reclaimableRecords[name] || strings.HasPrefix(name, "trash/") {
			status.ReclaimableBytes += size
		}
		if strings.HasPrefix(name, "trash/") {
			status.TrashBytes += size
		}
	}
	for _, count := range inv.processes {
		status.ProcessClaims += count
	}
	for _, count := range inv.candidates {
		status.CandidateClaims += count
	}
	for id := range inv.generations {
		size := bytesByRecord[filepath.Join("generations", id)]
		if inv.selected != nil && inv.selected.Archive.SHA256 == id {
			status.SelectedBytes += size
		}
		if inv.candidates[id] > 0 {
			status.CandidateBytes += size
		}
		if inv.processes[id] > 0 {
			status.ProcessBytes += size
		}
		if inv.uses[id] > 0 {
			status.ExecutionBytes += size
		}
		if inv.uncertain[id] > 0 {
			status.UncertainBytes += size
		}
		if inv.protected(id) || inv.selectionError != nil || inv.generationErrors[id] != nil {
			status.ProtectedBytes += size
		} else {
			status.ReclaimableBytes += size
		}
	}
	for id, count := range inv.uses {
		status.ExecutionPins += count
		status.UncertainPins += inv.uncertain[id]
	}
	return status, nil
}

// Inspect is read-only. It never executes programs, starts preparation or
// changes persisted grace deadlines.
func (s *Store) Inspect() (Status, error) {
	status := Status{
		LimitBytes:            s.config.MaxBytes,
		BootRecoveryAvailable: s.boot.Verified,
	}
	err := s.withLock(func() error {
		inv, err := s.inspectInventoryLocked(false)
		if err != nil {
			status.InventoryError = err.Error()
			return err
		}
		status, err = s.statusLocked(inv)
		if err != nil {
			status.InventoryError = err.Error()
		}
		return errors.Join(err, inv.damageError())
	})
	if err != nil && status.SelectionError == "" && len(status.GenerationErrors) == 0 && status.InventoryError == "" {
		status.InventoryError = err.Error()
	}
	return status, err
}

// Sweep retires unselected releases without process or execution ownership.
// Failed cleanup is separate from the ability to use a matching runtime.
func (s *Store) Sweep() error {
	var cleanupVersion uint64
	err := s.withLock(func() error {
		cleanupVersion = s.cleanupErrorVersion
		if err := s.retryOwnerReleasesLocked(); err != nil {
			return err
		}
		inv, err := s.inventoryLocked(true)
		if err != nil {
			return err
		}
		for _, id := range s.candidates(inv, false) {
			if err := s.removeOwned(filepath.Join("generations", id)); err != nil {
				return err
			}
		}
		return inv.cleanupError
	})
	result := errors.Join(err, s.flushTrash())
	s.finishSweep(cleanupVersion, result)
	return result
}

func (s *Store) candidates(inv inventory, pressure bool) []string {
	var ids []string
	for id := range inv.generations {
		if !inv.protected(id) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		left, right := inv.generations[ids[i]], inv.generations[ids[j]]
		if left.LastUsed.Equal(right.LastUsed) {
			return ids[i] < ids[j]
		}
		return left.LastUsed.Before(right.LastUsed)
	})
	if pressure || len(ids) == 0 {
		return ids
	}
	// Keep the most recently used obsolete generation for a bounded rollback
	// grace. The selected installation never participates in retirement.
	last := ids[len(ids)-1]
	grace := s.config.PreviousRetention
	if s.now().Before(inv.generations[last].Unneeded.Add(grace)) {
		ids = ids[:len(ids)-1]
	}
	return ids
}

func (s *Store) diskBytes(name string) (int64, error) {
	total, _, err := s.diskByteCounts(name)
	return total, err
}

func (s *Store) diskByteCounts(name string) (int64, map[string]int64, error) {
	root, err := s.root.OpenRoot(name)
	if err != nil {
		return 0, nil, err
	}
	defer root.Close()
	records := make(map[string]int64)
	var total int64
	err = fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("nonregular runtime store entry: %s", path)
		}
		if info.Size() < 0 || total > int64(^uint64(0)>>1)-info.Size() {
			return errors.New("runtime byte accounting overflow")
		}
		total += info.Size()
		parent, rest, found := strings.Cut(path, "/")
		if found {
			id, _, child := strings.Cut(rest, "/")
			if child {
				records[parent+"/"+id] += info.Size()
			}
		}
		return nil
	})
	return total, records, err
}

// Every source parent is synced again before successor deletion. Visibility of
// a trash rename alone is not crash durability. Trash is never executable state.
func (s *Store) flushTrash() error {
	var names []string
	err := s.withLock(func() error {
		if err := s.syncSelection(); err != nil {
			return err
		}
		for _, parent := range []string{"generations", "staging"} {
			if err := syncDirectory(s.root, parent); err != nil {
				return err
			}
		}
		var err error
		names, err = s.trashSnapshotLocked()
		return err
	})
	if err != nil {
		return err
	}
	return s.deleteTrash(names)
}

func (inv inventory) protected(id string) bool {
	return (inv.selected != nil && inv.selected.Archive.SHA256 == id) || inv.processes[id] > 0 || inv.candidates[id] > 0 || inv.preparing[id] > 0 || inv.uses[id] > 0
}
