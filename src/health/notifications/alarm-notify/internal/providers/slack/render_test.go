// SPDX-License-Identifier: GPL-3.0-or-later

package slack

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderSlack(t *testing.T) {
	full := testutil.ExpectedEvent()
	full.URL = "https://example.com/alert?id=1&view=chart#details"
	critical := full
	critical.Status = "CRITICAL"
	recovery := full
	recovery.Status, recovery.PreviousStatus = "CLEAR", "CRITICAL"
	minimal := notifyevent.Event{
		Version:    1,
		IncidentID: "test",
		Node:       "node",
		Alert:      "alert",
		Status:     "WARNING",
		Summary:    "Something happened",
		Timestamp:  full.Timestamp,
	}
	escaping := minimal
	escaping.Summary = "<@USER> & <!here> *literal*"
	tests := map[string]struct {
		event        notifyevent.Event
		fixture      string
		replacements []string
	}{
		"warning full": {event: full, fixture: "slack-full.json"},
		"critical full": {
			event:        critical,
			fixture:      "slack-full.json",
			replacements: []string{"WARNING", "CRITICAL", `"warning"`, `"danger"`},
		},
		"recovery full": {
			event:        recovery,
			fixture:      "slack-full.json",
			replacements: []string{"CLEAR → WARNING", "CRITICAL → CLEAR", "WARNING", "CLEAR", `"warning"`, `"good"`},
		},
		"minimal unknown values": {event: minimal, fixture: "slack-minimal.json"},
		"escaping and no mentions": {
			event:        escaping,
			fixture:      "slack-minimal.json",
			replacements: []string{"Something happened", "&lt;@USER&gt; &amp; &lt;!here&gt; *literal*"},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			payload, err := renderSlack(test.event)
			require.NoError(t, err)
			got, err := json.Marshal(payload)
			require.NoError(t, err)
			want, err := os.ReadFile(filepath.Join("testdata", test.fixture))
			require.NoError(t, err)
			assert.JSONEq(t, strings.NewReplacer(test.replacements...).Replace(string(want)), string(got))
		})
	}
}

func TestRenderSlackLimits(t *testing.T) {
	base := notifyevent.Event{
		Node:      "node",
		Alert:     "alert",
		Status:    "WARNING",
		Summary:   "summary",
		Timestamp: time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC),
	}
	tests := map[string]struct {
		field string
		value string
		err   string
	}{
		"section boundary":        {field: "info", value: strings.Repeat("x", 3000)},
		"section overflow":        {field: "info", value: strings.Repeat("x", 3001), err: "3000-character"},
		"summary includes status": {field: "summary", value: strings.Repeat("x", 3000-len("WARNING: "))},
		"summary overflow": {
			field: "summary",
			value: strings.Repeat("x", 3001-len("WARNING: ")),
			err:   "3000-character",
		},
		"field Unicode boundary": {field: "node", value: strings.Repeat("界", 2000-len("Node\n"))},
		"field overflow": {
			field: "node",
			value: strings.Repeat("x", 2001-len("Node\n")),
			err:   "2000-character",
		},
		"escaping boundary": {field: "node", value: strings.Repeat("&", 399)},
		"escaping overflow": {field: "node", value: strings.Repeat("&", 400), err: "2000-character"},
		"URL boundary": {
			field: "url",
			value: "https://example.com/" + strings.Repeat("x", 3000-len("<https://example.com/|View alert>")),
		},
		"URL overflow": {
			field: "url",
			value: "https://example.com/" + strings.Repeat("x", 3001-len("<https://example.com/|View alert>")),
			err:   "link exceeds the 3000-character",
		},
		"URL escaped overflow": {
			field: "url",
			value: "https://example.com/?q=" + strings.Repeat("&", 600),
			err:   "link exceeds the 3000-character",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			event := base
			switch test.field {
			case "info":
				event.Info = test.value
			case "summary":
				event.Summary = test.value
			case "node":
				event.Node = test.value
			case "url":
				event.URL = test.value
			}
			got, err := renderSlack(event)
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				assert.Equal(t, slackMessage{}, got)
				assert.NotContains(t, err.Error(), test.value)
				return
			}
			require.NoError(t, err)
			assert.NotEmpty(t, got.Attachments)
		})
	}
}

func TestSlackLinkText(t *testing.T) {
	tests := map[string]struct {
		url  string
		text string
	}{
		"query and fragment": {
			url:  "https://example.com/alert?a=1&b=2#details",
			text: "<https://example.com/alert?a=1&amp;b=2#details|View alert>",
		},
		"control characters cannot introduce mentions": {
			url:  "https://example.com/?q=><!here>|label`",
			text: "<https://example.com/?q=%3E%3C!here%3E%7Clabel%60|View alert>",
		},
		"existing escapes preserved": {
			url:  "https://example.com/a%20b?q=%3Ctag%3E%7C",
			text: "<https://example.com/a%20b?q=%3Ctag%3E%7C|View alert>",
		},
		"spaces encoded": {
			url:  "https://example.com/a b?q=c d",
			text: "<https://example.com/a%20b?q=c%20d|View alert>",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := slackLinkText(test.url)
			require.NoError(t, err)
			assert.Equal(t, slackText{Type: "mrkdwn", Text: test.text, Verbatim: true}, got)
		})
	}
}
