// SPDX-License-Identifier: GPL-3.0-or-later

package gotify

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderPush(t *testing.T) {
	for name, test := range map[string]struct {
		status, previous string
		gotifyPriority   int
	}{
		"warning":  {"WARNING", "CLEAR", 4},
		"critical": {"CRITICAL", "CLEAR", 10},
		"clear":    {"CLEAR", "CRITICAL", 1},
	} {
		t.Run(name, func(t *testing.T) {
			event := testutil.ExpectedEvent()
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
			gotifyText := text
			if test.url != "" {
				gotifyText += "\n" + test.url
			}
			assert.Equal(
				t,
				gotifyMessage{Title: test.node + " WARNING: " + test.summary, Message: gotifyText, Priority: 4},
				renderGotify(event),
			)
		})
	}
}

func TestReadPushResponse(t *testing.T) {
	const success = `{"id":25,"appid":5,"message":"accepted"}`
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
	for name, body := range map[string]string{"zero ID": `{"id":0}`, "negative ID": `{"id":-1}`, "string ID": `{"id":"25"}`, "fraction ID": `{"id":1.2}`} {
		cases[name] = struct {
			status    int
			body, err string
		}{200, body, "invalid"}
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			reader := strings.NewReader(test.body)
			body := &testutil.TrackingBody{Reader: reader}
			err := readGotifyResponse(&http.Response{StatusCode: test.status, Body: body})
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
