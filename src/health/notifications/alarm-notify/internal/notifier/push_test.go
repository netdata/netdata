// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"encoding/json"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	notifymsg "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderPush(t *testing.T) {
	for name, test := range map[string]struct {
		status, previous, priority, tag string
		gotifyPriority                  int
	}{
		"warning":  {"WARNING", "CLEAR", "high", "warning", 4},
		"critical": {"CRITICAL", "CLEAR", "urgent", "red_circle", 10},
		"clear":    {"CLEAR", "CRITICAL", "default", "white_check_mark", 1},
	} {
		t.Run(name, func(t *testing.T) {
			event := expectedEvent()
			event.URL = "https://example.com/alert?id=1&view=chart#details"
			event.Status, event.PreviousStatus = test.status, test.previous
			fixture, err := os.ReadFile(filepath.Join("testdata", "gotify-full.json"))
			require.NoError(t, err)
			var want gotifyMessage
			require.NoError(t, json.Unmarshal(fixture, &want))
			want.Priority = test.gotifyPriority
			want.Title = "test-node " + test.status + ": Temperature is high"
			want.Message = strings.ReplaceAll(want.Message, "CLEAR → WARNING", test.previous+" → "+test.status)
			assert.Equal(t, want, renderGotify(event))
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
				Timestamp: expectedEvent().Timestamp,
				URL:       test.url,
			}
			text := test.summary + "\nNode: " + test.node + "\nAlert: " + test.alert + "\nStatus: WARNING\nTime: 2026-09-14T12:00:00Z"
			gotifyText := text
			if test.url != "" {
				gotifyText += "\n" + test.url
			}
			assert.Equal(
				t,
				gotifyMessage{Title: test.node + " WARNING: " + test.summary, Message: gotifyText, Priority: 4},
				renderGotify(event),
			)
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
	for provider, p := range map[string]struct {
		read    func(*http.Response) error
		success string
	}{
		"gotify": {readGotifyResponse, `{"id":25,"appid":5,"message":"accepted"}`},
		"ntfy":   {readNtfyResponse, `{"id":"test-message","event":"message","topic":"alerts","time":1750000000}`},
	} {
		t.Run(provider, func(t *testing.T) {
			cases := map[string]struct {
				status    int
				body, err string
			}{
				"success": {200, p.success, ""}, "missing ID": {200, `{}`, "invalid"}, "null": {200, `null`, "invalid"},
				"invalid JSON": {
					200,
					"synthetic-private-value",
					"invalid",
				}, "multiple documents": {200, p.success + `{}`, "invalid"},
				"boundary":  {200, p.success + strings.Repeat(" ", httpclient.ResponseLimit-len(p.success)), ""},
				"oversized": {200, strings.Repeat(" ", httpclient.ResponseLimit+1), "256 KiB"},
				"HTTP error": {
					403,
					"synthetic-private-value",
					"HTTP 403",
				}, "wrong success": {201, p.success, "HTTP 201"},
			}
			if provider == "gotify" {
				for name, body := range map[string]string{"zero ID": `{"id":0}`, "negative ID": `{"id":-1}`, "string ID": `{"id":"25"}`, "fraction ID": `{"id":1.2}`} {
					cases[name] = struct {
						status    int
						body, err string
					}{200, body, "invalid"}
				}
			} else {
				for name, body := range map[string]string{"blank ID": `{"id":" ","event":"message"}`, "number ID": `{"id":1,"event":"message"}`, "wrong event": `{"id":"test","event":"open"}`, "missing event": `{"id":"test"}`} {
					cases[name] = struct {
						status    int
						body, err string
					}{200, body, "invalid"}
				}
			}
			for name, test := range cases {
				t.Run(name, func(t *testing.T) {
					reader := strings.NewReader(test.body)
					body := &telegramTestBody{Reader: reader}
					err := p.read(&http.Response{StatusCode: test.status, Body: body})
					if test.err == "" {
						require.NoError(t, err)
					} else {
						require.ErrorContains(t, err, test.err)
						assert.NotContains(t, err.Error(), "synthetic-private-value")
					}
					assert.True(t, body.closed)
					if test.status != 200 {
						assert.Equal(t, len(test.body), reader.Len())
					}
				})
			}
		})
	}
}
