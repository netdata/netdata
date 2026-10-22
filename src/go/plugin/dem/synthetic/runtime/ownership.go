// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gofrs/flock"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

type claimRecord struct {
	Generation string `json:"generation"`
	Pending    bool   `json:"pending,omitempty"`
}

// Claim protects a pending or accepted configuration, including a runtime not
// prepared yet. It is process-owned, never execution-lifetime evidence.
type Claim struct {
	store         *Store
	name          string
	generation    string
	lock          io.Closer
	mu            sync.Mutex
	closed        bool
	released      bool
	commitStarted bool
}

// ReserveDemand protects an identity without changing generation timestamps.
// Call Commit only after configuration adoption; Close aborts unused reservations.
func (s *Store) ReserveDemand(id string) (*Claim, error) {
	if !validID(id) {
		return nil, errors.New("invalid requested runtime identity")
	}
	var claim *Claim
	err := s.withLock(func() error {
		name, lock, err := s.newOwner("claims", claimRecord{Generation: id, Pending: true})
		if err == nil {
			claim = &Claim{store: s, name: name, generation: id, lock: lock}
		}
		return err
	})
	if err != nil {
		if claim != nil {
			err = errors.Join(err, claim.Close())
		}
		return nil, err
	}
	return claim, nil
}

// Commit promotes reserved protection to accepted demand. An I/O failure retains
// ownership, so the accepted caller can retry without exposing files to eviction.
func (c *Claim) Commit() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("runtime demand is closed")
	}
	c.commitStarted = true
	return c.store.withLock(func() error {
		if err := saveJSON(c.store.root, filepath.Join(c.name, "owner.json"), claimRecord{Generation: c.generation}); err != nil {
			return err
		}
		g, err := c.store.loadGeneration(c.generation)
		if errors.Is(err, ErrUnavailable) {
			return nil
		}
		if err != nil {
			return err
		}
		if g.Unneeded.IsZero() {
			return nil
		}
		g.Unneeded = time.Time{}
		return saveJSON(c.store.root, filepath.Join("generations", c.generation, "prepared.json"), g)
	})
}

// Demand is the immediate reservation/commit path for explicit preparation.
func (s *Store) Demand(id string) (*Claim, error) {
	claim, err := s.ReserveDemand(id)
	if err != nil {
		return nil, err
	}
	if err := claim.Commit(); err != nil {
		return nil, errors.Join(err, claim.Close())
	}
	return claim, nil
}

func (c *Claim) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.released {
		return nil
	}
	c.closed = true
	// Terminal ownership ends independently of coordinated record reclamation.
	// If local release fails, Store retains the handle even for one-shot callers.
	err := c.store.releaseOwner(c.lock)
	if err == nil {
		c.released = true
		err = c.store.withLock(func() error {
			if err := c.store.removeOwned(c.name); err != nil {
				return err
			}
			if !c.commitStarted {
				return nil
			}
			_, err := c.store.inventoryLocked(true)
			return err
		})
	}
	c.store.recordCleanupError(err)
	return err
}

// releaseOwner is only for process-local owner locks. Dead claim/staging records
// are reclaimed by inventory; releasing a use lock never proves child completion.
func (s *Store) releaseOwner(owner io.Closer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.releaseOwnerLocked(owner)
}

func (s *Store) releaseOwnerLocked(owner io.Closer) error {
	err := owner.Close()
	if err != nil {
		s.releasingOwners[owner] = struct{}{}
	} else {
		delete(s.releasingOwners, owner)
	}
	return err
}

func (s *Store) retryOwnerReleasesLocked() error {
	var result error
	for owner := range s.releasingOwners {
		result = errors.Join(result, s.releaseOwnerLocked(owner))
	}
	return result
}

func (s *Store) recordCleanupError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recordCleanupErrorLocked(err)
}

func (s *Store) recordCleanupErrorLocked(err error) {
	if err != nil {
		s.cleanupError = err
		s.cleanupErrorVersion++
	}
}

func (s *Store) finishPreparation(name string, owner io.Closer) {
	err := s.releaseOwner(owner)
	if err == nil {
		err = s.withLock(func() error { return s.removeOwned(name) })
	}
	s.recordCleanupError(errors.Join(err, s.flushTrash()))
}

func (s *Store) newOwner(parent string, record any) (_ string, _ *flock.Flock, result error) {
	id, err := randomID()
	if err != nil {
		return "", nil, err
	}
	name := filepath.Join("initializing", id)
	if err := s.root.Mkdir(name, 0700); err != nil {
		return "", nil, err
	}
	if err := ensureLock(s.root, filepath.Join(name, "owner.lock")); err != nil {
		return "", nil, err
	}
	lock := flock.New(filepath.Join(s.path, name, "owner.lock"))
	defer func() {
		if result != nil {
			releaseErr := s.releaseOwnerLocked(lock)
			s.recordCleanupErrorLocked(releaseErr)
			result = errors.Join(result, releaseErr)
		}
	}()
	if err := lock.Lock(); err != nil {
		return "", nil, err
	}
	if err := saveJSON(s.root, filepath.Join(name, "owner.json"), record); err != nil {
		return "", nil, err
	}
	final := filepath.Join(parent, id)
	if err := s.root.Rename(name, final); err != nil {
		return "", nil, err
	}
	if err := errors.Join(syncDirectory(s.root, "initializing"), syncDirectory(s.root, parent)); err != nil {
		return "", nil, err
	}
	return final, lock, nil
}

