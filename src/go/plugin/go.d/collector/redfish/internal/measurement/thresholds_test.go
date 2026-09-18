// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func thresholdTestSensor(value any, thresholds any) *Resource {
	return &Resource{
		Key:              "sensor",
		Kind:             "sensor",
		URI:              "/redfish/v1/Chassis/1/Sensors/1",
		AcquisitionState: "readable",
		Data: map[string]any{
			"ReadingType": "Temperature", "ReadingUnits": "Cel", "Reading": value,
			"Status":     map[string]any{"Health": "OK", "State": "Enabled"},
			"Thresholds": thresholds,
		},
	}
}

func projectThresholdStatus(t *testing.T, p *Projector, node *Resource, seconds int, want string) Result {
	t.Helper()
	result, err := p.Project([]*Resource{node}, true, time.Unix(1000+int64(seconds), 0))
	require.NoError(t, err)
	require.NotEmpty(t, result.Sensors)
	assert.Equal(t, want, result.Sensors[0].DerivedHealth)
	var states []string
	for _, observation := range result.Observations {
		if observation.Metric == "derived_health" {
			states = append(states, observation.State)
		}
	}
	if want == "" || want == "unavailable" {
		assert.Empty(t, states, "unavailable or inapplicable status is a metric gap")
	} else {
		assert.Equal(t, []string{want}, states)
	}
	return result
}

func TestDerivedHealthBoundariesAndSourceIndependence(t *testing.T) {
	limits := map[string]any{
		"LowerCaution":      map[string]any{"Reading": 10},
		"LowerCritical":     map[string]any{"Reading": 5},
		"UpperCaution":      map[string]any{"Reading": 40},
		"UpperCriticalUser": map[string]any{"Reading": 45},
		"UpperFatal":        map[string]any{"Reading": 50},
	}
	for _, test := range []struct {
		value int
		want  string
	}{
		{4, "critical"}, {5, "warning"}, {9, "warning"}, {10, "ok"},
		{40, "ok"}, {41, "warning"}, {45, "warning"}, {46, "critical"}, {51, "critical"},
	} {
		for _, health := range []any{"OK", "Warning", "Critical", "vendor-status", nil} {
			node := thresholdTestSensor(test.value, limits)
			node.Data["Status"] = map[string]any{"Health": health}
			result := projectThresholdStatus(t, New("", "", nil), node, 0, test.want)
			wantHealth, _ := health.(string)
			assert.Equal(t, wantHealth, result.Sensors[0].Health)
			var sourceStates []string
			for _, observation := range result.Observations {
				if observation.Metric == "system_hw_sensor_temperature_alarm" {
					sourceStates = append(sourceStates, observation.State)
				}
			}
			if alarm := healthAlarm(wantHealth); alarm != "" {
				assert.Equal(t, []string{alarm}, sourceStates)
			} else {
				assert.Empty(t, sourceStates)
			}
		}
	}
}

func TestDerivedHealthUnavailableAndInapplicable(t *testing.T) {
	for name, test := range map[string]struct {
		change func(*Resource)
		want   string
	}{
		"no thresholds":   {func(n *Resource) { delete(n.Data, "Thresholds") }, ""},
		"null thresholds": {func(n *Resource) { n.Data["Thresholds"] = nil }, ""},
		"null limit": {func(n *Resource) {
			n.Data["Thresholds"] = map[string]any{"UpperCritical": map[string]any{"Reading": nil}}
		}, ""},
		"disabled limit ignores unknown settings": {func(n *Resource) {
			n.Data["Thresholds"] = map[string]any{"UpperCritical": map[string]any{"Reading": 10, "Activation": "Disabled", "DwellTime": nil}}
		}, ""},
		"missing reading":          {func(n *Resource) { delete(n.Data, "Reading"); delete(n.Data, "Status") }, "unavailable"},
		"null reading":             {func(n *Resource) { n.Data["Reading"] = nil }, "unavailable"},
		"invalid reading":          {func(n *Resource) { n.Data["Reading"] = "hot" }, "unavailable"},
		"outside advertised range": {func(n *Resource) { n.Data["ReadingRangeMax"] = 15 }, "unavailable"},
		"disabled sensor":          {func(n *Resource) { n.Data["Enabled"] = false }, "unavailable"},
		"absent sensor":            {func(n *Resource) { n.Data["Status"] = map[string]any{"State": "Absent"} }, "unavailable"},
		"null state with disabled sensor": {func(n *Resource) {
			n.Data["Enabled"] = false
			n.Data["Status"] = map[string]any{"State": nil}
		}, "unavailable"},
		"null enabled with absent sensor": {func(n *Resource) {
			n.Data["Enabled"] = nil
			n.Data["Status"] = map[string]any{"State": "Absent"}
		}, "unavailable"},
		"malformed threshold map":    {func(n *Resource) { n.Data["Thresholds"] = 3 }, "unavailable"},
		"malformed threshold object": {func(n *Resource) { n.Data["Thresholds"] = map[string]any{"UpperCritical": 10} }, "unavailable"},
		"malformed limit": {func(n *Resource) {
			n.Data["Thresholds"] = map[string]any{"UpperCritical": map[string]any{"Reading": "10"}}
		}, "unavailable"},
	} {
		t.Run(name, func(t *testing.T) {
			node := thresholdTestSensor(20, map[string]any{"UpperCritical": map[string]any{"Reading": 10}})
			test.change(node)
			projectThresholdStatus(t, New("", "", nil), node, 0, test.want)
		})
	}
	for _, setting := range []struct {
		key   string
		value any
	}{
		{"Activation", nil}, {"Activation", "Either"}, {"Activation", "Decreasing"},
		{"DwellTime", nil}, {"DwellTime", "PT"}, {"DwellTime", 10}, {"DwellTime", "-PT1S"},
		{"HysteresisDuration", nil}, {"HysteresisDuration", "forever"},
		{"HysteresisReading", nil}, {"HysteresisReading", "2"},
	} {
		config := map[string]any{"Reading": 10, setting.key: setting.value}
		node := thresholdTestSensor(20, map[string]any{"UpperCritical": config})
		projectThresholdStatus(t, New("", "", nil), node, 0, "unavailable")
	}
}

