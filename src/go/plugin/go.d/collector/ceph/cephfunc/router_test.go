// SPDX-License-Identifier: GPL-3.0-or-later

package cephfunc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

func TestMethods(t *testing.T) {
	methods := Methods()
	require.Len(t, methods, 4)
	assert.Equal(t, []string{
		MethodHealth, MethodOSDs, MethodPools, MethodDaemons,
	}, []string{methods[0].ID, methods[1].ID, methods[2].ID, methods[3].ID})
	assert.Empty(t, methods[0].RequiredParams)

	tests := map[string]struct {
		method funcapi.FunctionConfig
	}{
		MethodOSDs:    {method: methods[1]},
		MethodPools:   {method: methods[2]},
		MethodDaemons: {method: methods[3]},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require.Len(t, test.method.RequiredParams, 1)
			assert.Equal(t, ParamLimit, test.method.RequiredParams[0].ID)
			assert.Equal(t, "500", test.method.RequiredParams[0].Options[1].ID)
			assert.True(t, test.method.RequiredParams[0].Options[1].Default)
		})
	}
}

func TestRouterTableResponses(t *testing.T) {
	size := int64(3)
	enabled := true
	deps := fakeDeps{
		health: HealthResult{
			Rows: []HealthRow{{
				ID: "CHECK#0000", Code: "CHECK", Severity: "HEALTH_WARN", Summary: "summary", Count: 1, Detail: "detail",
			}},
			Total: 1,
		},
		osds: OSDResult{
			Rows:  []OSDRow{{ID: 1, UUID: "uuid-1", Name: "osd.1", Up: true, In: true}},
			Total: 1,
		},
		pools: PoolResult{
			Rows:  []PoolRow{{Name: "pool-a", Type: "replicated", Size: &size}},
			Total: 1,
		},
		daemons: DaemonResult{
			Rows:  []DaemonRow{{ID: "mon.a", Type: "mon", Name: "a", Active: &enabled}},
			Total: 1,
		},
	}
	router := NewRouter(deps)

	tests := map[string]struct{}{
		MethodHealth:  {},
		MethodOSDs:    {},
		MethodPools:   {},
		MethodDaemons: {},
	}
	for method := range tests {
		t.Run(method, func(t *testing.T) {
			response := router.Handle(context.Background(), method, nil)
			require.Equal(t, 200, response.Status)
			data := response.Data.([][]any)
			require.Len(t, data, 1)
			assert.Equal(t, len(response.Columns), len(data[0]))
			assert.NotEmpty(t, response.DefaultSortColumn)
			assert.Empty(t, response.RequiredParams)
		})
	}
}

func TestRouterResponsesMatchFunctionUISchema(t *testing.T) {
	schemaBytes, err := os.ReadFile("../../../../../../plugins.d/FUNCTION_UI_SCHEMA.json")
	require.NoError(t, err)
	var schemaDocument any
	require.NoError(t, json.Unmarshal(schemaBytes, &schemaDocument))
	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("schema.json", schemaDocument))
	schema, err := compiler.Compile("schema.json")
	require.NoError(t, err)

	size := int64(3)
	enabled := true
	deps := fakeDeps{
		health: HealthResult{
			Rows:  []HealthRow{{ID: "CHECK#0000", Code: "CHECK", Severity: "HEALTH_WARN"}},
			Total: 1,
		},
		osds: OSDResult{
			Rows:  []OSDRow{{ID: 1, UUID: "uuid-1", Name: "osd.1", Up: true, In: true}},
			Total: 1,
		},
		pools: PoolResult{
			Rows:  []PoolRow{{Name: "pool-a", Type: "replicated", Size: &size}},
			Total: 1,
		},
		daemons: DaemonResult{
			Rows:  []DaemonRow{{ID: "mon.a", Type: "mon", Name: "a", Active: &enabled}},
			Total: 1,
		},
	}
	router := NewRouter(deps)
	methods := Methods()
	tests := map[string]struct {
		method funcapi.FunctionConfig
	}{
		MethodHealth:  {method: methods[0]},
		MethodOSDs:    {method: methods[1]},
		MethodPools:   {method: methods[2]},
		MethodDaemons: {method: methods[3]},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			response := router.Handle(context.Background(), test.method.ID, nil)
			params := test.method.RequiredParams
			if len(response.RequiredParams) != 0 {
				params = response.RequiredParams
			}
			accepted := make([]string, 0, len(params))
			required := make([]map[string]any, 0, len(params))
			for _, param := range params {
				accepted = append(accepted, param.ID)
				required = append(required, param.RequiredParam())
			}
			payload := map[string]any{
				"v": 3, "update_every": 10, "status": response.Status, "type": "table",
				"has_history": false, "help": response.Help, "accepted_params": accepted,
				"required_params": required, "columns": response.Columns, "data": response.Data,
				"default_sort_column": response.DefaultSortColumn,
			}
			encoded, err := json.Marshal(payload)
			require.NoError(t, err)
			var decoded any
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			require.NoError(t, schema.Validate(decoded))
		})
	}
}

