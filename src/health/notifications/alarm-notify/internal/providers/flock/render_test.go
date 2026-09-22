// SPDX-License-Identifier: GPL-3.0-or-later

package flock

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderChatWebhooks(t *testing.T) {
	for name, test := range map[string]struct{ status, previous, color string }{
		"warning": {"WARNING", "CLEAR", "#f0ad4e"}, "critical": {"CRITICAL", "CLEAR", "#d9534f"},
		"clear": {"CLEAR", "CRITICAL", "#5cb85c"},
	} {
		t.Run(name, func(t *testing.T) {
			event := testutil.ExpectedEvent()
			event.Status, event.PreviousStatus = test.status, test.previous
			event.URL = "https://example.com/alert?id=1&view=chart#details"
			for provider, p := range map[string]struct{ payload any }{
				"flock": {renderFlock(event)},
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
	for name, test := range map[string]struct{ node, alert, summary, url string }{
		"minimal":            {node: "node", alert: "alert", summary: "summary"},
		"Unicode and quotes": {node: "節点", alert: "alert's \"name\"", summary: "<b>a&b</b> *text*\n😀", url: "https://example.com/a,b;c?x=\"quoted\"&y=1#fragment"},
		"long content":       {node: "node", alert: "alert", summary: strings.Repeat("界😀", 3000)},
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
			flockAttachment := map[string]any{"color": "#f0ad4e", "title": test.alert, "description": text}
			flock := map[string]any{
				"sendAs":      map[string]any{"name": "netdata on " + test.node},
				"text":        test.node + " WARNING: " + test.summary,
				"attachments": []any{flockAttachment},
			}
			if test.url != "" {
				flockAttachment["url"] = test.url
			}

			for provider, p := range map[string]struct{ got, want any }{
				"flock": {renderFlock(event), flock},
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
