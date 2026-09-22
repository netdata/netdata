// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || darwin

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/commandexec"
	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunSyslog(t *testing.T) {
	for name, test := range map[string]struct {
		status, mode string
		remote       bool
		code         int
		priority     string
		change       func(map[string]any, *notifyevent.Event)
		message      string
	}{
		"warning":         {status: "WARNING", priority: "local6.warning"},
		"critical":        {status: "CRITICAL", priority: "local6.crit"},
		"clear":           {status: "CLEAR", priority: "local6.info"},
		"remote override": {status: "WARNING", remote: true, priority: "daemon.notice"},
		"failure":         {status: "WARNING", mode: "fail", code: 1, priority: "local6.warning"},
		"escaped controls": {
			status: "WARNING", priority: "local6.warning",
			change: func(dst map[string]any, event *notifyevent.Event) {
				dst["prefix"] = "alert\n"
				event.Node, event.Chart, event.Units = "node\x1b[31m", "chart\r", "C\u0085\u2028\u2029"
			},
			message: `alert\n WARNING on node\x1b[31m at 2026-09-14T12:00:00Z: chart\r 42.5 C\u0085\u2028\u2029`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			mode := test.mode
			if mode == "" {
				mode = "record"
			}
			dst, capture := testCommandDestination(t, mode)
			dst["type"] = "syslog"
			wantArgs := []string{"-p", test.priority}
			if test.remote {
				dst["host"], dst["port"], dst["facility"], dst["level"] = "logs.example.org", 1514, "daemon", "notice"
				dst["args"] = []string{"--tcp"}
				wantArgs = append(wantArgs, "-n", "logs.example.org", "-P", "1514", "--tcp")
			}
			event := testutil.ExpectedEvent()
			event.Status = test.status
			if test.change != nil {
				test.change(dst, &event)
			}
			message := test.message
			if message == "" {
				message = "netdata " + test.status + " on test-node at 2026-09-14T12:00:00Z: test.chart 42.5 C"
			}
			wantArgs = append(wantArgs, "--", message)
			data, err := json.Marshal(event)
			require.NoError(t, err)
			cfg := testConfig{Version: 1, Destinations: map[string]map[string]any{"target": dst}}
			var stdout, stderr bytes.Buffer
			code := Run(context.Background(), []string{"send", "--config", writeCommandConfig(t, cfg), "--destination", "target"}, bytes.NewReader(data), &stdout, &stderr)
			assert.Equal(t, test.code, code, stderr.String())
			assert.Empty(t, stdout.String())
			assert.NotContains(t, stderr.String(), "synthetic-private-value")
			if test.code != 0 {
				assert.Contains(t, stderr.String(), "status 7")
			}
			wantEnv := []string{"PATH=" + commandexec.DefaultPath}
			for k, v := range dst["env"].(map[string]string) {
				wantEnv = append(wantEnv, k+"="+v)
			}
			slices.Sort(wantEnv)
			assert.Equal(t, []commandCapture{{Args: wantArgs, Env: wantEnv, Input: ""}}, readCommandCaptures(t, capture))
		})
	}
}

func TestRunSyslogIsolation(t *testing.T) {
	for name, test := range map[string]struct {
		mode string
		code int
	}{
		"remote then local": {mode: "record"}, "last target fails": {mode: "fail"},
	} {
		t.Run(name, func(t *testing.T) {
			remote, remoteCapture := testCommandDestination(t, "record")
			remote["type"], remote["host"] = "syslog", "logs.example.org"
			local, localCapture := testCommandDestination(t, test.mode)
			local["type"] = "syslog"
			cfg := testConfig{Version: 1, Destinations: map[string]map[string]any{"remote": remote, "local": local}, Routing: notifier.Routing{Roles: map[string][]string{"ops": {"remote", "local", "local"}}}}
			var stdout, stderr bytes.Buffer
			assert.Equal(t, test.code, Run(context.Background(), []string{"send", "--config", writeCommandConfig(t, cfg), "--role", "ops"}, strings.NewReader(testutil.ValidEvent), &stdout, &stderr), stderr.String())
			assert.Empty(t, stdout.String())
			message := "netdata WARNING on test-node at 2026-09-14T12:00:00Z: test.chart 42.5 C"
			for name, test := range map[string]struct {
				path string
				args []string
			}{
				"remote": {remoteCapture, []string{"-p", "local6.warning", "-n", "logs.example.org", "--", message}},
				"local":  {localCapture, []string{"-p", "local6.warning", "--", message}},
			} {
				t.Run(name, func(t *testing.T) {
					captures := readCommandCaptures(t, test.path)
					require.Len(t, captures, 1)
					assert.Equal(t, test.args, captures[0].Args)
				})
			}
			if test.mode == "fail" {
				assert.Contains(t, stderr.String(), "1 succeeded, 1 failed")
			}
		})
	}
}

func TestRunSyslogNoLaunch(t *testing.T) {
	for name, test := range map[string]struct {
		validate, unselected, nul bool
		secret                    string
		code                      int
		message                   string
	}{
		"validate missing secret":   {validate: true, secret: "${env:NOTIFIER_TEST_UNSET_SYSLOG_SECRET}", message: "configuration is valid"},
		"unselected missing secret": {unselected: true, secret: "${env:NOTIFIER_TEST_UNSET_SYSLOG_SECRET}", message: "0 succeeded"},
		"selected missing secret":   {secret: "${env:NOTIFIER_TEST_UNSET_SYSLOG_SECRET}", code: 1, message: "not set"},
		"message NUL":               {nul: true, code: 1, message: "syslog message must not contain NUL"},
	} {
		t.Run(name, func(t *testing.T) {
			dst, capture := testCommandDestination(t, "record")
			dst["type"] = "syslog"
			if test.secret != "" {
				dst["env"].(map[string]string)["TOKEN"] = test.secret
			}
			cfg := testConfig{Version: 1, Destinations: map[string]map[string]any{"target": dst}}
			args := []string{"send", "--config", writeCommandConfig(t, cfg)}
			if test.validate {
				args[0] = "validate"
			} else if test.unselected {
				args = append(args, "--role", "silent")
			} else {
				args = append(args, "--destination", "target")
			}
			event := testutil.ExpectedEvent()
			if test.nul {
				event.Node = "synthetic-private-value\x00"
			}
			data, err := json.Marshal(event)
			require.NoError(t, err)
			var stdout, stderr bytes.Buffer
			assert.Equal(t, test.code, Run(context.Background(), args, bytes.NewReader(data), &stdout, &stderr), stderr.String())
			assert.Contains(t, stdout.String()+stderr.String(), test.message)
			assert.NotContains(t, stderr.String(), "synthetic-private-value")
			_, err = os.Stat(capture)
			assert.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}
