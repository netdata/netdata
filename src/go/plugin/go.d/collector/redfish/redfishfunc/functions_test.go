// SPDX-License-Identifier: GPL-3.0-or-later

package redfishfunc

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/registry"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testDeps struct {
	snapshots     []testInventorySnapshot
	available     bool
	roots         map[string]string
	catalogVisits *int
	sliceVisits   *int
}

type testInventorySnapshot struct {
	Job  string
	Rows []map[string]any
}

func (d testDeps) VisitInventoryCatalog(
	ctx context.Context,
	maxJobs int,
	visitJob func(string) bool,
	visitHost func(uri, name string) bool,
	visitKind func(string) bool,
) bool {
	if len(d.snapshots) > maxJobs {
		return false
	}
	for _, snapshot := range d.snapshots {
		if ctx.Err() != nil {
			return false
		}
		if d.catalogVisits != nil {
			*d.catalogVisits++
		}
		if !visitJob(snapshot.Job) {
			return false
		}
		for _, row := range snapshot.Rows {
			if uri := mapString(row, "host_uri"); uri != "" {
				if !visitHost(uri, mapString(row, "host_name")) {
					return false
				}
			}
			if kind := mapString(row, "resource_kind"); kind != "" {
				if !visitKind(kind) {
					return false
				}
			}
		}
	}
	return true
}

func (d testDeps) VisitInventorySlice(
	ctx context.Context,
	job, host, resourceKind string,
	visit func(map[string]any) bool,
) (int, bool) {
	for _, snapshot := range d.snapshots {
		if snapshot.Job != job {
			continue
		}
		var rows []map[string]any
		for _, row := range snapshot.Rows {
			if mapString(row, "host_uri") == host && mapString(row, "resource_kind") == resourceKind {
				rows = append(rows, row)
			}
		}
		slices.SortStableFunc(rows, func(a, b map[string]any) int {
			return strings.Compare(mapString(a, "sort_key"), mapString(b, "sort_key"))
		})
		for _, row := range rows {
			if ctx.Err() != nil {
				break
			}
			if d.sliceVisits != nil {
				*d.sliceVisits++
			}
			if !visit(maps.Clone(row)) {
				break
			}
		}
		return len(rows), len(rows) > 0
	}
	return 0, false
}

func (d testDeps) AnyBackendAvailable() bool   { return d.available }
func (d testDeps) LogRoots() map[string]string { return d.roots }

type callbackDeps struct {
	catalog func(
		context.Context,
		int,
		func(string) bool,
		func(string, string) bool,
		func(string) bool,
	) bool
	slice func(context.Context, string, string, string, func(map[string]any) bool) (int, bool)
}

func (d callbackDeps) VisitInventoryCatalog(
	ctx context.Context,
	maxJobs int,
	visitJob func(string) bool,
	visitHost func(string, string) bool,
	visitKind func(string) bool,
) bool {
	if d.catalog == nil {
		return true
	}
	return d.catalog(ctx, maxJobs, visitJob, visitHost, visitKind)
}

func (d callbackDeps) VisitInventorySlice(
	ctx context.Context,
	job, host, kind string,
	visit func(map[string]any) bool,
) (int, bool) {
	if d.slice == nil {
		return 0, false
	}
	return d.slice(ctx, job, host, kind, visit)
}

func (callbackDeps) AnyBackendAvailable() bool   { return false }
func (callbackDeps) LogRoots() map[string]string { return nil }

func TestConfigsExposeExactProcessFunctions(t *testing.T) {
	deps := testDeps{available: true}
	configs := Configs(deps)()
	require.Len(t, configs, 2)

	inventory := configs[0]
	assert.Equal(t, "inventory", inventory.ID)
	assert.Equal(t, "redfish:inventory", inventory.FunctionName)
	assert.Equal(t, "Redfish Inventory", inventory.Name)
	assert.Equal(t, 60, inventory.UpdateEvery)
	assert.Equal(t, "table", inventory.ResponseType)
	assert.True(t, inventory.RawRequest)
	assert.False(t, inventory.RequireCloud)
	assert.Nil(t, inventory.Available)

	logs := configs[1]
	assert.Equal(t, "logs", logs.ID)
	assert.Equal(t, "redfish:logs", logs.FunctionName)
	assert.Equal(t, "Redfish Logs", logs.Name)
	assert.Equal(t, 1, logs.UpdateEvery)
	assert.Equal(t, "logs", logs.ResponseType)
	assert.Equal(t, "logs", logs.Tags)
	assert.True(t, logs.RawRequest)
	assert.True(t, logs.RequireCloud)
	require.NotNil(t, logs.Available)
	assert.True(t, logs.Available())

	deps.available = false
	configs = Configs(deps)()
	assert.False(t, configs[1].Available())
}

