// SPDX-License-Identifier: GPL-3.0-or-later

package message

import (
	"testing"
	"time"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/stretchr/testify/assert"
)

func TestFields(t *testing.T) {
	for name, test := range map[string]struct {
		event event.Event
		want  []Field
	}{
		"minimal": {
			event: event.Event{Node: "node", Alert: "alert", Status: "WARNING"},
			want:  []Field{{"Node", "node"}, {"Alert", "alert"}, {"Status", "WARNING"}},
		},
		"unchanged status and zero value": {
			event: event.Event{Node: "node", Alert: "alert", Status: "WARNING", PreviousStatus: "WARNING", Value: new(float64(0))},
			want:  []Field{{"Node", "node"}, {"Alert", "alert"}, {"Status", "WARNING"}, {"Value", "0"}},
		},
		"transition and optional facts": {
			event: event.Event{Node: "節点", Alert: "<alert>", Status: "CLEAR", PreviousStatus: "CRITICAL", Chart: "cpu", Context: "system.cpu", Value: new(float64(1.25)), PreviousValue: new(float64(42.5)), Units: "%"},
			want:  []Field{{"Node", "節点"}, {"Alert", "<alert>"}, {"Status", "CRITICAL → CLEAR"}, {"Chart", "cpu"}, {"Context", "system.cpu"}, {"Value", "1.25 %"}, {"Previous value", "42.5 %"}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.want, Fields(test.event))
		})
	}
}

func TestPlainText(t *testing.T) {
	for name, test := range map[string]struct {
		info, url  string
		includeURL bool
		want       string
	}{
		"minimal": {want: "Summary\nNode: node\nAlert: alert\nStatus: WARNING\nTime: 2026-09-16T12:00:00Z"},
		"details and URL": {info: "Details <&>\n第二行", url: "https://example.com/alert", includeURL: true,
			want: "Summary\nDetails <&>\n第二行\nNode: node\nAlert: alert\nStatus: WARNING\nTime: 2026-09-16T12:00:00Z\nhttps://example.com/alert"},
		"URL omitted by provider": {url: "https://example.com/alert",
			want: "Summary\nNode: node\nAlert: alert\nStatus: WARNING\nTime: 2026-09-16T12:00:00Z"},
	} {
		t.Run(name, func(t *testing.T) {
			notification := event.Event{Node: "node", Alert: "alert", Status: "WARNING", Summary: "Summary", Info: test.info, URL: test.url, Timestamp: time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)}
			assert.Equal(t, test.want, PlainText(notification, test.includeURL))
		})
	}
}

func TestEscapeControls(t *testing.T) {
	for name, test := range map[string]struct{ input, want string }{
		"Unicode and punctuation":    {"節点 😀 <&> \\", "節点 😀 <&> \\"},
		"line and terminal controls": {"a\n\r\t\x00\x1bb", `a\n\r\t\x00\x1bb`},
		"Unicode line separators":    {"a\u2028b\u2029c", `a\u2028b\u2029c`},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.want, EscapeControls(test.input))
		})
	}
}
