// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSNMPTrapsMethodsExposeReloadAndLogs(t *testing.T) {
	methods := snmpTrapsMethods()
	require.Len(t, methods, 2)

	byID := make(map[string]funcapi.MethodConfig, len(methods))
	for _, method := range methods {
		byID[method.ID] = method
	}

	assert.Contains(t, byID, reloadProfilesMethodID)
	logs := byID[snmpTrapsLogsMethodID]
	assert.Equal(t, "SNMP Trap Logs", logs.Name)
	assert.Equal(t, "logs", logs.Tags)
	assert.Equal(t, "logs", logs.ResponseType)
	assert.True(t, logs.RawRequest)
	assert.True(t, logs.RequireCloud)
	assert.True(t, logs.AgentWide)
}

func TestSNMPTrapsLogsFunctionInfoAndQuery(t *testing.T) {
	root := t.TempDir()
	writeTestTrapJournal(t, root, "local", "sdk local trap entry", "security")
	writeTestTrapJournal(t, root, "remote", "sdk remote trap entry", "availability")

	handler := newSNMPTrapsFunctionHandler(&Collector{})
	handler.journalRoot = root

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

func TestSNMPTrapsLogsFunctionUnavailableWithoutJournal(t *testing.T) {
	handler := newSNMPTrapsFunctionHandler(&Collector{})
	handler.journalRoot = filepath.Join(t.TempDir(), "missing")

	resp := handler.HandleRaw(context.Background(), funcapiRawRequest("logs", false, []byte(`{}`)))
	require.NotNil(t, resp)
	assert.Equal(t, 503, resp.Status)
	assert.Contains(t, resp.Message, "direct journal output has no sources")
}

func TestSNMPTrapsLogsFunctionRejectsUnknownMethod(t *testing.T) {
	handler := newSNMPTrapsFunctionHandler(&Collector{})
	handler.journalRoot = t.TempDir()

	resp := handler.HandleRaw(context.Background(), funcapiRawRequest("other", false, []byte(`{}`)))
	require.NotNil(t, resp)
	assert.Equal(t, 404, resp.Status)
}

func TestSNMPTrapsLogsFunctionRejectsInvalidJSON(t *testing.T) {
	handler := newSNMPTrapsFunctionHandler(&Collector{})
	handler.journalRoot = t.TempDir()

	resp := handler.HandleRaw(context.Background(), funcapiRawRequest("logs", false, []byte(`{`)))
	require.NotNil(t, resp)
	assert.Equal(t, 400, resp.Status)
	assert.Contains(t, resp.Message, "SNMP trap logs query failed")
}

func writeTestTrapJournal(t *testing.T, root, jobName, message, category string) string {
	t.Helper()

	w, err := NewJournalWriter(filepath.Join(root, jobName), RetentionConfig{}.makeJournalConfig())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, w.Close()) })

	now := time.Now().UnixMicro()
	require.NoError(t, w.WriteEntry([]JournalField{
		{Name: "MESSAGE", Value: []byte(message)},
		{Name: "PRIORITY", Value: []byte("4")},
		{Name: "SYSLOG_IDENTIFIER", Value: []byte(jobName)},
		{Name: "ND_LOG_SOURCE", Value: []byte("snmp-trap")},
		{Name: "TRAP_REPORT_TYPE", Value: []byte("trap")},
		{Name: "TRAP_OID", Value: []byte("1.3.6.1.6.3.1.1.5.1")},
		{Name: "TRAP_NAME", Value: []byte("SNMPv2-MIB::coldStart")},
		{Name: "TRAP_CATEGORY", Value: []byte(category)},
		{Name: "TRAP_SEVERITY", Value: []byte("warning")},
		{Name: "TRAP_SOURCE_IP", Value: []byte("192.0.2.1")},
		{Name: "TRAP_JSON", Value: []byte(`{"trap_oid":"1.3.6.1.6.3.1.1.5.1"}`)},
	}, now, monotonicUsec()))
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
