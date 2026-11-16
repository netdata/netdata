// SPDX-License-Identifier: GPL-3.0-or-later

package redfishfunc

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/acquisition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type logDeps struct {
	result           acquisition.LogResult
	err              error
	reads, discovery int
	selected         string
}

func (*logDeps) CurrentSnapshot() *Snapshot { return nil }
func (d *logDeps) Logs() LogReader          { return d }
func (d *logDeps) Services(context.Context) ([]acquisition.LogService, error) {
	d.discovery++
	return d.result.Services, d.err
}
func (d *logDeps) Entries(_ context.Context, service string) (acquisition.LogResult, error) {
	d.reads++
	d.selected = service
	return d.result, d.err
}

func TestLogFunctionTimeAndMissingValues(t *testing.T) {
	deps := &logDeps{
		result: acquisition.LogResult{
			Services: []acquisition.LogService{{URI: "/log", Name: "Events"}},
			Entries: []acquisition.LogEntry{
				{ID: "undated", Message: "Fan unknown", Severity: "Vendor", EventTimestamp: "invalid", Created: "bad"},
				{
					ID:             "fallback",
					Message:        "Fan failed",
					Severity:       "Warning",
					EventTimestamp: "invalid",
					Created:        "2023-11-14T22:13:21Z",
				},
				{
					ID:             "event",
					Message:        "Fan normal",
					Severity:       "OK",
					EventTimestamp: "2023-11-14T22:13:20Z",
					Created:        "2023-11-14T22:13:23Z",
				},
				{ID: "same time", Message: "Fan event", Severity: "Critical", EventTimestamp: "2023-11-14T22:13:20Z"},
				{ID: "outside", EventTimestamp: "2023-11-14T22:13:19Z", Severity: "OK"},
			},
		},
	}
	h := NewRouter(deps).(funcapi.RawMethodHandler)
	info := h.HandleRaw(t.Context(), funcapi.RawMethodRequest{
		Method: LogsMethod,
		Info:   true,
	})
	require.Equal(t, 200, info.Status)
	assert.Equal(t, 1, deps.discovery)
	assert.Zero(t, deps.reads)
	assert.Equal(t, "/log", info.RequiredParams[0].Options[0].ID)
	call := func(payload string) *funcapi.FunctionResponse {
		return h.HandleRaw(t.Context(), funcapi.RawMethodRequest{
			Method:  LogsMethod,
			Payload: []byte(payload),
		})
	}
	response := call(
		`{"after":1700000000,"before":1700000001,"last":1,"selections":{"service":["/log"],"severity":["all"]}}`,
	)
	require.Equal(t, 200, response.Status)
	validateTableSchema(t, response)
	rows := response.Data.([][]any)
	require.Len(t, rows, 4, "do not invent unique times or honor an unadvertised last cap")
	col := func(i int, name string) any { return rows[i][response.Columns[name].(map[string]any)["index"].(int)] }
	assert.Equal(t, "fallback", col(0, "Entry ID"))
	assert.Equal(t, "Created", col(0, "Time basis"))
	assert.Equal(t, int64(1700000001000), col(0, "Time"))
	assert.Equal(t, "event", col(1, "Entry ID"))
	assert.Equal(t, "EventTimestamp", col(1, "Time basis"))
	assert.Equal(t, "undated", col(3, "Entry ID"))
	assert.Nil(t, col(3, "Time"))
	assert.Equal(t, "Unknown", col(3, "Severity"))
	assert.Equal(t, "Vendor", col(3, "Reported severity"))
	assert.Contains(t, response.Help, "1 entries have unavailable time")
	assert.Contains(t, response.Help, "time range cannot filter")
	assert.Equal(
		t,
		"Unavailable",
		response.Columns["Time"].(map[string]any)["value_options"].(map[string]any)["default_value"],
	)
	require.Len(t, response.RequiredParams[0].Options, 1, "data preserves service choices")

	response = call(`{"selections":{"service":["/log"],"severity":["Warning"]},"query":"*failed*"}`)
	require.Equal(t, 200, response.Status)
	require.Len(t, response.Data.([][]any), 1)
	response = call(`{"service":"/log","query":"*FAILED*"}`)
	require.Equal(t, 200, response.Status)
	assert.Empty(t, response.Data.([][]any), "search is case-sensitive")
	response = call(`{"service":"/log","query":"!*failed* *Fan*"}`)
	require.Equal(t, 200, response.Status)
	assert.Len(t, response.Data.([][]any), 3)
}

