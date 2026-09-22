// SPDX-License-Identifier: GPL-3.0-or-later

package fleep

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
	for name, test := range map[string]struct{ status, previous string }{
		"warning": {"WARNING", "CLEAR"}, "critical": {"CRITICAL", "CLEAR"},
		"clear": {"CLEAR", "CRITICAL"},
	} {
		t.Run(name, func(t *testing.T) {
			event := testutil.ExpectedEvent()
			event.Status, event.PreviousStatus = test.status, test.previous
			event.URL = "https://example.com/alert?id=1&view=chart#details"
			for provider, p := range map[string]struct{ payload any }{
				"fleep": {renderFleep(Config{Sender: "Netdata"}, event)},
			} {
				t.Run(provider, func(t *testing.T) {
					fixture, err := os.ReadFile(filepath.Join("testdata", provider+"-full.json"))
					require.NoError(t, err)
					want := strings.ReplaceAll(string(fixture), "CLEAR → WARNING", test.previous+" → "+test.status)
					want = strings.ReplaceAll(want, "WARNING:", test.status+":")
					got, err := json.Marshal(p.payload)
					require.NoError(t, err)
					assert.JSONEq(t, want, string(got))
				})
			}
		})
	}
}

func TestChatWebhookContent(t *testing.T) {
	for name, test := range map[string]struct{ node, alert, summary, sender, url string }{
		"minimal":            {node: "node", alert: "alert", summary: "summary"},
		"Unicode and quotes": {node: "節点", alert: "alert's \"name\"", summary: "<b>a&b</b> *text*\n😀", sender: "監視 'bot'", url: "https://example.com/a,b;c?x=\"quoted\"&y=1#fragment"},
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
			fleep := map[string]any{"message": text}
			if test.sender != "" {
				fleep["user"] = test.sender
			}
			if test.url != "" {
				fleep["message"] = text + "\n" + test.url
			}

			for provider, p := range map[string]struct{ got, want any }{
				"fleep": {renderFleep(Config{Sender: test.sender}, event), fleep},
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
