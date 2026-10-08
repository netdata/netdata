// SPDX-License-Identifier: GPL-3.0-or-later

package bmc

import (
	"testing"

	"github.com/bougou/go-ipmi/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDescribeSDR_Latin1Name(t *testing.T) {
	// Type 3 ID strings are Latin-1; the SDK returns their bytes without UTF-8 conversion.
	sdr, err := types.ParseSDR(fullRecord(1, 10, string([]byte{'T', 0xe9, 'm', 'p'})), 0xffff)
	require.NoError(t, err)

	got := describeSDR(sdr)
	require.Len(t, got, 1)
	assert.True(t, got[0].supported)
	assert.Equal(t, Sensor{
		Key:       "i1_n10_t2_u1_Témp",
		Name:      "Témp",
		Type:      "Temperature",
		Component: componentOther,
		Unit:      UnitCelsius,
		State:     StateUnknown,
	}, got[0].sensor)
}

func TestUnitOf(t *testing.T) {
	// Legacy codes are libipmimonitoring's IPMI_MONITORING_SENSOR_UNITS_* values.
	tests := map[string]struct {
		unit       types.SensorUnit
		wantName   string
		wantLegacy int
	}{
		"degrees C": {
			unit: types.SensorUnit{
				BaseUnit: types.SensorUnitType_DegreesC,
			},
			wantName:   UnitCelsius,
			wantLegacy: 1,
		},
		"degrees F": {
			unit: types.SensorUnit{
				BaseUnit: types.SensorUnitType_DegreesF,
			},
			wantName:   UnitFahrenheit,
			wantLegacy: 2,
		},
		"volts": {
			unit: types.SensorUnit{
				BaseUnit: types.SensorUnitType_Volts,
			},
			wantName:   UnitVolts,
			wantLegacy: 3,
		},
		"amps": {
			unit: types.SensorUnit{
				BaseUnit: types.SensorUnitType_Amps,
			},
			wantName:   UnitAmps,
			wantLegacy: 4,
		},
		"RPM": {
			unit: types.SensorUnit{
				BaseUnit: types.SensorUnitType_RPM,
			},
			wantName:   UnitRPM,
			wantLegacy: 5,
		},
		"RPM per minute": {
			unit: types.SensorUnit{
				BaseUnit: types.SensorUnitType_RPM,
				RateUnit: types.SensorRateUnit_PerMin,
			},
			wantName:   UnitRPM,
			wantLegacy: 5,
		},
		"watts": {
			unit: types.SensorUnit{
				BaseUnit: types.SensorUnitType_Watts,
			},
			wantName:   UnitWatts,
			wantLegacy: 6,
		},
		"unitless percentage": {
			unit: types.SensorUnit{
				Percentage: true,
			},
			wantName:   UnitPercent,
			wantLegacy: 7,
		},
		"unsupported base unit": {
			unit: types.SensorUnit{
				BaseUnit: types.SensorUnitType_DegreesK,
			},
			wantLegacy: 0xff,
		},
		"rate other than RPM per minute": {
			unit: types.SensorUnit{
				BaseUnit: types.SensorUnitType_Volts,
				RateUnit: types.SensorRateUnit_PerSec,
			},
			wantLegacy: 0xff,
		},
		"modifier unit": {
			unit: types.SensorUnit{
				BaseUnit:     types.SensorUnitType_Volts,
				ModifierUnit: types.SensorUnitType_Amps,
			},
			wantLegacy: 0xff,
		},
		"modifier relation": {
			unit: types.SensorUnit{
				BaseUnit:         types.SensorUnitType_Volts,
				ModifierRelation: types.SensorModifierRelation_Div,
			},
			wantLegacy: 0xff,
		},
		"percentage of a base unit": {
			unit: types.SensorUnit{
				BaseUnit:   types.SensorUnitType_Volts,
				Percentage: true,
			},
			wantLegacy: 0xff,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			gotName, gotLegacy := unitOf(tc.unit)
			assert.Equal(t, tc.wantName, gotName)
			assert.Equal(t, tc.wantLegacy, gotLegacy)
		})
	}
}

func TestDescribeSDR_SharedSensors(t *testing.T) {
	tests := map[string]struct {
		first         uint8
		count         byte
		modifierType  byte // 00b numeric, 01b alphabetic, others reserved
		wantNumbers   []uint8
		wantSupported bool
	}{
		"ends below the reserved number": {
			first:         0xfd,
			count:         2,
			wantNumbers:   []uint8{0xfd, 0xfe},
			wantSupported: true,
		},
		"reaches the reserved number FFh": {
			first:       0xfe,
			count:       2,
			wantNumbers: []uint8{0xfe, 0xff},
		},
		"reserved instance modifier type": {
			first:        0x10,
			count:        2,
			modifierType: 0b10,
			wantNumbers:  []uint8{0x10, 0x11},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			record := compactRecord(1, tc.first, 0x08, "PSU")
			record[compactShareCount] = tc.modifierType<<4 | tc.count
			sdr, err := types.ParseSDR(record, 0xffff)
			require.NoError(t, err)

			var numbers []uint8
			for _, d := range describeSDR(sdr) {
				numbers = append(numbers, d.number)
				assert.Equal(t, tc.wantSupported, d.supported, "sensor %#02x", d.number)
			}
			assert.Equal(t, tc.wantNumbers, numbers)
		})
	}
}
