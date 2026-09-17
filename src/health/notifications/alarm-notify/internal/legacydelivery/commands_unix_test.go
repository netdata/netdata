// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || darwin

package legacydelivery

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/commandexec"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type legacyCommandCapture struct {
	Args, Env, Lines []string
	Input            string
}

func TestMain(m *testing.M) {
	if mode := os.Getenv("LEGACY_TEST_HELPER"); mode != "" {
		capture := legacyCommandCapture{Args: os.Args[1:], Env: os.Environ()}
		if mode == "irc" {
			scan := bufio.NewScanner(os.Stdin)
			for scan.Scan() {
				line := scan.Text()
				capture.Lines = append(capture.Lines, line)
				switch {
				case strings.HasPrefix(line, "USER "):
					fmt.Print(":server 001 notify :welcome\r\n")
				case strings.HasPrefix(line, "JOIN "):
					fmt.Printf(":notify!u@h JOIN :%s\r\n", strings.TrimPrefix(line, "JOIN "))
				case strings.HasPrefix(line, "PING "):
					fmt.Printf(":server PONG %s\r\n", strings.TrimPrefix(line, "PING "))
				case strings.HasPrefix(line, "QUIT "):
					goto done
				}
			}
		} else {
			data, _ := io.ReadAll(os.Stdin)
			capture.Input = string(data)
		}
	done:
		f, err := os.OpenFile(os.Getenv("LEGACY_TEST_CAPTURE"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			os.Exit(90)
		}
		err = json.NewEncoder(f).Encode(capture)
		_ = f.Close()
		if err != nil {
			os.Exit(91)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func commandFixture(t *testing.T, dir, name, mode string) (string, string) {
	t.Helper()
	self, err := os.Executable()
	require.NoError(t, err)
	path, capture := filepath.Join(dir, name), filepath.Join(dir, name+".json")
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
	script := "#!/bin/sh\nLEGACY_TEST_HELPER=" + quote(mode) + " LEGACY_TEST_CAPTURE=" + quote(capture) + " exec " + quote(self) + " \"$@\"\n"
	require.NoError(t, os.WriteFile(path, []byte(script), 0700))
	return path, capture
}

func TestLegacyCommandDelivery(t *testing.T) {
	for name, tt := range map[string]struct {
		method, tool, settings string
		args                   []string
		captures               int
	}{
		"email auto finds sendmail without curl": {method: "email", tool: "sendmail", settings: `SEND_EMAIL=AUTO; EMAIL_SENDER='Netdata <notify@example.org>'; EMAIL_PLAINTEXT_ONLY=YES; EMAIL_THREADING=NO; role_recipients_email[ops]='root ops@example.org'`, args: []string{"-t", "-i", "-f", "notify@example.org"}, captures: 1},
		"sms each phone":                         {method: "sms", tool: "sendsms", settings: `role_recipients_sms[ops]='123 456 123'`, captures: 2},
		"syslog compound":                        {method: "syslog", tool: "logger", settings: `logger_options='--tag test'; SYSLOG_FACILITY=local3; role_recipients_syslog[ops]='daemon.notice@[2001:db8::1]:514/remote local'`, captures: 2},
		"IRC default port":                       {method: "irc", tool: "nc", settings: `IRC_NETWORK=irc.example.org; IRC_NICKNAME=notify; IRC_REALNAME='Netdata alerts'; role_recipients_irc[ops]='#ops'`, args: []string{"irc.example.org", "6667"}, captures: 1},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			mode := "capture"
			if tt.method == "irc" {
				mode = "irc"
			}
			_, capturePath := commandFixture(t, dir, tt.tool, mode)
			t.Setenv("PATH", dir)
			t.Setenv("SYNTHETIC_PRIVATE_AMBIENT", "must-not-inherit")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			runner := &commandexec.Runner{}
			defer runner.CloseAndWait()
			n := notifier.Notification{Event: testutil.ExpectedEvent()}
			config := "SEND_" + strings.ToUpper(tt.method) + "=YES; " + tt.settings
			plan, names, err := Prepare(ctx, parsedPrograms(t, config), []string{tt.method}, []string{"ops"}, n, http.DefaultClient, runner)
			require.NoError(t, err)
			var results []notifier.Result
			require.NoError(t, plan.Deliver(ctx, names, n, func(r notifier.Result) { results = append(results, r) }))
			expectedResults := make([]notifier.Result, tt.captures)
			for i := range expectedResults {
				expectedResults[i] = notifier.Result{Destination: fmt.Sprintf("%s-%d", tt.method, i+1)}
			}
			assert.Equal(t, expectedResults, results)
			f, err := os.Open(capturePath)
			require.NoError(t, err)
			defer f.Close()
			var captures []legacyCommandCapture
			dec := json.NewDecoder(f)
			for {
				var c legacyCommandCapture
				err := dec.Decode(&c)
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
				captures = append(captures, c)
			}
			require.Len(t, captures, tt.captures)
			for _, c := range captures {
				assert.Contains(t, c.Env, "PATH="+commandexec.DefaultPath)
				assert.NotContains(t, strings.Join(c.Env, "\n"), "SYNTHETIC_PRIVATE_AMBIENT")
				if tt.args != nil {
					assert.Equal(t, tt.args, c.Args)
				}
			}
			switch tt.method {
			case "email":
				message, err := mail.ReadMessage(strings.NewReader(captures[0].Input))
				require.NoError(t, err)
				assert.Equal(t, "root, <ops@example.org>", message.Header.Get("To"))
				assert.Equal(t, "text/plain; charset=UTF-8", message.Header.Get("Content-Type"))
				assert.Empty(t, message.Header.Get("In-Reply-To"))
				assert.Empty(t, message.Header.Get("References"))
			case "sms":
				for i, r := range []string{"123", "456"} {
					assert.Equal(t, []string{r, "test-node needs attention: test.chart, Temperature is high = 42.5 C"}, captures[i].Args)
				}
			case "syslog":
				assert.Equal(t, []string{"-p", "daemon.notice", "-n", "2001:db8::1", "-P", "514", "--tag", "test", "--", "remote WARNING on test-node at 2026-09-14T12:00:00Z: test.chart 42.5 C"}, captures[0].Args)
				assert.Equal(t, []string{"-p", "local3.warning", "--tag", "test", "--", "local WARNING on test-node at 2026-09-14T12:00:00Z: test.chart 42.5 C"}, captures[1].Args)
			case "irc":
				assert.Contains(t, captures[0].Lines, "NICK notify")
				assert.Contains(t, captures[0].Lines, "USER notify 0 * :Netdata alerts")
				assert.Contains(t, captures[0].Lines, "JOIN #ops")
				assert.Contains(t, captures[0].Lines, "QUIT :Netdata notification sent")
			}
		})
	}
}

func TestLegacyToolAvailability(t *testing.T) {
	for name, tt := range map[string]struct {
		config string
		want   []string
		err    string
	}{
		"missing automatic sendmail":               {config: `SEND_EMAIL=AUTO; DEFAULT_RECIPIENT_EMAIL=root`},
		"missing tool needs no history":            {config: `DEFAULT_RECIPIENT_SMS='123|critical'`},
		"missing sendsms":                          {config: `DEFAULT_RECIPIENT_SMS=123`},
		"ineligible missing explicit tool":         {config: `sendsms=/does/not/exist; DEFAULT_RECIPIENT_SMS='123|nowarn'`},
		"relative explicit path":                   {config: `sendsms=relative; DEFAULT_RECIPIENT_SMS=123`, err: "absolute path"},
		"PATH override cannot be silently ignored": {config: `PATH=/configured/path; DEFAULT_RECIPIENT_SMS=123`, err: "legacy setting PATH"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("PATH", t.TempDir())
			plan, names, err := Prepare(context.Background(), parsedPrograms(t, tt.config), nil, []string{"ops"}, notifier.Notification{Event: testutil.ExpectedEvent()}, http.DefaultClient, nil)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				assert.Equal(t, notifier.Plan{}, plan)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, names)
		})
	}
}
