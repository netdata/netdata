// SPDX-License-Identifier: GPL-3.0-or-later

package alerta

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderMonitoringEvents(t *testing.T) {
	for name, test := range map[string]struct{ status, previous, severity string }{
		"warning": {"WARNING", "CLEAR", "warning"}, "critical": {"CRITICAL", "WARNING", "critical"}, "clear": {"CLEAR", "CRITICAL", "cleared"},
	} {
		t.Run(name, func(t *testing.T) {
			event := testutil.ExpectedEvent()
			event.Status, event.PreviousStatus = test.status, test.previous
			event.URL = "https://example.com/alert?id=1&view=chart#details"
			alerta, err := renderAlerta(Config{Environment: "Production"}, event)
			require.NoError(t, err)
			for provider, message := range map[string]any{"alerta": alerta} {
				t.Run(provider, func(t *testing.T) {
					fixture, err := os.ReadFile(filepath.Join("testdata", provider+"-full.json"))
					require.NoError(t, err)
					want := strings.ReplaceAll(string(fixture), "CLEAR → WARNING", test.previous+" → "+test.status)
					want = strings.ReplaceAll(want, "WARNING:", test.status+":")
					want = strings.ReplaceAll(want, `"warning"`, `"`+test.severity+`"`)
					want = strings.ReplaceAll(want, `\"status\":\"WARNING\"`, `\"status\":\"`+test.status+`\"`)
					want = strings.ReplaceAll(
						want,
						`\"previous_status\":\"CLEAR\"`,
						`\"previous_status\":\"`+test.previous+`\"`,
					)
					got, err := json.Marshal(message)
					require.NoError(t, err)
					assertMonitoringPayload(t, provider, want, string(got))
				})
			}
		})
	}
}

func assertMonitoringPayload(t *testing.T, provider, want, got string) {
	t.Helper()
	var wantObject, gotObject map[string]any
	require.NoError(t, json.Unmarshal([]byte(want), &wantObject))
	require.NoError(t, json.Unmarshal([]byte(got), &gotObject))

	wantRaw, ok := wantObject["rawData"].(string)
	require.True(t, ok)
	gotRaw, ok := gotObject["rawData"].(string)
	require.True(t, ok)
	assert.JSONEq(t, wantRaw, gotRaw)
	delete(wantObject, "rawData")
	delete(gotObject, "rawData")

	assert.Equal(t, wantObject, gotObject)
}

func TestMonitoringContentAndTargeting(t *testing.T) {
	for name, test := range map[string]struct{ chart, resource, alertEvent, environment string }{
		"minimal":          {resource: "節点", alertEvent: `alert "one"`, environment: "Production"},
		"regular chart":    {chart: "test.chart", resource: "節点", alertEvent: `test.chart.alert "one"`, environment: "Development"},
		"httpcheck":        {chart: "httpcheck.example", resource: "httpcheck.example", alertEvent: `alert "one"`, environment: "Custom 東京"},
		"httpcheck prefix": {chart: "httpcheck_status", resource: "httpcheck_status", alertEvent: `alert "one"`, environment: "Production"},
	} {
		t.Run(name, func(t *testing.T) {
			for status := range map[string]struct{}{"WARNING": {}, "CRITICAL": {}, "CLEAR": {}} {
				t.Run(status, func(t *testing.T) {
					event := notifyevent.Event{
						Version:    1,
						IncidentID: " ID:😀 ",
						Node:       "節点",
						Alert:      `alert "one"`,
						Chart:      test.chart,
						Summary:    "<b>a&b</b>\n😀",
						Status:     status,
						Timestamp:  time.Date(2026, 9, 15, 9, 0, 0, 123456789, time.FixedZone("offset", 3600)),
					}
					body := "<b>a&b</b>\n😀\nNode: 節点\nAlert: alert \"one\"\nStatus: " + status
					if test.chart != "" {
						body += "\nChart: " + test.chart
					}
					body += "\nTime: 2026-09-15T09:00:00+01:00"
					severity := strings.ToLower(status)
					if status == "CLEAR" {
						severity = "cleared"
					}
					got, err := renderAlerta(Config{Environment: test.environment}, event)
					require.NoError(t, err)
					assert.JSONEq(
						t,
						`{"version":1,"incident_id":" ID:😀 ","timestamp":"2026-09-15T09:00:00.123456789+01:00","node":"節点","alert":"alert \"one\"","summary":"<b>a&b</b>\n😀","status":"`+status+`","value":null,"previous_value":null`+monitoringChartJSON(
							test.chart,
						)+`}`,
						got.RawData,
					)
					got.RawData = ""
					assert.Equal(
						t,
						alertaEvent{Resource: test.resource, Event: test.alertEvent, Environment: test.environment,
							Severity: severity, Service: []string{"Netdata"}, Group: "Performance", Text: body, Tags: []string{"incident_id: ID:😀 "},
							Attributes: map[string]string{
								"name":    `alert "one"`,
								"chart":   test.chart,
								"context": "",
							}, Origin: "netdata/節点", Type: "netdataAlarm", CreateTime: "2026-09-15T08:00:00.123Z"},
						got,
					)

				})
			}
		})
	}
}

