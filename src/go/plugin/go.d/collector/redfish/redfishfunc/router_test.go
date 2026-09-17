// SPDX-License-Identifier: GPL-3.0-or-later

package redfishfunc

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/measurement"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type snapshotDeps struct{ value *Snapshot }

func (d *snapshotDeps) CurrentSnapshot() *Snapshot { return d.value }

func TestTablesAndMissingData(t *testing.T) {
	now := time.Unix(1000, 0)
	snapshot := &Snapshot{
		Available:   true,
		Complete:    false,
		CollectedAt: now,
		Components: []measurement.Component{
			{
				Key:                       "healthy",
				Name:                      "Healthy",
				Kind:                      "drive",
				Health:                    "OK",
				HealthRollup:              "Critical",
				Availability:              "readable",
				ObservedAt:                now,
				FailurePredictionReported: true,
				FailurePredicted:          false,
			},
			{Key: "unknown", Name: "Unknown", Kind: "fan", Availability: "unknown"},
		},
		Sensors: []measurement.Sensor{
			{
				Key:        "zero",
				Resource:   "Supply",
				Family:     "power",
				Units:      "watts",
				Role:       "power",
				SourcePath: "PolyPhasePowerWatts.Line1ToNeutral",
				Valid:      true,
				Value:      0,
				Health:     "OK",
				ObservedAt: now,
			},
			{
				Key:        "headroom",
				Resource:   "CPU",
				Family:     "temperature",
				Units:      "Celsius",
				Basis:      "headroom",
				SourcePath: "Sensor.Reading",
				Valid:      false,
				Health:     "vendor-status",
				ObservedAt: now,
			},
		},
	}
	deps := &snapshotDeps{
		value: snapshot,
	}
	router := NewRouter(deps)
	for _, method := range []string{"sensors", "hardware"} {
		t.Run(method, func(t *testing.T) {
			before, err := json.Marshal(snapshot)
			require.NoError(t, err)
			response := router.Handle(t.Context(), method, nil)
			require.Equal(t, 200, response.Status)
			assert.Contains(t, response.Help, "Partial")
			validateTableSchema(t, response)
			require.Contains(t, response.Columns, "rowOptions", "the UI reads row styling by this exact column key")
			optionsIndex := response.Columns["rowOptions"].(map[string]any)["index"].(int)
			for _, row := range response.Data.([][]any) {
				options := row[optionsIndex].(map[string]any)
				assert.Contains(t, []string{"normal", "notice", "warning", "critical"}, options["severity"])
			}
			for _, row := range response.Data.([][]any) {
				assert.Len(t, row, len(response.Columns))
			}
			after, err := json.Marshal(snapshot)
			require.NoError(t, err)
			assert.Equal(t, before, after)
		})
	}
	response := router.Handle(t.Context(), "sensors", nil)
	rows := response.Data.([][]any)
	value := func(row []any, col string) any { return row[response.Columns[col].(map[string]any)["index"].(int)] }
	assert.Contains(t, value(rows[0], "Sensor"), "headroom")
	assert.Nil(t, value(rows[0], "Reading"))
	assert.Equal(t, "Unknown: vendor-status", value(rows[0], "Health"))
	assert.Contains(t, value(rows[1], "Sensor"), "Line1 To Neutral")
	assert.Equal(t, float64(0), value(rows[1], "Reading"))
	assert.Equal(t, "count", response.Columns["Reading"].(map[string]any)["summary"], "mixed units are not summed")
	deps.value = &Snapshot{
		Available:   true,
		Complete:    true,
		CollectedAt: now,
	}
	response = router.Handle(t.Context(), "sensors", nil)
	assert.Empty(t, response.Data)
	assert.Contains(t, response.Help, "No rows")
	validateTableSchema(t, response)
	deps.value = &Snapshot{
		CollectedAt: now,
	}
	assert.Equal(t, 503, router.Handle(t.Context(), "hardware", nil).Status)
	deps.value = nil
	assert.Equal(t, 503, router.Handle(t.Context(), "sensors", nil).Status)
	assert.Equal(t, 404, router.Handle(t.Context(), "unknown", nil).Status)
	_, err := router.MethodParams(t.Context(), "unknown")
	assert.Error(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	assert.Equal(t, 499, router.Handle(ctx, "sensors", nil).Status)
}

func validateTableSchema(t *testing.T, response *funcapi.FunctionResponse) {
	t.Helper()
	raw, err := os.ReadFile("../../../../../../plugins.d/FUNCTION_UI_SCHEMA.json")
	require.NoError(t, err)
	var schemaDocument any
	require.NoError(t, json.Unmarshal(raw, &schemaDocument))
	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("function.json", schemaDocument))
	schema, err := compiler.Compile("function.json")
	require.NoError(t, err)
	// Match the standard envelope owned by jobmgr/functions.methodGeneration.
	wire, err := json.Marshal(map[string]any{"status": response.Status, "type": "table", "has_history": false,
		"columns": response.Columns, "data": response.Data, "help": response.Help, "default_sort_column": response.DefaultSortColumn})
	require.NoError(t, err)
	var document any
	require.NoError(t, json.Unmarshal(wire, &document))
	require.NoError(t, schema.Validate(document))
}