func TestDerivedHealthObservedDwellAndIndependentClearingConditions(t *testing.T) {
	// DMTF requires both clear conditions, but the timer starts at the original limit.
	for _, upper := range []bool{true, false} {
		name, direction, offset := "UpperCritical", "Increasing", -5
		if !upper {
			name, direction, offset = "LowerCritical", "Decreasing", 5
		}
		node := thresholdTestSensor(0, map[string]any{name: map[string]any{
			"Reading": 100, "Activation": direction, "DwellTime": "PT10S",
			"HysteresisDuration": "PT10S", "HysteresisReading": offset,
		}})
		p := New("", "", nil)
		for _, step := range []struct {
			seconds, value int
			want           string
		}{
			{0, 101, "unavailable"}, {5, 102, "unavailable"}, {10, 103, "critical"},
			{15, 99, "critical"}, {20, 98, "critical"}, {25, 95, "ok"},
			{30, 101, "ok"}, {35, 99, "ok"}, {40, 101, "ok"}, {50, 101, "critical"},
			{55, 99, "critical"}, {60, 101, "critical"}, {65, 94, "critical"}, {75, 94, "ok"},
		} {
			value := step.value
			if !upper {
				value = 200 - value
			}
			node.Data["Reading"] = value
			projectThresholdStatus(t, p, node, step.seconds, step.want)
		}
	}
}

func TestDerivedHealthResetsTemporalEvidence(t *testing.T) {
	for name, interrupt := range map[string]func(*testing.T, *Projector, *Resource){
		"partial missing reading": func(t *testing.T, p *Projector, n *Resource) {
			_, err := p.Project(nil, false, time.Unix(1005, 0))
			require.NoError(t, err)
		},
		"invalid numeric reading": func(t *testing.T, p *Projector, n *Resource) {
			n.Data["Reading"] = nil
			projectThresholdStatus(t, p, n, 5, "unavailable")
			n.Data["Reading"] = 101
		},
		"failed collection": func(t *testing.T, p *Projector, n *Resource) { p.ResetDerivedHealth() },
		"definition changed": func(t *testing.T, p *Projector, n *Resource) {
			n.Data["Thresholds"].(map[string]any)["UpperCritical"].(map[string]any)["Reading"] = 99
		},
		"source reset": func(t *testing.T, p *Projector, n *Resource) { n.Data["SensorResetTime"] = "2026-09-18T00:00:00Z" },
		"numeric threshold temporarily null": func(t *testing.T, p *Projector, n *Resource) {
			c := n.Data["Thresholds"].(map[string]any)["UpperCritical"].(map[string]any)
			c["Reading"] = nil
			projectThresholdStatus(t, p, n, 5, "")
			c["Reading"] = 100
		},
	} {
		t.Run(name, func(t *testing.T) {
			node := thresholdTestSensor(
				101,
				map[string]any{"UpperCritical": map[string]any{"Reading": 100, "DwellTime": "PT10S"}},
			)
			p := New("", "", nil)
			projectThresholdStatus(t, p, node, 0, "unavailable")
			interrupt(t, p, node)
			projectThresholdStatus(t, p, node, 10, "unavailable")
			projectThresholdStatus(t, p, node, 20, "critical")
		})
	}
	for _, seconds := range []int{-1, 0, 10} {
		node := thresholdTestSensor(
			101,
			map[string]any{"UpperCritical": map[string]any{"Reading": 100, "DwellTime": "PT10S"}},
		)
		p := New("", "", nil)
		projectThresholdStatus(t, p, node, 0, "unavailable")
		projectThresholdStatus(t, p, node, 10, "critical")
		projectThresholdStatus(t, p, node, seconds, "unavailable")
		projectThresholdStatus(t, New("", "", nil), node, 20, "unavailable")
	}
}

