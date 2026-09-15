// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRunRouting(t *testing.T) {
	tests := map[string]struct {
		roles     []string
		routing   Routing
		statuses  map[string]int
		input     string
		wantCalls []string
		wantCode  int
		wantLogs  []string
	}{
		"roles overlap": {
			roles: []string{"sysadmin", "dba", "sysadmin"},
			routing: Routing{
				Roles: map[string][]string{
					"sysadmin": {"primary", "shared"},
					"dba":      {"shared", "database"},
				},
			},
			wantCalls: []string{"primary", "shared", "database"},
			wantLogs:  []string{"3 succeeded, 0 failed"},
		},
		"missing role adds defaults": {
			roles: []string{"sysadmin", "unknown", "another"},
			routing: Routing{
				Default: []string{"fallback", "primary"},
				Roles:   map[string][]string{"sysadmin": {"primary"}},
			},
			wantCalls: []string{"primary", "fallback"},
		},
		"suppressed role ignores defaults": {
			roles: []string{"muted"},
			routing: Routing{
				Default: []string{"primary"},
				Roles:   map[string][]string{"muted": {}},
			},
			wantLogs: []string{"0 succeeded, 0 failed"},
		},
		"reserved roles skip defaults": {
			roles:   []string{"silent", "disabled"},
			routing: Routing{Default: []string{"primary"}},
		},
		"missing role without defaults": {roles: []string{"unknown"}},
		"failure before success": {
			roles:     []string{"sysadmin"},
			routing:   Routing{Roles: map[string][]string{"sysadmin": {"primary", "database"}}},
			statuses:  map[string]int{"primary": 503},
			wantCalls: []string{"primary", "database"},
			wantLogs: []string{
				`destination "primary" failed: webhook returned HTTP 503`,
				`destination "database" sent`,
				"1 succeeded, 1 failed",
			},
		},
		"failure after success": {
			roles:     []string{"sysadmin"},
			routing:   Routing{Roles: map[string][]string{"sysadmin": {"primary", "database"}}},
			statuses:  map[string]int{"database": 429},
			wantCalls: []string{"primary", "database"},
			wantLogs:  []string{"HTTP 429", "1 succeeded, 1 failed"},
		},
		"all fail": {
			roles:     []string{"sysadmin"},
			routing:   Routing{Roles: map[string][]string{"sysadmin": {"primary", "database"}}},
			statuses:  map[string]int{"primary": 401, "database": 503},
			wantCalls: []string{"primary", "database"},
			wantCode:  1,
			wantLogs: []string{
				"HTTP 401",
				"HTTP 503",
				"0 succeeded, 2 failed",
				"all selected destinations failed",
			},
		},
		"missing secret does not stop others": {
			roles:     []string{"sysadmin"},
			routing:   Routing{Roles: map[string][]string{"sysadmin": {"secret", "primary"}}},
			wantCalls: []string{"primary"},
			wantLogs: []string{
				`destination "secret" failed`,
				"environment variable is not set",
				"1 succeeded, 1 failed",
			},
		},
		"resolved invalid URL does not stop others": {
			roles:     []string{"sysadmin"},
			routing:   Routing{Roles: map[string][]string{"sysadmin": {"invalid", "primary"}}},
			wantCalls: []string{"primary"},
			wantLogs: []string{
				`destination "invalid" failed`,
				"absolute HTTP(S)",
				"1 succeeded, 1 failed",
			},
		},
		"structural error stops all": {
			roles:    []string{"sysadmin"},
			routing:  Routing{Roles: map[string][]string{"sysadmin": {"primary", "missing"}}},
			wantCode: 1,
			wantLogs: []string{"unconfigured destination"},
		},
		"invalid event stops all": {
			roles:    []string{"sysadmin"},
			routing:  Routing{Roles: map[string][]string{"sysadmin": {"primary", "database"}}},
			input:    "synthetic-private-value",
			wantCode: 1,
			wantLogs: []string{"invalid JSON"},
		},
	}
	t.Setenv("NOTIFIER_ROUTING_MISSING", "")
	require.NoError(t, os.Unsetenv("NOTIFIER_ROUTING_MISSING"))
	t.Setenv("NOTIFIER_ROUTING_INVALID", "synthetic-private-value")
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			type received struct {
				destination string
				event       Event
				err         error
			}
			requests := make(chan received, 16)
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					target := strings.TrimPrefix(r.URL.Path, "/")
					var event Event
					err := json.NewDecoder(r.Body).Decode(&event)
					requests <- received{target, event, err}
					status := test.statuses[target]
					if status == 0 {
						status = http.StatusNoContent
					}
					w.WriteHeader(status)
					_, _ = io.WriteString(w, "synthetic-private-value")
				}),
			)
			defer server.Close()
			cfg := Config{Version: 1, Routing: test.routing, Destinations: map[string]Destination{
				"secret":  {Type: "webhook", URL: "${env:NOTIFIER_ROUTING_MISSING}"},
				"invalid": {Type: "webhook", URL: "${env:NOTIFIER_ROUTING_INVALID}"},
			}}
			for _, target := range []string{"primary", "shared", "database", "fallback", "unused"} {
				cfg.Destinations[target] = Destination{
					Type: "webhook",
					URL:  server.URL + "/" + target + "?token=synthetic-private-value",
				}
			}
			data, err := yaml.Marshal(cfg)
			require.NoError(t, err)
			args := []string{"send", "--config", writeConfig(t, string(data))}
			for _, role := range test.roles {
				args = append(args, "--role", role)
			}
			input := test.input
			if input == "" {
				input = validEvent
			}
			var stdout, stderr bytes.Buffer
			assert.Equal(
				t,
				test.wantCode,
				Run(context.Background(), args, strings.NewReader(input), &stdout, &stderr),
			)
			assert.Empty(t, stdout.String())
			assert.NotContains(t, stderr.String(), "synthetic-private-value")
			assert.NotContains(t, stderr.String(), server.URL)
			for _, want := range test.wantLogs {
				assert.Contains(t, stderr.String(), want)
			}
			require.Len(t, requests, len(test.wantCalls))
			for _, target := range test.wantCalls {
				assert.Equal(t, received{destination: target, event: expectedEvent()}, <-requests)
			}
		})
	}
}

