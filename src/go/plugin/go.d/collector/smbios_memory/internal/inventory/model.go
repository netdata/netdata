// SPDX-License-Identifier: GPL-3.0-or-later

// Package inventory defines the firmware inventory shared with the read-only Function.
package inventory

import "time"

// Population is what firmware reports about a memory socket.
const (
	PopulationPopulated = "populated"
	PopulationEmpty     = "empty"
	PopulationUnknown   = "unknown"
)

// Inventory status: whether the current firmware table could be read.
const (
	InventoryAvailable   = "available"
	InventoryUnavailable = "unavailable"
)

// Comparison status: whether the current table can be compared with the accepted baseline.
const (
	ComparisonComparable   = "comparable"
	ComparisonUnavailable  = "unavailable"  // the firmware table could not be read
	ComparisonUncomparable = "uncomparable" // the table lacks stable slot identities or known capacities
	ComparisonUnbaselined  = "unbaselined"  // no baseline has been accepted and saved yet
	ComparisonStateError   = "state_error"  // the baseline state could not be loaded or saved
)

type Device struct {
	Handle          uint16  `json:"handle"`
	Locator         string  `json:"locator"`
	Bank            string  `json:"bank,omitempty"`
	Population      string  `json:"population"`
	Capacity        *uint64 `json:"capacity_bytes"`
	MemoryType      string  `json:"memory_type,omitempty"`
	FormFactor      string  `json:"form_factor,omitempty"`
	Manufacturer    string  `json:"manufacturer,omitempty"`
	Part            string  `json:"part,omitempty"`
	Serial          string  `json:"serial,omitempty"`
	Ranks           *uint64 `json:"ranks"`
	RatedSpeed      *uint64 `json:"rated_speed_mts"`
	ConfiguredSpeed *uint64 `json:"configured_speed_mts"`
}

// Table is one decoded firmware table: the physical system-memory devices and
// whether their totals and identities are reliable enough to compare.
type Table struct {
	Devices     []Device
	Capacity    *uint64 // total known capacity; nil when any contribution is unknown
	Populated   int
	Empty       int
	CountsKnown bool   // Populated and Empty cover every device
	Comparable  bool   // every device has a unique locator and a known capacity
	Reason      string // why the table is not comparable
}

type Row struct {
	Device           Device
	Availability     string
	Comparison       string
	BaselineCapacity *uint64
}

// Snapshot and everything it references are immutable after publication.
type Snapshot struct {
	ReadAt           time.Time
	InventoryStatus  string
	ComparisonStatus string
	Detail           string
	LossAt           time.Time
	Rows             []Row
}
