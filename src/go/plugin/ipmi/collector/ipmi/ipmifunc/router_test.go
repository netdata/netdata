// SPDX-License-Identifier: GPL-3.0-or-later

package ipmifunc

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/ipmi/collector/ipmi/internal/bmc"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeDeps struct {
	snapshot *bmc.Snapshot
}

func (d *fakeDeps) CurrentSnapshot() *bmc.Snapshot { return d.snapshot }

func TestRouter_SensorTable(t *testing.T) {
	temperature, voltage, fan, current := 25.125, 0.987, 1234.0, 14.42
	deps := &fakeDeps{
		snapshot: &bmc.Snapshot{
			CollectedAt: time.Unix(1700000000, 0),
			Sensors: []bmc.Sensor{
				{
					Key:       "temp1",
					Name:      "CPU Temp",
					Type:      "Temperature",
					Component: "Processor",
					Unit:      bmc.UnitCelsius,
					State:     bmc.StateNominal,
					Value:     &temperature,
				},
				{
					Key:       "temp2",
					Name:      "CPU Temp",
					Type:      "Temperature",
					Component: "Processor",
					Unit:      bmc.UnitCelsius,
					State:     bmc.StateNominal,
					Value:     &temperature,
				},
				{
					Key:       "voltage",
					Name:      "CPU Voltage",
					Type:      "Voltage",
					Component: "Processor",
					Unit:      bmc.UnitVolts,
					State:     bmc.StateWarning,
					Value:     &voltage,
				},
				{
					Key:       "fan",
					Name:      "Fan",
					Type:      "Fan",
					Component: "System",
					Unit:      bmc.UnitRPM,
					State:     bmc.StateNominal,
					Value:     &fan,
				},
				{
					Key:       "current",
					Name:      "Current",
					Type:      "Current",
					Component: "Power Supply",
					Unit:      bmc.UnitAmps,
					State:     bmc.StateCritical,
					Value:     &current,
				},
				{
					Key:       "presence",
					Name:      "PSU Status",
					Type:      "Power Supply",
					Component: "Power Supply",
					State:     bmc.StateNominal,
				},
				{
					Key:       "unavailable",
					Name:      "PSU Temp",
					Type:      "Temperature",
					Component: "Power Supply",
					Unit:      bmc.UnitCelsius,
					State:     bmc.StateUnknown,
				},
			},
		},
	}

	response := NewRouter(deps).Handle(t.Context(), methodSensors, funcapi.ResolvedParams{})
	require.Equal(t, 200, response.Status)

	normal := map[string]string{"severity": "normal"}
	assert.Equal(t, [][]any{
		{"CPU Temp", "Temperature", "Processor", 25.125, "Celsius", "nominal", "temp1", normal},
		{"CPU Temp", "Temperature", "Processor", 25.125, "Celsius", "nominal", "temp2", normal},
		{
			"CPU Voltage",
			"Voltage",
			"Processor",
			0.987,
			"Volts",
			"warning",
			"voltage",
			map[string]string{"severity": "warning"},
		},
		{"Fan", "Fan", "System", 1234.0, "RPM", "nominal", "fan", normal},
		{
			"Current",
			"Current",
			"Power Supply",
			14.42,
			"Amps",
			"critical",
			"current",
			map[string]string{"severity": "critical"},
		},
		{"PSU Status", "Power Supply", "Power Supply", nil, nil, "nominal", "presence", normal},
		{"PSU Temp", "Temperature", "Power Supply", nil, "Celsius", "unknown", "unavailable", normal},
	}, response.Data)
	assert.Equal(t, "Type", response.DefaultSortColumn)
	assert.Equal(t, funcapi.ChartingConfig{
		Charts: map[string]funcapi.ChartConfig{
			"Sensors": {Name: "Sensors", Type: "stacked-bar", Columns: []string{"Sensor"}},
		},
		DefaultCharts: funcapi.DefaultCharts{
			{Chart: "Sensors", GroupBy: "Component"},
			{Chart: "Sensors", GroupBy: "State"},
		},
	}, response.ChartingConfig)

	// Presentation semantics that the structural JSON schema cannot enforce.
	wantColumns := map[string]map[string]any{
		"Sensor": {
			"index":      0,
			"type":       "string",
			"visible":    true,
			"sticky":     true,
			"full_width": true,
			"summary":    "count",
			"filter":     "multiselect",
		},
		"Type":      {"index": 1, "type": "string", "visible": true, "summary": "count", "filter": "multiselect"},
		"Component": {"index": 2, "type": "string", "visible": true, "summary": "count", "filter": "multiselect"},
		"Reading": {
			"index":         3,
			"type":          "float",
			"visible":       true,
			"sort":          "descending",
			"summary":       "sum",
			"value_options": map[string]any{"transform": "number", "decimal_points": 2, "default_value": nil},
		},
		"Units":      {"index": 4, "type": "string", "visible": true, "summary": "count", "filter": "multiselect"},
		"State":      {"index": 5, "type": "string", "visible": true, "summary": "count", "filter": "multiselect"},
		"Key":        {"index": 6, "type": "string", "visible": false},
		"rowOptions": {"index": 7, "type": "none", "visible": false, "visualization": "rowOptions", "dummy": true},
	}
	require.Len(t, response.Columns, len(wantColumns))
	for name, want := range wantColumns {
		col, ok := response.Columns[name].(map[string]any)
		require.True(t, ok, "column %s", name)
		assert.Subset(t, col, want, "column %s", name)
		assert.Equal(t, name == "Key", col["unique_key"], "column %s: only Key identifies rows", name)
	}

	assertMatchesFunctionSchema(t, response)
}

