// SPDX-License-Identifier: GPL-3.0-or-later

package kafka

import (
	"encoding/json"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"path/filepath"
	"testing"
	"time"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKafkaConfig(t *testing.T) {
	for name, test := range map[string]struct {
		change func(*Config)
		err    string
	}{
		"IPv4": {}, "IPv6": {change: func(d *Config) { d.SenderIP = "2001:db8::1" }},
		"HTTP":             {change: func(d *Config) { d.URL = "http://localhost/kafka" }},
		"env URL":          {change: func(d *Config) { d.URL = "${env:UNREAD_KAFKA_URL}" }},
		"file URL":         {change: func(d *Config) { d.URL = "${file:" + filepath.Join(t.TempDir(), "unread") + "}" }},
		"missing URL":      {change: func(d *Config) { d.URL = "" }, err: "absolute HTTP(S)"},
		"relative URL":     {change: func(d *Config) { d.URL = "/synthetic-private-value" }, err: "absolute HTTP(S)"},
		"userinfo":         {change: func(d *Config) { d.URL = "https://user:synthetic-private-value@example.com" }, err: "user information"},
		"fragment":         {change: func(d *Config) { d.URL += "#synthetic-private-value" }, err: "fragment"},
		"reference syntax": {change: func(d *Config) { d.URL = "${exec:synthetic-private-value}" }, err: "env and file"},
		"missing IP":       {change: func(d *Config) { d.SenderIP = "" }, err: "literal IPv4 or IPv6"},
		"hostname":         {change: func(d *Config) { d.SenderIP = "synthetic-private-value" }, err: "literal IPv4 or IPv6"},
		"CIDR":             {change: func(d *Config) { d.SenderIP += "/24" }, err: "literal IPv4 or IPv6"},
		"zone":             {change: func(d *Config) { d.SenderIP = "fe80::1%eth0" }, err: "without a zone"},
		"port":             {change: func(d *Config) { d.SenderIP += ":80" }, err: "literal IPv4 or IPv6"},
		"whitespace":       {change: func(d *Config) { d.SenderIP += "\n" }, err: "literal IPv4 or IPv6"},
		"IP reference":     {change: func(d *Config) { d.SenderIP = "${env:SENDER_IP}" }, err: "literal IPv4 or IPv6"},
	} {
		t.Run(name, func(t *testing.T) {
			dst := Config{URL: "https://example.com/kafka?key=synthetic-private-value", SenderIP: "192.0.2.1"}
			if test.change != nil {
				test.change(&dst)
			}
			checkFormConfig(t, dst, test.err)
		})
	}
}

const kafkaFullJSON = `{
  "host_ip": "192.0.2.1", "when": 1789387200, "name": "test_alert", "chart": "test.chart",
  "status": "WARNING", "old_status": "CLEAR", "value": 42.5, "old_value": 0,
  "duration": 0, "non_clear_duration": 123, "units": "C",
  "info": "A quote: \"hot\"\nUnicode: θερμοκρασία"
}`

func TestRenderKafka(t *testing.T) {
	for name, test := range map[string]struct {
		change func(*notifyevent.Event)
		fields map[string]any
	}{
		"complete": {},
		"critical": {change: func(e *notifyevent.Event) { e.Status, e.PreviousStatus = "CRITICAL", "WARNING" }, fields: map[string]any{"status": "CRITICAL", "old_status": "WARNING"}},
		"clear":    {change: func(e *notifyevent.Event) { e.Status, e.PreviousStatus = "CLEAR", "CRITICAL" }, fields: map[string]any{"status": "CLEAR", "old_status": "CRITICAL"}},
		"missing facts": {change: func(e *notifyevent.Event) {
			e.Chart, e.PreviousStatus, e.Units, e.Info = "", "", "", ""
			e.Value, e.PreviousValue, e.Duration, e.NonClearDuration = nil, nil, nil, nil
		}, fields: map[string]any{"chart": "", "old_status": "", "units": "", "info": "", "value": nil, "old_value": nil, "duration": nil, "non_clear_duration": nil}},
		"maximum seconds": {change: func(e *notifyevent.Event) {
			e.Duration, e.NonClearDuration = new(uint32(4294967295)), new(uint32(4294967295))
		}, fields: map[string]any{"duration": uint32(4294967295), "non_clear_duration": uint32(4294967295)}},
		"timezone and subsecond": {change: func(e *notifyevent.Event) {
			e.Timestamp = time.Date(2026, 9, 14, 15, 0, 0, 999999999, time.FixedZone("offset", 10800))
		}},
		"escaped fields": {change: func(e *notifyevent.Event) {
			e.Alert, e.Chart, e.Info, e.Units = "alert\"\\\n", "chart\t\x00", "<tag>& Καλημέρα\r\n", "\u2028"
		}, fields: map[string]any{"name": "alert\"\\\n", "chart": "chart\t\x00", "info": "<tag>& Καλημέρα\r\n", "units": "\u2028"}},
	} {
		t.Run(name, func(t *testing.T) {
			event := testutil.ExpectedEvent()
			event.Duration, event.NonClearDuration = new(uint32(0)), new(uint32(123))
			if test.change != nil {
				test.change(&event)
			}
			got, err := json.Marshal(renderKafka(Config{SenderIP: "192.0.2.1"}, event))
			require.NoError(t, err)
			var want map[string]any
			require.NoError(t, json.Unmarshal([]byte(kafkaFullJSON), &want))
			for key, value := range test.fields {
				want[key] = value
			}
			wantJSON, err := json.Marshal(want)
			require.NoError(t, err)
			assert.JSONEq(t, string(wantJSON), string(got))
		})
	}
}
