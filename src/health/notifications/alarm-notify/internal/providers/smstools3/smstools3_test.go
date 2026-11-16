// SPDX-License-Identifier: GPL-3.0-or-later
package smstools3

import (
	"strings"
	"testing"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderSMSTools3(t *testing.T) {
	for name, test := range map[string]struct {
		change func(*notifyevent.Event)
		want   string
	}{
		"warning":                 {want: "test-node needs attention: test.chart, Temperature is high = 42.5 C"},
		"critical":                {change: func(e *notifyevent.Event) { e.Status = "CRITICAL" }, want: "test-node is critical: test.chart, Temperature is high = 42.5 C"},
		"clear omits value":       {change: func(e *notifyevent.Event) { e.Status = "CLEAR" }, want: "test-node recovered: test.chart, Temperature is high"},
		"missing chart and value": {change: func(e *notifyevent.Event) { e.Chart, e.Value = "", nil }, want: "test-node needs attention: Temperature is high"},
		"zero no units":           {change: func(e *notifyevent.Event) { e.Value, e.Units = new(float64), "" }, want: "test-node needs attention: test.chart, Temperature is high = 0"},
		"underscores":             {change: func(e *notifyevent.Event) { e.Summary = "high_temperature" }, want: "test-node needs attention: test.chart, high temperature = 42.5 C"},
		"Unicode at cap":          {change: func(e *notifyevent.Event) { e.Node = strings.Repeat("界", 160) }, want: strings.Repeat("界", 160)},
		"Unicode over cap":        {change: func(e *notifyevent.Event) { e.Node = strings.Repeat("界", 161) }, want: strings.Repeat("界", 160)},
		"ASCII over cap":          {change: func(e *notifyevent.Event) { e.Node = strings.Repeat("a", 161) }, want: strings.Repeat("a", 160)},
	} {
		t.Run(name, func(t *testing.T) {
			event := testutil.ExpectedEvent()
			if test.change != nil {
				test.change(&event)
			}
			assert.Equal(t, test.want, renderSMSTools3(event))
		})
	}
}

func TestConfig(t *testing.T) {
	for name, test := range map[string]struct {
		to      string
		invalid bool
	}{
		"SMS phone": {to: "+15005550009"}, "SMS missing phone": {invalid: true}, "SMS option injection": {to: "-Asynthetic-private-value", invalid: true}, "SMS header injection": {to: "123\nTo: synthetic-private-value", invalid: true},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := New(Config{Executable: "/usr/bin/sendsms", To: test.to}, nil)
			if test.invalid {
				require.ErrorContains(t, err, "phone number")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConstructorOwnsConfiguration(t *testing.T) {
	original := func() Config {
		return Config{Executable: "/usr/bin/helper", To: "15005550009", Env: map[string]string{"TOKEN": "literal"}}
	}
	for name, mutate := range map[string]func(*Config){
		"environment": func(c *Config) { c.Env["TOKEN"] = "changed" },
	} {
		t.Run(name, func(t *testing.T) {
			config := original()
			sender, err := New(config, nil)
			require.NoError(t, err)
			mutate(&config)
			assert.Equal(t, original(), sender.config)
		})
	}
}
