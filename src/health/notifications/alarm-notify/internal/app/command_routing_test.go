// SPDX-License-Identifier: GPL-3.0-or-later

package app

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

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRunRouting(t *testing.T) {
	tests := map[string]struct {
		roles     []string
		routing   notifier.Routing
		statuses  map[string]int
		input     string
		wantCalls []string
		wantCode  int
		wantLogs  []string
	}{
		"roles overlap": {
			roles: []string{"sysadmin", "dba", "sysadmin"},
			routing: notifier.Routing{
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
			routing: notifier.Routing{
				Default: []string{"fallback", "primary"},
				Roles:   map[string][]string{"sysadmin": {"primary"}},
			},
			wantCalls: []string{"primary", "fallback"},
		},
		"suppressed role ignores defaults": {
			roles: []string{"muted"},
			routing: notifier.Routing{
				Default: []string{"primary"},
				Roles:   map[string][]string{"muted": {}},
			},
			wantLogs: []string{"0 succeeded, 0 failed"},
		},
		"reserved roles skip defaults": {
			roles:   []string{"silent", "disabled"},
			routing: notifier.Routing{Default: []string{"primary"}},
		},
		"missing role without defaults": {roles: []string{"unknown"}},
		"failure before success": {
			roles:     []string{"sysadmin"},
			routing:   notifier.Routing{Roles: map[string][]string{"sysadmin": {"primary", "database"}}},
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
			routing:   notifier.Routing{Roles: map[string][]string{"sysadmin": {"primary", "database"}}},
			statuses:  map[string]int{"database": 429},
			wantCalls: []string{"primary", "database"},
			wantLogs:  []string{"HTTP 429", "1 succeeded, 1 failed"},
		},
		"all fail": {
			roles:     []string{"sysadmin"},
			routing:   notifier.Routing{Roles: map[string][]string{"sysadmin": {"primary", "database"}}},
			statuses:  map[string]int{"primary": 401, "database": 503},
			wantCalls: []string{"primary", "database"},
			wantCode:  1,
			wantLogs: []string{
				"HTTP 401",
				"HTTP 503",
				"0 succeeded, 2 failed",
				"all attempted destinations failed",
			},
		},
		"missing secret does not stop others": {
			roles:     []string{"sysadmin"},
			routing:   notifier.Routing{Roles: map[string][]string{"sysadmin": {"secret", "primary"}}},
			wantCalls: []string{"primary"},
			wantLogs: []string{
				`destination "secret" failed`,
				"environment variable is not set",
				"1 succeeded, 1 failed",
			},
		},
		"resolved invalid URL does not stop others": {
			roles:     []string{"sysadmin"},
			routing:   notifier.Routing{Roles: map[string][]string{"sysadmin": {"invalid", "primary"}}},
			wantCalls: []string{"primary"},
			wantLogs: []string{
				`destination "invalid" failed`,
				"absolute HTTP(S)",
				"1 succeeded, 1 failed",
			},
		},
		"structural error stops all": {
			roles:    []string{"sysadmin"},
			routing:  notifier.Routing{Roles: map[string][]string{"sysadmin": {"primary", "missing"}}},
			wantCode: 1,
			wantLogs: []string{"unconfigured destination"},
		},
		"invalid event stops all": {
			roles:    []string{"sysadmin"},
			routing:  notifier.Routing{Roles: map[string][]string{"sysadmin": {"primary", "database"}}},
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
				event       notifyevent.Event
				err         error
			}
			requests := make(chan received, 16)
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					target := strings.TrimPrefix(r.URL.Path, "/")
					var event notifyevent.Event
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
			cfg := testConfig{Version: 1, Routing: test.routing, Destinations: map[string]map[string]any{
				"secret":  {"type": "webhook", "url": "${env:NOTIFIER_ROUTING_MISSING}"},
				"invalid": {"type": "webhook", "url": "${env:NOTIFIER_ROUTING_INVALID}"},
			}}
			for _, target := range []string{"primary", "shared", "database", "fallback", "unused"} {
				cfg.Destinations[target] = map[string]any{
					"type": "webhook",
					"url":  server.URL + "/" + target + "?token=synthetic-private-value",
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
				input = testutil.ValidEvent
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
				assert.Equal(t, received{destination: target, event: testutil.ExpectedEvent()}, <-requests)
			}
		})
	}
}

