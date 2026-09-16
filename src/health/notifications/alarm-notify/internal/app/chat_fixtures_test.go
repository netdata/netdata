// SPDX-License-Identifier: GPL-3.0-or-later

package app

import (
	"fmt"
	"os"
	"strings"
	"testing"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/stretchr/testify/require"
)

func teamsMatrixTestEvent(status, variant string) notifyevent.Event {
	event := expectedEvent()
	if variant == "minimal" {
		event = notifyevent.Event{
			Version:    1,
			IncidentID: "test-incident",
			Timestamp:  event.Timestamp,
			Node:       "node",
			Alert:      "alert",
			Summary:    "summary",
		}
	} else {
		event.URL = "https://example.com/alert?id=1&view=chart#details"
		event.PreviousStatus = map[string]string{"WARNING": "CLEAR", "CRITICAL": "WARNING", "CLEAR": "CRITICAL"}[status]
	}
	event.Status = status
	return event
}

func teamsMatrixTestFixture(t *testing.T, provider, status, variant string) string {
	t.Helper()
	data, err := os.ReadFile(fixturePath(fmt.Sprintf("%s-%s-%s.json", provider, strings.ToLower(status), variant)))
	require.NoError(t, err)
	return string(data)
}

type pushoverMessage struct {
	Token     string `json:"token"`
	User      string `json:"user"`
	Title     string `json:"title"`
	Message   string `json:"message"`
	HTML      int    `json:"html"`
	Priority  int    `json:"priority"`
	Timestamp int64  `json:"timestamp"`
	URL       string `json:"url,omitempty"`
	URLTitle  string `json:"url_title,omitempty"`
}

type gotifyMessage struct {
	Title    string `json:"title"`
	Message  string `json:"message"`
	Priority int    `json:"priority"`
}

const pushoverTestToken = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
const pushoverTestUser = "UUUUUUUUUUUUUUUUUUUUUUUUUUUUUU"

func teamsMatrixTestDestination(provider string) map[string]any {
	if provider == "msteams" {
		return map[string]any{"type": provider, "url": "https://example.com/teams?sig=synthetic-url-secret"}
	}
	return map[string]any{"type": provider, "api_url": "https://example.com/matrix/", "access_token": "synthetic-token", "room_id": "!room:example.org"}
}

type ntfyAction struct {
	Action string `json:"action"`
	Label  string `json:"label"`
	URL    string `json:"url"`
	Clear  bool   `json:"clear"`
}
