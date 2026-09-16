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

func TestRenderPushbullet(t *testing.T) {
	full := expectedEvent()
	full.URL = "https://example.com/alert?id=1&view=chart#details"
	critical, clear := full, full
	critical.Status = "CRITICAL"
	clear.Status, clear.PreviousStatus = "CLEAR", "CRITICAL"
	minimal := notifyevent.Event{Node: "node", Alert: "alert", Status: "WARNING", Summary: "summary", Timestamp: full.Timestamp}
	dst := Destination{Email: "ops@example.com", SourceDeviceID: "test-source-device"}
	for name, test := range map[string]struct {
		event        notifyevent.Event
		dst          Destination
		fixture      string
		replacements []string
	}{
		"warning link":                    {event: full, dst: dst, fixture: "pushbullet-full.json"},
		"critical link":                   {event: critical, dst: dst, fixture: "pushbullet-full.json", replacements: []string{"WARNING", "CRITICAL"}},
		"clear link":                      {event: clear, dst: dst, fixture: "pushbullet-full.json", replacements: []string{"CLEAR → WARNING", "CRITICAL → CLEAR", "WARNING", "CLEAR"}},
		"channel note and unknown values": {event: minimal, dst: Destination{ChannelTag: "test-alerts"}, fixture: "pushbullet-minimal.json"},
	} {
		t.Run(name, func(t *testing.T) {
			data, err := json.Marshal(renderPushbullet(test.dst, test.event))
			require.NoError(t, err)
			want, err := os.ReadFile(filepath.Join("testdata", test.fixture))
			require.NoError(t, err)
			assert.JSONEq(t, strings.NewReplacer(test.replacements...).Replace(string(want)), string(data))
		})
	}
}

func TestPushbulletPlainText(t *testing.T) {
	for name, text := range map[string]string{
		"JSON characters": "<b>\"quoted\" & 'text'</b>\\\n😀", "long Unicode": strings.Repeat("界😀", 3000),
	} {
		t.Run(name, func(t *testing.T) {
			event := notifyevent.Event{
				Node:      "node",
				Alert:     "alert",
				Status:    "WARNING",
				Summary:   text,
				Info:      text,
				Timestamp: expectedEvent().Timestamp,
			}
			want := pushbulletMessage{Type: "note", Email: "ops@example.com", Title: "node WARNING: " + text,
				Body: text + "\n" + text + "\nNode: node\nAlert: alert\nStatus: WARNING\nTime: 2026-09-14T12:00:00Z"}
			encoded, err := json.Marshal(renderPushbullet(Destination{Email: "ops@example.com"}, event))
			require.NoError(t, err)
			var decoded pushbulletMessage
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			assert.Equal(t, want, decoded)
		})
	}
}

func TestReadPushbulletResponse(t *testing.T) {
	const success = `{"iden":"test-push","type":"link"}`
	for name, test := range map[string]struct {
		status    int
		body, err string
	}{
		"created":       {status: 200, body: success},
		"null error":    {status: 200, body: `{"iden":"test-push","error":null}`},
		"missing id":    {status: 200, body: `{}`, err: "invalid pushbullet response"},
		"blank id":      {status: 200, body: `{"iden":" "}`, err: "invalid pushbullet response"},
		"null id":       {status: 200, body: `{"iden":null}`, err: "invalid pushbullet response"},
		"wrong type id": {status: 200, body: `{"iden":123}`, err: "invalid pushbullet response"},
		"null":          {status: 200, body: `null`, err: "invalid pushbullet response"},
		"invalid JSON":  {status: 200, body: "synthetic-private-value", err: "invalid pushbullet response"},
		"trailing JSON": {status: 200, body: success + `{}`, err: "invalid pushbullet response"},
		"error object":  {status: 200, body: `{"iden":"test-push","error":{"message":"synthetic-private-value"}}`, err: "API rejected"},
		"size boundary": {status: 200, body: success + strings.Repeat(" ", httpclient.ResponseLimit-len(success))},
		"oversized":     {status: 200, body: success + strings.Repeat(" ", httpclient.ResponseLimit), err: "256 KiB"},
		"HTTP error":    {status: 400, body: "synthetic-private-value", err: "HTTP 400"},
		"unconfirmed":   {status: 204, err: "HTTP 204"},
	} {
		t.Run(name, func(t *testing.T) {
			reader := strings.NewReader(test.body)
			body := &telegramTestBody{Reader: reader}
			err := readPushbulletResponse(&http.Response{StatusCode: test.status, Body: body})
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