func TestRunFanoutCancellation(t *testing.T) {
	for name, test := range map[string]struct {
		provider string
		cancel   bool
	}{
		"webhook deadline": {provider: "webhook"}, "webhook cancellation": {provider: "webhook", cancel: true},
		"rocketchat deadline": {provider: "rocketchat"}, "rocketchat cancellation": {provider: "rocketchat", cancel: true},
		"flock deadline": {provider: "flock"}, "flock cancellation": {provider: "flock", cancel: true},
		"fleep deadline": {provider: "fleep"}, "fleep cancellation": {provider: "fleep", cancel: true},
		"ilert deadline": {provider: "ilert"}, "ilert cancellation": {provider: "ilert", cancel: true},
		"signl4 deadline": {provider: "signl4"}, "signl4 cancellation": {provider: "signl4", cancel: true},
	} {
		t.Run(name, func(t *testing.T) {
			started := make(chan struct{})
			stopped := make(chan struct{})
			cleanup := make(chan struct{})
			var afterCalls atomic.Int32
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/blocked", "/blocked/events":
						_, _ = io.Copy(io.Discard, r.Body)
						close(started)
						select {
						case <-r.Context().Done():
							close(stopped)
						case <-cleanup:
						}
					case "/after":
						afterCalls.Add(1)
						w.WriteHeader(http.StatusNoContent)
					default:
						w.WriteHeader(http.StatusNoContent)
					}
				}),
			)
			defer server.Close()
			defer close(cleanup)
			blocked := fmt.Sprintf("type: %s, url: %q", test.provider, server.URL+"/blocked")
			if test.provider == "ilert" {
				blocked = fmt.Sprintf("type: ilert, api_url: %q, integration_key: synthetic-key", server.URL+"/blocked")
			}
			config := fmt.Sprintf(`version: 1
destinations:
  first: {type: webhook, url: %q}
  blocked: {%s}
  after: {type: webhook, url: %q}
routing:
  roles:
    sysadmin: [first, blocked, after]
`, server.URL+"/first", blocked, server.URL+"/after")
			path := writeConfig(t, config)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var stdout, stderr bytes.Buffer
			done := make(chan int, 1)
			go func() {
				done <- Run(ctx, []string{"send", "--config", path, "--role", "sysadmin", "--timeout", "500ms"}, strings.NewReader(validEvent), &stdout, &stderr)
			}()
			select {
			case <-started:
			case <-time.After(2 * time.Second):
				t.Fatal("second destination was not attempted")
			}
			if test.cancel {
				cancel()
			}
			select {
			case code := <-done:
				assert.Equal(
					t,
					1,
					code,
					"command cancellation overrides earlier successful delivery",
				)
			case <-time.After(2 * time.Second):
				t.Fatal("command did not stop")
			}
			assert.Empty(t, stdout.String())
			assert.Contains(t, stderr.String(), `destination "first" sent`)
			assert.Contains(t, stderr.String(), "1 succeeded")
			if test.cancel {
				assert.Contains(t, stderr.String(), "notification canceled")
			} else {
				assert.Contains(t, stderr.String(), "notification timed out")
			}
			select {
			case <-stopped:
			case <-time.After(time.Second):
				t.Fatal("receiver did not observe request cancellation")
			}
			assert.Zero(
				t,
				afterCalls.Load(),
				"do not start remaining destinations after cancellation",
			)
		})
	}
}

func TestRunQuotesDestinationNames(t *testing.T) {
	for name, destination := range map[string]string{"newline": "first\nsecond", "terminal escape": "first\x1bsecond"} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusNoContent)
				}),
			)
			defer server.Close()
			cfg := Config{
				Version: 1,
				Destinations: map[string]Destination{
					destination: {Type: "webhook", URL: server.URL},
				},
			}
			data, err := yaml.Marshal(cfg)
			require.NoError(t, err)
			var stdout, stderr bytes.Buffer
			code := Run(
				context.Background(),
				[]string{
					"send",
					"--config",
					writeConfig(t, string(data)),
					"--destination",
					destination,
				},
				strings.NewReader(validEvent),
				&stdout,
				&stderr,
			)
			assert.Zero(t, code)
			assert.Empty(t, stdout.String())
			assert.Equal(
				t,
				fmt.Sprintf(
					"alarm-notify: destination %q sent\nalarm-notify: delivery summary: 1 succeeded, 0 failed\n",
					destination,
				),
				stderr.String(),
			)
		})
	}
}