func monitoringChartJSON(chart string) string {
	if chart == "" {
		return ""
	}
	return `,"chart":"` + chart + `"`
}

func TestAlertaNavigationEscaping(t *testing.T) {
	for name, url := range map[string]string{
		"quotes": `https://example.com/?x="quoted"&y='value'`, "markup": "https://example.com/<value>?a=1&b=2",
	} {
		t.Run(name, func(t *testing.T) {
			event := testutil.ExpectedEvent()
			event.URL = url
			got, err := renderAlerta(Config{Environment: "Production"}, event)
			require.NoError(t, err)
			want := map[string]string{
				"quotes": `<a href="https://example.com/?x=&#34;quoted&#34;&amp;y=&#39;value&#39;">View Netdata</a>`,
				"markup": `<a href="https://example.com/&lt;value&gt;?a=1&amp;b=2">View Netdata</a>`,
			}
			assert.Equal(t, want[name], got.Attributes["moreInfo"])
		})
	}
}

const alertaTestAck = `{"status":"ok","id":"synthetic-alert"}`

func TestMonitoringAcknowledgments(t *testing.T) {
	for provider, p := range map[string]struct {
		read func(*http.Response) error
		ack  string
	}{
		"alerta": {readAlertaResponse, alertaTestAck},
	} {
		t.Run(provider, func(t *testing.T) {
			for name, test := range map[string]struct {
				status            int
				body, err         string
				unread, readError bool
			}{
				"created":        {status: 201, body: p.ack},
				"invalid JSON":   {status: 201, body: "synthetic-private-value", err: "invalid"},
				"empty":          {status: 201, err: "invalid"},
				"null":           {status: 201, body: "null", err: "acknowledge"},
				"array":          {status: 201, body: "[]", err: "invalid"},
				"missing fields": {status: 201, body: "{}", err: "acknowledge"},
				"second JSON":    {status: 201, body: p.ack + `{}`, err: "invalid"},
				"oversized":      {status: 201, body: p.ack + strings.Repeat(" ", 256*1024), err: "256 KiB"},
				"read failure":   {status: 201, readError: true, err: "transport failed"},
				"redirect":       {status: 307, err: "HTTP 307", unread: true},
				"unauthorized":   {status: 401, err: "HTTP 401", unread: true},
				"rate limit":     {status: 429, err: "HTTP 429", unread: true},
				"server error":   {status: 500, err: "HTTP 500", unread: true},
			} {
				t.Run(name, func(t *testing.T) {
					var reader io.Reader = strings.NewReader(test.body)
					failureReader := &monitoringFailReader{}
					if test.readError || test.unread {
						reader = failureReader
					}
					body := &testBody{Reader: reader}
					err := p.read(&http.Response{StatusCode: test.status, Body: body})
					assert.True(t, body.closed)
					if test.unread {
						assert.Zero(t, failureReader.reads)
					}
					if test.err == "" {
						require.NoError(t, err)
					} else {
						require.ErrorContains(t, err, test.err)
						assert.NotContains(t, err.Error(), "synthetic-private-value")
					}
				})
			}
		})
	}
	for name, test := range map[string]struct {
		status    int
		body, err string
	}{
		"dedup": {200, alertaTestAck, ""}, "suppressed": {202, "", "suppressed"}, "no content": {204, "", "HTTP 204"},
		"status error": {201, `{"status":"error","id":"synthetic-private-value"}`, "acknowledge"},
		"missing ID":   {201, `{"status":"ok"}`, "acknowledge"}, "blank ID": {201, `{"status":"ok","id":"  "}`, "acknowledge"},
	} {
		t.Run("alerta/"+name, func(t *testing.T) {
			body := &testBody{Reader: strings.NewReader(test.body)}
			err := readAlertaResponse(&http.Response{StatusCode: test.status, Body: body})
			assert.True(t, body.closed)
			if test.err == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, test.err)
			}
		})
	}

}

type monitoringFailReader struct{ reads int }

func (r *monitoringFailReader) Read([]byte) (int, error) {
	r.reads++
	return 0, errors.New("synthetic-private-value")
}
