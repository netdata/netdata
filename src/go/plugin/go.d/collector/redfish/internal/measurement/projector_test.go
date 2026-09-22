// SPDX-License-Identifier: GPL-3.0-or-later

package measurement_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/measurement"
	"github.com/stretchr/testify/require"
)

func TestProjectRateHistoryFollowsAcquisitionCompleteness(t *testing.T) {
	projector := measurement.New("https://fixture.example", nil)
	start := time.Unix(100, 0)
	energy := func(value string) *measurement.Resource {
		return &measurement.Resource{
			Kind:             "sensor",
			Key:              "energy-sensor",
			AcquisitionState: "readable",
			Data: map[string]any{
				"ReadingType":  "EnergyJoules",
				"ReadingUnits": "J",
				"Reading":      json.Number(value),
			},
		}
	}
	for _, step := range []struct {
		name      string
		resources []*measurement.Resource
		complete  bool
		seconds   int
		power     []float64
	}{
		{"establish baseline", []*measurement.Resource{energy("100")}, true, 0, nil},
		{"temporarily missing", nil, false, 10, nil},
		{"recovered resource", []*measurement.Resource{energy("200")}, true, 20, []float64{5}},
		{"confirmed removal", nil, true, 30, nil},
		{"return establishes fresh baseline", []*measurement.Resource{energy("300")}, true, 40, nil},
		{"next reading produces rate", []*measurement.Resource{energy("400")}, true, 50, []float64{10}},
	} {
		t.Run(step.name, func(t *testing.T) {
			result, err := projector.Project(
				step.resources,
				step.complete,
				start.Add(time.Duration(step.seconds)*time.Second),
			)
			require.NoError(t, err)
			var power []float64
			for _, observation := range result.Observations {
				if observation.Metric == "reading_power_value" {
					power = append(power, observation.Value)
				}
			}
			require.Equal(t, step.power, power)
		})
	}
}

func TestProjectLeavesAcquiredResourcesUnchanged(t *testing.T) {
	resources := []*measurement.Resource{
		{
			Kind:             "power_supply",
			Key:              "supply",
			URI:              "/redfish/v1/PowerSupplies/1",
			AcquisitionState: "readable",
			Data:             map[string]any{"PowerWatts": map[string]any{"Reading": json.Number("10")}},
			Enrichment: map[string]measurement.Enrichment{
				"power_supply_metrics": {
					URI:  "/redfish/v1/PowerSupplies/1/Metrics",
					Data: map[string]any{"InputPowerWatts": map[string]any{"Reading": json.Number("12")}},
				},
			},
		},
	}
	before, err := json.Marshal(resources)
	require.NoError(t, err)
	projector := measurement.New("https://fixture.example", nil)
	result, err := projector.Project(resources, true, time.Unix(100, 0))
	require.NoError(t, err)
	require.NotEmpty(t, result.Observations)
	require.NotEmpty(t, result.Diagnostics, "missing source health is returned as a diagnostic")
	after, err := json.Marshal(resources)
	require.NoError(t, err)
	require.Equal(t, string(before), string(after))
}