type useRecord struct {
	Generation string       `json:"generation"`
	Invocation string       `json:"invocation"`
	Boot       bootIdentity `json:"boot"`
}

// Use protects every access that may execute the managed runtime, including
// version probes. Pass Completion to ndexec, then Finish only after joining it.
// If launch never started, the owning caller may Finish(true). Lost callers
// cannot make that assertion; recovery requires a receipt or verified host reboot.
type Use struct {
	lock       io.Closer
	Paths      Paths
	Completion *ndexec.TreeCompletion
	store      *Store
	name       string
	mu         sync.Mutex
	finished   bool
}

func (s *Store) BeginUse(m Manifest) (*Use, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	var use *Use
	err := s.withLock(func() (result error) {
		paths, err := s.prepared(m)
		if err != nil {
			return err
		}
		id, err := randomID()
		if err != nil {
			return err
		}
		name := filepath.Join("initializing", id)
		completion, err := ndexec.CreateTreeCompletion(filepath.Join(s.path, name))
		if err != nil {
			return err
		}
		if err := ensureLock(s.root, filepath.Join(name, "owner.lock")); err != nil {
			return errors.Join(err, completion.Close())
		}
		lock := flock.New(filepath.Join(s.path, name, "owner.lock"))
		defer func() {
			if result != nil {
				// No executable path escaped. Preserve that proof if cleanup
				// fails after the record moved into the execution namespace.
				s.completedUses[name] = true
				cleanupErr := errors.Join(completion.Close(), s.releaseOwnerLocked(lock), s.removeOwned(name))
				s.recordCleanupErrorLocked(cleanupErr)
				result = errors.Join(result, cleanupErr)
			}
		}()
		if err := lock.Lock(); err != nil {
			return err
		}
		record := useRecord{Generation: m.Archive.SHA256, Invocation: completion.ID(), Boot: s.boot}
		if err := saveJSON(s.root, filepath.Join(name, "owner.json"), record); err != nil {
			return err
		}
		g, err := s.loadGeneration(m.Archive.SHA256)
		if err == nil {
			g.LastUsed = s.now().UTC()
			err = saveJSON(s.root, filepath.Join("generations", m.Archive.SHA256, "prepared.json"), g)
		}
		if err != nil {
			return err
		}
		final := filepath.Join("uses", id)
		if err := s.root.Rename(name, final); err != nil {
			return err
		}
		name = final
		if err := errors.Join(syncDirectory(s.root, "initializing"), syncDirectory(s.root, "uses")); err != nil {
			return err
		}
		use = &Use{Paths: paths, Completion: completion, store: s, name: final, lock: lock}
		return nil
	})
	if err != nil && use != nil {
		// No executable path escaped to the caller.
		return nil, errors.Join(err, use.Finish(true))
	}
	return use, err
}

func (u *Use) Finish(drained bool) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.finished {
		return nil
	}
	u.finished = true
	if drained {
		// Callers finish only once. Retain their proof before filesystem cleanup,
		// including failure to acquire coordination, so Sweep can retry it.
		u.store.mu.Lock()
		u.store.completedUses[u.name] = true
		u.store.mu.Unlock()
	}
	err := errors.Join(u.Completion.Close(), u.store.releaseOwner(u.lock))
	if drained {
		err = errors.Join(err, u.store.withLock(func() error {
			return u.store.removeOwned(u.name)
		}))
	}
	u.store.recordCleanupError(err)
	return err
}

func (s *Store) removeOwned(name string) error {
	id, err := randomID()
	if err != nil {
		return err
	}
	if _, err := s.root.Lstat(name); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	} else if err == nil && filepath.Dir(name) == "staging" {
		// Make a possible payload publication durable before abandoning its owner.
		if err := errors.Join(syncDirectory(s.root, name), syncDirectory(s.root, "generations")); err != nil {
			return err
		}
	}
	if err := s.root.Rename(name, filepath.Join("trash", id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// A successor may already have retired this record. Re-sync both parents
	// even when the source vanished, including retry after a failed fsync.
	err = errors.Join(syncDirectory(s.root, filepath.Dir(name)), syncDirectory(s.root, "trash"))
	if err == nil {
		delete(s.completedUses, name)
	}
	return err
}

// ownerAlive is only for in-process claims/preparation, never for execution pins.
func (s *Store) ownerAlive(name string) (bool, error) {
	info, err := s.root.Lstat(filepath.Join(name, "owner.lock"))
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() != 0 {
		return false, errors.New("invalid runtime operation owner")
	}
	lock := flock.New(filepath.Join(s.path, name, "owner.lock"))
	acquired, err := lock.TryLock()
	releaseErr := s.releaseOwnerLocked(lock)
	s.recordCleanupErrorLocked(releaseErr)
	return !acquired, errors.Join(err, releaseErr)
}

func (s *Store) childDirectories(parent string) ([]string, error) {
	f, err := s.root.Open(parent)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	entries, err := f.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() || !validID(entry.Name()) || entry.Type()&os.ModeSymlink != 0 {
			return nil, errors.New("invalid runtime ownership inventory")
		}
		names = append(names, filepath.Join(parent, entry.Name()))
	}
	return names, nil
}
