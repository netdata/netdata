// SPDX-License-Identifier: GPL-3.0-or-later

package browserruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// SelectionError means publication became visible but its crash durability could
// not be confirmed. Callers must inspect/reconcile instead of claiming rollback.
type SelectionError struct{ Err error }

func (e *SelectionError) Error() string {
	return "runtime selection publication is uncertain: " + e.Err.Error()
}

func (e *SelectionError) Unwrap() error { return e.Err }

// selection is the durable installation decision. Every selected generation is
// backed by an authenticated release, whose manifest identifies it. An explicit
// null release means nothing is selected: a new store, after uninstall, or while
// repair rebuilds the selection.
type selection struct {
	Release          *Release `json:"release"`
	AutomaticUpdates bool     `json:"automatic_updates,omitempty"`
}

func (s selection) manifest() *Manifest {
	if s.Release == nil {
		return nil
	}
	m := s.Release.Manifest
	return &m
}

func (s selection) validate() error {
	if s.Release == nil {
		if s.AutomaticUpdates {
			return errors.New("automatic runtime updates require an authenticated release")
		}
		return nil
	}
	return s.Release.ValidateMetadata()
}

func (s *Store) selectedLocked(reconcile bool) (*Release, error) {
	// A prior writer may have died after rename but before directory fsync. No
	// dependent deletion or executable admission precedes this durability barrier.
	if reconcile {
		if err := s.syncSelection(); err != nil {
			return nil, err
		}
	}
	record, err := s.readSelectionRecord()
	if err == nil && record.Release != nil {
		_, err = s.prepared(record.Release.Manifest)
		if errors.Is(err, ErrUnavailable) {
			err = damagedMetadata("selection.json", err)
		}
	}
	if isMetadataDamage(err) {
		// A declared but unusable selection needs repair, not an ordinary
		// first installation.
		return nil, damaged(err)
	}
	if err != nil {
		return nil, err
	}
	return record.Release, nil
}

func (s *Store) readSelection() (*Manifest, error) {
	record, err := s.readSelectionRecord()
	if err != nil {
		return nil, err
	}
	return record.manifest(), nil
}

// legacyEmptySelection is the exact record earlier builds wrote for a new or
// uninstalled store. It still means nothing is selected; every other former
// shape is damage. The next selection write replaces it.
const legacyEmptySelection = "{\"manifest\":null}\n"

func (s *Store) readSelectionRecord() (selection, error) {
	// Only an explicit null means uninstalled. Missing fields or a missing
	// record must not silently discard an installed release's protection.
	record := selection{
		Release: &Release{},
	}
	if err := readJSON(s.root, "selection.json", &record); errors.Is(err, os.ErrNotExist) {
		return record, damagedMetadata("selection.json", err)
	} else if isMetadataDamage(err) && s.isLegacyEmptySelection() {
		return selection{}, nil
	} else if err != nil {
		return record, err
	}
	if err := record.validate(); err != nil {
		return record, damagedMetadata("selection.json", err)
	}
	return record, nil
}

func (s *Store) isLegacyEmptySelection() bool {
	data, err := s.root.ReadFile("selection.json")
	return err == nil && string(data) == legacyEmptySelection
}

func (s *Store) selectLocked(record selection) error {
	// Split publication from directory sync so the caller can distinguish a failed
	// write from a visible selection whose durability is not yet known.
	if err := publishJSON(s.root, "selection.json", record, 0600); err != nil {
		return err
	}
	if err := s.syncSelection(); err != nil {
		return &SelectionError{
			Err: err,
		}
	}
	return nil
}

// Consumer freezes one selection for a process lifetime. Closing it prevents
// future BeginUse calls; already issued uses have independent execution receipts.
type Consumer struct {
	store    *Store
	manifest Manifest
	agent    *AgentRelease
	claim    *claim
	// selected consumers acquired a prepared generation that their claim
	// protects, so its later absence is damage. A candidate's generation does not
	// exist until it is prepared.
	selected bool
	mu       sync.Mutex
	closed   bool
}

func (c *Consumer) Manifest() Manifest { return c.manifest.clone() }

func (c *Consumer) BeginUse() (*Use, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, errors.New("runtime consumer is closed")
	}
	use, err := c.store.beginUse(c.manifest, c.agent)
	if c.selected && errors.Is(err, ErrUnavailable) {
		err = damagedMetadata(filepath.Join("generations", c.manifest.Archive.SHA256), err)
	}
	return use, publicDamage(err)
}

func (c *Consumer) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return c.store.closeClaim(c.claim)
}

// ValidateSelected lets an installation owner retain a runtime without changing
// its release or consent. It checks execution eligibility and the bound payload,
// including provenance, while excluding installers; running consumers may remain.
func (s *Store) ValidateSelected(ctx context.Context) error {
	return publicDamage(s.withPairOperation(ctx, func(inv inventory, recovery pairRecovery) error {
		if err := inv.damageError(); err != nil {
			return err
		}
		if recovery.Pending != nil {
			return ErrPairPending
		}
		if inv.release == nil {
			return ErrUnavailable
		}
		if err := inv.release.Validate(); err != nil {
			return err
		}
		if !inv.release.Manifest.MatchesHost() {
			return fmt.Errorf("%w: runtime does not match this architecture", ErrIncompatible)
		}
		return s.validateRetainedSelection(selection{
			Release: inv.release,
		})
	}))
}

func (s *Store) AcquireSelected() (*Consumer, error) {
	var consumer *Consumer
	err := s.withLock(func() error {
		release, err := s.selectedLocked(true)
		if err != nil {
			return err
		}
		if release == nil {
			return ErrUnavailable
		}
		if err := release.Validate(); err != nil {
			return err
		}
		m := release.Manifest
		if err := s.verifyPreparedAgent(m, release.Agent); err != nil {
			return err
		}
		owner, err := s.ledger.claimLocked(m.Archive.SHA256, processClaim)
		if err == nil {
			consumer = &Consumer{
				store:    s,
				manifest: m,
				agent:    release.Agent,
				claim:    owner,
				selected: true,
			}
		}
		return err
	})
	if err != nil && consumer != nil {
		return nil, errors.Join(err, consumer.Close())
	}
	return consumer, err
}
