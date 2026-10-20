// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCompatibilityFixturesModernReadings(t *testing.T) {
	client := fixtureClient()
	tests := map[string]struct {
		node         *Resource
		wantFamilies map[string]int
	}{
		"thermal metrics": {
			node: &Resource{
				Kind: "thermal_subsystem",
				Key:  "thermal-subsystem",
				Enrichment: map[string]Enrichment{
					"thermal_metrics:0": {Data: loadFixture(
						t,
						"telegraf-hpe-modern-thermal-metrics.min.json",
					)},
				},
			},
			wantFamilies: map[string]int{"temperature": 1},
		},
		"fan": {
			node: &Resource{
				Kind: "fan",
				Key:  "fan",
				Data: loadFixture(t, "telegraf-hpe-modern-fan.min.json"),
			},
			wantFamilies: map[string]int{"percentage": 1},
		},
		"power supply metrics": {
			node: &Resource{
				Kind: "power_supply",
				Key:  "power-supply",
				Enrichment: map[string]Enrichment{
					"power_supply_metrics:0": {Data: loadFixture(
						t,
						"telegraf-hpe-modern-power-supply-metrics.min.json",
					)},
				},
			},
			wantFamilies: map[string]int{
				"current": 1, "energy": 1, "frequency": 1, "percentage": 1,
				"power": 2, "rotational_speed": 1, "temperature": 1, "voltage": 1,
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := make(map[string]int)
			identities := make(map[string]struct{})
			for _, reading := range client.readingsForNode(test.node, time.Now()) {
				if !reading.Valid {
					continue
				}
				got[reading.Family]++
				if _, ok := identities[reading.Key]; ok {
					t.Fatalf("duplicate reading identity %q", reading.Key)
				}
				identities[reading.Key] = struct{}{}
			}
			require.Equal(t, test.wantFamilies, got)
		})
	}
}

func TestCompatibilityFixtureModernStandaloneSensor(t *testing.T) {
	client := fixtureClient()
	node := &Resource{
		Kind: "sensor",
		Key:  "standalone-sensor",
		Data: loadFixture(t, "checkmk-nvidia-modern-sensor.min.json"),
	}
	readings := client.readingsForNode(node, time.Now())
	require.Len(t, readings, 1)
	reading := readings[0]
	require.True(t, reading.Valid)
	require.Equal(t, "temperature", reading.Family)
	require.Equal(t, "clear", reading.SourceAlarm)

}
