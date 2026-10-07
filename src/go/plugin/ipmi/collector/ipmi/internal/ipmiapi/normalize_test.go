// SPDX-License-Identifier: GPL-3.0-or-later

package ipmiapi

import (
	"math"
	"testing"

	"github.com/bougou/go-ipmi/pkg/types"
)

func TestUnitConversion(t *testing.T) {
	for _, tc := range []struct {
		base         types.SensorUnitType
		unit, metric string
		legacy       int
	}{
		{1, "Celsius", "temperature_c", 1}, {2, "Fahrenheit", "temperature_f", 2}, {4, "Volts", "voltage", 3}, {5, "Amps", "ampere", 4}, {18, "RPM", "fan_speed", 5}, {6, "Watts", "power", 6},
	} {
		unit, metric, legacy := metricUnit(types.SensorUnit{BaseUnit: tc.base})
		if unit != tc.unit || metric != tc.metric || legacy != tc.legacy {
			t.Fatalf("base %d: %s %s %d", tc.base, unit, metric, legacy)
		}
	}
	for _, u := range []types.SensorUnit{
		{BaseUnit: 3}, {BaseUnit: 4, RateUnit: types.SensorRateUnit_PerSec}, {BaseUnit: 4, ModifierUnit: 5}, {BaseUnit: 4, Percentage: true}, {BaseUnit: 4, ModifierRelation: 1},
	} {
		if _, metric, _ := metricUnit(u); metric != "" {
			t.Fatalf("unsupported unit emitted %s", metric)
		}
	}
	if _, m, _ := metricUnit(types.SensorUnit{BaseUnit: 18, RateUnit: types.SensorRateUnit_PerMin}); m != "fan_speed" {
		t.Fatal("RPM per minute unsupported")
	}
	if u, m, l := metricUnit(types.SensorUnit{Percentage: true}); u != "%" || m != "reading_percent" || l != 7 {
		t.Fatal("percentage mapping")
	}
	for _, tc := range []struct {
		format types.SensorAnalogUnitFormat
		raw    byte
		want   float64
	}{{0, 25, 2.5}, {1, 0xe6, -2.5}, {2, 0xe7, -2.5}} {
		value := convertValue(tc.raw, types.SensorUnit{AnalogDataFormat: tc.format}, types.ReadingFactors{M: 1, R_Exp: -1}, 0)
		if value == nil || *value != tc.want {
			t.Fatalf("signed format %d: %v", tc.format, value)
		}
	}
	for _, linear := range []types.LinearizationFunc{types.LinearizationFunc_LN, types.LinearizationFunc_1X, types.LinearizationFunc_EXP10, 0x71, 0x0c} {
		raw := byte(0)
		if linear == types.LinearizationFunc_EXP10 {
			raw = 255
		}
		if got := convertValue(raw, types.SensorUnit{}, types.ReadingFactors{M: 511}, linear); got != nil {
			t.Fatalf("invalid value emitted %v for linearization %d", *got, linear)
		}
	}
	value := convertValue(0xf1, types.SensorUnit{AnalogDataFormat: 2}, types.ReadingFactors{M: 1, R_Exp: -1}, types.LinearizationFunc_EXP10)
	if value == nil || math.Abs(*value-0.03162277660168379) > 1e-14 {
		t.Fatalf("negative EXP10: %v", value)
	}
}

func TestComponentPatterns(t *testing.T) {
	for _, tc := range []struct {
		kind       byte
		name, want string
	}{
		{1, "DIMMA_TEMP", "Memory Module"}, {1, "cpu dimm temp", "Memory Module"}, {1, "cpu temp", "Processor"},
		{1, "mystery temperature", "Other"}, {1, "Memory RAID", "Memory"}, {1, "RAID", "Storage"},
		{1, "I/O temp BD", "Motherboard"}, {1, "PSU Temp", "Power Supply"}, {1, "sel", "System"},
		{7, "DIMM CPU", "Processor"}, {8, "unmatched", "Power Supply"}, {0x0c, "DIMM", "Memory"},
	} {
		_, got := sensorLabels(tc.kind, tc.name)
		if got != tc.want {
			t.Fatalf("%d %q: got %s want %s", tc.kind, tc.name, got, tc.want)
		}
	}
}

func TestSDRLatin1Name(t *testing.T) {
	// Type 3 encodes Latin-1 bytes; SDK Chars returns those bytes without UTF-8 conversion.
	record := fullRecord(1, 10, string([]byte{'T', 0xe9, 'm', 'p'}))
	sdr, err := types.ParseSDR(record, 0xffff)
	if err != nil {
		t.Fatal(err)
	}
	got := describe(sdr)
	if len(got) != 1 || got[0].sensor.Name != "Témp" || !got[0].supported {
		t.Fatalf("Latin-1 descriptor: %+v", got)
	}
}
