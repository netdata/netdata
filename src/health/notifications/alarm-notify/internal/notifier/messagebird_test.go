// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const messagebirdTestPath = "/messages"

func TestRenderMessageBird(t *testing.T) {
	full := expectedEvent()
	full.URL = "https://example.com/alert?id=1&view=chart#details"
	critical, clear := full, full
	critical.Status = "CRITICAL"
	clear.Status, clear.PreviousStatus = "CLEAR", "CRITICAL"
	minimal := Event{Node: "node", Alert: "alert", Status: "WARNING", Summary: "summary", Timestamp: full.Timestamp}
	for name, test := range map[string]struct {
		event             Event
		from, to, fixture string
		replacements      []string
	}{
		"warning":                      {event: full, from: "+15005550006", to: "+15005550009", fixture: "messagebird-full.json"},
		"critical":                     {event: critical, from: "+15005550006", to: "+15005550009", fixture: "messagebird-full.json", replacements: []string{"WARNING", "CRITICAL"}},
		"clear":                        {event: clear, from: "+15005550006", to: "+15005550009", fixture: "messagebird-full.json", replacements: []string{"CLEAR → WARNING", "CRITICAL → CLEAR"}},
		"sender ID and unknown fields": {event: minimal, from: "Netdata Ops", to: "+15005550009", fixture: "messagebird-minimal.json"},
	} {
		t.Run(name, func(t *testing.T) {
			fixture, err := os.ReadFile(filepath.Join("testdata", test.fixture))
			require.NoError(t, err)
			var want url.Values
			require.NoError(
				t,
				json.Unmarshal([]byte(strings.NewReplacer(test.replacements...).Replace(string(fixture))), &want),
			)
			got, err := url.ParseQuery(
				renderMessageBird(Destination{Originator: test.from, Recipient: test.to}, test.event).Encode(),
			)
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

func TestMessageBirdPlainText(t *testing.T) {
	for name, text := range map[string]string{
		"form characters": "<b>\"quoted\" & + = % 'text'</b>\\\n😀", "no shortening": strings.Repeat("界😀", 1700),
	} {
		t.Run(name, func(t *testing.T) {
			event := Event{
				Node:      "node",
				Alert:     "alert",
				Status:    "WARNING",
				Summary:   text,
				Timestamp: expectedEvent().Timestamp,
			}
			want := url.Values{
				"originator": {"12345"},
				"recipients": {"+15005550009"},
				"datacoding": {"auto"},
				"body":       {text + "\nNode: node\nAlert: alert\nStatus: WARNING\nTime: 2026-09-14T12:00:00Z"},
			}
			got, err := url.ParseQuery(
				renderMessageBird(Destination{Originator: "12345", Recipient: "+15005550009"}, event).Encode(),
			)
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

func TestReadMessageBirdResponse(t *testing.T) {
	const success = `{"id":"test-message","recipients":{"totalCount":1,"items":[{"recipient":15005550009,"status":"sent"}]}}`
	for name, test := range map[string]struct {
		status    int
		body, err string
	}{
		"created":            {status: 201, body: success},
		"missing id":         {status: 201, body: `{}`, err: "invalid messagebird response"},
		"blank id":           {status: 201, body: `{"id":" "}`, err: "invalid messagebird response"},
		"null id":            {status: 201, body: `{"id":null}`, err: "invalid messagebird response"},
		"wrong type":         {status: 201, body: `{"id":123}`, err: "invalid messagebird response"},
		"null":               {status: 201, body: `null`, err: "invalid messagebird response"},
		"invalid JSON":       {status: 201, body: "synthetic-private-value", err: "invalid messagebird response"},
		"trailing JSON":      {status: 201, body: success + `{}`, err: "invalid messagebird response"},
		"size boundary":      {status: 201, body: success + strings.Repeat(" ", notificationResponseLimit-len(success))},
		"oversized":          {status: 201, body: strings.Repeat(" ", notificationResponseLimit+1), err: "256 KiB"},
		"HTTP error":         {status: 400, body: "synthetic-private-value", err: "HTTP 400"},
		"wrong success code": {status: 200, body: success, err: "HTTP 200"},
		"no content":         {status: 204, err: "HTTP 204"},
	} {
		t.Run(name, func(t *testing.T) {
			reader := strings.NewReader(test.body)
			body := &telegramTestBody{Reader: reader}
			err := readMessageBirdResponse(&http.Response{StatusCode: test.status, Body: body})
			if test.err == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, test.err)
				assert.NotContains(t, err.Error(), "synthetic-private-value")
			}
			assert.True(t, body.closed)
			if test.status != 201 {
				assert.Equal(t, len(test.body), reader.Len())
			}
		})
	}
}
