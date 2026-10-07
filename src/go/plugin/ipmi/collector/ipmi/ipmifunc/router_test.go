// SPDX-License-Identifier: GPL-3.0-or-later

package ipmifunc

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/ipmi/collector/ipmi/internal/ipmiapi"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeDeps struct{ snapshot *ipmiapi.Snapshot }

func (d *fakeDeps) CurrentSnapshot() *ipmiapi.Snapshot { return d.snapshot }
func TestSensorTable(t *testing.T) {
	temperature, voltage, fan, current := 25.125, 0.987, 1234.0, 14.42
	d := &fakeDeps{snapshot: &ipmiapi.Snapshot{CollectedAt: time.Unix(1700000000, 0), Sensors: []ipmiapi.Sensor{
		{Key: "temp1", Name: "CPU Temp", Type: "Temperature", Component: "Processor", Unit: "Celsius", State: "nominal", Value: &temperature},
		{Key: "temp2", Name: "CPU Temp", Type: "Temperature", Component: "Processor", Unit: "Celsius", State: "nominal", Value: &temperature},
		{Key: "voltage", Name: "CPU Voltage", Type: "Voltage", Component: "Processor", Unit: "Volts", State: "warning", Value: &voltage},
		{Key: "fan", Name: "Fan", Type: "Fan", Component: "System", Unit: "RPM", State: "nominal", Value: &fan},
		{Key: "current", Name: "Current", Type: "Current", Component: "Power Supply", Unit: "Amps", State: "critical", Value: &current},
		{Key: "presence", Name: "PSU Status", Type: "Power Supply", Component: "Power Supply", State: "nominal"},
		{Key: "unavailable", Name: "PSU Temp", Type: "Temperature", Component: "Power Supply", Unit: "Celsius", State: "unknown"},
	}}}
	response := NewRouter(d).Handle(t.Context(), methodSensors, funcapi.ResolvedParams{})
	require.Equal(t, 200, response.Status)
	assert.Equal(t, [][]any{
		{"CPU Temp", "Temperature", "Processor", 25.125, "Celsius", "nominal", "temp1", map[string]string{"severity": "normal"}},
		{"CPU Temp", "Temperature", "Processor", 25.125, "Celsius", "nominal", "temp2", map[string]string{"severity": "normal"}},
		{"CPU Voltage", "Voltage", "Processor", 0.987, "Volts", "warning", "voltage", map[string]string{"severity": "warning"}},
		{"Fan", "Fan", "System", 1234.0, "RPM", "nominal", "fan", map[string]string{"severity": "normal"}},
		{"Current", "Current", "Power Supply", 14.42, "Amps", "critical", "current", map[string]string{"severity": "critical"}},
		{"PSU Status", "Power Supply", "Power Supply", nil, nil, "nominal", "presence", map[string]string{"severity": "normal"}},
		{"PSU Temp", "Temperature", "Power Supply", nil, "Celsius", "unknown", "unavailable", map[string]string{"severity": "normal"}},
	}, response.Data)
	assert.Equal(t, "Type", response.DefaultSortColumn)
	assert.Equal(t, funcapi.ChartingConfig{
		Charts:        map[string]funcapi.ChartConfig{"Sensors": {Name: "Sensors", Type: "stacked-bar", Columns: []string{"Sensor"}}},
		DefaultCharts: funcapi.DefaultCharts{{Chart: "Sensors", GroupBy: "Component"}, {Chart: "Sensors", GroupBy: "State"}},
	}, response.ChartingConfig)

	// Check presentation semantics that the structural JSON schema cannot enforce.
	expectedColumns := map[string]map[string]any{
		"Sensor":     {"index": 0, "type": "string", "visible": true, "sticky": true, "full_width": true, "summary": "count", "filter": "multiselect"},
		"Type":       {"index": 1, "type": "string", "visible": true, "summary": "count", "filter": "multiselect"},
		"Component":  {"index": 2, "type": "string", "visible": true, "summary": "count", "filter": "multiselect"},
		"Reading":    {"index": 3, "type": "float", "visible": true, "sort": "descending", "summary": "sum", "value_options": map[string]any{"transform": "number", "decimal_points": 2, "default_value": nil}},
		"Units":      {"index": 4, "type": "string", "visible": true, "summary": "count", "filter": "multiselect"},
		"State":      {"index": 5, "type": "string", "visible": true, "summary": "count", "filter": "multiselect"},
		"Key":        {"index": 6, "type": "string", "visible": false},
		"rowOptions": {"index": 7, "type": "none", "visible": false, "visualization": "rowOptions", "dummy": true},
	}
	require.Len(t, response.Columns, len(expectedColumns))
	for name, expected := range expectedColumns {
		col, ok := response.Columns[name].(map[string]any)
		require.True(t, ok, "column %s", name)
		assert.Subset(t, col, expected, "column %s", name)
		assert.Equal(t, name == "Key", col["unique_key"], "column %s", name)
	}

	// Include charting in the serialized handler envelope, as the framework does.
	payload := map[string]any{"status": response.Status, "type": "table", "columns": response.Columns, "data": response.Data,
		"default_sort_column": response.DefaultSortColumn, "help": response.Help,
		"charts": response.Charts, "default_charts": response.DefaultCharts.Build()}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	var decoded any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	schemaBytes, err := os.ReadFile("../../../../../../plugins.d/FUNCTION_UI_SCHEMA.json")
	require.NoError(t, err)
	var schemaDoc any
	require.NoError(t, json.Unmarshal(schemaBytes, &schemaDoc))
	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("schema.json", schemaDoc))
	schema, err := compiler.Compile("schema.json")
	require.NoError(t, err)
	require.NoError(t, schema.Validate(decoded))
}

