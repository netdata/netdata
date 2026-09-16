// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

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

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderMonitoringEvents(t *testing.T) {
	for name, test := range map[string]struct{ status, previous, severity string }{
		"warning": {"WARNING", "CLEAR", "warning"}, "critical": {"CRITICAL", "WARNING", "critical"}, "clear": {"CLEAR", "CRITICAL", "cleared"},
	} {
		t.Run(name, func(t *testing.T) {
			event := expectedEvent()
			event.Status, event.PreviousStatus = test.status, test.previous
			event.URL = "https://example.com/alert?id=1&view=chart#details"
			alerta, err := renderAlerta(Destination{Environment: "Production"}, event)
			require.NoError(t, err)
			dynatrace, err := renderDynatrace(Destination{EntitySelector: `type(HOST),tag("netdata")`}, event)
			require.NoError(t, err)
			for provider, message := range map[string]any{"alerta": alerta, "dynatrace": dynatrace} {
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
	if provider == "alerta" {
		wantRaw, ok := wantObject["rawData"].(string)
		require.True(t, ok)
		gotRaw, ok := gotObject["rawData"].(string)
		require.True(t, ok)
		assert.JSONEq(t, wantRaw, gotRaw)
		delete(wantObject, "rawData")
		delete(gotObject, "rawData")
	}
	assert.Equal(t, wantObject, gotObject)
}

func TestMonitoringContentAndTargeting(t *testing.T) {
	for name, test := range map[string]struct{ chart, resource, alertEvent, environment, selector, source, eventType string }{
		"minimal":          {resource: "節点", alertEvent: `alert "one"`, environment: "Production", selector: `type(HOST),tag("netdata")`, source: "Netdata Alarm", eventType: "CUSTOM_INFO"},
		"regular chart":    {chart: "test.chart", resource: "節点", alertEvent: `test.chart.alert "one"`, environment: "Development", selector: `entityId("HOST-0123456789ABCDEF")`, source: "Source '😀'", eventType: "CUSTOM_ALERT"},
		"httpcheck":        {chart: "httpcheck.example", resource: "httpcheck.example", alertEvent: `alert "one"`, environment: "Custom 東京", selector: "type(HOST)", source: "Netdata Alarm", eventType: "CUSTOM_DEPLOYMENT"},
		"httpcheck prefix": {chart: "httpcheck_status", resource: "httpcheck_status", alertEvent: `alert "one"`, environment: "Production", selector: "type(HOST)", source: "Netdata Alarm", eventType: "WARNING"},
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
					got, err := renderAlerta(Destination{Environment: test.environment}, event)
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
					dynatrace, err := renderDynatrace(
						Destination{EntitySelector: test.selector, Source: test.source, EventType: test.eventType},
						event,
					)
					require.NoError(t, err)
					assert.Equal(
						t,
						dynatraceEvent{
							EventType:      test.eventType,
							Title:          "節点 " + status + ": <b>a&b</b>\n😀",
							EntitySelector: test.selector,
							Properties: map[string]string{
								"dt.event.source":      test.source,
								"dt.event.description": body,
								"netdata.incident_id":  " ID:😀 ",
								"netdata.timestamp":    "2026-09-15T09:00:00.123456789+01:00",
							},
						},
						dynatrace,
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
			event := expectedEvent()
			event.URL = url
			got, err := renderAlerta(Destination{Environment: "Production"}, event)
			require.NoError(t, err)
			want := map[string]string{
				"quotes": `<a href="https://example.com/?x=&#34;quoted&#34;&amp;y=&#39;value&#39;">View Netdata</a>`,
				"markup": `<a href="https://example.com/&lt;value&gt;?a=1&amp;b=2">View Netdata</a>`,
			}
			assert.Equal(t, want[name], got.Attributes["moreInfo"])
		})
	}
}

func TestDynatracePropertyLimits(t *testing.T) {
	for name, test := range map[string]struct {
		field  string
		length int
		fail   bool
	}{
		"ID max": {"id", 4096, false}, "ID too long": {"id", 4097, true},
		"source max": {"source", 4096, false}, "source too long": {"source", 4097, true},
		"description too long": {"info", 4097, true},
	} {
		t.Run(name, func(t *testing.T) {
			event, dst := expectedEvent(), Destination{EntitySelector: "type(HOST)"}
			value := strings.Repeat("界", test.length)
			switch test.field {
			case "id":
				event.IncidentID = value
			case "source":
				dst.Source = value
			case "info":
				event.Info = value
			}
			_, err := renderDynatrace(dst, event)
			if test.fail {
				require.EqualError(t, err, "dynatrace event property exceeds 4096 characters")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

const alertaTestAck = `{"status":"ok","id":"synthetic-alert"}`
const dynatraceTestAck = `{"reportCount":1,"eventIngestResults":[{"status":"OK","correlationId":"synthetic-event"}]}`

func TestMonitoringAcknowledgments(t *testing.T) {
	for provider, p := range map[string]struct {
		read func(*http.Response) error
		ack  string
	}{
		"alerta": {readAlertaResponse, alertaTestAck}, "dynatrace": {readDynatraceResponse, dynatraceTestAck},
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
					body := &telegramTestBody{Reader: reader}
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
			body := &telegramTestBody{Reader: strings.NewReader(test.body)}
			err := readAlertaResponse(&http.Response{StatusCode: test.status, Body: body})
			assert.True(t, body.closed)
			if test.err == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, test.err)
			}
		})
	}
	for name, test := range map[string]struct {
		body string
		fail bool
	}{
		"many entities":       {`{"reportCount":2,"eventIngestResults":[{"status":"OK","correlationId":"one"},{"status":"OK","correlationId":"two"}]}`, false},
		"zero entities":       {`{"reportCount":0,"eventIngestResults":[]}`, true},
		"partial failure":     {`{"reportCount":2,"eventIngestResults":[{"status":"OK","correlationId":"one"},{"status":"INVALID_ENTITY_TYPE"}]}`, true},
		"invalid timestamps":  {`{"reportCount":1,"eventIngestResults":[{"status":"INVALID_TIMESTAMPS"}]}`, true},
		"invalid metadata":    {`{"reportCount":1,"eventIngestResults":[{"status":"INVALID_METADATA"}]}`, true},
		"unknown status":      {`{"reportCount":1,"eventIngestResults":[{"status":"synthetic-private-value","correlationId":"one"}]}`, true},
		"missing correlation": {`{"reportCount":1,"eventIngestResults":[{"status":"OK"}]}`, true},
		"blank correlation":   {`{"reportCount":1,"eventIngestResults":[{"status":"OK","correlationId":" "}]}`, true},
		"mismatched count":    {`{"reportCount":2,"eventIngestResults":[{"status":"OK","correlationId":"one"}]}`, true},
		"negative count":      {`{"reportCount":-1,"eventIngestResults":[]}`, true},
	} {
		t.Run("dynatrace/"+name, func(t *testing.T) {
			body := &telegramTestBody{Reader: strings.NewReader(test.body)}
			err := readDynatraceResponse(&http.Response{StatusCode: 201, Body: body})
			assert.True(t, body.closed)
			if test.fail {
				require.ErrorContains(t, err, "acknowledge")
				assert.NotContains(t, err.Error(), "synthetic-private-value")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

type monitoringFailReader struct{ reads int }

func (r *monitoringFailReader) Read([]byte) (int, error) {
	r.reads++
	return 0, errors.New("synthetic-private-value")
}
