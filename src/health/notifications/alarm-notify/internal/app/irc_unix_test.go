// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || darwin

package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/commandexec"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This is an owned protocol peer reached through the real process runner, never a live IRC network.
func ircCommandHelper(mode string) int {
	captured := commandCapture{Args: os.Args[1:], Env: os.Environ()}
	slices.Sort(captured.Env)
	record := func() bool {
		f, err := os.OpenFile(os.Getenv("NOTIFIER_TEST_CAPTURE"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return false
		}
		err = json.NewEncoder(f).Encode(captured)
		_ = f.Close()
		return err == nil
	}
	if !record() {
		return 80
	}
	input := bufio.NewReader(os.Stdin)
	read := func() string {
		line, err := input.ReadString('\n')
		if err != nil || !strings.HasSuffix(line, "\r\n") || len(line) > 512 || !utf8.ValidString(line) {
			return ""
		}
		captured.Input += line
		if !record() {
			return ""
		}
		return strings.TrimSuffix(line, "\r\n")
	}
	write := func(line string) { _, _ = fmt.Fprint(os.Stdout, line+"\r\n") }
	if read() != "NICK notify" || read() != "USER notify 0 * :Netdata alerts" {
		return 81
	}
	switch mode {
	case "irc-empty":
		return 0
	case "irc-exit":
		return 7
	case "irc-nick-error":
		write(":server 433 * notify :synthetic-private-value")
		return 0
	case "irc-bad-frame":
		write("PING :bad\x00token")
		return 0
	case "irc-oversized":
		write("NOTICE notify :" + strings.Repeat("synthetic-private-value", 1000))
		return 0
	case "irc-flood-ping":
		for {
			write("PING :" + strings.Repeat("x", 400))
		}
	}
	write("PING :server-token")
	if read() != "PONG :server-token" {
		return 82
	}
	write(":server 001 assigned :Welcome")
	write(":server 005 assigned CASEMAPPING=rfc1459 :supported")
	write(":server 422 assigned :No MOTD")
	if read() != "JOIN #alerts" {
		return 83
	}
	if mode == "irc-join-error" {
		write(":server 473 assigned #alerts :synthetic-private-value")
		return 0
	}
	write(":other!u@host JOIN :#alerts")
	write(":assigned!u@host NICK :changed")
	write("@id=test :CHANGED!u@host JOIN :#ALERTS")
	for {
		line := read()
		if line == "QUIT :Netdata notification sent" {
			if _, err := input.ReadByte(); err != io.EOF {
				return 84
			}
			write("PING :late-token")
			write("ERROR :Closing Link: synthetic-private-value")
			if mode == "irc-late-exit" {
				return 7
			}
			return 0
		}
		if !strings.HasPrefix(line, "PRIVMSG #alerts :") {
			return 85
		}
		ping := read()
		if !strings.HasPrefix(ping, "PING :") {
			return 86
		}
		token := strings.TrimPrefix(ping, "PING :")
		switch mode {
		case "irc-send-error":
			write(":server 404 changed #alerts :synthetic-private-value")
			return 0
		case "irc-kick":
			write(":operator!u@host KICK #alerts changed :synthetic-private-value")
			return 0
		case "irc-error":
			write("ERROR :synthetic-private-value")
			return 0
		case "irc-wrong-pong":
			write(":server PONG server :wrong-token")
			return 0
		}
		write("PING :during-send")
		if read() != "PONG :during-send" {
			return 87
		}
		write(":server PONG server :" + token)
	}
}

func ircHelperDestination(t *testing.T, mode string) (map[string]any, string) {
	t.Helper()
	dst, capture := testCommandDestination(t, mode)
	dst["type"], dst["host"], dst["nickname"], dst["realname"], dst["channel"] = "irc", "irc.example.com", "notify", "Netdata alerts", "#alerts"
	return dst, capture
}

func TestRunIRC(t *testing.T) {
	for name, test := range map[string]struct {
		mode, info string
		wantInfo   string
		code       int
		message    string
	}{
		"handshake and PING":          {mode: "irc-ok"},
		"long unicode":                {mode: "irc-ok", info: strings.Repeat("温度🙂", 900), wantInfo: strings.Repeat("温度🙂", 900)},
		"literal control text":        {mode: "irc-ok", info: "before\r\nQUIT\n\x01ACTION\x01\x00after\\nPRIVMSG #other :literal", wantInfo: `before, QUIT, \x01ACTION\x01\x00after\nPRIVMSG #other :literal`},
		"empty reply":                 {mode: "irc-empty", code: 1, message: "irc connection closed before exchange completed"},
		"failed process empty reply":  {mode: "irc-exit", code: 1, message: "irc connection closed before exchange completed; command exited with status 7"},
		"nickname rejection":          {mode: "irc-nick-error", code: 1, message: "numeric 433"},
		"join rejection":              {mode: "irc-join-error", code: 1, message: "numeric 473"},
		"send rejection":              {mode: "irc-send-error", code: 1, message: "numeric 404"},
		"kick":                        {mode: "irc-kick", code: 1, message: "removed from the channel"},
		"server error":                {mode: "irc-error", code: 1, message: "terminated the connection"},
		"wrong synchronization":       {mode: "irc-wrong-pong", code: 1, message: "closed before exchange completed"},
		"nonzero exit after exchange": {mode: "irc-late-exit", code: 1, message: "command exited with status 7"},
		"bad framing":                 {mode: "irc-bad-frame", code: 1, message: "invalid protocol framing"},
		"oversized reply":             {mode: "irc-oversized", code: 1, message: "bounded protocol line"},
	} {
		t.Run(name, func(t *testing.T) {
			dst, capture := ircHelperDestination(t, test.mode)
			t.Setenv("NOTIFIER_TEST_AMBIENT_SECRET", "must-not-be-inherited")
			t.Setenv("NOTIFIER_TEST_EXPLICIT_SECRET", "synthetic-private-value")
			dst["env"].(map[string]string)["TOKEN"] = "${env:NOTIFIER_TEST_EXPLICIT_SECRET}"
			event := testutil.ExpectedEvent()
			if test.info != "" {
				event.Info = test.info
			}
			input, err := json.Marshal(event)
			require.NoError(t, err)
			cfg := testConfig{Version: 1, Destinations: map[string]map[string]any{"target": dst}}
			var stdout, stderr bytes.Buffer
			require.Equal(t, test.code, Run(context.Background(), []string{"send", "--config", writeCommandConfig(t, cfg), "--destination", "target"}, bytes.NewReader(input), &stdout, &stderr), stderr.String())
			assert.Empty(t, stdout.String())
			assert.Contains(t, stderr.String(), test.message)
			for _, private := range []string{"synthetic-private-value", "must-not-be-inherited", dst["host"].(string), dst["channel"].(string), dst["executable"].(string)} {
				assert.NotContains(t, stderr.String(), private)
			}
			captures := readCommandCaptures(t, capture)
			require.NotEmpty(t, captures)
			got := captures[len(captures)-1]
			assert.Equal(t, commandCapture{Args: []string{"irc.example.com", "6667"}, Env: []string{"GORACE=atexit_sleep_ms=0", "NOTIFIER_TEST_CAPTURE=" + capture, "NOTIFIER_TEST_COMMAND_HELPER=" + test.mode, "PATH=" + commandexec.DefaultPath, "TOKEN=synthetic-private-value"}, Input: got.Input}, got)
			if test.code != 0 {
				return
			}
			lines := strings.Split(strings.TrimSuffix(got.Input, "\r\n"), "\r\n")
			require.GreaterOrEqual(t, len(lines), 8)
			assert.Equal(t, []string{"NICK notify", "USER notify 0 * :Netdata alerts", "PONG :server-token", "JOIN #alerts"}, lines[:4])
			assert.Equal(t, "QUIT :Netdata notification sent", lines[len(lines)-1])
			var content strings.Builder
			tokens := map[string]bool{}
			for pos := 4; pos < len(lines)-1; pos += 3 {
				require.Less(t, pos+2, len(lines)-1)
				require.True(t, strings.HasPrefix(lines[pos], "PRIVMSG #alerts :"))
				assert.LessOrEqual(t, len(lines[pos])+2, 512)
				assert.LessOrEqual(t, len(":CHANGED!u@host "+lines[pos])+2, 512)
				assert.True(t, utf8.ValidString(lines[pos]))
				body := strings.TrimPrefix(lines[pos], "PRIVMSG #alerts :")
				assert.NotContains(t, body, "\x00")
				assert.NotContains(t, body, "\x01")
				content.WriteString(body)
				assert.Regexp(t, `^PING :[A-Z2-7]{26}$`, lines[pos+1])
				assert.False(t, tokens[lines[pos+1]])
				tokens[lines[pos+1]] = true
				assert.Equal(t, "PONG :during-send", lines[pos+2])
			}
			wantInfo := test.wantInfo
			if wantInfo == "" {
				wantInfo = `A quote: "hot", Unicode: θερμοκρασία`
			}
			assert.Equal(t, "Temperature is high, "+wantInfo+", Node: test-node, Alert: test_alert, Status: CLEAR → WARNING, Chart: test.chart, Context: test.context, Value: 42.5 C, Previous value: 0 C, Time: 2026-09-14T12:00:00Z", content.String())
			if test.info == "" {
				assert.Equal(t, 8, len(lines))
			}
		})
	}
}

func TestRunIRCSelection(t *testing.T) {
	for name, test := range map[string]struct {
		selection, secret string
		validate          bool
		code, calls       int
		message           string
	}{
		"validate":               {validate: true, secret: "${env:NOTIFIER_TEST_MISSING_IRC_SECRET}", message: "configuration is valid"},
		"unused":                 {selection: "silent", secret: "${env:NOTIFIER_TEST_MISSING_IRC_SECRET}"},
		"any success and dedupe": {selection: "mixed", calls: 1, message: "1 succeeded, 1 failed"},
		"missing secret":         {secret: "${env:NOTIFIER_TEST_MISSING_IRC_SECRET}", code: 1, message: "not set"},
		"all failures":           {selection: "bad", code: 1, message: "all attempted destinations failed"},
	} {
		t.Run(name, func(t *testing.T) {
			dst, capture := ircHelperDestination(t, "irc-ok")
			if test.secret != "" {
				dst["env"].(map[string]string)["TOKEN"] = test.secret
			}
			bad := maps.Clone(dst)
			bad["executable"] = filepath.Join(t.TempDir(), "missing")
			bad["env"] = nil
			cfg := testConfig{Version: 1, Destinations: map[string]map[string]any{"target": dst, "bad": bad}, Routing: notifier.Routing{Roles: map[string][]string{"mixed": {"bad", "target", "target"}, "bad": {"bad"}}}}
			args := []string{"send", "--config", writeCommandConfig(t, cfg)}
			if test.validate {
				args[0] = "validate"
			} else if test.selection != "" {
				args = append(args, "--role", test.selection)
			} else {
				args = append(args, "--destination", "target")
			}
			var stdout, stderr bytes.Buffer
			assert.Equal(t, test.code, Run(context.Background(), args, strings.NewReader(testutil.ValidEvent), &stdout, &stderr), stderr.String())
			assert.Contains(t, stdout.String()+stderr.String(), test.message)
			if test.calls == 0 {
				_, err := os.Stat(capture)
				assert.ErrorIs(t, err, os.ErrNotExist)
				return
			}
			calls := 0
			for _, capture := range readCommandCaptures(t, capture) {
				if capture.Input == "" {
					calls++
				}
			}
			assert.Equal(t, test.calls, calls)
		})
	}
}

func TestRunIRCBlockedPipes(t *testing.T) {
	dst, capture := ircHelperDestination(t, "irc-flood-ping")
	cfg := testConfig{Version: 1, Destinations: map[string]map[string]any{"target": dst}}
	var stdout, stderr bytes.Buffer
	require.Equal(t, 1, Run(context.Background(), []string{"send", "--config", writeCommandConfig(t, cfg), "--destination", "target", "--timeout", "2s"}, strings.NewReader(testutil.ValidEvent), &stdout, &stderr))
	assert.Contains(t, stderr.String(), "timed out")
	captures := readCommandCaptures(t, capture)
	require.NotEmpty(t, captures)
	assert.Equal(t, "NICK notify\r\nUSER notify 0 * :Netdata alerts\r\n", captures[len(captures)-1].Input)
}
