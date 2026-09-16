// SPDX-License-Identifier: GPL-3.0-or-later

package syslog

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyslogConfig(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	for name, test := range map[string]struct {
		change func(*Config)
		err    string
	}{
		"defaults": {},
		"full": {change: func(d *Config) {
			d.Facility, d.Level, d.Prefix = "daemon", "notice", "Netdata alerts"
			d.Host, d.Port = "logs.example.org", new(field.Integer(1514))
			d.Args, d.Env = []string{"--tcp", "--rfc3164"}, map[string]string{"TOKEN": "${env:UNREAD_SYSLOG_SECRET}"}
		}},
		"IPv4":                {change: func(d *Config) { d.Host = "192.0.2.1" }},
		"IPv6":                {change: func(d *Config) { d.Host = "2001:db8::1" }},
		"IPv6 zone":           {change: func(d *Config) { d.Host = "fe80::1%eth0" }},
		"aliases":             {change: func(d *Config) { d.Facility, d.Level = "security", "warn" }},
		"relative executable": {change: func(d *Config) { d.Executable = "synthetic-private-value" }, err: "absolute path"},
		"facility":            {change: func(d *Config) { d.Facility = "synthetic-private-value" }, err: "facility"},
		"level":               {change: func(d *Config) { d.Level = "synthetic-private-value" }, err: "severity"},
		"uppercase":           {change: func(d *Config) { d.Level = "WARNING" }, err: "lowercase"},
		"prefix NUL":          {change: func(d *Config) { d.Prefix = "synthetic-private-value\x00" }, err: "NUL"},
		"arg NUL":             {change: func(d *Config) { d.Args = []string{"synthetic-private-value\x00"} }, err: "NUL"},
		"port needs host":     {change: func(d *Config) { d.Port = new(field.Integer(514)) }, err: "requires host"},
		"port zero":           {change: func(d *Config) { d.Host, d.Port = "localhost", new(field.Integer(0)) }, err: "1 to 65535"},
		"port negative":       {change: func(d *Config) { d.Host, d.Port = "localhost", new(field.Integer(-1)) }, err: "1 to 65535"},
		"port overflow":       {change: func(d *Config) { d.Host, d.Port = "localhost", new(field.Integer(65536)) }, err: "1 to 65535"},
		"max port":            {change: func(d *Config) { d.Host, d.Port = "localhost", new(field.Integer(65535)) }},
	} {
		t.Run(name, func(t *testing.T) {
			dst := Config{Executable: executable}
			if test.change != nil {
				test.change(&dst)
			}
			checkConfig(t, dst, test.err)
		})
	}
	for name, value := range map[string]string{
		"host and port": "localhost:514", "bracketed IP": "[2001:db8::1]", "URL": "https://example.com",
		"option": "--synthetic-private-value", "whitespace": "two hosts", "control": "host\x00", "reference": "${env:HOST}",
	} {
		t.Run(name, func(t *testing.T) {
			checkConfig(t, Config{Executable: executable, Host: value}, "syslog host")
		})
	}
	for name, value := range map[string]string{"fraction": "514.5", "quoted": "'514'", "bool": "true", "overflow": "9223372036854775808"} {
		t.Run("port/"+name, func(t *testing.T) {
			_, err := readConfig(strings.NewReader("version: 1\ndestinations:\n  test:\n    type: syslog\n    port: " + value))
			require.ErrorContains(t, err, "invalid YAML")
		})
	}
}

