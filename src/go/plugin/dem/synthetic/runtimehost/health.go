// SPDX-License-Identifier: GPL-3.0-or-later

package runtimehost

import (
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/logger"
)

// Each operation owns its error slot. A successful sweep cannot erase an
// unresolved execution failure, nor can a healthy invocation erase failed cleanup.
type healthChange struct {
	operation string
	err       error
	changed   bool
}

func trackHealth(operation string, previous, current error) healthChange {
	return healthChange{operation: operation, err: current, changed: errorText(previous) != errorText(current)}
}

func (c healthChange) log(log *logger.Logger) {
	if !c.changed {
		return
	}
	if c.err != nil {
		log.Warningf("DEM runtime %s failed: %v", c.operation, c.err)
	} else {
		log.Noticef("DEM runtime %s recovered", c.operation)
	}
}

func labelled(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func (h *Host) updateMaintenanceLocked(operation string, err error) healthChange {
	change := trackHealth(operation, h.maintenance[operation], err)
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
	change.log(h.log)
}
func (h *Host) recordFinishError(err error) {
	h.mu.Lock()
	h.finishVersion++
	change := h.updateMaintenanceLocked("execution completion", err)
	h.mu.Unlock()
	change.log(h.log)
}
func (h *Host) maintenanceErrorLocked() error {
	return errors.Join(
		labelled("cleanup", h.maintenance["cleanup"]),
		labelled("execution completion", h.maintenance["execution completion"]),
	)
}
