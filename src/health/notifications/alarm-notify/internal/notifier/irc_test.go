// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"bufio"
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testIRCDestination(t *testing.T) Destination {
	t.Helper()
	return Destination{Type: "irc", Executable: filepath.Join(t.TempDir(), "nc"), Host: "irc.example.com", Nickname: "netdata-alert", Realname: "Netdata alerts", Channel: "#alerts"}
}

func TestIRCConfig(t *testing.T) {
	for name, test := range map[string]struct {
		change func(*Destination)
		err    string
	}{
		"defaults":               {},
		"IPv6":                   {change: func(d *Destination) { d.Host = "2001:db8::1"; d.Port = new(configInteger(16667)) }},
		"realname with unicode":  {change: func(d *Destination) { d.Realname = "Netdata ειδοποιήσεις" }},
		"local channel":          {change: func(d *Destination) { d.Channel = "&local" }},
		"channel mask":           {change: func(d *Destination) { d.Channel = "#alerts:*.example.com" }},
		"explicit env":           {change: func(d *Destination) { d.Env = map[string]string{"TOKEN": "${env:UNREAD_IRC_TOKEN}"} }},
		"no executable":          {change: func(d *Destination) { d.Executable = "" }, err: "absolute path"},
		"no host":                {change: func(d *Destination) { d.Host = "" }, err: "irc host"},
		"host option":            {change: func(d *Destination) { d.Host = "-l" }, err: "irc host"},
		"host reference":         {change: func(d *Destination) { d.Host = "${env:HOST}" }, err: "irc host"},
		"port zero":              {change: func(d *Destination) { d.Port = new(configInteger(0)) }, err: "irc port"},
		"port overflow":          {change: func(d *Destination) { d.Port = new(configInteger(65536)) }, err: "irc port"},
		"missing nickname":       {change: func(d *Destination) { d.Nickname = "" }, err: "irc nickname"},
		"nickname injection":     {change: func(d *Destination) { d.Nickname = "nick\r\nQUIT" }, err: "irc nickname"},
		"nickname leading digit": {change: func(d *Destination) { d.Nickname = "1nick" }, err: "irc nickname"},
		"nickname line limit":    {change: func(d *Destination) { d.Nickname = strings.Repeat("n", 507) }, err: "irc nickname"},
		"no realname":            {change: func(d *Destination) { d.Realname = "" }, err: "irc realname"},
		"realname NUL":           {change: func(d *Destination) { d.Realname = "name\x00" }, err: "irc realname"},
		"realname line limit":    {change: func(d *Destination) { d.Realname = strings.Repeat("a", 510) }, err: "irc realname"},
		"no channel":             {change: func(d *Destination) { d.Channel = "" }, err: "irc channel"},
		"channel list":           {change: func(d *Destination) { d.Channel = "#one,#two" }, err: "irc channel"},
		"channel key":            {change: func(d *Destination) { d.Channel = "#one secret" }, err: "irc channel"},
		"channel injection":      {change: func(d *Destination) { d.Channel = "#one\r\nQUIT" }, err: "irc channel"},
		"channel length":         {change: func(d *Destination) { d.Channel = "#" + strings.Repeat("a", 50) }, err: "irc channel"},
		"nickname target":        {change: func(d *Destination) { d.Channel = "somebody" }, err: "irc channel"},
	} {
		t.Run(name, func(t *testing.T) {
			dst := testIRCDestination(t)
			if test.change != nil {
				test.change(&dst)
			}
			checkFormConfig(t, dst, test.err)
		})
	}
}

func TestIRCFieldIsolation(t *testing.T) {
	fields := reflect.TypeFor[Destination]()
	allowed := map[string]bool{"Type": true, "Executable": true, "Env": true, "Host": true, "Port": true, "Nickname": true, "Realname": true, "Channel": true}
	for i := 0; i < fields.NumField(); i++ {
		field := fields.Field(i)
		if allowed[field.Name] {
			continue
		}
		t.Run(field.Name, func(t *testing.T) {
			dst := testIRCDestination(t)
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
	for provider := range map[string]struct{}{"email": {}, "kafka": {}, "awssns": {}, "syslog": {}, "command": {}, "smstools3": {}, "webhook": {}, "rocketchat": {}} {
		for field := range map[string]struct{}{"nickname": {}, "realname": {}} {
			t.Run(provider+"/"+field, func(t *testing.T) {
				dst := Destination{Type: provider}
				if field == "nickname" {
					dst.Nickname = "nick"
				} else {
					dst.Realname = "real"
				}
				checkFormConfig(t, dst, "nickname and realname require irc")
			})
		}
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
			event := expectedEvent()
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
