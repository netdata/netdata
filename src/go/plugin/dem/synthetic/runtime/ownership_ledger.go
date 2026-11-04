// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/gofrs/flock"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

// ownershipLedger coordinates lifetime proof only. Payload owners share this
// exclusion for admission and establish their own barriers before deleting trash.
type ownershipLedger struct {
	path                string
	root                *os.Root
	mu                  sync.Mutex
	lock                *flock.Flock
	boot                bootIdentity
	completedUses       map[string]bool
	releasingOwners     map[io.Closer]struct{}
	cleanupError        error
	cleanupErrorVersion uint64
}

func newOwnershipLedger(path string, root *os.Root) *ownershipLedger {
	// Bootstrap creates lock files explicitly. Reopening must never replace a
	// lost lock identity while another process can still hold the old inode.
	return &ownershipLedger{
		path:            path,
		root:            root,
		boot:            hostBoot(),
		lock:            flock.New(filepath.Join(path, "coordination.lock"), flock.SetFlag(os.O_RDWR)),
		completedUses:   make(map[string]bool),
		releasingOwners: make(map[io.Closer]struct{}),
	}
}

func openOwnershipLedger(path string) (*ownershipLedger, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("ownership ledger requires an absolute path")
	}
	canonical, root, err := openRuntimeRoot(path, ownershipRootLayout)
	if err != nil {
		return nil, err
	}
	ledger := newOwnershipLedger(canonical, root)
	if err := ledger.withLock(func() error { return ownershipRootLayout.validate(root) }); err != nil {
		return nil, errors.Join(err, ledger.Close())
	}
	return ledger, nil
}

func (s *ownershipLedger) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := errors.Join(s.retryOwnerReleasesLocked(), s.lock.Close()); err != nil {
		return err
	}
	return s.root.Close()
}

func (s *ownershipLedger) withLock(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.lock.Lock(); err != nil {
		return err
	}
	return errors.Join(fn(), s.lock.Unlock())
}

type claimRecord struct {
	Generation string `json:"generation"`
	Kind       string `json:"kind"`
}

// claim protects a process or candidate, never surviving-child lifetime.
type claim struct {
	ledger *ownershipLedger
	name   string
	lock   io.Closer
	mu     sync.Mutex
	closed bool
}

func (s *ownershipLedger) claimLocked(id, kind string) (*claim, error) {
	if !validID(id) {
		return nil, errors.New("invalid runtime identity")
	}
	name, lock, err := s.newOwner("claims", claimRecord{Generation: id, Kind: kind})
	if err != nil {
		return nil, err
	}
	return &claim{ledger: s, name: name, lock: lock}, nil
}

func (c *claim) Close() error {
	_, err := c.close()
	return err
}

// close reports whether this call retired the claim, so payload owners can
// reconcile their own retention state once without coupling proof to payloads.
func (c *claim) close() (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false, nil
	}
	err := c.ledger.releaseOwner(c.lock)
	if err == nil {
		c.closed = true
		err = c.ledger.withLock(func() error { return c.ledger.removeOwned(c.name) })
	}
	c.ledger.recordCleanupError(err)
	return c.closed, err
}

// releaseOwner is only for process-local owner locks. Dead claim/staging records
// are reclaimed by inventory; releasing a use lock never proves child completion.
func (s *ownershipLedger) releaseOwner(owner io.Closer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.releaseOwnerLocked(owner)
}

func (s *ownershipLedger) releaseOwnerLocked(owner io.Closer) error {
	err := owner.Close()
	if err != nil {
		s.releasingOwners[owner] = struct{}{}
	} else {
		delete(s.releasingOwners, owner)
	}
	return err
}

func (s *ownershipLedger) retryOwnerReleasesLocked() error {
	var result error
	for owner := range s.releasingOwners {
		result = errors.Join(result, s.releaseOwnerLocked(owner))
	}
	return result
}

func (s *ownershipLedger) recordCleanupError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recordCleanupErrorLocked(err)
}

func (s *ownershipLedger) recordCleanupErrorLocked(err error) {
	if err != nil {
		s.cleanupError = err
		s.cleanupErrorVersion++
	}
}

