// SPDX-License-Identifier: GPL-3.0-or-later

package smbios_memory

import (
	"reflect"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/smbios_memory/internal/inventory"
)

// slotKey identifies a physical memory slot across observations. Firmware may
// reuse a device locator in every bank ("DIMM 0" on each channel), so the bank
// locator is part of the identity. The DMI handle is not: it can change with
// the record order or a firmware update.
type slotKey struct{ bank, locator string }

func slotOf(d inventory.Device) slotKey {
	return slotKey{
		bank:    d.Bank,
		locator: d.Locator,
	}
}

// proposedState is the state a comparable table leads to. A loss against the
// accepted slots keeps the baseline and records the loss; otherwise the table's
// populated slots become the baseline, so additions are accepted only after
// proving that no previously accepted slot shrank.
func proposedState(current *persistentState, table *inventory.Table, owner string, now time.Time) *persistentState {
	if current != nil {
		if loss := detectLoss(current.Baseline, table.Devices); len(loss) > 0 {
			next := *current
			if !reflect.DeepEqual(loss, current.Loss) {
				next.Loss, next.LossAt = loss, now
			}
			return &next
		}
	}

	next := &persistentState{
		Version:    stateVersion,
		Owner:      owner,
		AcceptedAt: now,
		Baseline:   populatedDevices(table.Devices),
	}
	if current != nil && sameCapacities(current.Baseline, next.Baseline) {
		// Descriptive metadata changes are not baseline changes or state writes.
		next.Baseline = current.Baseline
		next.AcceptedAt = current.AcceptedAt
	}
	return next
}

// detectLoss reports every accepted slot that is absent, empty or smaller now.
func detectLoss(accepted, devices []inventory.Device) []discrepancy {
	bySlot := make(map[slotKey]inventory.Device, len(devices))
	for _, d := range devices {
		bySlot[slotOf(d)] = d
	}
	var loss []discrepancy
	for _, old := range accepted {
		d, exists := bySlot[slotOf(old)]
		var capacity uint64
		if exists {
			capacity = *d.Capacity
		}
		if capacity < *old.Capacity {
			loss = append(loss, discrepancy{
				Bank:    old.Bank,
				Locator: old.Locator,
				Missing: !exists || d.Population == inventory.PopulationEmpty,
				Deficit: *old.Capacity - capacity,
			})
		}
	}
	return loss
}

func populatedDevices(devices []inventory.Device) []inventory.Device {
	var populated []inventory.Device
	for _, d := range devices {
		if d.Population == inventory.PopulationPopulated {
			populated = append(populated, d)
		}
	}
	return populated
}

func sameCapacities(a, b []inventory.Device) bool {
	if len(a) != len(b) {
		return false
	}
	bySlot := make(map[slotKey]uint64, len(a))
	for _, d := range a {
		bySlot[slotOf(d)] = *d.Capacity
	}
	for _, d := range b {
		if capacity, ok := bySlot[slotOf(d)]; !ok || capacity != *d.Capacity {
			return false
		}
	}
	return true
}
