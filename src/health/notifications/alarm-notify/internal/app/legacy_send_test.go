// SPDX-License-Identifier: GPL-3.0-or-later

package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunLegacyDelivery(t *testing.T) {
	const dynatrace = `SEND_DYNATRACE=YES; DYNATRACE_SPACE=space; DYNATRACE_SERVER=https://example.org; DYNATRACE_TOKEN=synthetic-private-value; DYNATRACE_TAG_VALUE=tag; DYNATRACE_EVENT=CUSTOM_INFO`
	for name, tt := range map[string]struct {
		overlay      string
		methods      []string
		input        string
		code, calls  int
		summary, err string
	}{
		"ordered files":               {overlay: `role_recipients_discord[ops]=selected; DEFAULT_RECIPIENT_DISCORD=disabled`, calls: 1, summary: "1 succeeded, 0 failed"},
		"all methods by default":      {calls: 1, summary: "1 succeeded, 0 failed"},
		"eligible gap rejects all":    {overlay: `SLACK_WEBHOOK_URL=https://example.org; DEFAULT_RECIPIENT_SLACK=channel`, code: 1, err: "legacy slack", summary: "0 succeeded, 0 failed"},
		"dynatrace gap rejects all":   {overlay: dynatrace, code: 1, err: "legacy dynatrace", summary: "0 succeeded, 0 failed"},
		"filter excludes dynatrace":   {overlay: dynatrace, methods: []string{"discord"}, calls: 1, summary: "1 succeeded, 0 failed"},
		"disabled dynatrace":          {overlay: dynatrace + "; SEND_DYNATRACE=NO", calls: 1, summary: "1 succeeded, 0 failed"},
		"filter excludes gap":         {overlay: `SLACK_WEBHOOK_URL=https://example.org; DEFAULT_RECIPIENT_SLACK=channel`, methods: []string{"discord"}, calls: 1, summary: "1 succeeded, 0 failed"},
		"missing history rejects all": {overlay: `DEFAULT_RECIPIENT_DISCORD='channel|critical'`, code: 1, err: "critical_seen_since_clear", summary: "0 succeeded, 0 failed"},
		"stateless skip":              {overlay: `DEFAULT_RECIPIENT_DISCORD='channel|nowarn|critical'`, summary: "0 succeeded, 0 failed"},
		"history permits":             {overlay: `DEFAULT_RECIPIENT_DISCORD='channel|critical'`, input: strings.Replace(testutil.ValidEvent, `"version": 1`, `"version": 1, "critical_seen_since_clear": true`, 1), calls: 1, summary: "1 succeeded, 0 failed"},
		"bad overlay":                 {overlay: `source synthetic-private-value`, code: 1, err: "file 2:", summary: "0 succeeded, 0 failed"},
		"curl customization":          {overlay: `curl_options='--header synthetic-private-value'`, code: 1, err: "curl_options", summary: "0 succeeded, 0 failed"},
		"invalid event":               {input: `{"version": "synthetic-private-value"}`, code: 1, err: "invalid JSON event", summary: "0 succeeded, 0 failed"},
		"unknown method":              {methods: []string{"synthetic-private-value"}, code: 1, err: "unknown legacy method", summary: "0 succeeded, 0 failed"},
	} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(200) }))
			defer server.Close()
			args := []string{"send-legacy", "--config", writeConfig(t, fmt.Sprintf("DISCORD_WEBHOOK_URL='%s/synthetic-private-value'; DEFAULT_RECIPIENT_DISCORD=channel", server.URL)), "--role", "ops", "--config", writeConfig(t, tt.overlay)}
			for _, method := range tt.methods {
				args = append(args, "--method", method)
			}
			input := tt.input
			if input == "" {
				input = testutil.ValidEvent
			}
			var stdout, stderr bytes.Buffer
			assert.Equal(t, tt.code, Run(context.Background(), args, strings.NewReader(input), &stdout, &stderr), stderr.String())
			assert.Empty(t, stdout.String())
			assert.Contains(t, stderr.String(), tt.summary)
			assert.Contains(t, stderr.String(), tt.err)
			assert.NotContains(t, stderr.String(), "synthetic-private-value")
			assert.Equal(t, int32(tt.calls), calls.Load())
		})
	}
}

