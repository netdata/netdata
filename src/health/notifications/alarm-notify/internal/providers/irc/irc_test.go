// SPDX-License-Identifier: GPL-3.0-or-later

package irc

import (
	"bufio"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testIRCConfig(t *testing.T) Config {
	t.Helper()
	return Config{Executable: filepath.Join(t.TempDir(), "nc"), Host: "irc.example.com", Nickname: "netdata-alert", Realname: "Netdata alerts", Channel: "#alerts"}
}

func TestIRCConfig(t *testing.T) {
	for name, test := range map[string]struct {
		change func(*Config)
		err    string
	}{
		"defaults":               {},
		"IPv6":                   {change: func(d *Config) { d.Host = "2001:db8::1"; d.Port = new(field.Integer(16667)) }},
		"realname with unicode":  {change: func(d *Config) { d.Realname = "Netdata ειδοποιήσεις" }},
		"local channel":          {change: func(d *Config) { d.Channel = "&local" }},
		"channel mask":           {change: func(d *Config) { d.Channel = "#alerts:*.example.com" }},
		"explicit env":           {change: func(d *Config) { d.Env = map[string]string{"TOKEN": "${env:UNREAD_IRC_TOKEN}"} }},
		"no executable":          {change: func(d *Config) { d.Executable = "" }, err: "absolute path"},
		"no host":                {change: func(d *Config) { d.Host = "" }, err: "irc host"},
		"host option":            {change: func(d *Config) { d.Host = "-l" }, err: "irc host"},
		"host reference":         {change: func(d *Config) { d.Host = "${env:HOST}" }, err: "irc host"},
		"port zero":              {change: func(d *Config) { d.Port = new(field.Integer(0)) }, err: "irc port"},
		"port overflow":          {change: func(d *Config) { d.Port = new(field.Integer(65536)) }, err: "irc port"},
		"missing nickname":       {change: func(d *Config) { d.Nickname = "" }, err: "irc nickname"},
		"nickname injection":     {change: func(d *Config) { d.Nickname = "nick\r\nQUIT" }, err: "irc nickname"},
		"nickname leading digit": {change: func(d *Config) { d.Nickname = "1nick" }, err: "irc nickname"},
		"nickname line limit":    {change: func(d *Config) { d.Nickname = strings.Repeat("n", 507) }, err: "irc nickname"},
		"no realname":            {change: func(d *Config) { d.Realname = "" }, err: "irc realname"},
		"realname NUL":           {change: func(d *Config) { d.Realname = "name\x00" }, err: "irc realname"},
		"realname line limit":    {change: func(d *Config) { d.Realname = strings.Repeat("a", 510) }, err: "irc realname"},
		"no channel":             {change: func(d *Config) { d.Channel = "" }, err: "irc channel"},
		"channel list":           {change: func(d *Config) { d.Channel = "#one,#two" }, err: "irc channel"},
		"channel key":            {change: func(d *Config) { d.Channel = "#one secret" }, err: "irc channel"},
		"channel injection":      {change: func(d *Config) { d.Channel = "#one\r\nQUIT" }, err: "irc channel"},
		"channel length":         {change: func(d *Config) { d.Channel = "#" + strings.Repeat("a", 50) }, err: "irc channel"},
		"nickname target":        {change: func(d *Config) { d.Channel = "somebody" }, err: "irc channel"},
	} {
		t.Run(name, func(t *testing.T) {
			dst := testIRCConfig(t)
			if test.change != nil {
				test.change(&dst)
			}
			checkConfig(t, dst, test.err)
		})
	}
}

func TestRenderIRC(t *testing.T) {
	for name, test := range map[string]struct{ info, want string }{
		"literal escapes": {`literal\nPRIVMSG #other :text`, `literal\nPRIVMSG #other :text`},
		"newlines":        {"one\r\ntwo\rthree\nfour", "one, two, three, four"},
		"controls":        {"CTCP \x01ACTION\x01 NUL\x00 tab\t escape\x1b", "CTCP \\x01ACTION\\x01 NUL\\x00 tab\\t escape\\x1b"},
		"unicode":         {"温度 θερμοκρασία", "温度 θερμοκρασία"},
	} {
		t.Run(name, func(t *testing.T) {
			event := testutil.ExpectedEvent()
			event.Info = test.info
			assert.Equal(t, "Temperature is high, "+test.want+", Node: test-node, Alert: test_alert, Status: CLEAR → WARNING, Chart: test.chart, Context: test.context, Value: 42.5 C, Previous value: 0 C, Time: 2026-09-14T12:00:00Z", renderIRC(event))
		})
	}
}

func TestReadIRC(t *testing.T) {
	for name, test := range map[string]struct {
		line string
		want ircMessage
		err  string
	}{
		"welcome":          {line: ":server 001 nick :Welcome to IRC\r\n", want: ircMessage{prefix: "server", command: "001", params: []string{"nick", "Welcome to IRC"}}},
		"ping":             {line: "PING :opaque : token\r\n", want: ircMessage{command: "PING", params: []string{"opaque : token"}}},
		"LF compatibility": {line: "PING :token\n", want: ircMessage{command: "PING", params: []string{"token"}}},
		"tagged join":      {line: "@id=123;time=test :nick!u@host JOIN :#alerts\r\n", want: ircMessage{prefix: "nick!u@host", command: "JOIN", params: []string{"#alerts"}}},
		"long tags":        {line: "@x=" + strings.Repeat("x", 7000) + " :server NOTICE nick :hello\r\n", want: ircMessage{prefix: "server", command: "NOTICE", params: []string{"nick", "hello"}}},
		"empty line":       {line: "\r\n"},
		"NUL":              {line: "PING :a\x00b\r\n", err: "invalid protocol framing"},
		"embedded CR":      {line: "PING :a\rb\r\n", err: "invalid protocol framing"},
		"overlong":         {line: "PING :" + strings.Repeat("x", 506) + "\r\n", err: "overlong protocol line"},
		"oversized tags":   {line: "@x=" + strings.Repeat("x", 8190) + " PING :x\r\n", err: "invalid message tags"},
		"unterminated":     {line: "PING :partial", err: "complete bounded protocol line"},
		"unbounded":        {line: strings.Repeat("x", 10000), err: "complete bounded protocol line"},
		"no command":       {line: ":server \r\n", err: "invalid protocol command"},
		"bad command":      {line: "12X nick :error\r\n", err: "invalid protocol command"},
		"bad prefix":       {line: ":server\r\n", err: "invalid protocol prefix"},
		"too many params":  {line: "CMD " + strings.Repeat("arg ", 16) + "\r\n", err: "too many protocol parameters"},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := readIRC(bufio.NewReaderSize(strings.NewReader(test.line), 8192+512))
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				assert.Equal(t, ircMessage{}, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
	_, err := readIRC(bufio.NewReader(strings.NewReader("")))
	require.ErrorIs(t, err, io.EOF)
}

func TestIRCCaseMapping(t *testing.T) {
	for name, test := range map[string]struct{ mapping, want string }{"ascii": {"ascii", "nick[\\]^"}, "strict": {"strict-rfc1459", "nick{|}^"}, "rfc": {"rfc1459", "nick{|}~"}} {
		t.Run(name, func(t *testing.T) { assert.Equal(t, test.want, ircFold("Nick[\\]^", test.mapping)) })
	}
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
		return Config{Executable: "/usr/bin/helper", Host: "irc.example.com", Port: new(field.Integer(6667)), Nickname: "notify", Realname: "Netdata", Channel: "#alerts", Env: map[string]string{"TOKEN": "literal"}}
	}
	for name, mutate := range map[string]func(*Config){
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
