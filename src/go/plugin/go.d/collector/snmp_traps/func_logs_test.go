// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/snmptrapsfunc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSNMPTrapsMethodsExposeLogsOnly(t *testing.T) {
	methods := snmpTrapsMethods()
	require.Len(t, methods, 1)

	byID := make(map[string]funcapi.FunctionConfig, len(methods))
	for _, method := range methods {
		byID[method.ID] = method
	}

	logs := byID[snmpTrapsLogsMethodID]
	assert.Equal(t, snmpTrapsFunctionName, logs.FunctionName)
	assert.Equal(t, "SNMP Trap Logs", logs.Name)
	assert.Equal(t, "logs", logs.Tags)
	assert.Equal(t, "logs", logs.ResponseType)
	assert.True(t, logs.RawRequest)
	assert.True(t, logs.RequireCloud)
	assert.NotNil(t, logs.Available)
}

func TestSNMPTrapsJournalFunctionUsesPublicFunctionName(t *testing.T) {
	fn := snmptrapsfunc.NewJournalFunction()
	assert.Equal(t, snmpTrapsFunctionName, fn.Config.FunctionName)
	assert.Equal(t, "Trap Jobs", fn.Config.SourceSelectorName)
	assert.Equal(t, "Select the trap jobs to query", fn.Config.SourceSelectorHelp)
	assert.Equal(t, "TRAP_NAME", fn.Config.DefaultHistogram)
	assert.Contains(t, fn.Config.DefaultViewKeys, "TRAP_REVERSE_DNS")
}

func TestSNMPTrapsLogsFunctionInfoAndQuery(t *testing.T) {
	root := t.TempDir()
	writeTestTrapJournal(t, root, "local", "sdk local trap entry", "security")
	writeTestTrapJournal(t, root, "remote", "sdk remote trap entry", "availability")

	handler := snmptrapsfunc.NewHandler(root)

	info := handler.HandleRaw(context.Background(), funcapiRawRequest("logs", true, nil))
	require.NotNil(t, info)
	require.NotNil(t, info.RawResponse)
	assert.Equal(t, 200, info.RawResponse["status"])
	assert.Equal(t, true, info.RawResponse["has_history"])

	rawInfo, err := json.Marshal(info.RawResponse)
	require.NoError(t, err)
	assert.Contains(t, string(rawInfo), "__logs_sources")
	assert.Contains(t, string(rawInfo), "all")
	assert.Contains(t, string(rawInfo), "local")
	assert.Contains(t, string(rawInfo), "remote")
	assertLogsSourceSelectorMetadata(t, info.RawResponse)

	defaults := handler.HandleRaw(context.Background(), funcapiRawRequest("logs", false, []byte(`{
  "last": 10,
  "direction": "backward",
  "facets": []
}`)))
	require.NotNil(t, defaults)
	require.NotNil(t, defaults.RawResponse)
	assert.Equal(t, 200, defaults.RawResponse["status"])
	assertResponseFacetIDs(t, defaults.RawResponse, snmptrapsfunc.DefaultLogFacets())
	assertResponseColumnVisible(t, defaults.RawResponse, "TRAP_NAME")

	query := handler.HandleRaw(context.Background(), funcapiRawRequest("logs", false, []byte(`{
  "last": 10,
  "direction": "backward",
  "facets": ["TRAP_CATEGORY", "TRAP_SEVERITY"]
}`)))
	require.NotNil(t, query)
	require.NotNil(t, query.RawResponse)
	assert.Equal(t, 200, query.RawResponse["status"])

	raw, err := json.Marshal(query.RawResponse)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "sdk local trap entry")
	assert.Contains(t, string(raw), "sdk remote trap entry")
	assert.Contains(t, string(raw), "TRAP_CATEGORY")
	assert.Contains(t, string(raw), "security")

	filtered := handler.HandleRaw(context.Background(), funcapiRawRequest("logs", false, []byte(`{
  "last": 10,
  "direction": "backward",
  "selections": {
    "__logs_sources": ["local"]
  }
}`)))
	require.NotNil(t, filtered)
	require.NotNil(t, filtered.RawResponse)
	assert.Equal(t, 200, filtered.RawResponse["status"])

	rawFiltered, err := json.Marshal(filtered.RawResponse)
	require.NoError(t, err)
	assert.Contains(t, string(rawFiltered), "sdk local trap entry")
	assert.NotContains(t, string(rawFiltered), "sdk remote trap entry")
}

func TestSNMPTrapsLogsFunctionDefaultsWithHighVarbindRow(t *testing.T) {
	root := t.TempDir()
	writeHighVarbindTrapJournal(t, root, "local", 250)

	handler := snmptrapsfunc.NewHandler(root)
	resp := handler.HandleRaw(context.Background(), funcapiRawRequest("logs", false, []byte(`{
  "last": 10,
  "direction": "backward",
  "facets": []
}`)))
	require.NotNil(t, resp)
	require.NotNil(t, resp.RawResponse)
	assert.Equal(t, 200, resp.RawResponse["status"])
	assertResponseFacetIDs(t, resp.RawResponse, snmptrapsfunc.DefaultLogFacets())
	assertResponseColumnVisible(t, resp.RawResponse, "TRAP_NAME")
	assertResponseHistogramID(t, resp.RawResponse, "TRAP_NAME")
	assertResponseHistogramTotal(t, resp.RawResponse, "IF-MIB::linkDown", 1)
}