func TestInventoryInfoExposesExactSelectorsAndColumns(t *testing.T) {
	deps := testDeps{snapshots: []testInventorySnapshot{
		{
			Job: "job-b",
			Rows: []map[string]any{{
				"host_uri":      "/redfish/v1/Systems/2",
				"host_name":     "Host Two",
				"resource_kind": "fan",
			}},
		},
		{
			Job: "job-a",
			Rows: []map[string]any{{
				"host_uri":      "/redfish/v1/Systems/1",
				"host_name":     "Host One",
				"resource_kind": "system",
			}},
		},
	}}
	handler := newTestHandler(t, deps)
	response := handler.HandleRaw(context.Background(), rawRequest("inventory", true, nil))

	payload := requireRawInventoryPayload(t, response)
	require.Equal(t, 200, payload["status"])
	assert.Equal(t, "sort_key", payload["default_sort_column"])
	assert.Equal(t, []json.RawMessage{}, payload["data"])
	selectors, _, fits, err := buildInventorySelectors(context.Background(), deps, maxInventoryResponseBytes)
	require.NoError(t, err)
	require.True(t, fits)
	assert.Equal(t, []string{"job-a", "job-b"}, selectors.jobs)
	assert.Equal(
		t,
		[]string{"/redfish/v1/Systems/1", "/redfish/v1/Systems/2"},
		selectors.hosts,
	)
	assert.Equal(t, []string{"fan", "system"}, selectors.resourceKinds)

	contract := registry.MustCompile()
	columns := requireInventoryColumns(t, payload)
	require.Len(t, columns, len(contract.Columns))
	for _, column := range contract.Columns {
		wire, ok := columns[column.ID].(map[string]any)
		require.True(t, ok, "column %q wire type = %T", column.ID, columns[column.ID])
		assertColumnPresentation(t, column, wire)
	}
}

func TestInventoryHostSelectorUsesURIWhenJobsDisagreeOnName(t *testing.T) {
	const hostURI = "/redfish/v1/Systems/1"
	deps := testDeps{snapshots: []testInventorySnapshot{
		{Job: "job-a", Rows: []map[string]any{{"host_uri": hostURI, "host_name": "Host A"}}},
		{Job: "job-b", Rows: []map[string]any{{"host_uri": hostURI, "host_name": "Host B"}}},
	}}
	selectors, _, fits, err := buildInventorySelectors(context.Background(), deps, maxInventoryResponseBytes)
	require.NoError(t, err)
	require.True(t, fits)

	require.Equal(t, []string{hostURI}, selectors.hosts)
	required := decodeRequiredParams(t, selectors.requiredParams)
	require.Equal(t, hostURI, required[1].Options[0].ID)
	require.Equal(t, hostURI, required[1].Options[0].Name)
}

func TestNormalizeLogsPayloadRejectsMalformedSelectionsObject(t *testing.T) {
	_, err := normalizeLogsPayload(
		rawRequest("logs", false, []byte(`null`)),
		map[string]string{"default": "backend-key"},
	)
	require.ErrorContains(t, err, "request must be a JSON object")

	for name, payload := range map[string]string{
		"string":  `{"selections":"all"}`,
		"array":   `{"selections":[]}`,
		"number":  `{"selections":1}`,
		"boolean": `{"selections":true}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := normalizeLogsPayload(
				rawRequest("logs", false, []byte(payload)),
				map[string]string{"default": "backend-key"},
			)
			require.ErrorContains(t, err, "selections must be an object")
		})
	}

	for name, payload := range map[string]string{
		"absent":        `{}`,
		"null":          `{"selections":null}`,
		"empty sources": `{"selections":{"__logs_sources":[]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			encoded, err := normalizeLogsPayload(
				rawRequest("logs", false, []byte(payload)),
				map[string]string{"default": "backend-key"},
			)
			require.NoError(t, err)
			var decoded map[string]any
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			_, ok := decoded["selections"].(map[string]any)
			require.True(t, ok)
		})
	}
}

