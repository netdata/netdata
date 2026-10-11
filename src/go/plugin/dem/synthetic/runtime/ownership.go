// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gofrs/flock"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

type claimRecord struct {
	Generation string `json:"generation"`
}

// Claim protects accepted demand, including demand awaiting preparation. It does
// not prove execution lifetime; exiting the owner releases only this claim.
type Claim struct {
	store  *Store
	name   string
	lock   *flock.Flock
	mu     sync.Mutex
	closed bool
}

func (s *Store) Demand(id string) (*Claim, error) {
	if !validID(id) {
		return nil, errors.New("invalid requested runtime identity")
	}
	var claim *Claim
	err := s.withLock(func() error {
		name, lock, err := s.newOwner("claims", claimRecord{Generation: id})
		if err != nil {
			return err
		}
		claim = &Claim{store: s, name: name, lock: lock}
		if g, err := s.loadGeneration(id); err == nil {
			g.Unneeded = time.Time{}
			if err := saveJSON(s.root, filepath.Join("generations", id, "prepared.json"), g); err != nil {
				lock.Close()
				return errors.Join(err, s.removeOwned(name))
			}
		} else if !errors.Is(err, ErrUnavailable) {
			lock.Close()
			return errors.Join(err, s.removeOwned(name))
		}
		return nil
	})
	return claim, err
}

func (c *Claim) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	return c.store.withLock(func() error {
		// No metadata writer remains when the owner lock is released.
		err := c.lock.Close()
		if err != nil {
			return err
		}
		if err := c.store.removeOwned(c.name); err != nil {
			return err
		}
		_, err = c.store.inventoryLocked(true)
		return err
	})
}

func (s *Store) newOwner(parent string, record any) (string, *flock.Flock, error) {
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
	if err := lock.Lock(); err != nil {
		lock.Close()
		return "", nil, err
	}
	if err := saveJSON(s.root, filepath.Join(name, "owner.json"), record); err != nil {
		lock.Close()
		return "", nil, err
	}
	final := filepath.Join(parent, id)
	if err := s.root.Rename(name, final); err != nil {
		lock.Close()
		return "", nil, err
	}
	if err := errors.Join(syncDirectory(s.root, "initializing"), syncDirectory(s.root, parent)); err != nil {
		lock.Close()
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
	lock       *flock.Flock
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
	err := s.withLock(func() error {
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
			completion.Close()
			return err
		}
		lock := flock.New(filepath.Join(s.path, name, "owner.lock"))
		if err := lock.Lock(); err != nil {
			lock.Close()
			completion.Close()
			return err
		}
		keepLock := false
		defer func() {
			if !keepLock {
				lock.Close()
			}
		}()
		record := useRecord{Generation: m.Archive.SHA256, Invocation: completion.ID(), Boot: s.boot}
		if err := saveJSON(s.root, filepath.Join(name, "owner.json"), record); err != nil {
			completion.Close()
			// This call has not exposed any executable path to a caller yet.
			return errors.Join(err, s.removeOwned(name))
		}
		g, err := s.loadGeneration(m.Archive.SHA256)
		if err == nil {
			g.LastUsed = s.now().UTC()
			err = saveJSON(s.root, filepath.Join("generations", m.Archive.SHA256, "prepared.json"), g)
		}
		if err != nil {
			completion.Close()
			return errors.Join(err, s.removeOwned(name))
		}
		final := filepath.Join("uses", id)
		if err := s.root.Rename(name, final); err != nil {
			completion.Close()
			return errors.Join(err, s.removeOwned(name))
		}
		if err := errors.Join(syncDirectory(s.root, "initializing"), syncDirectory(s.root, "uses")); err != nil {
			completion.Close()
			return errors.Join(err, s.removeOwned(final))
		}
		keepLock = true
		use = &Use{Paths: paths, Completion: completion, store: s, name: final, lock: lock}
		return nil
	})
	return use, err
}

func (u *Use) Finish(drained bool) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.finished {
		return nil
	}
	u.finished = true
	err := errors.Join(u.Completion.Close(), u.lock.Close())
	if !drained {
		return err
	}
	return errors.Join(err, u.store.withLock(func() error {
		u.store.completedUses[u.name] = true
		return u.store.removeOwned(u.name)
	}))
}

func (s *Store) removeOwned(name string) error {
	id, err := randomID()
	if err != nil {
		return err
	}
	if filepath.Dir(name) == "staging" {
		// Make a possible payload publication durable before abandoning its owner.
		if err := errors.Join(syncDirectory(s.root, name), syncDirectory(s.root, "generations")); err != nil {
			return err
		}
	}
	if err := s.root.Rename(name, filepath.Join("trash", id)); err != nil {
		return err
	}
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
	defer lock.Close()
	acquired, err := lock.TryLock()
	return !acquired, err
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