func TestSNMPTrapsLogsFunctionUnavailableWithoutJournal(t *testing.T) {
	handler := snmptrapsfunc.NewHandler(filepath.Join(t.TempDir(), "missing"))

	resp := handler.HandleRaw(context.Background(), funcapiRawRequest("logs", false, []byte(`{}`)))
	require.NotNil(t, resp)
	assert.Equal(t, 503, resp.Status)
	assert.Contains(t, resp.Message, "direct journal output has no sources")
}

func TestSNMPTrapsLogsFunctionRejectsUnknownMethod(t *testing.T) {
	handler := snmptrapsfunc.NewHandler(t.TempDir())

	resp := handler.HandleRaw(context.Background(), funcapiRawRequest("other", false, []byte(`{}`)))
	require.NotNil(t, resp)
	assert.Equal(t, 404, resp.Status)
}

func TestSNMPTrapsLogsFunctionRejectsInvalidJSON(t *testing.T) {
	handler := snmptrapsfunc.NewHandler(t.TempDir())

	resp := handler.HandleRaw(context.Background(), funcapiRawRequest("logs", false, []byte(`{`)))
	require.NotNil(t, resp)
	assert.Equal(t, 400, resp.Status)
	assert.Contains(t, resp.Message, "SNMP trap logs query failed")
}

func writeTestTrapJournal(t *testing.T, root, jobName, message, category string) string {
	t.Helper()

	w, err := newTestJournalWriter(filepath.Join(root, jobName), RetentionConfig{}.makeJournalConfig())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, w.Close()) })

	now := time.Now().UnixMicro()
	require.NoError(t, w.WriteEntry([]JournalField{
		{Name: "MESSAGE", Value: []byte(message)},
		{Name: "PRIORITY", Value: []byte("4")},
		{Name: "SYSLOG_IDENTIFIER", Value: []byte(jobName)},
		{Name: "TRAP_JOB", Value: []byte(jobName)},
		{Name: "_HOSTNAME", Value: []byte(jobName + "-host")},
		{Name: "ND_LOG_SOURCE", Value: []byte("snmp-trap")},
		{Name: "TRAP_REPORT_TYPE", Value: []byte("trap")},
		{Name: "TRAP_OID", Value: []byte("1.3.6.1.6.3.1.1.5.1")},
		{Name: "TRAP_NAME", Value: []byte("SNMPv2-MIB::coldStart")},
		{Name: "TRAP_CATEGORY", Value: []byte(category)},
		{Name: "TRAP_SEVERITY", Value: []byte("warning")},
		{Name: "TRAP_SOURCE_IP", Value: []byte("192.0.2.1")},
		{Name: "TRAP_DEVICE_VENDOR", Value: []byte("test-vendor")},
		{Name: "TRAP_JSON", Value: []byte(`{"trap_oid":"1.3.6.1.6.3.1.1.5.1"}`)},
	}, now, 1000))
	require.NoError(t, w.Sync())
	return w.JournalDirectory()
}

func writeHighVarbindTrapJournal(t *testing.T, root, jobName string, varbinds int) string {
	t.Helper()

	w, err := newTestJournalWriter(filepath.Join(root, jobName), RetentionConfig{}.makeJournalConfig())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, w.Close()) })

	fields := []JournalField{
		{Name: "MESSAGE", Value: []byte("high varbind trap entry")},
		{Name: "PRIORITY", Value: []byte("4")},
		{Name: "SYSLOG_IDENTIFIER", Value: []byte(jobName)},
		{Name: "TRAP_JOB", Value: []byte(jobName)},
		{Name: "_HOSTNAME", Value: []byte(jobName + "-host")},
		{Name: "ND_LOG_SOURCE", Value: []byte("snmp-trap")},
		{Name: "TRAP_REPORT_TYPE", Value: []byte("trap")},
		{Name: "TRAP_OID", Value: []byte("1.3.6.1.6.3.1.1.5.3")},
		{Name: "TRAP_NAME", Value: []byte("IF-MIB::linkDown")},
		{Name: "TRAP_CATEGORY", Value: []byte("state_change")},
		{Name: "TRAP_SEVERITY", Value: []byte("warning")},
		{Name: "TRAP_SOURCE_IP", Value: []byte("192.0.2.1")},
		{Name: "TRAP_DEVICE_VENDOR", Value: []byte("test-vendor")},
	}
	for i := range varbinds {
		fields = append(fields, JournalField{
			Name:  fmt.Sprintf("TRAP_VAR_SYNTHETIC_%03d", i),
			Value: fmt.Appendf(nil, "enum-%03d", i),
		})
	}
	fields = append(fields, JournalField{
		Name:  "TRAP_JSON",
		Value: []byte(`{"trap_oid":"1.3.6.1.6.3.1.1.5.3"}`),
	})

	now := time.Now().UnixMicro()
	require.NoError(t, w.WriteEntry(fields, now, 1000))
	require.NoError(t, w.Sync())
	return w.JournalDirectory()
}