func TestInventoryDataRequiresAndAppliesExactSourceSlice(t *testing.T) {
	earlier := map[string]any{
		"sort_key":      "02",
		"host_uri":      "/redfish/v1/Systems/1",
		"host_name":     "Host One",
		"resource_kind": "fan",
		"resource_uri":  "/redfish/v1/Chassis/1/Fans/2",
		"name":          "Fan Two",
		"health":        "warning",
	}
	first := map[string]any{
		"sort_key":      "01",
		"host_uri":      "/redfish/v1/Systems/1",
		"host_name":     "Host One",
		"resource_kind": "fan",
		"resource_uri":  "/redfish/v1/Chassis/1/Fans/1",
		"name":          "Fan One",
		"health":        "ok",
	}
	otherKind := map[string]any{
		"sort_key":      "03",
		"host_uri":      "/redfish/v1/Systems/1",
		"resource_kind": "system",
		"resource_uri":  "/redfish/v1/Systems/1",
	}
	otherHost := map[string]any{
		"sort_key":      "04",
		"host_uri":      "/redfish/v1/Systems/2",
		"resource_kind": "fan",
		"resource_uri":  "/redfish/v1/Chassis/2/Fans/1",
	}
	deps := testDeps{snapshots: []testInventorySnapshot{{
		Job:  "job-a",
		Rows: []map[string]any{earlier, first, otherKind, otherHost},
	}}}
	handler := newTestHandler(t, deps)
	response := handler.HandleRaw(
		context.Background(),
		rawRequest("inventory", false, selectionPayload("job-a", "/redfish/v1/Systems/1", "fan")),
	)

	payload := requireRawInventoryPayload(t, response)
	require.Equal(t, 200, payload["status"])
	assert.Equal(t, "sort_key", payload["default_sort_column"])
	data := requireInventoryData(t, payload)
	require.Len(t, data, 2)
	wireColumns := requireInventoryColumns(t, payload)

	columns := inventoryColumnsForKind("fan")
	require.Len(t, data[0], len(columns))
	assert.Equal(t, "01", data[0][columnIndex(t, wireColumns, "sort_key")])
	assert.Equal(t, "Fan One", data[0][columnIndex(t, wireColumns, "name")])
	assert.Equal(t, "ok", data[0][columnIndex(t, wireColumns, "health")])
	assert.Equal(t, "02", data[1][columnIndex(t, wireColumns, "sort_key")])
	assert.Equal(t, "Fan Two", data[1][columnIndex(t, wireColumns, "name")])
	assert.Equal(t, "warning", data[1][columnIndex(t, wireColumns, "health")])

	for index, column := range columns {
		assert.Equal(t, first[column.ID], data[0][index], "column %q is misaligned", column.ID)
	}
}

func TestInventoryReturnsCompleteTenThousandRowSliceWithoutPagination(t *testing.T) {
	const rows = 10_000
	inventory := make([]map[string]any, 0, rows)
	for index := range rows {
		inventory = append(inventory, map[string]any{
			"sort_key":      fmt.Sprintf("%05d", index),
			"host_uri":      "/redfish/v1/Systems/1",
			"host_name":     "Host One",
			"resource_kind": "fan",
			"resource_uri":  fmt.Sprintf("/redfish/v1/Chassis/1/Fans/%d", index),
			"name":          fmt.Sprintf("Fan %d", index),
		})
	}
	response := newTestHandler(t, testDeps{snapshots: []testInventorySnapshot{{
		Job: "job-a", Rows: inventory,
	}}}).HandleRaw(
		context.Background(),
		rawRequest("inventory", false, selectionPayload("job-a", "/redfish/v1/Systems/1", "fan")),
	)

	payload := requireRawInventoryPayload(t, response)
	require.Equal(t, 200, payload["status"])
	data := requireInventoryData(t, payload)
	require.Len(t, data, rows)
	wireColumns := requireInventoryColumns(t, payload)
	assert.Equal(t, "00000", data[0][columnIndex(t, wireColumns, "sort_key")])
	assert.Equal(t, "09999", data[rows-1][columnIndex(t, wireColumns, "sort_key")])
}

func TestInventoryResponseRejectsEncodedOversizeResult(t *testing.T) {
	payload := inventoryResponsePayload(
		map[string]any{},
		[]json.RawMessage{json.RawMessage(`[` + `"` + strings.Repeat("x", 1024) + `"` + `]`)},
		inventorySelectors{requiredParams: json.RawMessage(`[]`)},
	)
	encoded, err := json.Marshal(payload)
	require.NoError(t, err)
	rejected := boundedInventoryResponseAt(payload, 1, len(encoded)-1)
	assert.Equal(t, 413, rejected.Status)
	assert.Contains(t, rejected.Message, "1 rows")
	assert.Contains(t, rejected.Message, "encoded")

	accepted := boundedInventoryResponseAt(payload, 1, len(encoded))
	assert.Equal(t, payload, accepted.RawResponse)
}