func TestDerivedHealthLegacyNullPlaceholders(t *testing.T) {
	// Lenovo documents mixed numeric/null limits; null is not a zero limit.
	for _, source := range []struct{ model, path, property string }{
		{"deprecated_thermal", "Temperatures", "ReadingCelsius"},
		{"deprecated_thermal", "Fans", "Reading"},
		{"deprecated_power", "Voltages", "ReadingVolts"},
	} {
		node := &Resource{
			Key:         "legacy",
			Kind:        "sensor",
			SourceModel: source.model,
			SourcePath:  source.path,
			Data: map[string]any{source.property: 25, "LowerThresholdCritical": nil,
				"UpperThresholdNonCritical": 43, "UpperThresholdCritical": 47, "UpperThresholdFatal": 50},
		}
		p := New("", "", nil)
		projectThresholdStatus(t, p, node, 0, "ok")
		node.Data[source.property] = 44
		projectThresholdStatus(t, p, node, 1, "warning")
		node.Data[source.property] = 49
		projectThresholdStatus(t, p, node, 2, "critical")
	}
}

func TestDerivedHealthPreservesThresholdUnitsThroughExcerptFallback(t *testing.T) {
	node := thresholdTestSensor(nil, map[string]any{"UpperCritical": map[string]any{"Reading": 90}})
	delete(node.Data, "Reading")
	node.Data["ReadingType"], node.Data["ReadingUnits"] = "EnergyWh", "W.h"
	node.SensorExcerpts = []SensorExcerpt{
		{Path: "EnergyJoules", Type: "EnergyJoules", Units: "J", Data: map[string]any{"Reading": 360000}},
	}
	p := New("", "", nil)
	projectThresholdStatus(t, p, node, 0, "critical")
	node.SensorExcerpts[0].Data["Reading"] = 396000
	result := projectThresholdStatus(t, p, node, 10, "critical")
	require.Len(t, result.Sensors, 2)
	assert.Equal(t, "", result.Sensors[1].DerivedHealth, "energy-derived watts never inherit energy thresholds")
	assert.True(t, result.Sensors[1].Calculated)
}

func TestDerivedHealthKeepsKnownMaximumSeverity(t *testing.T) {
	node := thresholdTestSensor(101, map[string]any{
		"UpperCritical": map[string]any{"Reading": 100},
		"UpperCaution":  map[string]any{"Reading": 90, "DwellTime": "PT30S"},
	})
	projectThresholdStatus(t, New("", "", nil), node, 0, "critical")
}

func TestDerivedHealthStoredEnergyUsesWattHours(t *testing.T) {
	for _, test := range []struct {
		name                      string
		limit, offset, clearValue int
	}{
		{"UpperCritical", 40, -5, 35},
		{"LowerCritical", 60, 5, 65},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := map[string]any{
				"Reading": 50,
				"Thresholds": map[string]any{test.name: map[string]any{
					"Reading": test.limit, "HysteresisReading": test.offset,
				}},
			}
			node := &Resource{
				Key:  "battery",
				Kind: "battery",
				Enrichment: map[string]Enrichment{
					"battery_metrics": {Data: map[string]any{"StoredEnergyWattHours": source}},
				},
			}
			p := New("", "", nil)
			result := projectThresholdStatus(t, p, node, 0, "critical")
			assert.Equal(t, "watt-hours", result.Sensors[0].Units)
			assert.Equal(t, float64(50), result.Sensors[0].Value)
			source["Reading"] = test.clearValue
			projectThresholdStatus(t, p, node, 1, "ok")
		})
	}
}

func TestDerivedHealthNullableSensorAvailability(t *testing.T) {
	for name, change := range map[string]func(*Resource){
		"null enabled": func(n *Resource) { n.Data["Enabled"] = nil },
		"null state":   func(n *Resource) { n.Data["Status"].(map[string]any)["State"] = nil },
		"both null": func(n *Resource) {
			n.Data["Enabled"] = nil
			n.Data["Status"].(map[string]any)["State"] = nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			node := thresholdTestSensor(20, map[string]any{"UpperCritical": map[string]any{"Reading": 10}})
			change(node)
			p := New("", "", nil)
			projectThresholdStatus(t, p, node, 0, "critical")
			node.Data["Reading"] = 5
			projectThresholdStatus(t, p, node, 1, "ok")
		})
	}
}
