// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"time"
)

var ErrDamaged = errors.New("runtime metadata is damaged; stop consumers and use runtime repair or uninstall")

// damaged reports damage at a public boundary. The cause is kept as text only:
// a declared but unusable record must never satisfy errors.Is for an ordinary
// condition such as ErrUnavailable or os.ErrNotExist.
func damaged(err error) error {
	return fmt.Errorf("%w: %v", ErrDamaged, err)
}

type reservation struct {
	Generation string `json:"generation"`
	Bytes      int64  `json:"bytes"`
}

type inventory struct {
	ownershipInventory
	selected         *Manifest
	release          *Release
	automaticUpdates bool
	selectionError   error
	generationErrors map[string]error
	generations      map[string]generation
	preparing        map[string]int
	reservations     map[string]reservation
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
	record, err := s.readSelectionRecord()
	if err != nil {
		if !isMetadataDamage(err) {
			return inv, err
		}
		inv.selectionError = err
	} else {
		inv.selected = record.manifest()
		inv.release = record.Release
		inv.automaticUpdates = record.AutomaticUpdates
	}
	names, err := s.ledger.childDirectories("generations")
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
		case !sameExecutable(g.Manifest, *inv.selected):
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

func (s *Store) ownershipInventoryLocked(reconcile bool) (inventory, error) {
	owned, err := s.ledger.inventoryLocked(reconcile)
	inv := inventory{
		ownershipInventory: owned,
		generations:        make(map[string]generation),
		preparing:          make(map[string]int),
		reservations:       make(map[string]reservation),
	}
	if err != nil {
		return inv, err
	}
	staging, err := s.ledger.childDirectories("staging")
	if err != nil {
		return inv, err
	}
	for _, name := range staging {
		alive, err := s.ledger.ownerAlive(name)
		if err != nil {
			return inv, err
		}
		if !alive {
			inv.reclaimableRecords[name] = true
			if !reconcile {
				continue
			}
			if err := s.removeOwned(name); err != nil {
				return inv, err
			}
			continue
		}
		var r reservation
		if err := readJSON(s.root, filepath.Join(name, "owner.json"), &r); err != nil {
			return inv, err
		}
		if !validID(r.Generation) || r.Bytes <= 0 {
			return inv, errors.New("invalid runtime reservation")
		}
		inv.reservations[name] = r
		inv.protectedRecords[name] = true
		inv.preparing[r.Generation]++
	}
	return inv, nil
}

func (inv inventory) damageError() error {
	if inv.selectionError == nil && len(inv.generationErrors) == 0 {
		return nil
	}
	errs := []error{inv.selectionError}
	ids := make([]string, 0, len(inv.generationErrors))
	for id := range inv.generationErrors {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		errs = append(errs, inv.generationErrors[id])
	}
	return damaged(errors.Join(errs...))
}

func (inv inventory) protected(id string) bool {
	return (inv.selected != nil && inv.selected.Archive.SHA256 == id) || inv.processes[id] > 0 || inv.candidates[id] > 0 || inv.preparing[id] > 0 || inv.uses[id] > 0
}

// inUse reports consumers, candidates, executions or reservations that block
// uninstall and repair.
func (inv inventory) inUse() bool {
	return len(inv.processes) > 0 || len(inv.candidates) > 0 || len(inv.uses) > 0 || len(inv.reservations) > 0
}