func TestInventoryDataStopsAtTheEncodedResponseLimit(t *testing.T) {
	const rows = 10_000
	visits := 0
	deps := testDeps{
		sliceVisits: &visits,
		snapshots: []testInventorySnapshot{{
			Job: "job-a",
			Rows: func() []map[string]any {
				result := make([]map[string]any, rows)
				for index := range result {
					result[index] = map[string]any{
						"sort_key":      fmt.Sprintf("%05d", index),
						"host_uri":      "host-a",
						"resource_kind": "fan",
						"name":          "fan",
					}
				}
				return result
			}(),
		}},
	}
	selectors := testInventorySelectors(t, "job-a", "host-a", "fan")
	selections := map[string]string{"__job": "job-a", "host": "host-a", "resource_kind": "fan"}
	base := inventoryResponsePayload(
		buildInventoryColumns(inventoryColumnsForKind("fan")),
		[]json.RawMessage{},
		selectors,
	)
	baseSize, fits, err := boundedJSONSize(base, maxInventoryResponseBytes)
	require.NoError(t, err)
	require.True(t, fits)

	response := buildInventoryDataResponseAt(
		context.Background(), deps, selectors, selections, baseSize+32,
	)
	assert.Equal(t, 413, response.Status)
	assert.Contains(t, response.Message, fmt.Sprintf("%d rows", rows))
	assert.Less(t, visits, rows)
}

func TestInventoryDataRejectsOneOversizeRow(t *testing.T) {
	deps := testDeps{snapshots: []testInventorySnapshot{{
		Job: "job-a",
		Rows: []map[string]any{{
			"sort_key":      "01",
			"host_uri":      "host-a",
			"resource_kind": "fan",
			"name":          strings.Repeat("<", 1<<20),
		}},
	}}}
	selectors := testInventorySelectors(t, "job-a", "host-a", "fan")
	selections := map[string]string{"__job": "job-a", "host": "host-a", "resource_kind": "fan"}
	base := inventoryResponsePayload(
		buildInventoryColumns(inventoryColumnsForKind("fan")),
		[]json.RawMessage{},
		selectors,
	)
	baseSize, fits, err := boundedJSONSize(base, maxInventoryResponseBytes)
	require.NoError(t, err)
	require.True(t, fits)

	response := buildInventoryDataResponseAt(
		context.Background(), deps, selectors, selections, baseSize+(4<<10),
	)
	assert.Equal(t, 413, response.Status)
	assert.Contains(t, response.Message, "1 rows")
}

func TestInventoryDataReportsAnUnencodableRow(t *testing.T) {
	deps := testDeps{snapshots: []testInventorySnapshot{{
		Job: "job-a",
		Rows: []map[string]any{{
			"sort_key":      "01",
			"host_uri":      "host-a",
			"resource_kind": "fan",
			"name":          func() {},
		}},
	}}}
	selectors := testInventorySelectors(t, "job-a", "host-a", "fan")
	response := buildInventoryDataResponseAt(
		context.Background(),
		deps,
		selectors,
		map[string]string{"__job": "job-a", "host": "host-a", "resource_kind": "fan"},
		maxInventoryResponseBytes,
	)
	assert.Equal(t, 500, response.Status)
	assert.Contains(t, response.Message, "unsupported type")
}

func TestInventorySelectorEncodingHasAnExactBoundary(t *testing.T) {
	deps := testDeps{snapshots: []testInventorySnapshot{
		{Job: "job-b", Rows: []map[string]any{{
			"host_uri": "/redfish/v1/Systems/2", "host_name": "Host Two", "resource_kind": "fan",
		}}},
		{Job: "job-a", Rows: []map[string]any{{
			"host_uri": "/redfish/v1/Systems/1", "host_name": "Host One", "resource_kind": "system",
		}}},
	}}
	selectors, _, fits, err := buildInventorySelectors(context.Background(), deps, maxInventoryResponseBytes)
	require.NoError(t, err)
	require.True(t, fits)
	require.Greater(t, len(selectors.requiredParams), 1)

	exact, _, fits, err := buildInventorySelectors(context.Background(), deps, len(selectors.requiredParams))
	require.NoError(t, err)
	require.True(t, fits)
	assert.Equal(t, selectors.requiredParams, exact.requiredParams)
	required := decodeRequiredParams(t, exact.requiredParams)
	for _, param := range required {
		require.GreaterOrEqual(t, len(param.Options), 2)
		assert.True(t, param.Options[0].DefaultSelected)
		assert.False(t, param.Options[1].DefaultSelected)
	}

	_, _, fits, err = buildInventorySelectors(context.Background(), deps, len(selectors.requiredParams)-1)
	require.NoError(t, err)
	assert.False(t, fits)
}

