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

type selection struct {
	Manifest *Manifest `json:"manifest"`
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
		// first installation. Do not expose ErrUnavailable through this error.
		return nil, fmt.Errorf("%w: %v", ErrDamaged, err)
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (s *Store) readSelection() (*Manifest, error) {
	// Only an explicit null means uninstalled. Missing fields or a missing
	// record must not silently discard an installed release's protection.
	record := selection{
		Manifest: &Manifest{},
	}
	if err := readJSON(s.root, "selection.json", &record); errors.Is(err, os.ErrNotExist) {
		return nil, damagedMetadata("selection.json", err)
	} else if err != nil {
		return nil, err
	}
	if record.Manifest != nil {
		if err := record.Manifest.Validate(); err != nil {
			return nil, damagedMetadata("selection.json", err)
		}
	}
	return record.Manifest, nil
}

func (s *Store) selectLocked(m *Manifest) error {
	// Split publication from directory sync so the caller can distinguish a failed
	// write from a visible selection whose durability is not yet known.
	if err := publishJSON(s.root, "selection.json", selection{
		Manifest: m,
	}); err != nil {
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
	return c.claim.Close()
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
	operation *flock.Flock
	sealed    bool
	finished  bool
}

func (s *Store) lockInstallation(ctx context.Context) (*flock.Flock, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	lock := flock.New(filepath.Join(s.path, "installation.lock"))
	locked, err := lock.TryLockContext(ctx, 50*time.Millisecond)
	if !locked || err != nil {
		return nil, errors.Join(err, ctx.Err(), lock.Close())
	}
	return lock, nil
}

func (s *Store) BeginInstall(ctx context.Context, m Manifest) (*Candidate, error) {
	return s.beginCandidate(ctx, m, false)
}

// BeginRepair rebuilds the requested release and retires damaged generations.
// It requires every consumer and execution to be drained, and holds installer
// exclusion through qualification/publication. It never resets ownership proof.
func (s *Store) BeginRepair(ctx context.Context, m Manifest) (*Candidate, error) {
	return s.beginCandidate(ctx, m, true)
}

func (s *Store) beginCandidate(ctx context.Context, m Manifest, repair bool) (*Candidate, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	operation, err := s.lockInstallation(ctx)
	if err != nil {
		return nil, err
	}
	var consumer *Consumer
	err = s.withLock(func() error {
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
		Consumer:  consumer,
		operation: operation,
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
		return c.store.selectLocked(&c.manifest)
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
		err = c.claim.Close()
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
		if err := s.selectLocked(nil); err != nil {
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
		if err := s.selectLocked(nil); err != nil {
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
	return len(inv.processes) > 0 || len(inv.candidates) > 0 || len(inv.uses) > 0 || len(inv.reservations) > 0
}
