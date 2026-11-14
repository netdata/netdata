// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"errors"
	"os"
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
	return s.Release.Validate()
}

func (s *Store) selectedLocked(reconcile bool) (*Manifest, error) {
	// A prior writer may have died after rename but before directory fsync. No
	// dependent deletion or executable admission precedes this durability barrier.
	if reconcile {
		if err := s.syncSelection(); err != nil {
			return nil, err
		}
	}
	m, err := s.readSelection()
	if err == nil && m != nil {
		_, err = s.prepared(*m)
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
	return m, nil
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
	claim    *claim
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
	return c.store.beginUse(c.manifest)
}

func (c *Consumer) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return c.store.closeClaim(c.claim)
}

func (s *Store) AcquireSelected() (*Consumer, error) {
	var consumer *Consumer
	err := s.withLock(func() error {
		m, err := s.selectedLocked(true)
		if err != nil {
			return err
		}
		if m == nil {
			return ErrUnavailable
		}
		owner, err := s.ledger.claimLocked(m.Archive.SHA256, "process")
		if err == nil {
			consumer = &Consumer{
				store:    s,
				manifest: *m,
				claim:    owner,
			}
		}
		return err
	})
	if err != nil && consumer != nil {
		return nil, errors.Join(err, consumer.Close())
	}
	return consumer, err
}