func TestInventorySelectorCatalogStopsBeforeRetainingEveryHost(t *testing.T) {
	const offered = 1_000_000
	visits := 0
	deps := callbackDeps{catalog: func(
		_ context.Context,
		_ int,
		visitJob func(string) bool,
		visitHost func(string, string) bool,
		_ func(string) bool,
	) bool {
		if !visitJob("job") {
			return false
		}
		for index := range offered {
			visits++
			if !visitHost(fmt.Sprintf("host-%d", index), "") {
				return false
			}
		}
		return true
	}}
	_, _, fits, err := buildInventorySelectors(context.Background(), deps, 4<<10)
	require.NoError(t, err)
	assert.False(t, fits)
	assert.Less(t, visits, offered)
}

func TestInventorySelectorRejectsAnOversizeFinalHostLabel(t *testing.T) {
	visits := 0
	deps := callbackDeps{catalog: func(
		_ context.Context,
		_ int,
		visitJob func(string) bool,
		visitHost func(string, string) bool,
		_ func(string) bool,
	) bool {
		if !visitJob("job") {
			return false
		}
		visits++
		return visitHost("host", strings.Repeat("x", 8<<10))
	}}
	_, _, fits, err := buildInventorySelectors(context.Background(), deps, 4<<10)
	require.NoError(t, err)
	assert.False(t, fits)
	assert.Equal(t, 1, visits)
}

func TestInventorySelectorConflictCanReduceOversizeNamesToTheURI(t *testing.T) {
	const hostURI = "/redfish/v1/Systems/1"
	largeName := strings.Repeat("x", 8<<10)
	deps := testDeps{snapshots: []testInventorySnapshot{
		{Job: "job-a", Rows: []map[string]any{{"host_uri": hostURI, "host_name": largeName + "a"}}},
		{Job: "job-b", Rows: []map[string]any{{"host_uri": hostURI, "host_name": largeName + "b"}}},
	}}
	selectors, _, fits, err := buildInventorySelectors(context.Background(), deps, 1<<10)
	require.NoError(t, err)
	require.True(t, fits)
	required := decodeRequiredParams(t, selectors.requiredParams)
	require.Len(t, required, 3)
	require.Len(t, required[1].Options, 1)
	assert.Equal(t, hostURI, required[1].Options[0].ID)
	assert.Equal(t, hostURI, required[1].Options[0].Name)
}

func TestInventorySelectorCatalogHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	visits := 0
	deps := callbackDeps{catalog: func(
		_ context.Context,
		_ int,
		visitJob func(string) bool,
		visitHost func(string, string) bool,
		_ func(string) bool,
	) bool {
		if !visitJob("job") {
			return false
		}
		for index := range 1_000_000 {
			visits++
			if index == 10 {
				cancel()
			}
			if !visitHost(fmt.Sprintf("host-%d", index), "") {
				return false
			}
		}
		return true
	}}
	_, _, fits, err := buildInventorySelectors(ctx, deps, maxInventoryResponseBytes)
	require.ErrorIs(t, err, context.Canceled)
	assert.False(t, fits)
	assert.LessOrEqual(t, visits, 11)
}

func TestInventoryValueSortIsDeterministicAndCancelable(t *testing.T) {
	values := []string{"c", "a", "d", "b", "a"}
	require.NoError(t, sortInventoryValues(context.Background(), values, strings.Compare))
	assert.Equal(t, []string{"a", "a", "b", "c", "d"}, values)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancel()
	values = make([]string, 100_000)
	require.ErrorIs(t, sortInventoryValues(ctx, values, strings.Compare), context.Canceled)
}

type countedJSONValue struct {
	calls *int
	size  int
}

func (v countedJSONValue) MarshalJSON() ([]byte, error) {
	*v.calls = *v.calls + 1
	return []byte(`"` + strings.Repeat("x", v.size) + `"`), nil
}

func TestBoundedJSONSizeStopsBeforeEncodingTheCompleteOversizeValue(t *testing.T) {
	calls := 0
	values := make([]any, 10_000)
	for index := range values {
		values[index] = countedJSONValue{calls: &calls, size: 1_024}
	}
	_, fits, err := boundedJSONSize(values, 4<<10)
	require.NoError(t, err)
	require.False(t, fits)
	require.Less(t, calls, len(values))
}

func TestBoundedJSONStringSizeMatchesEncodingJSON(t *testing.T) {
	for _, value := range []string{
		"plain",
		"quote\" slash\\ controls\x00\n",
		"html <>&",
		"unicode π \u2028 \u2029",
		string([]byte{'a', 0xff, 'b'}),
	} {
		encoded, err := json.Marshal(value)
		require.NoError(t, err)
		assert.Equal(t, len(encoded), boundedJSONStringSize(value, len(encoded)), "%q", value)
	}
	assert.Equal(t, 17, boundedJSONStringSize(strings.Repeat("<", 1_000), 16))
	_, fits, err := appendBoundedJSONString(nil, strings.Repeat("<", 1_000), 16)
	require.NoError(t, err)
	assert.False(t, fits)
}

