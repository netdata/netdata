// SPDX-License-Identifier: GPL-3.0-or-later

package redfish_logs

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/redfishfunc"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfishruntime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type functionTestDeps struct {
	runtime *redfishruntime.Runtime
}

func (d functionTestDeps) VisitInventoryCatalog(
	context.Context,
	int,
	func(string) bool,
	func(string, string) bool,
	func(string) bool,
) bool {
	return true
}
func (d functionTestDeps) VisitInventorySlice(
	context.Context,
	string,
	string,
	string,
	func(map[string]any) bool,
) (int, bool) {
	return 0, false
}
func (d functionTestDeps) AnyBackendAvailable() bool {
	return d.runtime.AnyBackendAvailable()
}
func (d functionTestDeps) LogRoots() map[string]string { return d.runtime.LogRoots() }

func TestLogsFunctionQueriesAndSlicesRegisteredBackends(t *testing.T) {
	root := t.TempDir()
	runtime := redfishruntime.New()
	appendFunctionTestEntry(t, runtime, root, "default", "default log entry", "warning")
	appendFunctionTestEntry(t, runtime, root, "isolated", "isolated log entry", "critical")

	handler, ok := redfishfunc.Handler(functionTestDeps{runtime: runtime})(nil).(funcapi.RawMethodHandler)
	require.True(t, ok)

	info := handler.HandleRaw(context.Background(), functionRawRequest(true, nil))
	require.NotNil(t, info.RawResponse)
	assert.Equal(t, 200, info.RawResponse["status"])
	assert.Equal(t, true, info.RawResponse["has_history"])
	rawInfo, err := json.Marshal(info.RawResponse)
	require.NoError(t, err)
	assert.Contains(t, string(rawInfo), "__logs_sources")
	assert.Contains(t, string(rawInfo), "default")
	assert.Contains(t, string(rawInfo), "isolated")

	all := handler.HandleRaw(context.Background(), functionRawRequest(false, []byte(`{
  "last": 10,
  "direction": "backward",
  "facets": []
}`)))
	require.NotNil(t, all.RawResponse)
	assert.Equal(t, 200, all.RawResponse["status"])
	rawAll, err := json.Marshal(all.RawResponse)
	require.NoError(t, err)
	assert.Contains(t, string(rawAll), "default log entry")
	assert.Contains(t, string(rawAll), "isolated log entry")
	assert.Contains(t, string(rawAll), "REDFISH_SEVERITY")
	columns, ok := all.RawResponse["columns"].(map[string]any)
	require.True(t, ok, "response columns type = %T", all.RawResponse["columns"])
	timestamp, ok := columns["timestamp"].(map[string]any)
	require.True(t, ok, "timestamp column type = %T", columns["timestamp"])
	assert.Equal(t, "none", timestamp["filter"])
	for key, value := range columns {
		column, ok := value.(map[string]any)
		require.True(t, ok, "column %s type = %T", key, value)
		assert.NotEqual(t, "range", column["filter"], "column %s", key)
	}

	selected := handler.HandleRaw(context.Background(), functionRawRequest(false, []byte(`{
  "last": 10,
  "direction": "backward",
  "selections": {
    "__logs_sources": ["default"]
  }
}`)))
	require.NotNil(t, selected.RawResponse)
	assert.Equal(t, 200, selected.RawResponse["status"])
	rawSelected, err := json.Marshal(selected.RawResponse)
	require.NoError(t, err)
	assert.Contains(t, string(rawSelected), "default log entry")
	assert.NotContains(t, string(rawSelected), "isolated log entry")

	unknown := handler.HandleRaw(context.Background(), functionRawRequest(false, []byte(`{
  "selections": {
    "__logs_sources": ["missing"]
  }
}`)))
	assert.Equal(t, 400, unknown.Status)
	assert.Contains(t, unknown.Message, `unknown backend "missing"`)
}

func TestLogsFunctionUnavailableWithoutReadyBackend(t *testing.T) {
	runtime := redfishruntime.New()
	handler, ok := redfishfunc.Handler(functionTestDeps{runtime: runtime})(nil).(funcapi.RawMethodHandler)
	require.True(t, ok)

	response := handler.HandleRaw(context.Background(), functionRawRequest(false, []byte(`{}`)))
	assert.Equal(t, 503, response.Status)
	assert.Contains(t, response.Message, "no ready Redfish log backend")
}

func appendFunctionTestEntry(
	t *testing.T,
	runtime *redfishruntime.Runtime,
	root, name, message, severity string,
) {
	t.Helper()
	key, _ := backendDigest(name)
	dir := filepath.Join(root, key)
	require.NoError(t, ensurePrivateDirectory(dir))
	backend, err := newJournalBackend(dir, 20<<20, fixedJournalHost{})
	require.NoError(t, err)
	registration, err := runtime.RegisterBackend(name, key, dir, backend)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, registration.Close(context.Background()))
		require.NoError(t, backend.Close())
	})

	recordKey := stableTestRecordKey(name)
	result, err := backend.Append(context.Background(), []redfishruntime.JournalEntry{{
		RealtimeUsec:       uint64(time.Now().UnixMicro()),
		SourceRealtimeUsec: uint64(time.Now().Add(-time.Second).UnixMicro()),
		Fields: map[string]string{
			"MESSAGE":               message,
			"PRIORITY":              "4",
			"SYSLOG_IDENTIFIER":     "redfish",
			"ND_LOG_SOURCE":         "redfish",
			"REDFISH_RECORD_KEY":    recordKey,
			"REDFISH_BACKEND":       name,
			"REDFISH_ENDPOINT_JOB":  "endpoint-" + name,
			"REDFISH_HOST_NAME":     "host-" + name,
			"REDFISH_SEVERITY":      severity,
			"REDFISH_MESSAGE_ID":    "Base.1.0.Test",
			"REDFISH_ENTRY_TYPE":    "Event",
			"REDFISH_RESOURCE_KIND": "system",
			"REDFISH_ENTRY_URI":     "/redfish/v1/Managers/1/LogServices/1/Entries/1",
			"REDFISH_ENTRY_ID":      "1",
			"REDFISH_JSON":          `{"test":true}`,
		},
	}})
	require.NoError(t, err)
	require.Equal(t, 1, result.Committed)
}

func functionRawRequest(info bool, payload []byte) funcapi.RawMethodRequest {
	return funcapi.RawMethodRequest{
		Method: "logs", Info: info, Payload: payload, Timeout: 3 * time.Second,
	}
}

func stableTestRecordKey(value string) string {
	key, full := backendDigest("test-record-" + value)
	_ = key
	return full
}
