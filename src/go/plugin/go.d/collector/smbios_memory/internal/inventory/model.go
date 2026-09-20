// SPDX-License-Identifier: GPL-3.0-or-later

// Package inventory defines the firmware inventory shared with the read-only Function.
package inventory

import "time"

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

type Table struct {
	CountsKnown bool
	Devices     []Device
	Comparable  bool
	Reason      string
	Capacity    *uint64
	Populated   int
	Empty       int
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