func TestInventoryDataResponseMatchesFunctionUISchema(t *testing.T) {
	response := newTestHandler(t, testDeps{snapshots: []testInventorySnapshot{{
		Job: "job-a",
		Rows: []map[string]any{{
			"sort_key":      "01",
			"host_uri":      "/redfish/v1/Systems/1",
			"host_name":     "Host One",
			"resource_kind": "fan",
			"resource_uri":  "/redfish/v1/Chassis/1/Fans/1",
			"name":          "Fan One",
		}},
	}}}).HandleRaw(
		context.Background(),
		rawRequest("inventory", false, selectionPayload("job-a", "/redfish/v1/Systems/1", "fan")),
	)
	payload := requireRawInventoryPayload(t, response)
	require.Equal(t, 200, payload["status"])
	validateFunctionPayload(t, payload)
}

func TestInventoryRejectsInvalidOrEmptySelections(t *testing.T) {
	deps := testDeps{snapshots: []testInventorySnapshot{{
		Job: "job-a",
		Rows: []map[string]any{{
			"sort_key":      "01",
			"host_uri":      "/redfish/v1/Systems/1",
			"resource_kind": "fan",
		}},
	}}}
	handler := newTestHandler(t, deps)

	tests := map[string][]byte{
		"invalid JSON":     []byte(`{`),
		"missing selector": []byte(`{"selections":{"__job":["job-a"]}}`),
		"multiple values":  []byte(`{"selections":{"__job":["job-a","job-b"],"host":["/redfish/v1/Systems/1"],"resource_kind":["fan"]}}`),
		"unknown job":      selectionPayload("missing", "/redfish/v1/Systems/1", "fan"),
		"unknown host":     selectionPayload("job-a", "/redfish/v1/Systems/missing", "fan"),
		"unknown kind":     selectionPayload("job-a", "/redfish/v1/Systems/1", "drive"),
	}
	for name, payload := range tests {
		t.Run(name, func(t *testing.T) {
			response := handler.HandleRaw(context.Background(), rawRequest("inventory", false, payload))
			assert.Equal(t, 400, response.Status)
			assert.Contains(t, response.Message, "Redfish inventory query failed")
		})
	}

	empty := newTestHandler(t, testDeps{}).HandleRaw(
		context.Background(),
		rawRequest("inventory", false, selectionPayload("job-a", "host", "fan")),
	)
	assert.Equal(t, 503, empty.Status)
}

func TestInventoryRejectsCrossSnapshotCombinationWithNoCurrentSlice(t *testing.T) {
	deps := testDeps{snapshots: []testInventorySnapshot{
		{
			Job: "job-a",
			Rows: []map[string]any{{
				"sort_key":      "01",
				"host_uri":      "/redfish/v1/Systems/1",
				"resource_kind": "fan",
			}},
		},
		{
			Job: "job-b",
			Rows: []map[string]any{{
				"sort_key":      "02",
				"host_uri":      "/redfish/v1/Systems/2",
				"resource_kind": "drive",
			}},
		},
	}}
	response := newTestHandler(t, deps).HandleRaw(
		context.Background(),
		rawRequest("inventory", false, selectionPayload("job-a", "/redfish/v1/Systems/2", "drive")),
	)
	assert.Equal(t, 400, response.Status)
	assert.Contains(t, response.Message, "do not identify a current slice")
}

func TestFunctionHandlerRejectsUnknownMethod(t *testing.T) {
	response := newTestHandler(t, testDeps{}).HandleRaw(
		context.Background(),
		rawRequest("other", false, []byte(`{}`)),
	)
	assert.Equal(t, 404, response.Status)
}

func TestInventoryEnumColumnsUsePills(t *testing.T) {
	for _, id := range []string{
		"row_type",
		"health",
		"reading_type",
		"threshold_upper_critical_activation",
	} {
		column, ok := inventoryColumnByID(id)
		if !ok {
			t.Fatalf("column %q is missing", id)
		}
		if column.Type != funcapi.FieldTypeString {
			t.Errorf("column %q type = %q, want %q", id, column.Type, funcapi.FieldTypeString)
		}
		if column.Visualization != funcapi.FieldVisualPill {
			t.Errorf("column %q visualization = %q, want %q", id, column.Visualization, funcapi.FieldVisualPill)
		}
		if column.Filter != funcapi.FieldFilterMultiselect {
			t.Errorf("column %q filter = %q, want %q", id, column.Filter, funcapi.FieldFilterMultiselect)
		}
	}
}

