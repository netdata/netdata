// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLegacyPowerReadingRequiresActualConsumption(t *testing.T) {
	for name, test := range map[string]struct {
		data map[string]any
		want []float64
	}{
		"requested only":            {data: map[string]any{"PowerRequestedWatts": json.Number("400")}},
		"available only":            {data: map[string]any{"PowerAvailableWatts": json.Number("500")}},
		"null consumed with budget": {data: map[string]any{"PowerConsumedWatts": nil, "PowerAvailableWatts": json.Number("500")}},
		"zero consumed with budget": {data: map[string]any{"PowerConsumedWatts": json.Number("0"), "PowerAvailableWatts": json.Number("500")}, want: []float64{0}},
		"actual consumed":           {data: map[string]any{"PowerConsumedWatts": json.Number("123"), "PowerRequestedWatts": json.Number("400")}, want: []float64{123}},
	} {
		t.Run(name, func(t *testing.T) {
			node := &Resource{
				Key:         "power",
				Kind:        "sensor",
				SourceModel: "deprecated_power",
				SourcePath:  "PowerControl",
				Data:        test.data,
			}
			var got []float64
			for _, reading := range (New("", nil)).readingsForNode(node, time.Now()) {
				if reading.Valid {
					got = append(got, reading.Value)
				}
			}
			assert.Equal(t, test.want, got)
		})
	}
}

func TestExcerptSourceFormsProduceOneReading(t *testing.T) {
	for name, source := range map[string]any{
		"primitive": json.Number("20"),
		"object":    map[string]any{"Reading": json.Number("20")},
	} {
		t.Run(name, func(t *testing.T) {
			node := &Resource{
				Key:  "fan",
				Kind: "fan",
				Data: map[string]any{"SpeedPercent": source},
			}
			readings := (New("", nil)).readingsForNode(node, time.Now())
			if !assert.Len(t, readings, 1) {
				return
			}
			assert.True(t, readings[0].Valid)
			assert.Equal(t, float64(20), readings[0].Value)
		})
	}
}

func TestReadingSourceHealthIsIndependentOfNumericValue(t *testing.T) {
	type readingResult struct {
		Valid bool
		Value float64
		Alarm string
	}
	for name, test := range map[string]struct {
		value   any
		present bool
		health  string
		want    readingResult
	}{
		"source OK beyond threshold": {value: json.Number("90"), present: true, health: "OK", want: readingResult{
			Valid: true,
			Value: 90,
			Alarm: "clear",
		}},
		"warning without value": {health: "Warning", want: readingResult{
			Alarm: "warning",
		}},
		"critical with null value": {present: true, health: "Critical", want: readingResult{
			Alarm: "critical",
		}},
		"critical with invalid value": {value: "invalid", present: true, health: "Critical", want: readingResult{
			Alarm: "critical",
		}},
		"zero is a reading": {value: json.Number("0"), present: true, want: readingResult{
			Valid: true,
		}},
		"false is not a numeric reading": {value: false, present: true, want: readingResult{}},
		"missing health is not inferred": {value: json.Number("90"), present: true, want: readingResult{
			Valid: true,
			Value: 90,
		}},
	} {
		for sourceName, makeNode := range map[string]func(map[string]any) *Resource{
			"standalone sensor": func(data map[string]any) *Resource {
				data["ReadingType"], data["ReadingUnits"] = "Percent", "%"
				return &Resource{
					Key:  "sensor",
					Kind: "sensor",
					Data: data,
				}
			},
			"object excerpt": func(data map[string]any) *Resource {
				return &Resource{
					Key:  "fan",
					Kind: "fan",
					Data: map[string]any{"SpeedPercent": data},
				}
			},
			"array excerpt": func(data map[string]any) *Resource {
				return &Resource{
					Key:            "sensor",
					Kind:           "sensor",
					SourceModel:    "embedded_sensor_excerpt",
					SensorExcerpts: []SensorExcerpt{{Path: "FanSpeedPercent[0]", Type: "Percent", Units: "%", Data: data}},
				}
			},
		} {
			t.Run(sourceName+"/"+name, func(t *testing.T) {
				data := map[string]any{
					"Thresholds": map[string]any{"UpperCritical": map[string]any{"Reading": json.Number("70")}},
				}
				if test.present {
					data["Reading"] = test.value
				}
				if test.health != "" {
					data["Status"] = map[string]any{"Health": test.health}
				}
				node := makeNode(data)
				client := New("", nil)
				readings := client.readingsForNode(node, time.Now())
				if !assert.Len(t, readings, 1) {
					return
				}
				reading := readings[0]
				assert.Equal(
					t,
					test.want,
					readingResult{
						Valid: reading.Valid,
						Value: reading.Value,
						Alarm: reading.SourceAlarm,
					},
				)
				var states []string
				for _, observation := range client.readingObservations(node, reading) {
					if observation.Metric == reading.AlarmMetric && observation.State != "" {
						states = append(states, observation.State)
					}
				}
				var wantStates []string
				if test.want.Alarm != "" {
					wantStates = []string{test.want.Alarm}
				}
				assert.Equal(t, wantStates, states)
			})
		}
	}
}

func TestStoredEnergyExcerptKeepsGaugeSemantics(t *testing.T) {
	for name, source := range map[string]any{
		"primitive": json.Number("20"),
		"object":    map[string]any{"Reading": json.Number("20")},
	} {
		t.Run(name, func(t *testing.T) {
			node := &Resource{
				Key:  "battery",
				Kind: "battery",
				Enrichment: map[string]Enrichment{
					"battery_metrics": {Data: map[string]any{"StoredEnergyWattHours": source}},
				},
			}
			client := New("", nil)
			for _, at := range []time.Time{time.Unix(1000, 0), time.Unix(1060, 0)} {
				readings := client.readingsForNode(node, at)
				if !assert.Len(t, readings, 1) {
					return
				}
				assert.True(t, readings[0].Valid)
				assert.Equal(t, "stored_energy", readings[0].Family)
				assert.Equal(t, float64(20), readings[0].Value)
			}
		})
	}
}
