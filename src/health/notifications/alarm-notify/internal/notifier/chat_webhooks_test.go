// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderChatWebhooks(t *testing.T) {
	for name, test := range map[string]struct{ status, previous, color string }{
		"warning": {"WARNING", "CLEAR", "#f0ad4e"}, "critical": {"CRITICAL", "CLEAR", "#d9534f"},
		"clear": {"CLEAR", "CRITICAL", "#5cb85c"},
	} {
		t.Run(name, func(t *testing.T) {
			event := expectedEvent()
			event.Status, event.PreviousStatus = test.status, test.previous
			event.URL = "https://example.com/alert?id=1&view=chart#details"
			for provider, p := range map[string]struct{ payload any }{
				"rocketchat": {renderRocketChat(Destination{Channel: "#alerts"}, event)},
				"flock":      {renderFlock(event)}, "fleep": {renderFleep(Destination{Sender: "Netdata"}, event)},
			} {
				t.Run(provider, func(t *testing.T) {
					fixture, err := os.ReadFile(filepath.Join("testdata", provider+"-full.json"))
					require.NoError(t, err)
					want := strings.ReplaceAll(string(fixture), "CLEAR → WARNING", test.previous+" → "+test.status)
					want = strings.ReplaceAll(want, "WARNING:", test.status+":")
					want = strings.ReplaceAll(want, "#f0ad4e", test.color)
					got, err := json.Marshal(p.payload)
					require.NoError(t, err)
					assert.JSONEq(t, want, string(got))
				})
			}
		})
	}
}

func TestChatWebhookContent(t *testing.T) {
	for name, test := range map[string]struct{ node, alert, summary, sender, channel, url string }{
		"minimal":            {node: "node", alert: "alert", summary: "summary"},
		"Unicode and quotes": {node: "節点", alert: "alert's \"name\"", summary: "<b>a&b</b> *text*\n😀", sender: "監視 'bot'", channel: "@user", url: "https://example.com/a,b;c?x=\"quoted\"&y=1#fragment"},
		"long content":       {node: "node", alert: "alert", summary: strings.Repeat("界😀", 3000)},
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
			rocketAttachment := map[string]any{
				"color": "#f0ad4e",
				"title": test.alert,
				"ts":    "2026-09-14T12:00:00Z",
				"fields": []any{
					map[string]any{"title": "Node", "value": test.node, "short": true},
					map[string]any{"title": "Alert", "value": test.alert, "short": true},
					map[string]any{"title": "Status", "value": "WARNING", "short": true},
				},
			}
			flockAttachment := map[string]any{"color": "#f0ad4e", "title": test.alert, "description": text}
			rocket := map[string]any{
				"alias":       "netdata on " + test.node,
				"text":        test.node + " WARNING: " + test.summary,
				"parseUrls":   false,
				"attachments": []any{rocketAttachment},
			}
			flock := map[string]any{
				"sendAs":      map[string]any{"name": "netdata on " + test.node},
				"text":        test.node + " WARNING: " + test.summary,
				"attachments": []any{flockAttachment},
			}
			fleep := map[string]any{"message": text}
			if test.channel != "" {
				rocket["channel"] = test.channel
			}
			if test.sender != "" {
				fleep["user"] = test.sender
			}
			if test.url != "" {
				rocketAttachment["title_link"], flockAttachment["url"] = test.url, test.url
				fleep["message"] = text + "\n" + test.url
			}
			for provider, p := range map[string]struct{ got, want any }{
				"rocketchat": {renderRocketChat(Destination{Channel: test.channel}, event), rocket},
				"flock":      {renderFlock(event), flock}, "fleep": {renderFleep(Destination{Sender: test.sender}, event), fleep},
			} {
				t.Run(provider, func(t *testing.T) {
					got, err := json.Marshal(p.got)
					require.NoError(t, err)
					want, err := json.Marshal(p.want)
					require.NoError(t, err)
					assert.JSONEq(t, string(want), string(got))
				})
			}
		})
	}
}

func TestReadRocketChatResponse(t *testing.T) {
	const success = `{"success":true,"message":{"_id":"test-message"}}`
	for name, test := range map[string]struct {
		status    int
		body, err string
	}{
		"success": {200, success, ""}, "minimal": {200, `{"success":true}`, ""},
		"missing acknowledgment": {200, `{}`, "did not acknowledge"}, "null": {200, `null`, "did not acknowledge"},
		"negative acknowledgment": {200, `{"success":false,"error":"synthetic-private-value"}`, "did not acknowledge"},
		"top error":               {200, `{"success":true,"error":"synthetic-private-value"}`, "did not acknowledge"},
		"room success":            {200, `{"success":true,"responses":[{"channel":"#alerts","message":{"_id":"test"}}]}`, ""},
		"room failure":            {200, `{"success":true,"responses":[{"message":{}},{"error":"synthetic-private-value"}]}`, "every destination room"},
		"wrong type":              {200, `{"success":"true"}`, "invalid"}, "invalid JSON": {200, "synthetic-private-value", "invalid"},
		"multiple documents": {200, success + `{}`, "invalid"}, "empty": {200, "", "invalid"},
		"boundary":     {200, success + strings.Repeat(" ", httpclient.ResponseLimit-len(success)), ""},
		"oversized":    {200, strings.Repeat(" ", httpclient.ResponseLimit+1), "256 KiB"},
		"HTTP failure": {403, "synthetic-private-value", "HTTP 403"}, "wrong success": {201, success, "HTTP 201"},
	} {
		t.Run(name, func(t *testing.T) {
			reader := strings.NewReader(test.body)
			body := &telegramTestBody{Reader: reader}
			err := readRocketChatResponse(&http.Response{StatusCode: test.status, Body: body})
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
}
