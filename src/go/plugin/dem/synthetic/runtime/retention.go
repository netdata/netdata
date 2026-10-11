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
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

type reservation struct {
	Generation string `json:"generation"`
	Bytes      int64  `json:"bytes"`
}

type inventory struct {
	generations  map[string]generation
	demand       map[string]int
	uses         map[string]int
	uncertain    map[string]int
	reservations map[string]reservation
	cleanupError error
}

// Status describes logical managed files and protection. ReservedBytes is the
// unwritten remainder of active reservations, not additional used disk bytes.
type Status struct {
	UsedBytes             int64  `json:"used_bytes"`
	ReservedBytes         int64  `json:"reserved_bytes"`
	LimitBytes            int64  `json:"limit_bytes"`
	DemandGenerations     int    `json:"demand_generations"`
	ExecutionPins         int    `json:"execution_pins"`
	UncertainPins         int    `json:"uncertain_pins"`
	BootRecoveryAvailable bool   `json:"boot_recovery_available"`
	CleanupError          string `json:"cleanup_error,omitempty"`
}

func (s *Store) inventoryLocked(reconcile bool) (inventory, error) {
	inv := inventory{generations: make(map[string]generation), demand: make(map[string]int), uses: make(map[string]int), uncertain: make(map[string]int), reservations: make(map[string]reservation)}
	initializing, err := s.childDirectories("initializing")
	if err != nil {
		return inv, err
	}
	// Constructors hold coordination exclusion and expose no paths until the
	// initialized record is durably moved to its final ownership namespace.
	for _, name := range initializing {
		if !reconcile {
			continue
		}
		if err := s.removeOwned(name); err != nil {
			return inv, err
		}
	}
	claims, err := s.childDirectories("claims")
	if err != nil {
		return inv, err
	}
	for _, name := range claims {
		alive, err := s.ownerAlive(name)
		if err != nil {
			return inv, err
		}
		if !alive {
			if !reconcile {
				continue
			}
			if err := s.removeOwned(name); err != nil {
				return inv, err
			}
			continue
		}
		var claim claimRecord
		if err := readJSON(s.root, filepath.Join(name, "owner.json"), &claim); err != nil {
			return inv, err
		}
		if !validID(claim.Generation) {
			return inv, errors.New("invalid runtime demand binding")
		}
		inv.demand[claim.Generation]++
	}
	uses, err := s.childDirectories("uses")
	if err != nil {
		return inv, err
	}
	for _, name := range uses {
		var use useRecord
		if err := readJSON(s.root, filepath.Join(name, "owner.json"), &use); err != nil {
			return inv, err
		}
		if !validID(use.Generation) || !validID(use.Invocation) || (use.Boot.Verified && !validBootID(use.Boot.ID)) {
			return inv, errors.New("invalid runtime execution binding")
		}
		done, proofErr := ndexec.ReadTreeCompletion(filepath.Join(s.path, name), use.Invocation)
		if done || s.boot.supersedes(use.Boot) || s.completedUses[name] {
			if !reconcile {
				continue
			}
			if err := s.removeOwned(name); err != nil {
				inv.cleanupError = errors.Join(inv.cleanupError, err)
			}
			continue
		}
		inv.uses[use.Generation]++
		alive, ownerErr := s.ownerAlive(name)
		if ownerErr != nil || !alive {
			inv.uncertain[use.Generation]++
		}
		// A missing receipt may be a live execution. No PID/age guess is made.
		if proofErr != nil {
			inv.cleanupError = errors.Join(inv.cleanupError, proofErr)
		}
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
		inv.demand[r.Generation]++
	}
	generations, err := s.childDirectories("generations")
	if err != nil {
		return inv, err
	}
	now := s.now().UTC()
	for _, name := range generations {
		id := filepath.Base(name)
		g, err := s.loadGeneration(id)
		if err != nil {
			return inv, err
		}
		changed := false
		if inv.demand[id] > 0 {
			changed = !g.Unneeded.IsZero()
			g.Unneeded = time.Time{}
		} else if g.Unneeded.IsZero() {
			g.Unneeded = now
			changed = true
		}
		if changed && reconcile {
			if err := saveJSON(s.root, filepath.Join(name, "prepared.json"), g); err != nil {
				return inv, err
			}
		}
		inv.generations[id] = g
	}
	return inv, nil
}

func (s *Store) statusLocked(inv inventory) (Status, error) {
	status := Status{LimitBytes: s.config.MaxBytes, DemandGenerations: len(inv.demand), BootRecoveryAvailable: s.boot.Verified}
	if err := errors.Join(inv.cleanupError, s.cleanupError); err != nil {
		status.CleanupError = err.Error()
	}
	var err error
	var stagingBytes map[string]int64
	status.UsedBytes, stagingBytes, err = s.diskByteCounts(".")
	if err != nil {
		return status, err
	}
	for name, r := range inv.reservations {
		// Use the SAME observation for used and reserved bytes: writers run
		// outside exclusion, so two scans could undercount a growing operation.
		written := stagingBytes[name]
		if r.Bytes > written {
			status.ReservedBytes += r.Bytes - written
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
	var status Status
	err := s.withLock(func() error {
		inv, err := s.inventoryLocked(false)
		if err != nil {
			return err
		}
		status, err = s.statusLocked(inv)
		return err
	})
	return status, err
}

// Sweep runs after startup demand reconciliation and periodically thereafter.
// Failed cleanup is separate from the ability to use a matching runtime.
func (s *Store) Sweep() error {
	err := s.withLock(func() error {
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
	s.withLock(func() error { s.cleanupError = result; return nil })
	return result
}

func (s *Store) candidates(inv inventory, pressure bool) []string {
	var ids []string
	for id := range inv.generations {
		if inv.demand[id] == 0 && inv.uses[id] == 0 {
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
	// One previous generation is preferred while demand exists; without browser
	// demand only the last used generation receives the longer idle grace.
	last := ids[len(ids)-1]
	grace := s.config.PreviousRetention
	if len(inv.demand) == 0 {
		grace = s.config.IdleRetention
	}
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
	staging := make(map[string]int64)
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
		if strings.HasPrefix(path, "staging/") {
			id, _, found := strings.Cut(strings.TrimPrefix(path, "staging/"), "/")
			if found {
				staging["staging/"+id] += info.Size()
			}
		}
		return nil
	})
	return total, staging, err
}

// Every source parent is synced again before successor deletion. Visibility of
// a trash rename alone is not crash durability. Trash is never executable state.
func (s *Store) flushTrash() error {
	var names []string
	err := s.withLock(func() error {
		var err error
		names, err = s.childDirectories("trash")
		if err != nil {
			return err
		}
		for _, parent := range []string{"generations", "uses", "claims", "staging", "trash", "initializing"} {
			if err := syncDirectory(s.root, parent); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	var result error
	for _, name := range names {
		result = errors.Join(result, s.root.RemoveAll(name))
	}
	return errors.Join(result, syncDirectory(s.root, "trash"))
}