func TestLogQueryValidationAndTimeParsing(t *testing.T) {
	now := time.Unix(1700000000, 0)
	for name, payload := range map[string]string{
		"missing service": `{}`, "invalid JSON": `[`, "multiple services": `{"service":["a","b"]}`,
		"bad severity": `{"service":"a","severity":"Error"}`, "invalid time": `{"service":"a","after":"bad"}`,
		"fractional time": `{"service":"a","after":1.5}`, "null time": `{"service":"a","after":null}`,
		"reversed range": `{"service":"a","after":10,"before":9}`, "invalid pattern": `{"service":"a","query":"["}`,
		"multiple query strings": `{"service":"a","query":["a","b"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseLogQuery(funcapi.RawMethodRequest{
				Payload: []byte(payload),
			}, now)
			require.Error(t, err)
		})
	}
	q, err := parseLogQuery(
		funcapi.RawMethodRequest{
			Args:    []string{"service:old", "after:-3600", "before:0"},
			Payload: []byte(`{"selections":{"service":["selected"]}}`),
		},
		now,
	)
	require.NoError(t, err)
	assert.Equal(t, "old", q.service)
	assert.Equal(t, now.Unix()-3600, *q.after)
	assert.Equal(t, now.Unix(), *q.before)
	q, err = parseLogQuery(funcapi.RawMethodRequest{
		Payload: []byte(`{"service":"a","after":0,"before":1}`),
	}, now)
	require.NoError(t, err)
	assert.Zero(t, *q.after)
	ts, basis := logTime(acquisition.LogEntry{
		EventTimestamp: "1970-01-01T00:00:00Z",
	})
	assert.Equal(t, int64(0), ts.Unix())
	assert.Equal(t, "EventTimestamp", basis)
}

func TestLogFunctionFailureAndEmptyDiscovery(t *testing.T) {
	deps := &logDeps{}
	h := NewRouter(deps).(funcapi.RawMethodHandler)
	response := h.HandleRaw(t.Context(), funcapi.RawMethodRequest{
		Method: LogsMethod,
		Info:   true,
	})
	require.Equal(t, 200, response.Status)
	assert.Empty(t, response.RequiredParams[0].Options)
	assert.Contains(t, response.Help, "No linked log services")
	for _, tc := range []struct {
		err    error
		status int
	}{{context.Canceled, 499}, {context.DeadlineExceeded, 504}, {acquisition.ErrLogServiceUnavailable, 400}, {acquisition.ErrLogEntriesUnsupported, 400}, {errors.New("source failed"), 503}} {
		deps.err = tc.err
		response = h.HandleRaw(
			t.Context(),
			funcapi.RawMethodRequest{
				Method:  LogsMethod,
				Payload: json.RawMessage(`{"service":"a"}`),
			},
		)
		assert.Equal(t, tc.status, response.Status)
		assert.Nil(t, response.Data)
	}
	deps.err = nil
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	response = h.HandleRaw(ctx, funcapi.RawMethodRequest{
		Method:  LogsMethod,
		Payload: []byte(`{"service":"a"}`),
	})
	assert.Equal(t, 499, response.Status)
}

func TestLogQueryUsesFrameworkParameterPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name              string
		args              []string
		payload           string
		service, severity string
	}{
		{"arguments win", []string{"service:/argument", "severity:Critical"}, `{"service":"/payload","severity":"OK","selections":{"service":["/selection"],"severity":["Warning"]}}`, "/argument", "Critical"},
		{"selections win over payload", nil, `{"service":"/payload","severity":"OK","selections":{"service":["/selection"],"severity":["Warning"]}}`, "/selection", "Warning"},
		{"payload fallback", nil, `{"service":"/payload","severity":"OK"}`, "/payload", "OK"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q, err := parseLogQuery(funcapi.RawMethodRequest{
				Args:    tc.args,
				Payload: []byte(tc.payload),
			}, time.Now())
			require.NoError(t, err)
			assert.Equal(t, tc.service, q.service)
			assert.Equal(t, tc.severity, q.severity)
		})
	}
}
