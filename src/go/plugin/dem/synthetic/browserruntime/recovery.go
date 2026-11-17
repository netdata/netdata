// SPDX-License-Identifier: GPL-3.0-or-later

package browserruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
)

var ErrPairPending = errors.New("a paired update is pending; use its installation owner to activate or roll back")

// Recovery evidence is separate from the old selection/generation schemas. It
// protects bytes after the preparing helper exits, but never grants execution.
type pairRecovery struct {
	Format   int             `json:"format"`
	Previous *selection      `json:"previous,omitempty"`
	Pending  *pairTransition `json:"pending,omitempty"`
}

type pairTransition struct {
	Before selection `json:"before"`
	After  selection `json:"after"`
}

// PairRecoveryStatus reports stored selections, not what another Agent process
// is running. Its reader is the runtime status command.
type PairRecoveryStatus struct {
	State    string   `json:"state"`
	Previous *Release `json:"previous,omitempty"`
	Prepared *Release `json:"prepared,omitempty"`
}

func (r pairRecovery) status(current selection) *PairRecoveryStatus {
	if r.Previous == nil && r.Pending == nil {
		return nil
	}
	status := &PairRecoveryStatus{State: "rollback-available"}
	if r.Previous != nil {
		status.Previous = r.Previous.Release
		if reflect.DeepEqual(current, *r.Previous) {
			status.State = "restored"
		}
	}
	if r.Pending != nil {
		status.Prepared = r.Pending.After.Release
		switch {
		case reflect.DeepEqual(current, r.Pending.Before):
			status.State = "prepared"
		case reflect.DeepEqual(current, r.Pending.After):
			status.State = "activation-pending"
		default:
			status.State = "recovery-required"
		}
	}
	return status
}

func (r pairRecovery) selections() []selection {
	var result []selection
	if r.Previous != nil {
		result = append(result, *r.Previous)
	}
	if r.Pending != nil {
		result = append(result, r.Pending.Before, r.Pending.After)
	}
	return result
}

func (r pairRecovery) protects(id string) bool {
	for _, record := range r.selections() {
		if record.Release.Manifest.Archive.SHA256 == id {
			return true
		}
	}
	return false
}

func (s *Store) readPairRecovery() (pairRecovery, error) {
	var r pairRecovery
	err := readOwnerRecord(s.root, "recovery.json", &r)
	if errors.Is(err, os.ErrNotExist) {
		return pairRecovery{Format: 1}, nil // Earlier stores have no recovery record.
	}
	if err == nil && r.Format != 1 {
		err = errors.New("unsupported recovery record format")
	}
	for _, record := range r.selections() {
		if err != nil {
			break
		}
		if record.Release == nil {
			err = errors.New("recovery selection is missing its release")
		} else {
			err = record.validate()
		}
	}
	if err == nil && r.Pending != nil && r.Pending.After.Release.Agent == nil {
		err = errors.New("pending runtime has no paired Agent identity")
	}
	if err != nil {
		// Neither repair nor uninstall may discard an unreadable retention promise.
		return r, fmt.Errorf("runtime recovery evidence is unreadable; preserve recovery.json: %w", err)
	}
	return r, nil
}

func (s *Store) savePairRecovery(r pairRecovery) error {
	return saveJSON(s.root, "recovery.json", r)
}

func (s *Store) noPendingPair() error {
	r, err := s.readPairRecovery()
	if err == nil && r.Pending != nil {
		return ErrPairPending
	}
	return err
}

// PreparePair retains a verified target without changing selection. The helper
// must belong to the target Agent; its owner controls Agent installation and
// service stop/start. Initial enrollment uses Select.
func (c *Candidate) PreparePair() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("runtime candidate is closed")
	}
	if c.publication.Release.Agent == nil {
		return errors.New("paired preparation requires an Agent release identity")
	}
	if err := c.store.ValidateRelease(*c.publication.Release); err != nil {
		return err
	}
	return publicDamage(c.store.withLock(func() error {
		s := c.store
		if _, err := s.selectedLocked(true); err != nil {
			return err
		}
		current, err := s.readSelectionRecord()
		if err != nil {
			return err
		}
		if current.Release == nil {
			return errors.New("paired update requires an enrolled runtime; use runtime install first")
		}
		if err := s.validateRetainedSelection(current); err != nil {
			return err
		}
		if err := s.validateRetainedSelection(c.publication); err != nil {
			return err
		}
		r, err := s.readPairRecovery()
		if err != nil {
			return err
		}
		if r.Pending != nil {
			if reflect.DeepEqual(r.Pending.After, c.publication) &&
				(reflect.DeepEqual(r.Pending.Before, current) || reflect.DeepEqual(r.Pending.After, current)) {
				return nil
			}
			return ErrPairPending
		}
		// One recovery pair: beginning B -> C retires the older A promise.
		// Keeping B here also makes recovery retries stay on B after cancellation.
		r.Previous = &current
		r.Pending = &pairTransition{Before: current, After: c.publication}
		return s.savePairRecovery(r)
	}))
}

