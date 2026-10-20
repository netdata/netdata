// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

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
			node := &Resource{
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
				node.SensorExcerpts = []SensorExcerpt{{Path: "excerpt", Data: data}}
			}
			got := make(map[string]string)
			for _, reading := range (New("", nil)).readingsForNode(node, time.Unix(1, 0)) {
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
		"null": {value: nil, want: []rawReading{{
			Path: "power_supply_metrics.PolyPhasePowerWatts.Line1ToNeutral",
			Type: "Power", Units: "W", Basis: "Zero", Role: "power", ValuePresent: true, Primary: true,
		}}},
		"zero":                {value: json.Number("0"), want: []rawReading{{Path: "power_supply_metrics.PolyPhasePowerWatts.Line1ToNeutral", Type: "Power", Units: "W", Basis: "Zero", Role: "power", Value: json.Number("0"), ValuePresent: true, Primary: true}}},
		"object null reading": {value: map[string]any{"Reading": nil, "PhysicalContext": "PowerSupply"}, want: []rawReading{{Path: "power_supply_metrics.PolyPhasePowerWatts.Line1ToNeutral.Reading", Type: "Power", Units: "W", Basis: "Zero", Role: "power", ValuePresent: true, Primary: true, PhysicalContext: "PowerSupply", ReadingScoped: true}}},
	} {
		t.Run(name, func(t *testing.T) {
			node := &Resource{
				Kind: "power_supply",
				Enrichment: map[string]Enrichment{"power_supply_metrics": {Data: map[string]any{
					"PhysicalContext": "Chassis",
					"PolyPhasePowerWatts": map[string]any{
						"Line1ToNeutral": test.value,
						"VendorLine":     json.Number("99"),
					},
				}}},
			}
			require.Equal(t, test.want, excerptReadings(node))
		})
	}
}

// This cycle path is O(fixed source properties + accepted readings). Allocation
// counts track adapter overhead; ns/op is only a local-machine trend indicator.
func BenchmarkExcerptReadings(b *testing.B) {
	node := &Resource{
		Kind: "power_supply",
		Enrichment: map[string]Enrichment{"power_supply_metrics": {Data: map[string]any{
			"InputPowerWatts": json.Number("100"), "OutputPowerWatts": json.Number("90"),
			"PolyPhasePowerWatts": map[string]any{
				"Line1ToNeutral": json.Number("50"),
				"Line2ToNeutral": json.Number("50"),
			},
		}}},
	}
	b.ReportAllocs()
	for b.Loop() {
		if len(excerptReadings(node)) != 4 {
			b.Fatal("readings missing")
		}
	}
}

func TestElectricalAuxiliaryIdentityUsesSourceURI(t *testing.T) {
	client := fixtureClient()
	client.resolveProvenance = func(baseURI, raw string) (string, bool) {
		require.Equal(t, "/redfish/v1/Sensors/Owner", baseURI)
		switch raw {
		case "/redfish/v1/Sensors/1", "https://fixture.example/redfish/v1/Sensors/1":
			return "/redfish/v1/Sensors/1", true
		case "/redfish/v1/Sensors/2":
			return "/redfish/v1/Sensors/2", true
		default:
			return "", false
		}
	}
	keyFor := func(path, uri string, standalone bool) string {
		data := map[string]any{"DataSourceUri": uri, "ApparentVA": json.Number("12")}
		node := &Resource{
			Kind: "sensor",
			Key:  "sensor",
			URI:  "/redfish/v1/Sensors/Owner",
			Data: data,
		}
		if !standalone {
			node.SourceModel = "embedded_sensor_excerpt"
			node.SensorExcerpts = []SensorExcerpt{{Path: path, Data: data}}
		}
		readings := client.readingsForNode(node, time.Unix(10, 0))
		require.Len(t, readings, 1)
		require.Equal(t, float64(12), readings[0].Value)
		return readings[0].Key
	}
	first := keyFor("SourceA", "/redfish/v1/Sensors/1", false)
	require.Equal(t, first, keyFor("SourceB", "https://fixture.example/redfish/v1/Sensors/1", false))
	require.Equal(t, first, keyFor("Sensor", "/redfish/v1/Sensors/1", true))
	require.NotEqual(t, first, keyFor("SourceA", "/redfish/v1/Sensors/2", false))
	require.NotEqual(t, keyFor("SourceA", "", false), keyFor("SourceB", "", false))
}

func TestElectricalAuxiliaryNonzeroNormalization(t *testing.T) {
	// kVAh/kVARh convert to joule-equivalent by 1000 watts per kW * 3600 seconds per hour.
	for name, standalone := range map[string]bool{"standalone": true, "excerpt": false} {
		t.Run(name, func(t *testing.T) {
			data := map[string]any{
				"ApparentkVAh":  json.Number("1.5"),
				"ReactivekVARh": json.Number("2"),
				"PowerFactor":   json.Number("0.8"),
				"SpeedRPM":      json.Number("1800"),
			}
			node := &Resource{
				Kind: "sensor",
				Key:  "sensor",
				Data: data,
			}
			if !standalone {
				node.SourceModel = "embedded_sensor_excerpt"
				node.SensorExcerpts = []SensorExcerpt{{Path: "excerpt", Data: data}}
			}
			got := make(map[string]float64)
			for _, reading := range fixtureClient().readingsForNode(node, time.Unix(10, 0)) {
				require.True(t, reading.Valid)
				got[reading.Role] = reading.Value
			}
			require.Equal(
				t,
				map[string]float64{
					"apparent_kvah":  5400000,
					"reactive_kvarh": 7200000,
					"power_factor":   0.8,
					"speed_rpm":      1800,
				},
				got,
			)
		})
	}
}

func BenchmarkElectricalAuxiliaryReadings(b *testing.B) {
	data := map[string]any{
		"ApparentVA":    json.Number("12"),
		"ReactiveVAR":   json.Number("3"),
		"DataSourceUri": "/redfish/v1/Sensors/1",
	}
	b.ReportAllocs()
	for b.Loop() {
		if len(electricalAuxiliaryReadings(data, "Sensor", true, "", "", "")) != 2 {
			b.Fatal("auxiliary readings missing")
		}
	}
}