func TestRouter_SensorIdentityAcrossSnapshots(t *testing.T) {
	const (
		readingIndex = 3
		stateIndex   = 5
		keyIndex     = 6
	)
	value := 42.5
	first := bmc.Sensor{
		Key:   "sensor1",
		Name:  "Temperature",
		Type:  "Temperature",
		Unit:  bmc.UnitCelsius,
		State: bmc.StateNominal,
		Value: &value,
	}
	second := first
	second.Key = "sensor2"
	deps := &fakeDeps{
		snapshot: &bmc.Snapshot{
			Sensors: []bmc.Sensor{first, second},
		},
	}
	r := NewRouter(deps)

	before := r.Handle(t.Context(), methodSensors, funcapi.ResolvedParams{})
	require.Equal(t, keyIndex, before.Columns["Key"].(map[string]any)["index"])
	rows := before.Data.([][]any)
	require.Len(t, rows, 2)
	assert.NotEqual(t, rows[0][keyIndex], rows[1][keyIndex], "duplicate display names stay distinct")

	// A new snapshot changes order, reading availability and health.
	second.Value = nil
	second.State = bmc.StateUnknown
	first.State = bmc.StateCritical
	deps.snapshot = &bmc.Snapshot{
		Sensors: []bmc.Sensor{second, first},
	}

	after := r.Handle(t.Context(), methodSensors, funcapi.ResolvedParams{})
	updated := after.Data.([][]any)
	require.Len(t, updated, 2)
	assert.Equal(
		t,
		[]any{rows[1][keyIndex], nil, bmc.StateUnknown},
		[]any{updated[0][keyIndex], updated[0][readingIndex], updated[0][stateIndex]},
	)
	assert.Equal(t, []any{rows[0][keyIndex], bmc.StateCritical}, []any{updated[1][keyIndex], updated[1][stateIndex]})
	assert.Equal(t, bmc.StateNominal, rows[0][stateIndex], "an earlier response keeps its snapshot")
}

func TestRouter_Errors(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := map[string]struct {
		ctx        context.Context
		snapshot   *bmc.Snapshot
		method     string
		wantStatus int
	}{
		"no successful collection": {ctx: context.Background(), method: methodSensors, wantStatus: 503},
		"unknown method": {
			ctx:        context.Background(),
			snapshot:   &bmc.Snapshot{},
			method:     "unknown",
			wantStatus: 404,
		},
		"canceled request": {ctx: canceled, snapshot: &bmc.Snapshot{}, method: methodSensors, wantStatus: 499},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			r := NewRouter(&fakeDeps{
				snapshot: tc.snapshot,
			})
			assert.Equal(t, tc.wantStatus, r.Handle(tc.ctx, tc.method, funcapi.ResolvedParams{}).Status)
		})
	}
}

func TestRouter_MethodParams(t *testing.T) {
	r := NewRouter(&fakeDeps{})

	params, err := r.MethodParams(t.Context(), methodSensors)
	require.NoError(t, err)
	assert.Empty(t, params)

	_, err = r.MethodParams(t.Context(), "unknown")
	assert.Error(t, err)
}

// assertMatchesFunctionSchema validates the response envelope the framework
// serializes, including charting, against the Function UI schema.
func assertMatchesFunctionSchema(t *testing.T, response *funcapi.FunctionResponse) {
	t.Helper()
	payload := map[string]any{
		"status":              response.Status,
		"type":                "table",
		"columns":             response.Columns,
		"data":                response.Data,
		"default_sort_column": response.DefaultSortColumn,
		"help":                response.Help,
		"charts":              response.Charts,
		"default_charts":      response.DefaultCharts.Build(),
	}
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
