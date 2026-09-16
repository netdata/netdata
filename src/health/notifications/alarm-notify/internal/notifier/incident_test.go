// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderIncidentEvents(t *testing.T) {
	for name, test := range map[string]struct{ status, previous, ilertType, signl4Status string }{
		"warning":  {"WARNING", "CLEAR", "ALERT", "new"},
		"critical": {"CRITICAL", "WARNING", "ALERT", "new"},
		"clear":    {"CLEAR", "CRITICAL", "RESOLVE", "resolved"},
	} {
		t.Run(name, func(t *testing.T) {
			event := expectedEvent()
			event.Status, event.PreviousStatus = test.status, test.previous
			event.URL = "https://example.com/alert?id=1&view=chart#details"
			for provider, p := range map[string]struct{ message any }{
				"ilert":  {renderIlert(Destination{IntegrationKey: "synthetic-key"}, event)},
				"signl4": {renderSIGNL4(event)},
			} {
				t.Run(provider, func(t *testing.T) {
					fixture, err := os.ReadFile(filepath.Join("testdata", provider+"-full.json"))
					require.NoError(t, err)
					want := strings.ReplaceAll(string(fixture), "CLEAR → WARNING", test.previous+" → "+test.status)
					want = strings.ReplaceAll(want, "WARNING:", test.status+":")
					want = strings.ReplaceAll(want, `"WARNING"`, `"`+test.status+`"`)
					want = strings.ReplaceAll(
						want,
						`"previous_status": "CLEAR"`,
						`"previous_status": "`+test.previous+`"`,
					)
					want = strings.ReplaceAll(want, `"ALERT"`, `"`+test.ilertType+`"`)
					want = strings.ReplaceAll(want, `"new"`, `"`+test.signl4Status+`"`)
					got, err := json.Marshal(p.message)
					require.NoError(t, err)
					assert.JSONEq(t, want, string(got))
				})
			}
		})
	}
}

func TestIncidentContent(t *testing.T) {
	for name, test := range map[string]struct{ node, alert, summary, url string }{
		"minimal":            {node: "node", alert: "alert", summary: "summary"},
		"Unicode and quotes": {node: "節点", alert: "alert's \"name\"", summary: "<b>a&b</b> *text*\n😀", url: "https://example.com/a,b;c?x=\"quoted\"&y=1#fragment"},
		"long content":       {node: "node", alert: "alert", summary: strings.Repeat("界😀", 3000)},
	} {
		t.Run(name, func(t *testing.T) {
			event := notifyevent.Event{
				Version:    1,
				IncidentID: "incident",
				Node:       test.node,
				Alert:      test.alert,
				Summary:    test.summary,
				Status:     "WARNING",
				Timestamp:  expectedEvent().Timestamp,
				URL:        test.url,
			}
			text := test.summary + "\nNode: " + test.node + "\nAlert: " + test.alert + "\nStatus: WARNING\nTime: 2026-09-14T12:00:00Z"
			custom := map[string]any{"version": 1, "incident_id": "incident", "node": test.node, "alert": test.alert,
				"summary": test.summary, "status": "WARNING", "timestamp": "2026-09-14T12:00:00Z", "value": nil, "previous_value": nil}
			ilert := map[string]any{"integrationKey": "synthetic-key", "eventType": "ALERT",
				"alertKey": "d4191834714542dcf3e5d8a6ab386c9b72259430730157b6e5c76469cbb6a622",
				"summary":  test.node + " WARNING: " + test.summary, "details": text, "customDetails": custom}
			signl4 := map[string]any{"Title": test.node + " WARNING: " + test.summary, "Message": text,
				"Severity": "WARNING", "X-S4-ExternalID": "incident", "X-S4-Status": "new", "X-S4-SourceSystem": "Netdata"}
			if test.url != "" {
				custom["url"] = test.url
				ilert["links"] = []any{map[string]any{"href": test.url, "text": "View alert"}}
				signl4["Message"] = text + "\n" + test.url
			}
			for provider, p := range map[string]struct{ got, want any }{
				"ilert":  {renderIlert(Destination{IntegrationKey: "synthetic-key"}, event), ilert},
				"signl4": {renderSIGNL4(event), signl4},
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
