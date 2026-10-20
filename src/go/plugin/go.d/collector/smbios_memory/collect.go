// SPDX-License-Identifier: GPL-3.0-or-later

package smbios_memory

import (
	"cmp"
	"context"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/smbios_memory/internal/inventory"
)

// collect reads the firmware table, reconciles it with the accepted baseline,
// publishes the Function snapshot and writes the host-level metrics.
func (c *Collector) collect(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.baseline.load()

	now := c.now()
	table, readErr := c.readTable()
	if err := ctx.Err(); err != nil {
		return err
	}

	snapshot := newSnapshot(now, table, readErr)

	var writeErr error
	if table != nil && table.Comparable {
		writeErr = c.baseline.reconcile(table, now)
		if c.baseline.current != nil && writeErr == nil {
			snapshot.ComparisonStatus = inventory.ComparisonComparable
		}
	}
	if writeErr == nil {
		writeErr = c.baseline.flush()
	}
	if err := cmp.Or(c.baseline.err, writeErr); err != nil {
		snapshot.ComparisonStatus = inventory.ComparisonStateError
		snapshot.Detail = joinDetail(snapshot.Detail, err.Error())
	}
	if snapshot.Detail != "" {
		c.Limit("smbios_memory:collection", 1, time.Hour).Warningf("Memory inventory: %s", snapshot.Detail)
	}

	if state := c.baseline.current; state != nil {
		snapshot.LossAt = state.LossAt
	}
	snapshot.Rows = inventoryRows(table, c.baseline.current, snapshot.ComparisonStatus)
	c.snapshot.Store(snapshot)

	c.writeMetrics(table, snapshot)
	// Domain failures are observable states; returning an error would abort their metrics.
	return nil
}

// newSnapshot classifies the read result before any baseline comparison.
func newSnapshot(now time.Time, table *inventory.Table, readErr error) *inventory.Snapshot {
	s := &inventory.Snapshot{ReadAt: now}
	switch {
	case readErr != nil:
		s.InventoryStatus = inventory.InventoryUnavailable
		s.ComparisonStatus = inventory.ComparisonUnavailable
		s.Detail = readErr.Error()
	case !table.Comparable:
		s.InventoryStatus = inventory.InventoryAvailable
		s.ComparisonStatus = inventory.ComparisonUncomparable
		s.Detail = table.Reason
	default:
		s.InventoryStatus = inventory.InventoryAvailable
		s.ComparisonStatus = inventory.ComparisonUnbaselined
	}
	return s
}

func joinDetail(detail, more string) string {
	if detail == "" {
		return more
	}
	return detail + "; " + more
}