func TestRouterHealthTruncationIsExplicit(t *testing.T) {
	deps := fakeDeps{
		health: HealthResult{
			Rows: []HealthRow{
				{
					ID:       "A#0000",
					Code:     "A",
					Severity: "HEALTH_ERR",
					Summary:  strings.Repeat("é", maxFunctionTextLength),
					Detail:   strings.Repeat("x", maxFunctionTextLength+1),
				},
				{ID: "B#0000", Code: "B", Severity: "HEALTH_WARN"},
			},
			Total: 3,
		},
	}
	response := (&router{
		deps: deps,
	}).health(context.Background(), 1)
	require.Equal(t, 200, response.Status)
	data := response.Data.([][]any)
	require.Len(t, data, 1)
	assert.Contains(t, response.Help, "Showing the 1 most severe of 3")

	detailIndex := response.Columns["detail"].(map[string]any)["index"].(int)
	summaryIndex := response.Columns["summary"].(map[string]any)["index"].(int)
	detailTruncatedIndex := response.Columns["detail_truncated"].(map[string]any)["index"].(int)
	resultTruncatedIndex := response.Columns["truncated"].(map[string]any)["index"].(int)
	assert.Len(t, data[0][detailIndex].(string), maxFunctionTextLength)
	assert.LessOrEqual(t, len(data[0][summaryIndex].(string)), maxFunctionTextLength)
	assert.True(t, utf8.ValidString(data[0][summaryIndex].(string)))
	assert.Equal(t, true, data[0][detailTruncatedIndex])
	assert.Equal(t, true, data[0][resultTruncatedIndex])
}

func TestRouterSourceErrors(t *testing.T) {
	response := NewRouter(fakeDeps{
		healthErr: &SourceError{
			Status:  403,
			Message: "read permission required",
		},
	}).
		Handle(context.Background(), MethodHealth, nil)
	assert.Equal(t, 403, response.Status)
}

func TestRouterMethodParams(t *testing.T) {
	router := NewRouter(fakeDeps{})
	tests := map[string]struct {
		method    string
		wantError bool
	}{
		"known method":   {method: MethodPools},
		"unknown method": {method: "unknown", wantError: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			params, err := router.MethodParams(context.Background(), test.method)
			if test.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Empty(t, params)
		})
	}
}

func TestRouterInventoryLimitParams(t *testing.T) {
	tests := map[string]struct {
		params       funcapi.ResolvedParams
		wantStatus   int
		wantCaptured int
		wantData     bool
	}{
		"supported limit": {
			params:       funcapi.ResolveParams(inventoryParams(), map[string][]string{ParamLimit: {"2500"}}),
			wantStatus:   200,
			wantCaptured: 2500,
			wantData:     true,
		},
		"unsupported limit": {
			params: funcapi.ResolvedParams{
				ParamLimit: {IDs: []string{"999"}},
			},
			wantStatus: 400,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			captured := 0
			deps := fakeDeps{
				onCall: func(method string, _ context.Context, limit int) {
					if method == MethodOSDs {
						captured = limit
					}
				},
			}
			response := NewRouter(deps).Handle(context.Background(), MethodOSDs, test.params)
			require.Equal(t, test.wantStatus, response.Status)
			assert.Equal(t, test.wantCaptured, captured)
			if test.wantData {
				assert.NotNil(t, response.Data)
			} else {
				assert.Nil(t, response.Data)
			}
		})
	}
}

func TestRouterInventoryDoesNotReturnPartialRows(t *testing.T) {
	tests := map[string]struct {
		result OSDResult
	}{
		"incomplete response": {result: OSDResult{
			Rows:  []OSDRow{{ID: 1, UUID: "uuid-1"}},
			Total: 2,
		}},
		"above selected limit": {result: OSDResult{
			Total: DefaultInventoryLimit + 1,
		}},
		"above hard limit": {result: OSDResult{
			Total: MaxInventoryLimit + 1,
		}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			response := NewRouter(fakeDeps{
				osds: test.result,
			}).Handle(context.Background(), MethodOSDs, nil)
			assert.Equal(t, 422, response.Status)
			assert.Nil(t, response.Data)
		})
	}
}

