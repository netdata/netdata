// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

// Use protects every access that may execute the managed runtime, including
// version probes. Pass Completion to ndexec, then Finish only after joining it.
// If launch never started, the owning caller may Finish(true). Lost callers
// cannot make that assertion; recovery requires a receipt or verified host reboot.
type Use struct {
	*executionLease
	Paths Paths
}

func (s *Store) beginUse(m Manifest) (*Use, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	var use *Use
	err := s.withLock(func() (result error) {
		paths, err := s.prepared(m)
		if err != nil {
			return err
		}
		lease, err := s.newUseLocked(m.Archive.SHA256)
		if err != nil {
			return err
		}
		defer func() {
			if result != nil {
				result = errors.Join(result, lease.abortLocked())
			}
		}()
		g, err := s.loadGeneration(m.Archive.SHA256)
		if err == nil {
			g.LastUsed = s.now().UTC()
			err = saveJSON(s.root, filepath.Join("generations", m.Archive.SHA256, "prepared.json"), g)
		}
		if err != nil {
			return err
		}
		if err := lease.publishLocked(); err != nil {
			return err
		}
		use = &Use{Paths: paths, executionLease: lease}
		return nil
	})
	if err != nil && use != nil {
		// No executable path escaped to the caller.
		return nil, errors.Join(err, use.Finish(true))
	}
	return use, err
}

func (s *Store) finishPreparation(name string, owner io.Closer) {
	err := s.releaseOwner(owner)
	if err == nil {
		err = s.withLock(func() error { return s.removeOwned(name) })
	}
	s.recordCleanupError(errors.Join(err, s.flushTrash()))
}

// closeClaim preserves managed reconciliation after releasing common proof.
func (s *Store) closeClaim(c *claim) error {
	retired, err := c.close()
	if retired && err == nil {
		err = s.withLock(func() error { _, err := s.inventoryLocked(true); return err })
		s.recordCleanupError(err)
	}
	return err
}

// Managed payloads share trash with proof records, but their publication
// barriers remain the responsibility of Store.
func (s *Store) removeOwned(name string) error {
	switch filepath.Dir(name) {
	case "claims", "uses", "initializing":
		return s.ownershipLedger.removeOwned(name)
	case "staging":
		if _, err := s.root.Lstat(name); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		} else if err == nil {
			if err := errors.Join(syncDirectory(s.root, name), syncDirectory(s.root, "generations")); err != nil {
				return err
			}
		}
	case "generations":
	default:
		return errors.New("invalid managed retirement namespace")
	}
	return retireToTrash(s.root, name)
}
