// SPDX-License-Identifier: GPL-3.0-or-later

package runtimehost

import (
	"errors"
	"fmt"
	"sort"
)

// Each operation owns its error slot. A successful sweep cannot erase an
// unresolved claim failure, nor can a healthy invocation erase failed cleanup.
type healthChange struct {
	operation string
	err       error
	changed   bool
}

func (h *Host) updateMaintenanceLocked(operation string, err error) healthChange {
	change := healthChange{operation: operation, err: err, changed: errorText(h.maintenance[operation]) != errorText(err)}
	if err == nil {
		delete(h.maintenance, operation)
	} else {
		h.maintenance[operation] = err
	}
	return change
}
func (h *Host) setMaintenance(operation string, err error) {
	h.mu.Lock()
	change := h.updateMaintenanceLocked(operation, err)
	h.mu.Unlock()
	h.logHealth(change)
}
func (h *Host) logHealth(change healthChange) {
	if !change.changed {
		return
	}
	if change.err != nil {
		h.log.Warningf("DEM runtime %s failed: %v", change.operation, change.err)
	} else {
		h.log.Noticef("DEM runtime %s recovered", change.operation)
	}
}
func (h *Host) recordFinishError(err error) {
	h.mu.Lock()
	h.finishVersion++
	change := h.updateMaintenanceLocked("execution completion", err)
	h.mu.Unlock()
	h.logHealth(change)
}
func (h *Host) maintenanceErrorLocked() error {
	var result error
	for _, operation := range []string{"demand commit", "demand release", "cleanup", "execution completion"} {
		if err := h.maintenance[operation]; err != nil {
			result = errors.Join(result, fmt.Errorf("%s: %w", operation, err))
		}
	}
	return result
}
func stableErrors(errs []error) error {
	sort.Slice(errs, func(i, j int) bool { return errs[i].Error() < errs[j].Error() })
	return errors.Join(errs...)
}
