// SPDX-License-Identifier: GPL-3.0-or-later

package ntfy

import (
	"encoding/json"
	"mime"
	"net/http"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	notifymsg "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderPush(t *testing.T) {
	for name, test := range map[string]struct {
		status, previous, priority, tag string
	}{
		"warning":  {"WARNING", "CLEAR", "high", "warning"},
		"critical": {"CRITICAL", "CLEAR", "urgent", "red_circle"},
		"clear":    {"CLEAR", "CRITICAL", "default", "white_check_mark"},
	} {
		t.Run(name, func(t *testing.T) {
			event := testutil.ExpectedEvent()
			event.URL = "https://example.com/alert?id=1&view=chart#details"
			event.Status, event.PreviousStatus = test.status, test.previous
			wantHeaders := http.Header{
				"Title":    {"test-node: test alert"},
				"Priority": {test.priority},
				"Tags":     {test.tag},
				"Actions": {
					`[{"action":"view","label":"View node","url":"https://example.com/alert?id=1\u0026view=chart#details","clear":true}]`,
				},
			}
			assert.Equal(t, wantHeaders, renderNtfyHeaders(event))
		})
	}
}

func TestPushContent(t *testing.T) {
	for name, test := range map[string]struct{ node, alert, summary, url string }{
		"minimal":                     {"node", "alert", "summary", ""},
		"Unicode and header controls": {"節点\r\nPriority: urgent", strings.Repeat("界😀", 100), "<b>\"quoted\" & + = % 'text'</b>\n😀", "https://example.com/a,b;c?x=\"quoted\"&y=1#fragment"},
		"long message":                {"node", "alert", strings.Repeat("界😀", 3000), ""},
	} {
		t.Run(name, func(t *testing.T) {
			event := notifyevent.Event{
				Node:      test.node,
				Alert:     test.alert,
				Summary:   test.summary,
				Status:    "WARNING",
				Timestamp: testutil.ExpectedEvent().Timestamp,
				URL:       test.url,
			}
			text := test.summary + "\nNode: " + test.node + "\nAlert: " + test.alert + "\nStatus: WARNING\nTime: 2026-09-14T12:00:00Z"
			headers := renderNtfyHeaders(event)
			title, err := new(mime.WordDecoder).DecodeHeader(headers.Get("Title"))
			require.NoError(t, err)
			assert.Equal(t, test.node+": "+strings.ReplaceAll(test.alert, "_", " "), title)
			for _, values := range headers {
				for _, value := range values {
					assert.NotContains(t, value, "\r")
					assert.NotContains(t, value, "\n")
				}
			}
			want := http.Header{"Title": {headers.Get("Title")}, "Priority": {"high"}, "Tags": {"warning"}}
			if test.url != "" {
				var actions []ntfyAction
				require.NoError(t, json.Unmarshal([]byte(headers.Get("Actions")), &actions))
				assert.Equal(t, []ntfyAction{{Action: "view", Label: "View node", URL: test.url, Clear: true}}, actions)
				want.Set("Actions", headers.Get("Actions"))
			}
			assert.Equal(t, want, headers)
			assert.Equal(t, text, notifymsg.PlainText(event, false))
		})
	}
}

func TestNtfyLiteralMIMEHeaders(t *testing.T) {
	for name, test := range map[string]struct{ node, alert, url string }{
		"literal title":           {"=?UTF-8?B?SGVsbG8=?=", "test_alert", "https://example.com/alert"},
		"long literal title":      {strings.Repeat("prefix ", 50) + "=?UTF-8?B?SGVsbG8=?=", "test_alert", "https://example.com/alert"},
		"literal URL":             {"node", "alert", "https://example.com/?x==?UTF-8?B?Ig==?="},
		"Unicode and literal URL": {"節点", "alert", "https://example.com/界?x==?UTF-8?B?Ig==?="},
	} {
		t.Run(name, func(t *testing.T) {
			event := notifyevent.Event{Node: test.node, Alert: test.alert, Status: "WARNING", URL: test.url}
			headers := renderNtfyHeaders(event)
			decoder := new(mime.WordDecoder)
			title, err := decoder.DecodeHeader(headers.Get("Title"))
			require.NoError(t, err)
			assert.Equal(t, test.node+": "+strings.ReplaceAll(test.alert, "_", " "), title)
			// ntfy MIME-decodes the Actions header before parsing its JSON.
			text, err := decoder.DecodeHeader(headers.Get("Actions"))
			require.NoError(t, err)
			var actions []ntfyAction
			require.NoError(t, json.Unmarshal([]byte(text), &actions))
			assert.Equal(t, []ntfyAction{{Action: "view", Label: "View node", URL: test.url, Clear: true}}, actions)
		})
	}
}

func TestReadPushResponse(t *testing.T) {
	const success = `{"id":"test-message","event":"message","topic":"alerts","time":1750000000}`
	cases := map[string]struct {
		status    int
		body, err string
	}{
		"success": {200, success, ""}, "missing ID": {200, `{}`, "invalid"}, "null": {200, `null`, "invalid"},
		"invalid JSON": {
			200,
			"synthetic-private-value",
			"invalid",
		}, "multiple documents": {200, success + `{}`, "invalid"},
		"boundary":  {200, success + strings.Repeat(" ", httpclient.ResponseLimit-len(success)), ""},
		"oversized": {200, strings.Repeat(" ", httpclient.ResponseLimit+1), "256 KiB"},
		"HTTP error": {
			403,
			"synthetic-private-value",
			"HTTP 403",
		}, "wrong success": {201, success, "HTTP 201"},
	}
	for name, body := range map[string]string{"blank ID": `{"id":" ","event":"message"}`, "number ID": `{"id":1,"event":"message"}`, "wrong event": `{"id":"test","event":"open"}`, "missing event": `{"id":"test"}`} {
		cases[name] = struct {
			status    int
			body, err string
		}{200, body, "invalid"}
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			reader := strings.NewReader(test.body)
			body := &testutil.TrackingBody{Reader: reader}
			err := readNtfyResponse(&http.Response{StatusCode: test.status, Body: body})
			if test.err == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, test.err)
				assert.NotContains(t, err.Error(), "synthetic-private-value")
			}
			assert.True(t, body.Closed)
			if test.status != 200 {
				assert.Equal(t, len(test.body), reader.Len())
			}
		})
	}
}
