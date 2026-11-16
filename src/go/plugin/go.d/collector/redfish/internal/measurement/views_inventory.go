// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

import (
	"math"
	"time"
)

func (v *Component) addInventory(node *Resource) {
	data := Properties(node.Data)
	switch node.Kind {
	case "processor":
		v.TotalCores = inventoryNumber(data["TotalCores"], 1)
		v.EnabledCores = inventoryNumber(data["TotalEnabledCores"], 1)
		v.TotalThreads = inventoryNumber(data["TotalThreads"], 1)
	case "memory":
		v.CapacityBytes = inventoryNumber(data["CapacityMiB"], 1024*1024)
		v.MemoryType, _ = data.Text("MemoryDeviceType")
	case "drive":
		v.CapacityBytes = inventoryNumber(data["CapacityBytes"], 1)
		v.MediaType, _ = data.Text("MediaType")
		v.DriveProtocol, _ = data.Text("Protocol")
	case "volume":
		v.CapacityBytes = inventoryNumber(data["CapacityBytes"], 1)
		v.RAIDType, _ = data.Text("RAIDType")
	case "firmware", "software":
		v.Firmware, _ = data.Text("Version")
		if date, ok := data.Text("ReleaseDate"); ok {
			v.ReleaseDate, _ = time.Parse(time.RFC3339Nano, date)
		}
	case "assembly":
		v.HardwareVersion, _ = data.Text("Version")
		v.EngineeringRevision, _ = data.Text("EngineeringChangeLevel")
	}
}

// Inventory counts and capacities are nonnegative integers. Keep missing values
// distinct from zero, including when converting memory capacity from MiB to bytes.
func inventoryNumber(raw any, multiplier float64) *float64 {
	_, value, ok := numericValue(raw)
	if !ok || value < 0 || math.Trunc(value) != value {
		return nil
	}
	value *= multiplier
	if !isFinite(value) {
		return nil
	}
	return &value
}
