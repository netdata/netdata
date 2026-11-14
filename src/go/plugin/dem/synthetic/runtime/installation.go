// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gofrs/flock"
)

var ErrInUse = errors.New("runtime installation has consumers or unresolved executions")

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
	if err := publishJSON(s.root, "selection.json", record); err != nil {
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

func (c *Consumer) Manifest() Manifest { return c.manifest }
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
		owner, err := s.claimLocked(m.Archive.SHA256, "process")
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

// Candidate holds the installation transaction and generation protection through
// preparation, qualification and selection. Close aborts without changing the
// selected release unless Select has already published it.
type Candidate struct {
	*Consumer
	operation   *flock.Flock
	sealed      bool
	finished    bool
	publication selection
}

func (s *Store) lockInstallation(ctx context.Context) (*flock.Flock, error) {
	lock := flock.New(filepath.Join(s.path, "installation.lock"), flock.SetFlag(os.O_RDWR))
	locked, err := lock.TryLockContext(ctx, 50*time.Millisecond)
	if !locked || err != nil {
		return nil, errors.Join(err, ctx.Err(), lock.Close())
	}
	return lock, nil
}

// Ownership of operation transfers to the candidate, or is released on error.
// Repair rebuilds the release and retires damaged generations. It requires every
// consumer and execution to be drained and never resets ownership proof.
func (s *Store) candidateWithOperation(operation *flock.Flock, publication selection, repair bool) (*Candidate, error) {
	m := publication.Release.Manifest
	var consumer *Consumer
	err := s.withLock(func() error {
		if repair {
			if err := s.repairLocked(m); err != nil {
				return err
			}
		} else if _, err := s.selectedLocked(true); err != nil {
			return err
		}
		owner, err := s.claimLocked(m.Archive.SHA256, "candidate")
		if err == nil {
			consumer = &Consumer{
				store:    s,
				manifest: m,
				claim:    owner,
			}
		}
		return err
	})
	if err != nil {
		if consumer != nil {
			err = errors.Join(err, consumer.Close())
		}
		return nil, errors.Join(err, s.releaseOwner(operation))
	}
	return &Candidate{
		Consumer:    consumer,
		operation:   operation,
		publication: publication,
	}, nil
}

func (c *Candidate) Import(ctx context.Context, archive string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("runtime candidate is closed")
	}
	return c.store.importArchive(ctx, c.manifest, archive)
}
func (c *Candidate) Download(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("runtime candidate is closed")
	}
	return c.store.download(ctx, c.manifest)
}

// Select publishes a prepared candidate after the caller has qualified both
// supported workload kinds. Existing consumers keep their acquired generation.
func (c *Candidate) Select() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("runtime candidate is closed")
	}
	return c.store.withLock(func() error {
		if _, err := c.store.selectedLocked(true); err != nil {
			return err
		}
		if _, err := c.store.prepared(c.manifest); err != nil {
			return err
		}
		return c.store.selectLocked(c.publication)
	})
}

func (c *Candidate) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.finished {
		return nil
	}
	c.closed = true
	var err error
	if c.sealed {
		err = c.store.releaseOwner(c.claim.lock)
	} else {
		err = c.store.closeClaim(c.claim)
	}
	// Failed releases remain in Store's retry list, independently of the caller.
	err = errors.Join(err, c.store.releaseOwner(c.operation))
	c.finished = true
	return err
}

// Uninstall is serialized with installers. A refusal leaves selection unchanged.
// A successful clear prevents future process admission before bytes are retired.
func (s *Store) Uninstall(ctx context.Context) (result error) {
	operation, err := s.lockInstallation(ctx)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, s.releaseOwner(operation)) }()
	err = s.withLock(func() error {
		inv, err := s.inspectInventoryLocked(true)
		if err != nil {
			return err
		}
		if inv.inUse() {
			return ErrInUse
		}
		if err := s.selectLocked(selection{}); err != nil {
			return err
		}
		for id := range inv.generations {
			if err := s.removeOwned(filepath.Join("generations", id)); err != nil {
				return fmt.Errorf("runtime uninstalled, cleanup pending: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return s.flushTrash()
}

func (s *Store) repairLocked(m Manifest) error {
	inv, err := s.inspectInventoryLocked(true)
	if err != nil {
		return err
	}
	if inv.inUse() {
		return ErrInUse
	}
	// A same-release rebuild cannot leave its old selection admitting startup
	// consumers while those paths are retired. Preserve healthy other selections.
	if inv.selectionError != nil || (inv.selected != nil && inv.selected.Archive.SHA256 == m.Archive.SHA256) {
		if err := s.selectLocked(selection{}); err != nil {
			return err
		}
	}
	for id := range inv.generations {
		if id != m.Archive.SHA256 && inv.generationErrors[id] == nil {
			continue
		}
		if err := s.removeOwned(filepath.Join("generations", id)); err != nil {
			return fmt.Errorf("runtime repair cleanup pending: %w", err)
		}
	}
	return nil
}

func (inv inventory) inUse() bool {
	return inv.ownershipInventory.inUse() || len(inv.reservations) > 0
}
