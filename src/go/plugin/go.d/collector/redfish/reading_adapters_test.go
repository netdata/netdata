// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestElectricalAuxiliarySourceCoverage(t *testing.T) {
	data := map[string]any{
		"SpeedRPM": json.Number("0"), "ApparentVA": json.Number("0"), "ReactiveVAR": json.Number("0"),
		"ApparentkVAh": json.Number("0"), "ReactivekVARh": json.Number("0"), "CrestFactor": json.Number("0"),
		"PhaseAngleDegrees": json.Number(
			"0",
		), "PowerFactor": json.Number("0"), "THDPercent": json.Number("0"), "LoadPercent": json.Number("0"),
	}
	common := map[string]string{
		"speed_rpm": "rotational_speed", "apparent_va": "apparent_power", "reactive_var": "reactive_power",
		"apparent_kvah": "apparent_energy", "reactive_kvarh": "reactive_energy", "phase_angle_degrees": "phase_angle", "power_factor": "power_factor",
	}
	for name, standalone := range map[string]bool{"standalone": true, "excerpt": false} {
		t.Run(name, func(t *testing.T) {
			node := &graphNode{
				Kind: "sensor",
				Key:  "sensor",
				Data: data,
			}
			want := make(map[string]string)
			for role, family := range common {
				want[role] = family
			}
			if standalone {
				want["crest_factor"] = "crest_factor"
				want["thd_percent"] = "harmonic_distortion"
				want["load_percent"] = "percentage"
			} else {
				node.SourceModel = "embedded_sensor_excerpt"
				node.SensorExcerpts = []sensorExcerptSource{{Path: "excerpt", Data: data}}
			}
			got := make(map[string]string)
			for _, reading := range (&protocolClient{}).readingsForNode(node, time.Unix(1, 0)) {
				require.True(t, reading.Valid, reading.SourcePath)
				require.Zero(t, reading.Value, reading.SourcePath)
				got[reading.Role] = reading.Family
			}
			require.Equal(t, want, got)
		})
	}
}

func TestFixedExcerptMapPreservesValuePresenceAndContext(t *testing.T) {
	for name, test := range map[string]struct {
		value any
		want  []rawReading
	}{
		"null":                {value: nil},
		"zero":                {value: json.Number("0"), want: []rawReading{{Path: "power_supply.PolyPhasePowerWatts.Line1ToNeutral", Type: "Power", Units: "W", Basis: "Zero", Role: "power", Value: json.Number("0"), ValuePresent: true, Primary: true}}},
		"object null reading": {value: map[string]any{"Reading": nil, "PhysicalContext": "PowerSupply"}, want: []rawReading{{Path: "power_supply.PolyPhasePowerWatts.Line1ToNeutral.Reading", Type: "Power", Units: "W", Basis: "Zero", Role: "power", ValuePresent: true, Primary: true, PhysicalContext: "PowerSupply", ReadingScoped: true}}},
	} {
		t.Run(name, func(t *testing.T) {
			node := &graphNode{
				Kind: "power_supply",
				Enrichment: map[string]map[string]any{"power_supply_metrics": {
					"PhysicalContext":     "Chassis",
					"PolyPhasePowerWatts": map[string]any{"Line1ToNeutral": test.value, "VendorLine": json.Number("99")},
				}},
			}
			require.Equal(t, test.want, excerptReadings(node))
		})
	}
}

// This cycle path is O(fixed source properties + accepted readings). Allocation
// counts track adapter overhead; ns/op is only a local-machine trend indicator.
func BenchmarkExcerptReadings(b *testing.B) {
	node := &graphNode{
		Kind: "power_supply",
		Enrichment: map[string]map[string]any{"power_supply_metrics": {
			"InputPowerWatts": json.Number("100"), "OutputPowerWatts": json.Number("90"),
			"PolyPhasePowerWatts": map[string]any{"Line1ToNeutral": json.Number("50"), "Line2ToNeutral": json.Number("50")},
		}},
	}
	b.ReportAllocs()
	for b.Loop() {
		if len(excerptReadings(node)) != 4 {
			b.Fatal("readings missing")
		}
	}
}