// validateRetainedSelection checks stored payload integrity without requiring
// its execution contract to match this helper. Rollback never executes it.
func (s *Store) validateRetainedSelection(record selection) error {
	m := record.Release.Manifest
	if _, err := s.prepared(m); err != nil {
		return err
	}
	if err := s.validatePreparedPayload(m); err != nil {
		return err
	}
	return s.verifyPreparedAgent(m, record.Release.Agent)
}

func (s *Store) withPairOperation(ctx context.Context, fn func(inventory, pairRecovery) error) (result error) {
	operation, err := s.lockInstallation(ctx)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, s.ledger.releaseOwner(operation)) }()
	return s.withLock(func() error {
		inv, err := s.lenientInventoryLocked(true)
		if err != nil {
			return err
		}
		return fn(inv, inv.recovery)
	})
}

// ActivatePair selects the prepared target after the installation owner has
// stopped consumers and installed the matching Agent. A retry after selection
// publication finishes the recovery record; it does not re-download anything.
// Without a pending transition, it validates the exact selected pair so owners
// can use the same activation step after unchanged preparation.
func (s *Store) ActivatePair(ctx context.Context) error {
	return s.withPairOperation(ctx, func(inv inventory, r pairRecovery) error {
		if inv.inUse() {
			return ErrInUse
		}
		current, err := s.readSelectionRecord()
		if err != nil {
			return err
		}
		if r.Pending == nil {
			if current.Release != nil {
				if err := s.ValidateRelease(*current.Release); err != nil {
					return err
				}
				return s.validateRetainedSelection(current)
			}
			return errors.New("no paired runtime update is prepared")
		}
		if !reflect.DeepEqual(current, r.Pending.Before) && !reflect.DeepEqual(current, r.Pending.After) {
			return errors.New("selection changed outside the pending paired update; use its installation owner to recover")
		}
		if err := s.ValidateRelease(*r.Pending.After.Release); err != nil {
			return err
		}
		if err := s.validateRetainedSelection(r.Pending.After); err != nil {
			return err
		}
		if err := s.selectLocked(r.Pending.After); err != nil {
			return err
		}
		r.Previous, r.Pending = &r.Pending.Before, nil
		return s.savePairRecovery(r)
	})
}

// RollbackPair restores authenticated prior selection after drainage. The caller
// must restore its matching Agent before restarting. It also cancels preparation
// that never changed selection; unknown ownership evidence remains protected.
func (s *Store) RollbackPair(ctx context.Context) error {
	return s.rollbackPair(ctx, false)
}

// CancelPreparation is safe when preparation failed before its journal write:
// without a pending record it does nothing, never rolling back an extra pair.
func (s *Store) CancelPreparation(ctx context.Context) error {
	return s.rollbackPair(ctx, true)
}

func (s *Store) rollbackPair(ctx context.Context, preparedOnly bool) error {
	return s.withPairOperation(ctx, func(inv inventory, r pairRecovery) error {
		if preparedOnly && r.Pending == nil {
			return nil
		}
		if preparedOnly {
			if err := s.ValidateRelease(*r.Pending.After.Release); err != nil {
				return err
			}
		}
		target := r.Previous
		if r.Pending != nil {
			target = &r.Pending.Before
		}
		if target == nil {
			return errors.New("no previous runtime pair is retained")
		}
		if err := s.validateRetainedSelection(*target); err != nil {
			return err
		}
		current, err := s.readSelectionRecord()
		if err != nil && !isMetadataDamage(err) {
			return err
		}
		if err != nil || !reflect.DeepEqual(current, *target) {
			if inv.inUse() {
				return ErrInUse
			}
			if err := s.selectLocked(*target); err != nil {
				return err
			}
		}
		if r.Pending != nil {
			r.Previous, r.Pending = target, nil
			return s.savePairRecovery(r)
		}
		return nil // Keep the rollback point so an interrupted owner can retry.
	})
}
