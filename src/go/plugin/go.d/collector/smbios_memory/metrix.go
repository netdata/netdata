// SPDX-License-Identifier: GPL-3.0-or-later

package smbios_memory

import (
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/smbios_memory/internal/inventory"
)

var inventoryStates = []string{"available", "unavailable"}
var comparisonStates = []string{"comparable", "unavailable", "uncomparable", "unbaselined", "state_error"}
var lossStates = []string{"present", "absent", "unknown"}

type collectorMetrics struct {
	inventory, comparison, loss                  metrix.StateSetInstrument
	capacity, populated, empty, missing, deficit metrix.SnapshotGauge
}

func newCollectorMetrics(store metrix.CollectorStore) *collectorMetrics {
	m := store.Write().SnapshotMeter("")
	return &collectorMetrics{
		inventory: m.StateSet(
			"inventory_status",
			metrix.WithStateSetMode(metrix.ModeEnum),
			metrix.WithStateSetStates(inventoryStates...),
		),
		comparison: m.StateSet(
			"comparison_status",
			metrix.WithStateSetMode(metrix.ModeEnum),
			metrix.WithStateSetStates(comparisonStates...),
		),
		loss: m.StateSet(
			"confirmed_loss_status",
			metrix.WithStateSetMode(metrix.ModeEnum),
			metrix.WithStateSetStates(lossStates...),
		),
		capacity: m.Gauge(
			"installed_capacity_bytes",
		),
		populated: m.Gauge("populated_slots"),
		empty:     m.Gauge("empty_slots"),
		missing:   m.Gauge("missing_devices"),
		deficit:   m.Gauge("capacity_deficit_bytes"),
	}
}
func (m *collectorMetrics) observe(table *inventory.Table, snapshot *inventory.Snapshot, state *persistentState) {
	m.inventory.Enable(snapshot.InventoryStatus)
	m.comparison.Enable(snapshot.ComparisonStatus)
	loss := "unknown"
	if state != nil && len(state.Loss) > 0 {
		loss = "present"
	} else if snapshot.ComparisonStatus == "comparable" {
		loss = "absent"
	}
	m.loss.Enable(loss)
	if table != nil {
		if table.Capacity != nil {
			m.capacity.Observe(float64(*table.Capacity))
		}
		if table.CountsKnown {
			m.populated.Observe(float64(table.Populated))
			m.empty.Observe(float64(table.Empty))
		}
	}
	if snapshot.ComparisonStatus == "comparable" {
		var missing, deficit uint64
		for _, d := range state.Loss {
			if d.Missing {
				missing++
			}
			deficit += d.Deficit
		}
		m.missing.Observe(float64(missing))
		m.deficit.Observe(float64(deficit))
	}
}
