// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyslogConfig(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	for name, test := range map[string]struct {
		change func(*Destination)
		err    string
	}{
		"defaults": {},
		"full": {change: func(d *Destination) {
			d.Facility, d.Level, d.Prefix = "daemon", "notice", "Netdata alerts"
			d.Host, d.Port = "logs.example.org", new(configInteger(1514))
			d.Args, d.Env = []string{"--tcp", "--rfc3164"}, map[string]string{"TOKEN": "${env:UNREAD_SYSLOG_SECRET}"}
		}},
		"IPv4":                {change: func(d *Destination) { d.Host = "192.0.2.1" }},
		"IPv6":                {change: func(d *Destination) { d.Host = "2001:db8::1" }},
		"IPv6 zone":           {change: func(d *Destination) { d.Host = "fe80::1%eth0" }},
		"aliases":             {change: func(d *Destination) { d.Facility, d.Level = "security", "warn" }},
		"relative executable": {change: func(d *Destination) { d.Executable = "synthetic-private-value" }, err: "absolute path"},
		"facility":            {change: func(d *Destination) { d.Facility = "synthetic-private-value" }, err: "facility"},
		"level":               {change: func(d *Destination) { d.Level = "synthetic-private-value" }, err: "severity"},
		"uppercase":           {change: func(d *Destination) { d.Level = "WARNING" }, err: "lowercase"},
		"prefix NUL":          {change: func(d *Destination) { d.Prefix = "synthetic-private-value\x00" }, err: "NUL"},
		"arg NUL":             {change: func(d *Destination) { d.Args = []string{"synthetic-private-value\x00"} }, err: "NUL"},
		"port needs host":     {change: func(d *Destination) { d.Port = new(configInteger(514)) }, err: "requires host"},
		"port zero":           {change: func(d *Destination) { d.Host, d.Port = "localhost", new(configInteger(0)) }, err: "1 to 65535"},
		"port negative":       {change: func(d *Destination) { d.Host, d.Port = "localhost", new(configInteger(-1)) }, err: "1 to 65535"},
		"port overflow":       {change: func(d *Destination) { d.Host, d.Port = "localhost", new(configInteger(65536)) }, err: "1 to 65535"},
		"max port":            {change: func(d *Destination) { d.Host, d.Port = "localhost", new(configInteger(65535)) }},
	} {
		t.Run(name, func(t *testing.T) {
			dst := Destination{Type: "syslog", Executable: executable}
			if test.change != nil {
				test.change(&dst)
			}
			checkFormConfig(t, dst, test.err)
		})
	}
	for name, value := range map[string]string{
		"host and port": "localhost:514", "bracketed IP": "[2001:db8::1]", "URL": "https://example.com",
		"option": "--synthetic-private-value", "whitespace": "two hosts", "control": "host\x00", "reference": "${env:HOST}",
	} {
		t.Run(name, func(t *testing.T) {
			checkFormConfig(t, Destination{Type: "syslog", Executable: executable, Host: value}, "syslog host")
		})
	}
	for name, value := range map[string]string{"fraction": "514.5", "quoted": "'514'", "bool": "true", "overflow": "9223372036854775808"} {
		t.Run("port/"+name, func(t *testing.T) {
			_, err := readConfig(strings.NewReader("version: 1\ndestinations:\n  test:\n    type: syslog\n    port: " + value))
			require.ErrorContains(t, err, "invalid YAML")
		})
	}
}