func funcapiRawRequest(method string, info bool, payload []byte) funcapi.RawMethodRequest {
	return funcapi.RawMethodRequest{
		Method:  method,
		Info:    info,
		Payload: payload,
		Timeout: time.Second,
	}
}

func assertResponseHistogramID(t *testing.T, response map[string]any, want string) {
	t.Helper()
	histogram, ok := response["histogram"].(map[string]any)
	require.True(t, ok, "response histogram type = %T", response["histogram"])
	assert.Equal(t, want, histogram["id"])
}

func assertResponseHistogramTotal(t *testing.T, response map[string]any, dimension string, want uint64) {
	t.Helper()
	histogram, ok := response["histogram"].(map[string]any)
	require.True(t, ok, "response histogram type = %T", response["histogram"])
	chart, ok := histogram["chart"].(map[string]any)
	require.True(t, ok, "histogram chart type = %T", histogram["chart"])
	result, ok := chart["result"].(map[string]any)
	require.True(t, ok, "histogram result type = %T", chart["result"])
	view, ok := chart["view"].(map[string]any)
	require.True(t, ok, "histogram view type = %T", chart["view"])
	dimensions, ok := view["dimensions"].(map[string]any)
	require.True(t, ok, "histogram dimensions type = %T", view["dimensions"])
	names, ok := dimensions["names"].([]any)
	require.True(t, ok, "histogram dimension names type = %T", dimensions["names"])

	dimensionIndex := -1
	for index, nameAny := range names {
		if nameAny == dimension {
			dimensionIndex = index
			break
		}
	}
	require.NotEqual(t, -1, dimensionIndex, "missing histogram dimension %s in %#v", dimension, names)

	data, ok := result["data"].([]any)
	require.True(t, ok, "histogram data type = %T", result["data"])
	var total uint64
	for _, pointAny := range data {
		point, ok := pointAny.([]any)
		require.True(t, ok, "histogram point type = %T", pointAny)
		cellIndex := dimensionIndex + 1
		require.Less(t, cellIndex, len(point), "histogram point has too few cells")
		cell, ok := point[cellIndex].([]any)
		require.True(t, ok, "histogram cell type = %T", point[cellIndex])
		require.NotEmpty(t, cell)
		total += responseUint64(t, cell[0])
	}
	assert.Equal(t, want, total)
}

func responseUint64(t *testing.T, value any) uint64 {
	t.Helper()
	switch v := value.(type) {
	case uint64:
		return v
	case uint:
		return uint64(v)
	case int:
		require.GreaterOrEqual(t, v, 0)
		return uint64(v)
	case int64:
		require.GreaterOrEqual(t, v, int64(0))
		return uint64(v)
	case float64:
		require.GreaterOrEqual(t, v, float64(0))
		return uint64(v)
	default:
		require.Failf(t, "unexpected numeric value type", "value %v has type %T", value, value)
		return 0
	}
}

func assertResponseFacetIDs(t *testing.T, response map[string]any, want []string) {
	t.Helper()
	facets, ok := response["facets"].([]any)
	require.True(t, ok, "response facets type = %T", response["facets"])
	got := make(map[string]bool, len(facets))
	for _, facetAny := range facets {
		facet, ok := facetAny.(map[string]any)
		require.True(t, ok, "facet type = %T", facetAny)
		id, ok := facet["id"].(string)
		require.True(t, ok, "facet id type = %T", facet["id"])
		got[id] = true
	}
	for _, id := range want {
		assert.True(t, got[id], "expected default facet %s in %#v", id, got)
	}
}

func assertResponseColumnVisible(t *testing.T, response map[string]any, key string) {
	t.Helper()
	columns, ok := response["columns"].(map[string]any)
	require.True(t, ok, "response columns type = %T", response["columns"])
	column, ok := columns[key].(map[string]any)
	require.True(t, ok, "missing column %s in %#v", key, columns)
	assert.Equal(t, true, column["visible"])
}

func assertLogsSourceSelectorMetadata(t *testing.T, response map[string]any) {
	t.Helper()
	params, ok := response["required_params"].([]any)
	require.True(t, ok, "required_params type = %T", response["required_params"])
	for _, paramAny := range params {
		param, ok := paramAny.(map[string]any)
		require.True(t, ok, "required param type = %T", paramAny)
		if param["id"] != "__logs_sources" {
			continue
		}
		assert.Equal(t, "Trap Jobs", param["name"])
		assert.Equal(t, "Select the trap jobs to query", param["help"])
		return
	}
	require.Fail(t, "required_params missing __logs_sources", "%#v", params)
}