func TestSensorIdentityAcrossSnapshots(t *testing.T) {
	value := 42.5
	first := ipmiapi.Sensor{Key: "sensor1", Name: "Temperature", Type: "Temperature", Unit: "Celsius", State: "nominal", Value: &value}
	second := first
	second.Key = "sensor2"
	d := &fakeDeps{snapshot: &ipmiapi.Snapshot{Sensors: []ipmiapi.Sensor{first, second}}}
	r := NewRouter(d)
	before := r.Handle(t.Context(), methodSensors, funcapi.ResolvedParams{})
	keyColumn, ok := before.Columns["Key"].(map[string]any)
	require.True(t, ok, "sensor identity column")
	keyIndex := keyColumn["index"].(int)
	rows := before.Data.([][]any)
	require.Len(t, rows, 2)
	assert.NotEqual(t, rows[0][keyIndex], rows[1][keyIndex], "duplicate display names must remain distinct")

	// A new immutable snapshot changes order, reading availability and health.
	second.Value = nil
	second.State = "unknown"
	first.State = "critical"
	d.snapshot = &ipmiapi.Snapshot{Sensors: []ipmiapi.Sensor{second, first}}
	after := r.Handle(t.Context(), methodSensors, funcapi.ResolvedParams{})
	updatedRows := after.Data.([][]any)
	require.Len(t, updatedRows, 2)
	assert.Equal(t, rows[0][keyIndex], updatedRows[1][keyIndex])
	assert.Equal(t, rows[1][keyIndex], updatedRows[0][keyIndex])
	assert.Equal(t, "unknown", updatedRows[0][5])
	assert.Nil(t, updatedRows[0][3])
	assert.Equal(t, "critical", updatedRows[1][5])
	assert.Equal(t, "nominal", rows[0][5], "previous response must retain its snapshot")
}

func TestErrors(t *testing.T) {
	r := NewRouter(&fakeDeps{})
	assert.Equal(t, 503, r.Handle(t.Context(), methodSensors, funcapi.ResolvedParams{}).Status)
	assert.Equal(t, 404, r.Handle(t.Context(), "unknown", funcapi.ResolvedParams{}).Status)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	assert.Equal(t, 499, r.Handle(ctx, methodSensors, funcapi.ResolvedParams{}).Status)
	_, err := r.MethodParams(t.Context(), "unknown")
	require.Error(t, err)
}
