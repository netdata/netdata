// SPDX-License-Identifier: GPL-3.0-or-later

package smbios_memory

import (
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/smbios_memory/internal/inventory"
)

// Confirmed-loss status: whether the accepted baseline currently records a loss.
const (
	lossPresent = "present"
	lossAbsent  = "absent"
	lossUnknown = "unknown" // no comparable observation and no retained loss
)

var (
	inventoryStates  = []string{inventory.InventoryAvailable, inventory.InventoryUnavailable}
	comparisonStates = []string{
		inventory.ComparisonComparable,
		inventory.ComparisonUnavailable,
		inventory.ComparisonUncomparable,
		inventory.ComparisonUnbaselined,
		inventory.ComparisonStateError,
	}
	lossStates = []string{lossPresent, lossAbsent, lossUnknown}
)

type collectorMetrics struct {
	inventory  metrix.StateSetInstrument
	comparison metrix.StateSetInstrument
	loss       metrix.StateSetInstrument

	capacity  metrix.SnapshotGauge
	populated metrix.SnapshotGauge
	empty     metrix.SnapshotGauge
	missing   metrix.SnapshotGauge
	deficit   metrix.SnapshotGauge
}

func newCollectorMetrics(store metrix.CollectorStore) *collectorMetrics {
	m := store.Write().SnapshotMeter("")
	stateSet := func(name string, states []string) metrix.StateSetInstrument {
		return m.StateSet(name, metrix.WithStateSetMode(metrix.ModeEnum), metrix.WithStateSetStates(states...))
	}
	return &collectorMetrics{
		inventory:  stateSet("inventory_status", inventoryStates),
		comparison: stateSet("comparison_status", comparisonStates),
		loss:       stateSet("confirmed_loss_status", lossStates),
		capacity:   m.Gauge("installed_capacity_bytes"),
		populated:  m.Gauge("populated_slots"),
		empty:      m.Gauge("empty_slots"),
		missing:    m.Gauge("missing_devices"),
		deficit:    m.Gauge("capacity_deficit_bytes"),
	}
}
