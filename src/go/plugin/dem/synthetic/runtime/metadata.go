// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

var ErrDamaged = errors.New("runtime metadata is damaged; stop consumers and use runtime repair or uninstall")

type metadataError struct {
	name string
	err  error
}

func (e *metadataError) Error() string { return fmt.Sprintf("%s: %v", e.name, e.err) }
func (e *metadataError) Unwrap() error { return e.err }
func isMetadataDamage(err error) bool {
	var damage *metadataError
	return errors.As(err, &damage)
}

func damagedMetadata(name string, err error) error {
	return &metadataError{
		name: name,
		err:  err,
	}
}

func metadataDecodeError(name string, err error) error {
	// os.File.Read wraps operational errors in PathError. They must not become
	// permission to replace data, including an error from the trailing-value read.
	var operational *os.PathError
	if errors.As(err, &operational) {
		return err
	}
	return damagedMetadata(name, err)
}

// The strict inventory remains the authority for automatic retirement and
// ordinary installation. Diagnostics and explicit recovery use the same
// ownership evidence without requiring usable executable metadata.
func (s *Store) inventoryLocked(reconcile bool) (inventory, error) {
	inv, err := s.inspectInventoryLocked(reconcile)
	if err != nil {
		return inv, err
	}
	return inv, inv.damageError()
}

func (s *Store) inspectInventoryLocked(reconcile bool) (inventory, error) {
	if reconcile {
		if err := s.syncSelection(); err != nil {
			return inventory{}, err
		}
	}
	inv, err := s.ownershipInventoryLocked(reconcile)
	if err != nil {
		return inv, err
	}
	inv.generationErrors = make(map[string]error)
	inv.selected, err = s.readSelection()
	if err != nil {
		if !isMetadataDamage(err) {
			return inv, err
		}
		inv.selectionError = err
	}
	names, err := s.childDirectories("generations")
	if err != nil {
		return inv, err
	}
	for _, name := range names {
		id := filepath.Base(name)
		g, err := s.loadGeneration(id)
		if err != nil {
			if !isMetadataDamage(err) {
				return inv, err
			}
			inv.generationErrors[id] = err
		}
		// Keep every known directory in accounting, even if its record is bad.
		inv.generations[id] = g
	}
	if inv.selected != nil {
		id := inv.selected.Archive.SHA256
		g, present := inv.generations[id]
		switch {
		case !present:
			inv.selectionError = damagedMetadata("selection.json", ErrUnavailable)
		case inv.generationErrors[id] != nil:
			inv.selectionError = inv.generationErrors[id]
		case !sameManifest(g.Manifest, *inv.selected):
			inv.selectionError = damagedMetadata("selection.json", errors.New("prepared runtime manifest differs from selected release"))
		}
	}
	// Corruption is not ordinary inactivity. Only a healthy inventory may
	// establish or update automatic retirement deadlines.
	if reconcile && inv.damageError() == nil {
		now := s.now().UTC()
		for id, g := range inv.generations {
			changed := false
			if inv.protected(id) {
				changed = !g.Unneeded.IsZero()
				g.Unneeded = time.Time{}
			} else if g.Unneeded.IsZero() {
				g.Unneeded = now
				changed = true
			}
			if changed {
				if err := saveJSON(s.root, filepath.Join("generations", id, "prepared.json"), g); err != nil {
					return inv, err
				}
			}
			inv.generations[id] = g
		}
	}
	return inv, nil
}

func (inv inventory) damageError() error {
	if inv.selectionError == nil && len(inv.generationErrors) == 0 {
		return nil
	}
	errs := []error{ErrDamaged, inv.selectionError}
	ids := make([]string, 0, len(inv.generationErrors))
	for id := range inv.generationErrors {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		errs = append(errs, inv.generationErrors[id])
	}
	return errors.Join(errs...)
}
