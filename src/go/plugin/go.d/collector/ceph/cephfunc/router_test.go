// SPDX-License-Identifier: GPL-3.0-or-later

package cephfunc

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

func TestMethods(t *testing.T) {
	methods := Methods()
	require.Len(t, methods, 6)
	assert.Equal(t, []string{
		MethodHealth, MethodOSDs, MethodPools, MethodDaemons, MethodRGWMultisite, MethodRGWQuotas,
	}, []string{methods[0].ID, methods[1].ID, methods[2].ID, methods[3].ID, methods[4].ID, methods[5].ID})
}

func TestRouterTableResponses(t *testing.T) {
	used, objects, maxBytes, maxObjects, size := int64(50), int64(5), int64(100), int64(10), int64(3)
	enabled := true
	utilization := 50.0
	deps := fakeDeps{
		health: HealthResult{Rows: []HealthRow{{
			ID: "CHECK#0000", Code: "CHECK", Severity: "HEALTH_WARN", Summary: "summary", Count: 1, Detail: "detail",
		}}, Total: 1},
		osds:      OSDResult{Rows: []OSDRow{{ID: 1, UUID: "uuid-1", Name: "osd.1", Up: true, In: true}}, Total: 1},
		pools:     PoolResult{Rows: []PoolRow{{Name: "pool-a", Type: "replicated", Size: &size}}, Total: 1},
		daemons:   DaemonResult{Rows: []DaemonRow{{ID: "mon.a", Type: "mon", Name: "a", Active: &enabled}}, Total: 1},
		multisite: RGWMultisiteResult{Rows: []RGWMultisiteRow{{ID: "realm:r", Kind: "realm", Name: "r"}}, Total: 1},
		quotas: RGWQuotaResult{Rows: []RGWQuotaRow{{
			Key: "user:user-a", ID: "user-a", Kind: "user", Status: "ok", UsedBytes: &used, Objects: &objects,
			QuotaEnabled: &enabled, QuotaMaxBytes: &maxBytes, QuotaMaxObjects: &maxObjects, Utilization: &utilization,
		}}, Total: 1},
	}
	router := NewRouter(deps, enabledConfig(10))

	for _, method := range []string{MethodHealth, MethodOSDs, MethodPools, MethodDaemons, MethodRGWMultisite, MethodRGWQuotas} {
		t.Run(method, func(t *testing.T) {
			response := router.Handle(context.Background(), method, nil)
			require.Equal(t, 200, response.Status)
			data := response.Data.([][]any)
			require.Len(t, data, 1)
			assert.Equal(t, len(response.Columns), len(data[0]))
			assert.NotEmpty(t, response.DefaultSortColumn)
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

	used, objects, maxBytes, maxObjects, size := int64(50), int64(5), int64(100), int64(10), int64(3)
	enabled := true
	utilization := 50.0
	deps := fakeDeps{
		health:    HealthResult{Rows: []HealthRow{{ID: "CHECK#0000", Code: "CHECK", Severity: "HEALTH_WARN"}}, Total: 1},
		osds:      OSDResult{Rows: []OSDRow{{ID: 1, UUID: "uuid-1", Name: "osd.1", Up: true, In: true}}, Total: 1},
		pools:     PoolResult{Rows: []PoolRow{{Name: "pool-a", Type: "replicated", Size: &size}}, Total: 1},
		daemons:   DaemonResult{Rows: []DaemonRow{{ID: "mon.a", Type: "mon", Name: "a", Active: &enabled}}, Total: 1},
		multisite: RGWMultisiteResult{Rows: []RGWMultisiteRow{{ID: "realm:r", Kind: "realm", Name: "r"}}, Total: 1},
		quotas: RGWQuotaResult{Rows: []RGWQuotaRow{{
			Key: "user:user-a", ID: "user-a", Kind: "user", Status: "ok", UsedBytes: &used, Objects: &objects,
			QuotaEnabled: &enabled, QuotaMaxBytes: &maxBytes, QuotaMaxObjects: &maxObjects, Utilization: &utilization,
		}}, Total: 1},
	}
	router := NewRouter(deps, enabledConfig(10))
	for _, method := range []string{MethodHealth, MethodOSDs, MethodPools, MethodDaemons, MethodRGWMultisite, MethodRGWQuotas} {
		t.Run(method, func(t *testing.T) {
			response := router.Handle(context.Background(), method, nil)
			payload := map[string]any{
				"v": 3, "update_every": 10, "status": response.Status, "type": "table",
				"has_history": false, "help": response.Help, "accepted_params": []string{},
				"required_params": []string{}, "columns": response.Columns, "data": response.Data,
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

func TestRouterTruncationIsExplicit(t *testing.T) {
	deps := fakeDeps{health: HealthResult{
		Rows: []HealthRow{
			{ID: "A#0000", Code: "A", Severity: "HEALTH_ERR", Summary: strings.Repeat("é", maxFunctionTextLength), Detail: strings.Repeat("x", maxFunctionTextLength+1)},
			{ID: "B#0000", Code: "B", Severity: "HEALTH_WARN"},
		},
		Total: 3,
	}}
	config := enabledConfig(1)
	response := NewRouter(deps, config).Handle(context.Background(), MethodHealth, nil)
	require.Equal(t, 200, response.Status)
	data := response.Data.([][]any)
	require.Len(t, data, 1)
	assert.Contains(t, response.Help, "Showing 1 of 3")

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

func TestRouterRGWMultisiteUsesTopologyOrder(t *testing.T) {
	deps := fakeDeps{multisite: RGWMultisiteResult{Rows: []RGWMultisiteRow{
		{ID: "sync:s", Kind: "sync", Name: "s"},
		{ID: "zone:z", Kind: "zone", Name: "z"},
		{ID: "realm:r", Kind: "realm", Name: "r"},
		{ID: "zonegroup:zg", Kind: "zonegroup", Name: "zg"},
	}, Total: 4}}
	response := NewRouter(deps, enabledConfig(10)).Handle(context.Background(), MethodRGWMultisite, nil)
	require.Equal(t, 200, response.Status)
	data := response.Data.([][]any)
	kindIndex := response.Columns["kind"].(map[string]any)["index"].(int)
	assert.Equal(t, []string{"realm", "zonegroup", "zone", "sync"}, []string{
		data[0][kindIndex].(string), data[1][kindIndex].(string), data[2][kindIndex].(string), data[3][kindIndex].(string),
	})
}

func TestRouterDisabledAndSourceErrors(t *testing.T) {
	config := enabledConfig(10)
	config.Health.Disabled = true
	response := NewRouter(fakeDeps{}, config).Handle(context.Background(), MethodHealth, nil)
	assert.Equal(t, 404, response.Status)

	config.Health.Disabled = false
	response = NewRouter(fakeDeps{healthErr: &SourceError{Status: 403, Message: "read permission required"}}, config).
		Handle(context.Background(), MethodHealth, nil)
	assert.Equal(t, 403, response.Status)
}

func TestRouterMethodParams(t *testing.T) {
	router := NewRouter(fakeDeps{}, enabledConfig(10))
	params, err := router.MethodParams(context.Background(), MethodPools)
	require.NoError(t, err)
	assert.Nil(t, params)
	_, err = router.MethodParams(context.Background(), "unknown")
	require.Error(t, err)
}

type fakeDeps struct {
	health    HealthResult
	osds      OSDResult
	pools     PoolResult
	daemons   DaemonResult
	multisite RGWMultisiteResult
	quotas    RGWQuotaResult
	healthErr error
}

func (d fakeDeps) Health(context.Context, int) (HealthResult, error)  { return d.health, d.healthErr }
func (d fakeDeps) OSDs(context.Context, int) (OSDResult, error)       { return d.osds, nil }
func (d fakeDeps) Pools(context.Context, int) (PoolResult, error)     { return d.pools, nil }
func (d fakeDeps) Daemons(context.Context, int) (DaemonResult, error) { return d.daemons, nil }
func (d fakeDeps) RGWMultisite(context.Context, int) (RGWMultisiteResult, error) {
	return d.multisite, nil
}
func (d fakeDeps) RGWQuotas(context.Context, int) (RGWQuotaResult, error) { return d.quotas, nil }

func enabledConfig(limit int) Config {
	method := MethodConfig{Limit: limit}
	return Config{Health: method, OSDs: method, Pools: method, Daemons: method, RGWMultisite: method, RGWQuotas: method}
}

var _ funcapi.MethodHandler = NewRouter(fakeDeps{}, Config{})