func TestRunFanoutCancellation(t *testing.T) {
	for name, test := range map[string]struct {
		provider string
		cancel   bool
		policies bool
	}{
		"filtered webhook cancel":   {provider: "webhook", cancel: true, policies: true},
		"filtered webhook deadline": {provider: "webhook", policies: true},
		"msteams cancel":            {provider: "msteams", cancel: true}, "msteams deadline": {provider: "msteams"},
		"matrix cancel": {provider: "matrix", cancel: true}, "matrix deadline": {provider: "matrix"},
		"opsgenie create deadline": {provider: "opsgenie"}, "opsgenie create cancel": {provider: "opsgenie", cancel: true},
		"opsgenie close deadline": {provider: "opsgenie-close"}, "opsgenie close cancel": {provider: "opsgenie-close", cancel: true},
		"pagerduty v1 deadline": {provider: "pagerduty-v1"}, "pagerduty v1 cancel": {provider: "pagerduty-v1", cancel: true},
		"pagerduty v2 deadline": {provider: "pagerduty-v2"}, "pagerduty v2 cancel": {provider: "pagerduty-v2", cancel: true},
		"smseagle deadline": {provider: "smseagle"}, "smseagle cancellation": {provider: "smseagle", cancel: true},
		"prowl deadline": {provider: "prowl"}, "prowl cancellation": {provider: "prowl", cancel: true},
		"kavenegar deadline": {provider: "kavenegar"}, "kavenegar cancellation": {provider: "kavenegar", cancel: true},
		"webhook deadline": {provider: "webhook"}, "webhook cancellation": {provider: "webhook", cancel: true},
		"rocketchat deadline": {provider: "rocketchat"}, "rocketchat cancellation": {provider: "rocketchat", cancel: true},
		"flock deadline": {provider: "flock"}, "flock cancellation": {provider: "flock", cancel: true},
		"fleep deadline": {provider: "fleep"}, "fleep cancellation": {provider: "fleep", cancel: true},
		"ilert deadline": {provider: "ilert"}, "ilert cancellation": {provider: "ilert", cancel: true},
		"signl4 deadline": {provider: "signl4"}, "signl4 cancellation": {provider: "signl4", cancel: true},
		"alerta deadline": {provider: "alerta"}, "alerta cancellation": {provider: "alerta", cancel: true},
		"dynatrace deadline": {provider: "dynatrace"}, "dynatrace cancellation": {provider: "dynatrace", cancel: true},
	} {
		t.Run(name, func(t *testing.T) {
			started := make(chan struct{})
			stopped := make(chan struct{})
			cleanup := make(chan struct{})
			var afterCalls atomic.Int32
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					path := r.URL.Path
					if strings.HasPrefix(path, "/blocked/_matrix/client/v3/rooms/") {
						path = "/blocked"
					}
					switch path {
					case "/blocked/v2/alerts", "/blocked/v2/alerts/" + opsgenieTestAlias + "/close",
						"/blocked/generic/2010-04-15/create_event.json",
						"/blocked/v2/enqueue",
						"/blocked/api/v2/messages/sms",
						"/blocked/add",
						"/blocked/synthetic-key/sms/send.json",
						"/blocked",
						"/blocked/events",
						"/blocked/alert",
						"/blocked/api/v2/events/ingest":
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
			if test.provider == "matrix" {
				blocked = fmt.Sprintf(
					"type: matrix, api_url: %q, access_token: synthetic-token, room_id: %q",
					server.URL+"/blocked",
					"!room:example.org",
				)
			}
			if test.provider == "ilert" {
				blocked = fmt.Sprintf("type: ilert, api_url: %q, integration_key: synthetic-key", server.URL+"/blocked")
			}
			if test.provider == "alerta" {
				blocked = fmt.Sprintf("type: alerta, api_url: %q, environment: Production", server.URL+"/blocked")
			}
			if test.provider == "dynatrace" {
				blocked = fmt.Sprintf(
					"type: dynatrace, api_url: %q, api_token: synthetic-key, entity_selector: type(HOST)",
					server.URL+"/blocked",
				)
			}
			if strings.HasPrefix(test.provider, "pagerduty-v") {
				blocked = fmt.Sprintf(
					"type: pagerduty, api_url: %q, integration_key: %q, api_version: %s",
					server.URL+"/blocked",
					pagerDutyTestKey,
					strings.TrimPrefix(test.provider, "pagerduty-v"),
				)
			}
			if strings.HasPrefix(test.provider, "opsgenie") {
				blocked = fmt.Sprintf(
					"type: opsgenie, api_url: %q, api_key: %q",
					server.URL+"/blocked",
					opsgenieTestKey,
				)
			}
			if test.provider == "smseagle" {
				blocked = fmt.Sprintf(
					"type: smseagle, api_url: %q, access_token: synthetic-token, recipients: ['15005550009']",
					server.URL+"/blocked",
				)
			}
			if test.provider == "prowl" {
				blocked = fmt.Sprintf("type: prowl, api_url: %q, api_key: %q", server.URL+"/blocked", prowlTestKey)
			}
			if test.provider == "kavenegar" {
				blocked = fmt.Sprintf(
					"type: kavenegar, api_url: %q, api_key: synthetic-key, sender: '12345', recipient: '15005550009'",
					server.URL+"/blocked",
				)
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
			if test.policies {
				config += "  policies:\n    first: {nowarn: true}\n    after: {nowarn: true}\n"
			}
			path := writeConfig(t, config)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var stdout, stderr bytes.Buffer
			done := make(chan int, 1)
			input := testutil.ValidEvent
			if test.provider == "opsgenie-close" {
				input = strings.ReplaceAll(input, "WARNING", "CLEAR")
			}
			go func() {
				done <- Run(ctx, []string{"send", "--config", path, "--role", "sysadmin", "--timeout", "500ms"}, strings.NewReader(input), &stdout, &stderr)
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
			if test.policies {
				assert.Contains(t, stderr.String(), `destination "first" skipped: nowarn`)
				assert.Contains(t, stderr.String(), "0 succeeded")
				assert.Contains(t, stderr.String(), "1 skipped")
				assert.NotContains(t, stderr.String(), `destination "after"`)
			} else {
				assert.Contains(t, stderr.String(), `destination "first" sent`)
				assert.Contains(t, stderr.String(), "1 succeeded")
			}
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
			cfg := testConfig{
				Version: 1,
				Destinations: map[string]map[string]any{
					destination: {"type": "webhook", "url": server.URL},
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
				strings.NewReader(testutil.ValidEvent),
				&stdout,
				&stderr,
			)
			assert.Zero(t, code)
			assert.Empty(t, stdout.String())
			assert.Equal(
				t,
				fmt.Sprintf(
					"alarm-notify: destination %q sent\nalarm-notify: delivery summary: 1 succeeded, 0 failed, 0 skipped\n",
					destination,
				),
				stderr.String(),
			)
		})
	}
}