func TestSyslogFieldIsolation(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	fields := reflect.TypeFor[Destination]()
	for i := 0; i < fields.NumField(); i++ {
		field := fields.Field(i)
		switch field.Name {
		case "Type", "Executable", "Args", "Env", "Facility", "Level", "Prefix", "Host", "Port":
			continue
		}
		t.Run(field.Name, func(t *testing.T) {
			dst := Destination{Type: "syslog", Executable: executable}
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
	for provider := range map[string]struct{}{"webhook": {}, "slack": {}, "discord": {}, "telegram": {}, "pushover": {}, "pushbullet": {}, "twilio": {}, "messagebird": {}, "gotify": {}, "ntfy": {}, "rocketchat": {}, "flock": {}, "fleep": {}, "ilert": {}, "signl4": {}, "alerta": {}, "dynatrace": {}, "prowl": {}, "kavenegar": {}, "smseagle": {}, "pagerduty": {}, "opsgenie": {}, "msteams": {}, "matrix": {}, "command": {}, "smstools3": {}} {
		for name, dst := range map[string]Destination{"facility": {Facility: "daemon"}, "level": {Level: "notice"}, "prefix": {Prefix: "netdata"}, "host": {Host: "localhost"}, "port": {Port: new(configInteger(514))}} {
			t.Run(provider+"/"+name, func(t *testing.T) {
				dst.Type = provider
				require.ErrorContains(t, dst.validate(), "fields for another provider")
			})
		}
	}
}

func TestRenderSyslog(t *testing.T) {
	for name, test := range map[string]struct {
		dst    Destination
		change func(*Event)
		want   []string
		err    string
	}{
		"warning":                     {want: []string{"-p", "local6.warning", "--", "netdata WARNING on test-node at 2026-09-14T12:00:00Z: test.chart 42.5 C"}},
		"critical":                    {change: func(e *Event) { e.Status = "CRITICAL" }, want: []string{"-p", "local6.crit", "--", "netdata CRITICAL on test-node at 2026-09-14T12:00:00Z: test.chart 42.5 C"}},
		"clear retains current value": {change: func(e *Event) { e.Status = "CLEAR" }, want: []string{"-p", "local6.info", "--", "netdata CLEAR on test-node at 2026-09-14T12:00:00Z: test.chart 42.5 C"}},
		"overrides":                   {dst: Destination{Facility: "daemon", Level: "notice", Prefix: "Alert", Host: "2001:db8::1", Port: new(configInteger(1514)), Args: []string{"--tcp", "--rfc3164"}}, want: []string{"-p", "daemon.notice", "-n", "2001:db8::1", "-P", "1514", "--tcp", "--rfc3164", "--", "Alert WARNING on test-node at 2026-09-14T12:00:00Z: test.chart 42.5 C"}},
		"remote default port":         {dst: Destination{Host: "logs.example.org"}, want: []string{"-p", "local6.warning", "-n", "logs.example.org", "--", "netdata WARNING on test-node at 2026-09-14T12:00:00Z: test.chart 42.5 C"}},
		"leading option text":         {dst: Destination{Prefix: "--file=/synthetic-path"}, want: []string{"-p", "local6.warning", "--", "--file=/synthetic-path WARNING on test-node at 2026-09-14T12:00:00Z: test.chart 42.5 C"}},
		"missing chart and value":     {change: func(e *Event) { e.Chart, e.Value = "", nil }, want: []string{"-p", "local6.warning", "--", "netdata WARNING on test-node at 2026-09-14T12:00:00Z:"}},
		"zero no units":               {change: func(e *Event) { e.Value, e.Units = new(float64), "" }, want: []string{"-p", "local6.warning", "--", "netdata WARNING on test-node at 2026-09-14T12:00:00Z: test.chart 0"}},
		"Unicode":                     {change: func(e *Event) { e.Node = "κόμβος" }, want: []string{"-p", "local6.warning", "--", "netdata WARNING on κόμβος at 2026-09-14T12:00:00Z: test.chart 42.5 C"}},
		"prefix controls":             {dst: Destination{Prefix: "alert\x01\a\b\t\n\v\f\r\x1b\x1f\x7f"}, want: []string{"-p", "local6.warning", "--", `alert\x01\a\b\t\n\v\f\r\x1b\x1f\x7f WARNING on test-node at 2026-09-14T12:00:00Z: test.chart 42.5 C`}},
		"node newline and escape":     {change: func(e *Event) { e.Node = "node\n<134>forged\x1b[31m" }, want: []string{"-p", "local6.warning", "--", `netdata WARNING on node\n<134>forged\x1b[31m at 2026-09-14T12:00:00Z: test.chart 42.5 C`}},
		"chart C1 controls":           {change: func(e *Event) { e.Chart = "chart\u0080\u0085\u009b\u009f" }, want: []string{"-p", "local6.warning", "--", `netdata WARNING on test-node at 2026-09-14T12:00:00Z: chart\u0080\u0085\u009b\u009f 42.5 C`}},
		"units line separators":       {change: func(e *Event) { e.Units = "C\u2028\u2029" }, want: []string{"-p", "local6.warning", "--", `netdata WARNING on test-node at 2026-09-14T12:00:00Z: test.chart 42.5 C\u2028\u2029`}},
		"literal punctuation":         {dst: Destination{Prefix: `alert\n "quoted" 'text'`}, want: []string{"-p", "local6.warning", "--", `alert\n "quoted" 'text' WARNING on test-node at 2026-09-14T12:00:00Z: test.chart 42.5 C`}},
		"node NUL":                    {change: func(e *Event) { e.Node = "node\x00" }, err: "syslog message must not contain NUL"},
		"chart NUL":                   {change: func(e *Event) { e.Chart = "chart\x00" }, err: "syslog message must not contain NUL"},
		"units NUL":                   {change: func(e *Event) { e.Units = "C\x00" }, err: "syslog message must not contain NUL"},
		"unused info NUL":             {change: func(e *Event) { e.Info = "info\x00" }, want: []string{"-p", "local6.warning", "--", "netdata WARNING on test-node at 2026-09-14T12:00:00Z: test.chart 42.5 C"}},
	} {
		t.Run(name, func(t *testing.T) {
			event := expectedEvent()
			if test.change != nil {
				test.change(&event)
			}
			got, err := renderSyslog(test.dst, event)
			if test.err != "" {
				require.EqualError(t, err, test.err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, test.want, got)
		})
	}
}
