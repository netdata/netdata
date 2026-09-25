// SPDX-License-Identifier: GPL-3.0-or-later

package smbios_memory

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/smbios_memory/internal/inventory"
)

// Function row vocabulary. A row's Comparison is the comparison status until a
// baseline comparison refines it per slot.
const (
	availabilityCurrent      = "current firmware table"
	availabilityBaselineOnly = "baseline only; current data unavailable"
	availabilityAbsent       = "absent from current firmware table"

	comparisonUnchanged     = "unchanged"
	comparisonMissing       = "missing"
	comparisonReduced       = "reduced capacity"
	comparisonNotInBaseline = "not in populated baseline"
)

// inventoryRows lists every device in the current table, then every accepted
// slot the current table does not mention. A nil table means the firmware
// table could not be read this cycle.
func inventoryRows(table *inventory.Table, state *persistentState, comparison string) []inventory.Row {
	baseline := make(map[string]inventory.Device)
	lost := make(map[string]bool)
	if state != nil {
		for _, d := range state.Baseline {
			baseline[d.Locator] = d
		}
		for _, d := range state.Loss {
			lost[d.Locator] = true
		}
	}

	var rows []inventory.Row
	seen := make(map[string]bool)
	if table != nil {
		for _, d := range table.Devices {
			seen[d.Locator] = true
			old, accepted := baseline[d.Locator]
			rows = append(rows, currentRow(d, old, accepted, comparison))
		}
	}
	if state != nil {
		for _, d := range state.Baseline {
			if !seen[d.Locator] {
				rows = append(rows, baselineOnlyRow(d, lost[d.Locator], comparison, table != nil))
			}
		}
	}
	return rows
}

func currentRow(d, old inventory.Device, accepted bool, comparison string) inventory.Row {
	row := inventory.Row{
		Device:       d,
		Availability: availabilityCurrent,
		Comparison:   comparison,
	}
	if accepted {
		row.BaselineCapacity = old.Capacity
	}
	if comparison != inventory.ComparisonComparable {
		return row
	}
	switch {
	case !accepted:
		row.Comparison = comparisonNotInBaseline
	case d.Population == inventory.PopulationEmpty:
		row.Comparison = comparisonMissing
	case *d.Capacity < *old.Capacity:
		row.Comparison = comparisonReduced
	default:
		row.Comparison = comparisonUnchanged
	}
	return row
}

func baselineOnlyRow(d inventory.Device, lost bool, comparison string, tableRead bool) inventory.Row {
	row := inventory.Row{
		Availability:     availabilityBaselineOnly,
		Comparison:       comparison,
		BaselineCapacity: d.Capacity,
	}
	if tableRead {
		row.Availability = availabilityAbsent
	}
	// The device metadata describes the accepted device; it is not a current measurement.
	d.Capacity = nil
	d.Population = inventory.PopulationUnknown
	d.RatedSpeed = nil
	d.ConfiguredSpeed = nil
	d.Ranks = nil
	row.Device = d
	switch {
	case comparison == inventory.ComparisonComparable:
		row.Comparison = comparisonMissing
	case lost:
		row.Comparison = fmt.Sprintf("retained loss; %s", comparison)
	}
	return row
}