func TestRouterAppliesInternalDeadlines(t *testing.T) {
	tests := map[string]struct{}{
		MethodHealth:  {},
		MethodOSDs:    {},
		MethodPools:   {},
		MethodDaemons: {},
	}
	for method := range tests {
		t.Run(method, func(t *testing.T) {
			var duration time.Duration
			deps := fakeDeps{
				onCall: func(_ string, ctx context.Context, _ int) {
					deadline, ok := ctx.Deadline()
					require.True(t, ok)
					duration = time.Until(deadline)
				},
			}
			response := NewRouter(deps).Handle(context.Background(), method, nil)
			require.Equal(t, 200, response.Status)
			expected, ok := methodTimeout(method)
			require.True(t, ok)
			assert.Positive(t, duration)
			assert.LessOrEqual(t, duration, expected)
		})
	}
}

func TestRouterAcceptsMaximumOSDInventoryRowCount(t *testing.T) {
	rows := representativeMaximumOSDRows()
	params := funcapi.ResolveParams(inventoryParams(), map[string][]string{
		ParamLimit: {fmt.Sprint(MaxInventoryLimit)},
	})
	response := NewRouter(fakeDeps{
		osds: OSDResult{
			Rows:  rows,
			Total: len(rows),
		},
	}).
		Handle(context.Background(), MethodOSDs, params)
	require.Equal(t, 200, response.Status)
	data, ok := response.Data.([][]any)
	require.True(t, ok)
	assert.Len(t, data, MaxInventoryLimit)
}

var benchmarkPayload []byte

func BenchmarkRepresentativeMaximumOSDInventorySerialization(b *testing.B) {
	rows := representativeMaximumOSDRows()
	params := funcapi.ResolveParams(inventoryParams(), map[string][]string{
		ParamLimit: {fmt.Sprint(MaxInventoryLimit)},
	})
	router := NewRouter(fakeDeps{
		osds: OSDResult{
			Rows:  rows,
			Total: len(rows),
		},
	})
	b.ReportAllocs()
	for b.Loop() {
		response := router.Handle(context.Background(), MethodOSDs, params)
		var err error
		benchmarkPayload, err = json.Marshal(response)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func representativeMaximumOSDRows() []OSDRow {
	rows := make([]OSDRow, MaxInventoryLimit)
	for i := range rows {
		rows[i] = OSDRow{
			ID:                int64(i),
			UUID:              fmt.Sprintf("00000000-0000-0000-0000-%012d", i),
			Name:              fmt.Sprintf("osd.%d", i),
			Host:              fmt.Sprintf("storage-host-%04d.example.net", i),
			DeviceClass:       "nvme",
			Up:                true,
			In:                true,
			OperationalStatus: "working",
			TotalBytes:        8 << 40,
			UsedBytes:         4 << 40,
			AvailableBytes:    4 << 40,
			Utilization:       50,
			ReadBytesPerSec:   1 << 30,
			WriteBytesPerSec:  1 << 30,
			ReadOpsPerSec:     100000,
			WriteOpsPerSec:    100000,
			CommitLatencyMS:   1.25,
			ApplyLatencyMS:    1.5,
		}
	}
	return rows
}

type fakeDeps struct {
	health    HealthResult
	osds      OSDResult
	pools     PoolResult
	daemons   DaemonResult
	healthErr error
	onCall    func(string, context.Context, int)
}

func (d fakeDeps) Health(ctx context.Context, limit int) (HealthResult, error) {
	if d.onCall != nil {
		d.onCall(MethodHealth, ctx, limit)
	}
	return d.health, d.healthErr
}

func (d fakeDeps) OSDs(ctx context.Context, limit int) (OSDResult, error) {
	if d.onCall != nil {
		d.onCall(MethodOSDs, ctx, limit)
	}
	return d.osds, nil
}

func (d fakeDeps) Pools(ctx context.Context, limit int) (PoolResult, error) {
	if d.onCall != nil {
		d.onCall(MethodPools, ctx, limit)
	}
	return d.pools, nil
}

func (d fakeDeps) Daemons(ctx context.Context, limit int) (DaemonResult, error) {
	if d.onCall != nil {
		d.onCall(MethodDaemons, ctx, limit)
	}
	return d.daemons, nil
}

var _ funcapi.MethodHandler = NewRouter(fakeDeps{})
