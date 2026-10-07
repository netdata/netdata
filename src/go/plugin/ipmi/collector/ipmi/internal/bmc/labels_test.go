// SPDX-License-Identifier: GPL-3.0-or-later

package bmc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSensorTypeAndComponent(t *testing.T) {
	tests := map[string]struct {
		sensorType    uint8
		name          string
		wantType      string
		wantComponent string
	}{
		"memory module pattern": {
			sensorType:    0x01,
			name:          "DIMMA_TEMP",
			wantType:      "Temperature",
			wantComponent: "Memory Module",
		},
		"patterns are case-insensitive": {
			sensorType:    0x01,
			name:          "cpu dimm temp",
			wantType:      "Temperature",
			wantComponent: "Memory Module",
		},
		"processor pattern": {
			sensorType:    0x01,
			name:          "cpu temp",
			wantType:      "Temperature",
			wantComponent: "Processor",
		},
		"no pattern": {
			sensorType:    0x01,
			name:          "mystery temperature",
			wantType:      "Temperature",
			wantComponent: "Other",
		},
		"earlier memory pattern wins": {
			sensorType:    0x01,
			name:          "Memory RAID",
			wantType:      "Temperature",
			wantComponent: "Memory",
		},
		"storage pattern": {
			sensorType:    0x01,
			name:          "RAID",
			wantType:      "Temperature",
			wantComponent: "Storage",
		},
		"motherboard pattern": {
			sensorType:    0x01,
			name:          "I/O temp BD",
			wantType:      "Temperature",
			wantComponent: "Motherboard",
		},
		"power supply pattern": {
			sensorType:    0x01,
			name:          "PSU Temp",
			wantType:      "Temperature",
			wantComponent: "Power Supply",
		},
		"system pattern": {
			sensorType:    0x01,
			name:          "sel",
			wantType:      "Temperature",
			wantComponent: "System",
		},
		"processor type overrides name": {
			sensorType:    0x07,
			name:          "DIMM CPU",
			wantType:      "Processor",
			wantComponent: "Processor",
		},
		"power supply type without match": {
			sensorType:    0x08,
			name:          "unmatched",
			wantType:      "Power Supply",
			wantComponent: "Power Supply",
		},
		"OEM sensor type": {
			sensorType:    0xc0,
			name:          "vendor sensor",
			wantType:      "OEM",
			wantComponent: "Other",
		},
		"unrecognized sensor type": {
			sensorType:    0x2d,
			name:          "future sensor",
			wantType:      "Unrecognized",
			wantComponent: "Other",
		},
		"memory type overrides name": {
			sensorType:    0x0c,
			name:          "DIMM",
			wantType:      "Memory",
			wantComponent: "Memory",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			gotType, gotComponent := sensorTypeAndComponent(tc.sensorType, tc.name)
			assert.Equal(t, tc.wantType, gotType)
			assert.Equal(t, tc.wantComponent, gotComponent)
		})
	}
}
