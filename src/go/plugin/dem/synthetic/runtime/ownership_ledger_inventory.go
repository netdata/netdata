// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"errors"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

type ownershipInventory struct {
	processes          map[string]int
	candidates         map[string]int
	uses               map[string]int
	uncertain          map[string]int
	cleanupError       error
	protectedRecords   map[string]bool
	reclaimableRecords map[string]bool
}

func (s *ownershipLedger) inventoryLocked(reconcile bool) (ownershipInventory, error) {
	inv := ownershipInventory{
		processes:          make(map[string]int),
		candidates:         make(map[string]int),
		uses:               make(map[string]int),
		uncertain:          make(map[string]int),
		protectedRecords:   make(map[string]bool),
		reclaimableRecords: make(map[string]bool),
	}
	// Live completion proof also survives a successful rename followed by a
	// failed directory sync. Retry these names even if enumeration cannot see
	// the original record anymore.
	if reconcile {
		for name := range s.completedUses {
			if err := s.removeOwned(name); err != nil {
				inv.cleanupError = errors.Join(inv.cleanupError, err)
			}
		}
	}
	initializing, err := s.childDirectories("initializing")
	if err != nil {
		return inv, err
	}
	// Constructors hold coordination exclusion and expose no paths until the
	// initialized record is durably moved to its final ownership namespace.
	for _, name := range initializing {
		inv.reclaimableRecords[name] = true
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
			inv.reclaimableRecords[name] = true
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
			return inv, errors.New("invalid runtime processes binding")
		}
		switch claim.Kind {
		case "candidate":
			inv.candidates[claim.Generation]++
		case "process":
			inv.processes[claim.Generation]++
		default:
			return inv, errors.New("invalid runtime claim kind")
		}
		inv.protectedRecords[name] = true
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
			inv.reclaimableRecords[name] = true
			if !reconcile {
				continue
			}
			if err := s.removeOwned(name); err != nil {
				inv.cleanupError = errors.Join(inv.cleanupError, err)
			}
			continue
		}
		inv.uses[use.Generation]++
		inv.protectedRecords[name] = true
		alive, ownerErr := s.ownerAlive(name)
		if ownerErr != nil || !alive {
			inv.uncertain[use.Generation]++
		}
		// A missing receipt may be a live execution. No PID/age guess is made.
		if proofErr != nil {
			inv.cleanupError = errors.Join(inv.cleanupError, proofErr)
		}
	}
	return inv, nil
}

// finishSweep publishes health after trash deletion, outside coordination
// exclusion. A newer finalizer or sweep failure cannot be cleared by this sweep.
func (s *ownershipLedger) finishSweep(cleanupVersion uint64, result error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if result != nil {
		s.recordCleanupErrorLocked(result)
	} else if s.cleanupErrorVersion == cleanupVersion {
		s.cleanupError = nil
	}
}

func (inv ownershipInventory) inUse() bool {
	return len(inv.processes) > 0 || len(inv.candidates) > 0 || len(inv.uses) > 0
}
