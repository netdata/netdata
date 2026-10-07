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
	value := 12.25
	d := &fakeDeps{snapshot: &ipmiapi.Snapshot{CollectedAt: time.Unix(1700000000, 0), Sensors: []ipmiapi.Sensor{
		{Name: "Temperature", Type: "Temperature", Component: "System", Unit: "Celsius", State: "nominal", Value: &value},
		{Name: "Unavailable", State: "unknown"},
	}}}
	r := NewRouter(d)
	response := r.Handle(t.Context(), methodSensors, funcapi.ResolvedParams{})
	require.Equal(t, 200, response.Status)
	rows := response.Data.([][]any)
	require.Len(t, rows, 2)
	assert.Equal(t, 12.25, rows[0][3])
	assert.Nil(t, rows[1][3])
	assert.Equal(t, map[string]string{"severity": "normal"}, rows[0][6])
	// Validate actual handler fields against the canonical UI schema envelope.
	payload := map[string]any{"status": response.Status, "type": "table", "columns": response.Columns, "data": response.Data,
		"default_sort_column": response.DefaultSortColumn, "help": response.Help}
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
