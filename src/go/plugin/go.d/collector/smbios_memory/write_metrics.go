// SPDX-License-Identifier: GPL-3.0-or-later

package smbios_memory

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/smbios_memory/internal/inventory"
)

// writeMetrics publishes status every cycle and measured quantities only when
// they were observed: unavailable measurements leave gaps rather than zeroes.
func (c *Collector) writeMetrics(table *inventory.Table, snapshot *inventory.Snapshot) {
	m := c.metrics
	state := c.baseline.current

	m.inventory.Enable(snapshot.InventoryStatus)
	m.comparison.Enable(snapshot.ComparisonStatus)
	m.loss.Enable(lossStatus(snapshot, state))

	if table != nil {
		if table.Capacity != nil {
			m.capacity.Observe(float64(*table.Capacity))
		}
		if table.CountsKnown {
			m.populated.Observe(float64(table.Populated))
			m.empty.Observe(float64(table.Empty))
		}
	}
	if snapshot.ComparisonStatus == inventory.ComparisonComparable {
		missing, deficit := lossTotals(state.Loss)
		m.missing.Observe(float64(missing))
		m.deficit.Observe(float64(deficit))
	}
}

func lossStatus(snapshot *inventory.Snapshot, state *persistentState) string {
	switch {
	case state != nil && len(state.Loss) > 0:
		return lossPresent
	case snapshot.ComparisonStatus == inventory.ComparisonComparable:
		return lossAbsent
	default:
		return lossUnknown
	}
}

func lossTotals(loss []discrepancy) (missing, deficit uint64) {
	for _, d := range loss {
		if d.Missing {
			missing++
		}
		deficit += d.Deficit
	}
	return missing, deficit
}