func (s *ownershipLedger) newOwner(parent string, record any) (_ string, _ *flock.Flock, result error) {
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
	lock := flock.New(filepath.Join(s.path, name, "owner.lock"), flock.SetFlag(os.O_RDWR))
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

// executionLease has no executable paths or payload-policy dependency.
type executionLease struct {
	lock       io.Closer
	Completion *ndexec.TreeCompletion
	ledger     *ownershipLedger
	name       string
	mu         sync.Mutex
	finished   bool
}

// newUseLocked creates durable unpublished proof. The payload owner may perform
// its own writes before publishLocked; neither step exposes executable paths.
func (s *ownershipLedger) newUseLocked(resource string) (_ *executionLease, result error) {
	if !validID(resource) {
		return nil, errors.New("invalid runtime identity")
	}
	id, err := randomID()
	if err != nil {
		return nil, err
	}
	name := filepath.Join("initializing", id)
	completion, err := ndexec.CreateTreeCompletion(filepath.Join(s.path, name))
	if err != nil {
		return nil, err
	}
	if err := ensureLock(s.root, filepath.Join(name, "owner.lock")); err != nil {
		return nil, errors.Join(err, completion.Close())
	}
	lock := flock.New(filepath.Join(s.path, name, "owner.lock"), flock.SetFlag(os.O_RDWR))
	lease := &executionLease{ledger: s, name: name, Completion: completion, lock: lock}
	defer func() {
		if result != nil {
			result = errors.Join(result, lease.abortLocked())
		}
	}()
	if err := lock.Lock(); err != nil {
		return nil, err
	}
	record := useRecord{Generation: resource, Invocation: completion.ID(), Boot: s.boot}
	if err := saveJSON(s.root, filepath.Join(name, "owner.json"), record); err != nil {
		return nil, err
	}
	return lease, nil
}

func (u *executionLease) publishLocked() error {
	final := filepath.Join("uses", filepath.Base(u.name))
	if err := u.ledger.root.Rename(u.name, final); err != nil {
		return err
	}
	u.name = final
	return errors.Join(syncDirectory(u.ledger.root, "initializing"), syncDirectory(u.ledger.root, "uses"))
}

// No executable path escaped. Keep that proof across failed retirement or fsync.
func (u *executionLease) abortLocked() error {
	u.ledger.completedUses[u.name] = true
	err := errors.Join(u.Completion.Close(), u.ledger.releaseOwnerLocked(u.lock), u.ledger.removeOwned(u.name))
	u.ledger.recordCleanupErrorLocked(err)
	return err
}

func (u *executionLease) Finish(drained bool) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.finished {
		return nil
	}
	u.finished = true
	if drained {
		// Callers finish only once. Retain their proof before filesystem cleanup,
		// including failure to acquire coordination, so Sweep can retry it.
		u.ledger.mu.Lock()
		u.ledger.completedUses[u.name] = true
		u.ledger.mu.Unlock()
	}
	err := errors.Join(u.Completion.Close(), u.ledger.releaseOwner(u.lock))
	if drained {
		err = errors.Join(err, u.ledger.withLock(func() error {
			return u.ledger.removeOwned(u.name)
		}))
	}
	u.ledger.recordCleanupError(err)
	return err
}

func (s *ownershipLedger) removeOwned(name string) error {
	switch filepath.Dir(name) {
	case "claims", "uses", "initializing":
	default:
		return errors.New("invalid ownership retirement namespace")
	}
	err := retireToTrash(s.root, name)
	if err == nil {
		delete(s.completedUses, name)
	}
	return err
}

// ownerAlive is only for in-process claims/preparation, never for execution pins.
func (s *ownershipLedger) ownerAlive(name string) (bool, error) {
	info, err := s.root.Lstat(filepath.Join(name, "owner.lock"))
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() != 0 {
		return false, errors.New("invalid runtime operation owner")
	}
	lock := flock.New(filepath.Join(s.path, name, "owner.lock"), flock.SetFlag(os.O_RDWR))
	acquired, err := lock.TryLock()
	releaseErr := s.releaseOwnerLocked(lock)
	s.recordCleanupErrorLocked(releaseErr)
	return !acquired, errors.Join(err, releaseErr)
}

func (s *ownershipLedger) childDirectories(parent string) ([]string, error) {
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
