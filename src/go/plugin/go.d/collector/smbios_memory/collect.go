// SPDX-License-Identifier: GPL-3.0-or-later

package smbios_memory

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/smbios_memory/internal/inventory"
)

func (c *Collector) Collect(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !c.stateLoaded {
		// Init/Check also run for candidates while an older job may still own the state.
		// The first active Collect runs after that owner has stopped.
		c.state, c.stateErr = readState(c.statePath, c.owner)
		if errors.Is(c.stateErr, os.ErrNotExist) {
			c.stateErr = nil
		}
		if c.owner == "" {
			c.stateErr = errors.New("Agent registry identity is unavailable; baseline persistence disabled")
		}
		c.stateLoaded = true
	}
	now := c.now()
	table, readErr := c.read()
	if err := ctx.Err(); err != nil {
		return err
	}
	snapshot := &inventory.Snapshot{
		ReadAt:           now,
		InventoryStatus:  "unavailable",
		ComparisonStatus: "unavailable",
	}
	var writeErr error
	if readErr != nil {
		snapshot.Detail = readErr.Error()
	} else {
		snapshot.InventoryStatus = "available"
		snapshot.ComparisonStatus = "uncomparable"
		snapshot.Detail = table.Reason
		if table.Comparable {
			snapshot.ComparisonStatus = "unbaselined"
			if c.stateErr == nil && (c.state != nil || table.Populated > 0) {
				next := proposedState(c.state, table, c.owner, now)
				if c.dirty || !reflect.DeepEqual(c.state, next) {
					writeErr = saveState(c.statePath, next)
					if writeErr == nil {
						c.state = next
						c.dirty = false
					} else if c.state != nil && len(next.Loss) > 0 {
						// Remember observed loss even if storage is temporarily unavailable.
						// Neither baseline increases nor restorations are accepted on failure.
						c.state.Loss = next.Loss
						c.state.LossAt = next.LossAt
						c.dirty = true
					}
				}
				if c.state != nil && writeErr == nil {
					snapshot.ComparisonStatus = "comparable"
				}
			}
		}
	}
	if c.dirty && writeErr == nil {
		writeErr = saveState(c.statePath, c.state)
		if writeErr == nil {
			c.dirty = false
		}
	}
	if c.stateErr != nil || writeErr != nil {
		snapshot.ComparisonStatus = "state_error"
		err := c.stateErr
		if err == nil {
			err = writeErr
		}
		if snapshot.Detail != "" {
			snapshot.Detail += "; "
		}
		snapshot.Detail += err.Error()
	}
	if snapshot.Detail != "" {
		c.Limit("smbios_memory:collection", 1, time.Hour).Warningf("Memory inventory: %s", snapshot.Detail)
	}
	if c.state != nil {
		snapshot.LossAt = c.state.LossAt
	}
	snapshot.Rows = inventoryRows(table, c.state, snapshot.ComparisonStatus)
	c.snapshot.Store(snapshot)
	c.metrics.observe(table, snapshot, c.state)
	// Domain failures are observable states; returning an error would abort their metrics.
	return nil
}

func inventoryRows(table *inventory.Table, state *persistentState, comparison string) []inventory.Row {
	var rows []inventory.Row
	baseline := make(map[string]inventory.Device)
	loss := make(map[string]discrepancy)
	if state != nil {
		for _, d := range state.Baseline {
			baseline[d.Locator] = d
		}
		for _, d := range state.Loss {
			loss[d.Locator] = d
		}
	}
	seen := make(map[string]bool)
	if table != nil {
		for _, d := range table.Devices {
			seen[d.Locator] = true
			row := inventory.Row{
				Device:       d,
				Availability: "current firmware table",
				Comparison:   comparison,
			}
			if old, ok := baseline[d.Locator]; ok {
				row.BaselineCapacity = old.Capacity
				if comparison == "comparable" {
					row.Comparison = "unchanged"
					if d.Population == "empty" {
						row.Comparison = "missing"
					} else if *d.Capacity < *old.Capacity {
						row.Comparison = "reduced capacity"
					}
				}
			} else if comparison == "comparable" {
				row.Comparison = "not in populated baseline"
			}
			rows = append(rows, row)
		}
	}
	if state != nil {
		for _, d := range state.Baseline {
			if seen[d.Locator] {
				continue
			}
			row := inventory.Row{
				Device:           d,
				Availability:     "baseline only; current data unavailable",
				Comparison:       comparison,
				BaselineCapacity: d.Capacity,
			}
			// The device metadata describes the accepted device; it is not a current measurement.
			row.Device.Capacity = nil
			row.Device.Population = "unknown"
			row.Device.RatedSpeed = nil
			row.Device.ConfiguredSpeed = nil
			row.Device.Ranks = nil
			if comparison == "comparable" {
				row.Availability = "absent from current firmware table"
				row.Comparison = "missing"
			}
			if _, ok := loss[d.Locator]; ok && comparison != "comparable" {
				row.Comparison = fmt.Sprintf("retained loss; %s", comparison)
			}
			rows = append(rows, row)
		}
	}
	return rows
}
