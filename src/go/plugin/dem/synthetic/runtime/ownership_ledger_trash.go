// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"errors"
	"os"
	"path/filepath"
)

// retireToTrash is shared durability machinery, not retirement policy. Callers
// authorize the namespace and complete payload-specific publication barriers.
func retireToTrash(root *os.Root, name string) error {
	id, err := randomID()
	if err != nil {
		return err
	}
	if err := root.Rename(name, filepath.Join("trash", id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// Re-sync even when a successor already moved the record or fsync failed.
	return errors.Join(syncDirectory(root, filepath.Dir(name)), syncDirectory(root, "trash"))
}

// trashSnapshotLocked requires the payload owner's barriers first when sharing
// its root. This method never sweeps or deletes managed payloads by itself.
func (s *ownershipLedger) trashSnapshotLocked() ([]string, error) {
	names, err := s.childDirectories("trash")
	if err != nil {
		return nil, err
	}
	for _, parent := range []string{"uses", "claims", "trash", "initializing"} {
		if err := syncDirectory(s.root, parent); err != nil {
			return nil, err
		}
	}
	return names, nil
}

// Delete only the established snapshot outside coordination exclusion. New
// retirements are left for a later sweep with their own durability barriers.
func (s *ownershipLedger) deleteTrash(names []string) error {
	var result error
	for _, name := range names {
		result = errors.Join(result, s.root.RemoveAll(name))
	}
	return errors.Join(result, syncDirectory(s.root, "trash"))
}