func TestInventoryBooleanColumnsUseBooleanPills(t *testing.T) {
	column, ok := inventoryColumnByID("log_service_overflow")
	if !ok {
		t.Fatal("column \"log_service_overflow\" is missing")
	}
	if column.Type != funcapi.FieldTypeBoolean {
		t.Errorf("column type = %q, want %q", column.Type, funcapi.FieldTypeBoolean)
	}
	if column.Visualization != funcapi.FieldVisualPill {
		t.Errorf("column visualization = %q, want %q", column.Visualization, funcapi.FieldVisualPill)
	}
	if column.Filter != funcapi.FieldFilterMultiselect {
		t.Errorf("column filter = %q, want %q", column.Filter, funcapi.FieldFilterMultiselect)
	}
}

func assertColumnPresentation(t *testing.T, source registry.ColumnSpec, wire map[string]any) {
	t.Helper()
	assert.Equal(t, source.Order, wire["index"], source.ID)
	assert.Equal(t, source.Tooltip, wire["name"], source.ID)
	assert.Equal(t, source.Visible, wire["visible"], source.ID)
	assert.Equal(t, source.Sortable, wire["sortable"], source.ID)
	assert.Equal(t, source.Sticky, wire["sticky"], source.ID)
	assert.Equal(t, source.Unique, wire["unique_key"], source.ID)
	assert.Equal(t, source.Structured, wire["wrap"], source.ID)
	if source.Units == "" {
		assert.NotContains(t, wire, "units", source.ID)
	} else {
		assert.Equal(t, source.Units, wire["units"], source.ID)
	}

	wantType := funcapi.FieldTypeString
	wantTransform := funcapi.FieldTransformText
	wantVisualization := funcapi.FieldVisualValue
	wantSort := funcapi.FieldSortAscending
	wantSummary := funcapi.FieldSummaryCount
	wantFilter := funcapi.FieldFilterNone
	wantDecimals := 0
	switch source.Type {
	case registry.ColumnEnum:
		wantVisualization = funcapi.FieldVisualPill
		if source.Facet {
			wantFilter = funcapi.FieldFilterMultiselect
		}
	case registry.ColumnInteger:
		wantType = funcapi.FieldTypeInteger
		wantTransform = funcapi.FieldTransformNumber
		wantSort = funcapi.FieldSortDescending
		if source.Facet {
			wantFilter = funcapi.FieldFilterRange
		}
	case registry.ColumnFloat:
		wantType = funcapi.FieldTypeFloat
		wantTransform = funcapi.FieldTransformNumber
		wantSort = funcapi.FieldSortDescending
		wantDecimals = 3
		if source.Facet {
			wantFilter = funcapi.FieldFilterRange
		}
	case registry.ColumnBoolean:
		wantType = funcapi.FieldTypeBoolean
		wantTransform = funcapi.FieldTransformNone
		wantVisualization = funcapi.FieldVisualPill
		if source.Facet {
			wantFilter = funcapi.FieldFilterMultiselect
		}
	case registry.ColumnTimestamp:
		wantType = funcapi.FieldTypeTimestamp
		wantTransform = funcapi.FieldTransformDatetime
		wantSort = funcapi.FieldSortDescending
		if source.Facet {
			wantFilter = funcapi.FieldFilterRange
		}
	default:
		if source.Facet {
			wantFilter = funcapi.FieldFilterMultiselect
		}
	}
	if source.Additive {
		wantSummary = funcapi.FieldSummarySum
	}
	assert.Equal(t, wantType.String(), wire["type"], source.ID)
	assert.Equal(t, wantVisualization.String(), wire["visualization"], source.ID)
	assert.Equal(t, wantSort.String(), wire["sort"], source.ID)
	assert.Equal(t, wantSummary.String(), wire["summary"], source.ID)
	assert.Equal(t, wantFilter.String(), wire["filter"], source.ID)
	valueOptions, ok := wire["value_options"].(map[string]any)
	require.True(t, ok, "column %q value_options type = %T", source.ID, wire["value_options"])
	assert.Equal(t, wantTransform.String(), valueOptions["transform"], source.ID)
	assert.Equal(t, wantDecimals, valueOptions["decimal_points"], source.ID)
}

func rawRequest(method string, info bool, payload []byte) funcapi.RawMethodRequest {
	return funcapi.RawMethodRequest{
		Method: method, Info: info, Payload: payload, Timeout: time.Second,
	}
}

func newTestHandler(t *testing.T, deps Deps) funcapi.RawMethodHandler {
	t.Helper()
	handler, ok := Handler(deps)(nil).(funcapi.RawMethodHandler)
	require.True(t, ok)
	return handler
}