func TestRenderSyslog(t *testing.T) {
	for name, test := range map[string]struct {
		dst    Config
		change func(*notifyevent.Event)
		want   []string
		err    string
	}{
		"warning":                     {want: []string{"-p", "local6.warning", "--", "netdata WARNING on test-node at 2026-09-14T12:00:00Z: test.chart 42.5 C"}},
		"critical":                    {change: func(e *notifyevent.Event) { e.Status = "CRITICAL" }, want: []string{"-p", "local6.crit", "--", "netdata CRITICAL on test-node at 2026-09-14T12:00:00Z: test.chart 42.5 C"}},
		"clear retains current value": {change: func(e *notifyevent.Event) { e.Status = "CLEAR" }, want: []string{"-p", "local6.info", "--", "netdata CLEAR on test-node at 2026-09-14T12:00:00Z: test.chart 42.5 C"}},
		"overrides":                   {dst: Config{Facility: "daemon", Level: "notice", Prefix: "Alert", Host: "2001:db8::1", Port: new(field.Integer(1514)), Args: []string{"--tcp", "--rfc3164"}}, want: []string{"-p", "daemon.notice", "-n", "2001:db8::1", "-P", "1514", "--tcp", "--rfc3164", "--", "Alert WARNING on test-node at 2026-09-14T12:00:00Z: test.chart 42.5 C"}},
		"remote default port":         {dst: Config{Host: "logs.example.org"}, want: []string{"-p", "local6.warning", "-n", "logs.example.org", "--", "netdata WARNING on test-node at 2026-09-14T12:00:00Z: test.chart 42.5 C"}},
		"leading option text":         {dst: Config{Prefix: "--file=/synthetic-path"}, want: []string{"-p", "local6.warning", "--", "--file=/synthetic-path WARNING on test-node at 2026-09-14T12:00:00Z: test.chart 42.5 C"}},
		"missing chart and value":     {change: func(e *notifyevent.Event) { e.Chart, e.Value = "", nil }, want: []string{"-p", "local6.warning", "--", "netdata WARNING on test-node at 2026-09-14T12:00:00Z:"}},
		"zero no units":               {change: func(e *notifyevent.Event) { e.Value, e.Units = new(float64), "" }, want: []string{"-p", "local6.warning", "--", "netdata WARNING on test-node at 2026-09-14T12:00:00Z: test.chart 0"}},
		"Unicode":                     {change: func(e *notifyevent.Event) { e.Node = "κόμβος" }, want: []string{"-p", "local6.warning", "--", "netdata WARNING on κόμβος at 2026-09-14T12:00:00Z: test.chart 42.5 C"}},
		"prefix controls":             {dst: Config{Prefix: "alert\x01\a\b\t\n\v\f\r\x1b\x1f\x7f"}, want: []string{"-p", "local6.warning", "--", `alert\x01\a\b\t\n\v\f\r\x1b\x1f\x7f WARNING on test-node at 2026-09-14T12:00:00Z: test.chart 42.5 C`}},
		"node newline and escape":     {change: func(e *notifyevent.Event) { e.Node = "node\n<134>forged\x1b[31m" }, want: []string{"-p", "local6.warning", "--", `netdata WARNING on node\n<134>forged\x1b[31m at 2026-09-14T12:00:00Z: test.chart 42.5 C`}},
		"chart C1 controls":           {change: func(e *notifyevent.Event) { e.Chart = "chart\u0080\u0085\u009b\u009f" }, want: []string{"-p", "local6.warning", "--", `netdata WARNING on test-node at 2026-09-14T12:00:00Z: chart\u0080\u0085\u009b\u009f 42.5 C`}},
		"units line separators":       {change: func(e *notifyevent.Event) { e.Units = "C\u2028\u2029" }, want: []string{"-p", "local6.warning", "--", `netdata WARNING on test-node at 2026-09-14T12:00:00Z: test.chart 42.5 C\u2028\u2029`}},
		"literal punctuation":         {dst: Config{Prefix: `alert\n "quoted" 'text'`}, want: []string{"-p", "local6.warning", "--", `alert\n "quoted" 'text' WARNING on test-node at 2026-09-14T12:00:00Z: test.chart 42.5 C`}},
		"node NUL":                    {change: func(e *notifyevent.Event) { e.Node = "node\x00" }, err: "syslog message must not contain NUL"},
		"chart NUL":                   {change: func(e *notifyevent.Event) { e.Chart = "chart\x00" }, err: "syslog message must not contain NUL"},
		"units NUL":                   {change: func(e *notifyevent.Event) { e.Units = "C\x00" }, err: "syslog message must not contain NUL"},
		"unused info NUL":             {change: func(e *notifyevent.Event) { e.Info = "info\x00" }, want: []string{"-p", "local6.warning", "--", "netdata WARNING on test-node at 2026-09-14T12:00:00Z: test.chart 42.5 C"}},
	} {
		t.Run(name, func(t *testing.T) {
			event := testutil.ExpectedEvent()
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

func readConfig(r io.Reader) (testutil.Document[Config], error) {
	return testutil.ReadConfig(r, "syslog", func(c Config) error { _, err := New(c, nil); return err })
}

func checkConfig(t *testing.T, c Config, want string) {
	t.Helper()
	_, err := New(c, nil)
	if want != "" {
		require.ErrorContains(t, err, want)
		assert.NotContains(t, err.Error(), "synthetic-private-value")
	} else {
		require.NoError(t, err)
	}
}

func TestConstructorOwnsConfiguration(t *testing.T) {
	original := func() Config {
		return Config{Executable: "/usr/bin/helper", Host: "logs.example.com", Port: new(field.Integer(514)), Args: []string{"--tcp"}, Env: map[string]string{"TOKEN": "literal"}}
	}
	for name, mutate := range map[string]func(*Config){
		"arguments":   func(c *Config) { c.Args[0] = "changed" },
		"environment": func(c *Config) { c.Env["TOKEN"] = "changed" },
		"port":        func(c *Config) { *c.Port = 1 },
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
