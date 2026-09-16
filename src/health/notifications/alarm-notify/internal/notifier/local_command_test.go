// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandConfig(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	for name, test := range map[string]struct {
		change func(*Destination)
		err    string
	}{
		"defaults":     {},
		"literal args": {change: func(d *Destination) { d.Args = []string{"", "a b", "$(literal)", "${env:LITERAL}", "line\nbreak"} }},
		"explicit env": {change: func(d *Destination) {
			d.Env = map[string]string{"EMPTY": "", "TOKEN": "${env:UNREAD_COMMAND_SECRET}", "PATH": "/usr/bin:/bin", "_1": "x"}
		}},
		"file reference": {change: func(d *Destination) {
			d.Env = map[string]string{"TOKEN": "${file:" + filepath.Join(t.TempDir(), "unread") + "}"}
		}},
		"missing executable":    {change: func(d *Destination) { d.Executable = "" }, err: "absolute path"},
		"relative executable":   {change: func(d *Destination) { d.Executable = "synthetic-private-value" }, err: "absolute path"},
		"executable reference":  {change: func(d *Destination) { d.Executable = "${env:UNREAD_COMMAND}" }, err: "literal"},
		"executable NUL":        {change: func(d *Destination) { d.Executable += "\x00synthetic-private-value" }, err: "NUL"},
		"argument NUL":          {change: func(d *Destination) { d.Args = []string{"synthetic-private-value\x00"} }, err: "NUL"},
		"empty env name":        {change: func(d *Destination) { d.Env = map[string]string{"": "synthetic-private-value"} }, err: "env names"},
		"leading digit":         {change: func(d *Destination) { d.Env = map[string]string{"1TOKEN": "x"} }, err: "env names"},
		"env equals":            {change: func(d *Destination) { d.Env = map[string]string{"KEY=synthetic-private-value": "x"} }, err: "env names"},
		"env NUL":               {change: func(d *Destination) { d.Env = map[string]string{"TOKEN": "synthetic-private-value\x00"} }, err: "NUL"},
		"invalid env reference": {change: func(d *Destination) { d.Env = map[string]string{"TOKEN": "${cmd:synthetic-private-value}"} }, err: "env and file"},
		"SMS phone":             {change: func(d *Destination) { d.Type, d.To = "smstools3", "+15005550009" }},
		"SMS missing phone":     {change: func(d *Destination) { d.Type = "smstools3" }, err: "phone number"},
		"SMS option injection":  {change: func(d *Destination) { d.Type, d.To = "smstools3", "-Asynthetic-private-value" }, err: "phone number"},
		"SMS header injection":  {change: func(d *Destination) { d.Type, d.To = "smstools3", "123\nTo: synthetic-private-value" }, err: "phone number"},
		"SMS extra args":        {change: func(d *Destination) { d.Type, d.To, d.Args = "smstools3", "123", []string{"extra"} }, err: "fields for another provider"},
	} {
		t.Run(name, func(t *testing.T) {
			dst := Destination{Type: "command", Executable: executable}
			if test.change != nil {
				test.change(&dst)
			}
			checkFormConfig(t, dst, test.err)
		})
	}
}

func TestCommandHostZones(t *testing.T) {
	for provider := range map[string]struct{}{"irc": {}, "syslog": {}} {
		for name, test := range map[string]struct {
			host    string
			invalid bool
		}{
			"named zone":            {host: "fe80::1%eth0"},
			"numeric zone":          {host: "fe80::1%2"},
			"interface punctuation": {host: "fe80::1%eth-0.1_2"},
			"no zone":               {host: "2001:db8::1"},
			"IPv4":                  {host: "192.0.2.1"},
			"hostname":              {host: "irc.example.com"},
			"empty zone":            {host: "fe80::1%", invalid: true},
			"slash":                 {host: "fe80::1%eth/0", invalid: true},
			"backslash":             {host: "fe80::1%eth\\0", invalid: true},
			"colon":                 {host: "fe80::1%eth0:extra", invalid: true},
			"open bracket":          {host: "fe80::1%[eth0", invalid: true},
			"close bracket":         {host: "fe80::1%eth0]", invalid: true},
			"at sign":               {host: "fe80::1%eth@0", invalid: true},
			"query":                 {host: "fe80::1%eth?0", invalid: true},
			"fragment":              {host: "fe80::1%eth#0", invalid: true},
			"extra zone marker":     {host: "fe80::1%eth%0", invalid: true},
			"reference":             {host: "fe80::1%${env:ZONE}", invalid: true},
			"space":                 {host: "fe80::1%eth 0", invalid: true},
			"control":               {host: "fe80::1%eth\x00", invalid: true},
		} {
			t.Run(provider+"/"+name, func(t *testing.T) {
				dst := Destination{Type: provider, Executable: filepath.Join(t.TempDir(), "helper"), Host: test.host}
				if provider == "irc" {
					dst.Nickname, dst.Realname, dst.Channel = "notify", "Netdata alerts", "#alerts"
				}
				wantErr := ""
				if test.invalid {
					wantErr = provider + " host"
				}
				checkFormConfig(t, dst, wantErr)
			})
		}
	}
}

func TestCommandFieldIsolation(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	for provider := range map[string]struct{}{"command": {}, "smstools3": {}} {
		fields := reflect.TypeFor[Destination]()
		for i := 0; i < fields.NumField(); i++ {
			field := fields.Field(i)
			if field.Name == "Type" || field.Name == "Executable" || field.Name == "Env" ||
				provider == "command" && field.Name == "Args" || provider == "smstools3" && field.Name == "To" {
				continue
			}
			t.Run(provider+"/"+field.Name, func(t *testing.T) {
				dst := Destination{Type: provider, Executable: executable}
				if provider == "smstools3" {
					dst.To = "15005550009"
				}
				v := reflect.ValueOf(&dst).Elem().Field(i)
				switch v.Kind() {
				case reflect.String:
					v.SetString("synthetic-private-value")
				case reflect.Slice:
					v.Set(reflect.ValueOf([]string{"synthetic-private-value"}))
				case reflect.Map:
					v.Set(reflect.ValueOf(map[string]string{"key": "synthetic-private-value"}))
				default:
					v.Set(reflect.New(v.Type().Elem()))
				}
				checkFormConfig(t, dst, "fields for another provider")
			})
		}
	}
	for provider := range map[string]struct{}{"webhook": {}, "slack": {}, "discord": {}, "telegram": {}, "pushover": {}, "pushbullet": {}, "twilio": {}, "messagebird": {}, "gotify": {}, "ntfy": {}, "rocketchat": {}, "flock": {}, "fleep": {}, "ilert": {}, "signl4": {}, "alerta": {}, "dynatrace": {}, "prowl": {}, "kavenegar": {}, "smseagle": {}, "pagerduty": {}, "opsgenie": {}, "msteams": {}, "matrix": {}} {
		for name, dst := range map[string]Destination{"executable": {Executable: executable}, "empty args": {Args: []string{}}, "empty env": {Env: map[string]string{}}} {
			t.Run(provider+"/"+name, func(t *testing.T) {
				dst.Type = provider
				require.ErrorContains(t, dst.validate(), "fields for another provider")
			})
		}
	}
}

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
			event := expectedEvent()
			if test.change != nil {
				test.change(&event)
			}
			assert.Equal(t, test.want, renderSMSTools3(event))
		})
	}
}