func requireRawInventoryPayload(
	t *testing.T,
	response *funcapi.FunctionResponse,
) map[string]any {
	t.Helper()
	require.NotNil(t, response)
	require.NotNil(t, response.RawResponse, "status=%d message=%q", response.Status, response.Message)
	return response.RawResponse
}

func requireInventoryColumns(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()
	columns, ok := payload["columns"].(map[string]any)
	require.True(t, ok, "columns type = %T", payload["columns"])
	return columns
}

func requireInventoryData(t *testing.T, payload map[string]any) [][]any {
	t.Helper()
	raw, ok := payload["data"].([]json.RawMessage)
	require.True(t, ok, "data type = %T", payload["data"])
	data := make([][]any, 0, len(raw))
	for _, row := range raw {
		var values []any
		require.NoError(t, json.Unmarshal(row, &values))
		data = append(data, values)
	}
	return data
}

type requiredParamWire struct {
	ID      string `json:"id"`
	Options []struct {
		ID              string `json:"id"`
		Name            string `json:"name"`
		DefaultSelected bool   `json:"defaultSelected"`
	} `json:"options"`
}

func decodeRequiredParams(t *testing.T, raw json.RawMessage) []requiredParamWire {
	t.Helper()
	var params []requiredParamWire
	require.NoError(t, json.Unmarshal(raw, &params))
	return params
}

func selectionPayload(job, host, kind string) []byte {
	payload, err := json.Marshal(map[string]any{"selections": map[string][]string{
		"__job": {job}, "host": {host}, "resource_kind": {kind},
	}})
	if err != nil {
		panic(fmt.Sprintf("encode test selection: %v", err))
	}
	return payload
}

func testInventorySelectors(t *testing.T, job, host, kind string) inventorySelectors {
	t.Helper()
	required, fits, err := encodeInventoryRequiredParams(
		context.Background(),
		maxInventoryResponseBytes,
		[]string{job},
		[]selectorOption{{id: host, name: host}},
		[]string{kind},
	)
	require.NoError(t, err)
	require.True(t, fits)
	return inventorySelectors{
		jobs: []string{job}, hosts: []string{host}, resourceKinds: []string{kind}, requiredParams: required,
	}
}

func columnIndex(t *testing.T, columns map[string]any, id string) int {
	t.Helper()
	column, ok := columns[id].(map[string]any)
	require.True(t, ok, "column %q wire type = %T", id, columns[id])
	index, ok := column["index"].(int)
	require.True(t, ok, "column %q index type = %T", id, column["index"])
	return index
}

func inventoryColumnByID(id string) (inventoryColumn, bool) {
	for _, column := range inventoryColumnRegistry {
		if column.ID == id {
			return column, true
		}
	}
	return inventoryColumn{}, false
}

func validateFunctionPayload(t *testing.T, payload any) {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Dir(filename)
	var schemaPath string
	for {
		candidate := filepath.Join(root, "src", "plugins.d", "FUNCTION_UI_SCHEMA.json")
		if _, err := os.Stat(candidate); err == nil {
			schemaPath = candidate
			break
		}
		parent := filepath.Dir(root)
		require.NotEqual(t, parent, root, "repository root not found from %s", filename)
		root = parent
	}
	raw, err := os.ReadFile(schemaPath)
	require.NoError(t, err)
	var document any
	require.NoError(t, json.Unmarshal(raw, &document))
	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("schema.json", document))
	schema, err := compiler.Compile("schema.json")
	require.NoError(t, err)
	encoded, err := json.Marshal(payload)
	require.NoError(t, err)
	var normalized any
	require.NoError(t, json.Unmarshal(encoded, &normalized))
	require.NoError(t, schema.Validate(normalized))
}

func TestInventoryColumnRegistryHasNoDuplicateIDsOrIndexes(t *testing.T) {
	ids := make(map[string]struct{}, len(inventoryColumnRegistry))
	indexes := make(map[int]struct{}, len(inventoryColumnRegistry))
	for _, column := range inventoryColumnRegistry {
		if _, ok := ids[column.ID]; ok {
			t.Fatalf("duplicate inventory column ID %q", column.ID)
		}
		ids[column.ID] = struct{}{}
		sourceIndex := -1
		for _, source := range registry.MustCompile().Columns {
			if source.ID == column.ID {
				sourceIndex = source.Order
				break
			}
		}
		require.NotEqual(t, -1, sourceIndex, column.ID)
		if _, ok := indexes[sourceIndex]; ok {
			t.Fatalf("duplicate inventory column index %d", sourceIndex)
		}
		indexes[sourceIndex] = struct{}{}
	}
	assert.Equal(t, len(ids), len(indexes))
}