func TestRunLegacyOutcomes(t *testing.T) {
	for name, tt := range map[string]struct {
		first, second, code int
		summary             string
	}{
		"both accepted":   {200, 200, 0, "2 succeeded, 0 failed"},
		"partial success": {500, 200, 0, "1 succeeded, 1 failed"},
		"all failure":     {500, 500, 1, "0 succeeded, 2 failed"},
	} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path == "/first" {
					w.WriteHeader(tt.first)
				} else {
					w.WriteHeader(tt.second)
				}
				_, _ = io.WriteString(w, "synthetic-private-value")
			}))
			defer server.Close()
			config := fmt.Sprintf("DISCORD_WEBHOOK_URL='%s/first'; DEFAULT_RECIPIENT_DISCORD=channel; FLOCK_WEBHOOK_URL='%s/second'; DEFAULT_RECIPIENT_FLOCK=channel", server.URL, server.URL)
			var stdout, stderr bytes.Buffer
			code := Run(context.Background(), []string{"send-legacy", "--config", writeConfig(t, config), "--role", "ops"}, strings.NewReader(testutil.ValidEvent), &stdout, &stderr)
			assert.Equal(t, tt.code, code, stderr.String())
			assert.Contains(t, stderr.String(), tt.summary)
			assert.Equal(t, int32(2), calls.Load())
			assert.NotContains(t, stderr.String(), "synthetic-private-value")
		})
	}
}

func TestRunLegacyStockWithoutShell(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	marker := filepath.ToSlash(filepath.Join(t.TempDir(), "executed"))
	t.Setenv("BASH_ENV", marker)
	stock := filepath.Join("..", "..", "..", "health_alarm_notify.conf")
	overlay := writeConfig(t, fmt.Sprintf("custom_sender() { printf executed >'%s'; }", marker))
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"send-legacy", "--config", stock, "--config", overlay, "--role", "sysadmin"}, strings.NewReader(testutil.ValidEvent), &stdout, &stderr)
	assert.Equal(t, 0, code, stderr.String())
	assert.NoFileExists(t, marker)
	assert.Contains(t, stderr.String(), "0 succeeded, 0 failed")
}

func TestRunLegacyControl(t *testing.T) {
	for name, tt := range map[string]struct {
		args []string
		err  string
		code int
	}{
		"help":                 {args: []string{"--help"}},
		"role required":        {err: "provide --config", code: 1},
		"destination rejected": {args: []string{"--destination", "secret"}, err: "invalid command options", code: 1},
		"empty method":         {args: []string{"--role", "ops", "--method", ""}, err: "invalid command options", code: 1},
		"missing overlay":      {args: []string{"--role", "ops", "--config", "/synthetic-private-value"}, err: "file 2: could not open file", code: 1},
	} {
		t.Run(name, func(t *testing.T) {
			args := append([]string{"send-legacy", "--config", writeConfig(t, "")}, tt.args...)
			var stdout, stderr bytes.Buffer
			assert.Equal(t, tt.code, Run(context.Background(), args, strings.NewReader(testutil.ValidEvent), &stdout, &stderr))
			if tt.code == 0 {
				assert.Equal(t, usage, stdout.String())
			} else {
				assert.Contains(t, stderr.String(), tt.err)
			}
			assert.NotContains(t, stderr.String(), "synthetic-private-value")
		})
	}
}

func TestRunLegacyDeadline(t *testing.T) {
	for name, tt := range map[string]struct {
		blockInput bool
		cancel     bool
	}{
		"HTTP deadline": {}, "blocked event input": {blockInput: true}, "canceled": {cancel: true},
	} {
		t.Run(name, func(t *testing.T) {
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				select {
				case <-r.Context().Done():
				case <-release:
				}
			}))
			defer server.Close()
			defer close(release)
			cfg := writeConfig(t, fmt.Sprintf("DISCORD_WEBHOOK_URL='%s'; DEFAULT_RECIPIENT_DISCORD=channel", server.URL))
			var input io.Reader = strings.NewReader(testutil.ValidEvent)
			if tt.blockInput {
				r, w, err := os.Pipe()
				require.NoError(t, err)
				defer r.Close()
				defer w.Close()
				input = r
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tt.cancel {
				cancel()
			}
			var stdout, stderr bytes.Buffer
			start := time.Now()
			code := Run(ctx, []string{"send-legacy", "--config", cfg, "--role", "ops", "--timeout", "50ms"}, input, &stdout, &stderr)
			assert.Equal(t, 1, code)
			assert.Less(t, time.Since(start), time.Second)
			if tt.cancel {
				assert.Contains(t, stderr.String(), "notification canceled")
			} else {
				assert.Contains(t, stderr.String(), "notification timed out")
			}
		})
	}
}
