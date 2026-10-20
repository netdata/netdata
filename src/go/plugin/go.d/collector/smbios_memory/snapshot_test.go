// SPDX-License-Identifier: GPL-3.0-or-later

package smbios_memory

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/smbios_memory/internal/inventory"
)

// Availability of a baseline-only row depends on whether the firmware table was
// read this cycle, not on whether it could be compared.
func TestInventoryRows_BaselineOnlyAvailability(t *testing.T) {
	accepted := inventory.Device{Locator: "DIMM_B", Population: "populated", Capacity: uintPtr(64 << 30)}
	state := &persistentState{
		Baseline: []inventory.Device{accepted},
		Loss:     []discrepancy{{Locator: "DIMM_B", Missing: true, Deficit: 64 << 30}},
	}
	tableWithoutB := &inventory.Table{Devices: []inventory.Device{
		{Locator: "DIMM_A", Population: "populated", Capacity: uintPtr(64 << 30)},
	}}
	for name, tc := range map[string]struct {
		table      *inventory.Table
		comparison string
		want       inventory.Row
	}{
		"table unreadable": {
			comparison: "unavailable",
			want: inventory.Row{
				Availability: "baseline only; current data unavailable", Comparison: "retained loss; unavailable",
			},
		},
		"table read but not comparable": {
			table: tableWithoutB, comparison: "uncomparable",
			want: inventory.Row{
				Availability: "absent from current firmware table", Comparison: "retained loss; uncomparable",
			},
		},
		"table read and comparable": {
			table: tableWithoutB, comparison: "comparable",
			want: inventory.Row{Availability: "absent from current firmware table", Comparison: "missing"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			rows := inventoryRows(tc.table, state, tc.comparison)
			row := rows[len(rows)-1]
			tc.want.Device = inventory.Device{Locator: "DIMM_B", Population: "unknown"}
			tc.want.BaselineCapacity = accepted.Capacity
			assert.Equal(t, tc.want, row)
		})
	}
}
