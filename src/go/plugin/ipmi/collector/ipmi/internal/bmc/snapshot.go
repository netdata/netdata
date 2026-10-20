// SPDX-License-Identifier: GPL-3.0-or-later

package bmc

import "time"

// Sensor health states of the FreeIPMI default interpretation policy.
const (
	StateNominal  = "nominal"
	StateWarning  = "warning"
	StateCritical = "critical"
	StateUnknown  = "unknown"
)

// Units of numeric readings. A sensor in any other unit keeps its state but has no value.
const (
	UnitCelsius    = "Celsius"
	UnitFahrenheit = "Fahrenheit"
	UnitVolts      = "Volts"
	UnitAmps       = "Amps"
	UnitRPM        = "RPM"
	UnitWatts      = "Watts"
	UnitPercent    = "%"
)

// Snapshot is one completed collection. Collect never modifies a returned Snapshot.
type Snapshot struct {
	Sensors []Sensor
	// SELEntries is the raw System Event Log entry count; nil when not requested or unavailable.
	SELEntries  *int
	CollectedAt time.Time
	// Warnings summarize partially collected data in a fixed vocabulary.
	Warnings []string
}

// Sensor is one sensor of the BMC's SDR repository.
type Sensor struct {
	// Key identifies the sensor record; it does not change when the reading becomes unavailable.
	Key       string
	Name      string
	Type      string
	Component string
	// Unit is empty for discrete sensors and for numeric sensors in unsupported units.
	Unit  string
	State string
	// Value is nil when the reading is unavailable, unsupported or invalid.
	Value *float64
}
