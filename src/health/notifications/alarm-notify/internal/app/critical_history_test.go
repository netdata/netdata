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
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func criticalHistoryInput(t *testing.T, status, history string) []byte {
	t.Helper()
	event := testutil.ExpectedEvent()
	event.Status = status
	data, err := json.Marshal(event)
	require.NoError(t, err)
	if history != "" {
		data = append(data[:len(data)-1], []byte(`,"critical_seen_since_clear":`+history+`}`)...)
	}
	return data
}

func TestRunCriticalHistoryPreflight(t *testing.T) {
	t.Setenv("NOTIFIER_PREFLIGHT_MISSING", "")
	require.NoError(t, os.Unsetenv("NOTIFIER_PREFLIGHT_MISSING"))
	for name, test := range map[string]struct {
		history       string
		targets       []string
		missingSecret bool
	}{
		"unknown after missing secret":  {"", []string{"ordinary", "critical"}, true},
		"unknown before missing secret": {"null", []string{"critical", "ordinary"}, true},
		"missing before ordinary":       {"", []string{"critical", "ordinary"}, false},
		"missing after ordinary":        {"", []string{"ordinary", "critical"}, false},
		"null before ordinary":          {"null", []string{"critical", "ordinary"}, false},
		"null after ordinary":           {"null", []string{"ordinary", "critical"}, false},
	} {
		t.Run(name, func(t *testing.T) {
			requests := make(chan struct{}, 4)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests <- struct{}{}; w.WriteHeader(204) }))
			defer server.Close()
			cfg := testConfig{Version: 1, Destinations: map[string]map[string]any{
				"ordinary": {"type": "webhook", "url": server.URL}, "critical": {"type": "webhook", "url": server.URL},
			}, Routing: notifier.Routing{Default: test.targets, Policies: map[string]*notifier.DestinationPolicy{"critical": {Critical: true}}}}
			if test.missingSecret {
				cfg.Destinations["ordinary"]["url"] = "${env:NOTIFIER_PREFLIGHT_MISSING}"
			}
			data, err := yaml.Marshal(cfg)
			require.NoError(t, err)
			for _, status := range []string{"WARNING", "CLEAR"} {
				t.Run(status, func(t *testing.T) {
					var stdout, stderr bytes.Buffer
					assert.Equal(t, 1, Run(context.Background(), []string{"send", "--config", writeConfig(t, string(data)), "--role", "unknown"}, bytes.NewReader(criticalHistoryInput(t, status, test.history)), &stdout, &stderr))
					assert.Empty(t, stdout.String())
					assert.Empty(t, requests)
					assert.Contains(t, stderr.String(), "critical_seen_since_clear")
					assert.Contains(t, stderr.String(), "delivery summary: 0 succeeded, 0 failed, 0 skipped\n")
					assert.NotContains(t, stderr.String(), "destination ")
				})
			}
		})
	}
}

func TestRunCriticalHistoryResults(t *testing.T) {
	t.Setenv("NOTIFIER_CRITICAL_MISSING", "")
	require.NoError(t, os.Unsetenv("NOTIFIER_CRITICAL_MISSING"))
	for name, test := range map[string]struct {
		status, history string
		targets, calls  []string
		code            int
		logs            string
	}{
		"all skipped and deduplicated": {"WARNING", "false", []string{"critical", "critical"}, nil, 0, "destination \"critical\" skipped: critical\nalarm-notify: delivery summary: 0 succeeded, 0 failed, 1 skipped\n"},
		"clear skipped":                {"CLEAR", "false", []string{"critical"}, nil, 0, "destination \"critical\" skipped: critical\nalarm-notify: delivery summary: 0 succeeded, 0 failed, 1 skipped\n"},
		"mixed success":                {"WARNING", "false", []string{"critical", "fail", "ok"}, []string{"fail", "ok"}, 0, "destination \"critical\" skipped: critical\nalarm-notify: destination \"fail\" failed: webhook returned HTTP 503\nalarm-notify: destination \"ok\" sent\nalarm-notify: delivery summary: 1 succeeded, 1 failed, 1 skipped\n"},
		"all attempted fail":           {"WARNING", "false", []string{"fail", "critical"}, []string{"fail"}, 1, "destination \"fail\" failed: webhook returned HTTP 503\nalarm-notify: destination \"critical\" skipped: critical\nalarm-notify: delivery summary: 0 succeeded, 1 failed, 1 skipped\nalarm-notify: all attempted destinations failed\n"},
		"unselected unknown":           {"WARNING", "", []string{"ok"}, []string{"ok"}, 0, "destination \"ok\" sent\nalarm-notify: delivery summary: 1 succeeded, 0 failed, 0 skipped\n"},
		"nowarn unknown":               {"WARNING", "", []string{"filtered"}, nil, 0, "destination \"filtered\" skipped: nowarn\nalarm-notify: delivery summary: 0 succeeded, 0 failed, 1 skipped\n"},
		"noclear null":                 {"CLEAR", "null", []string{"filtered"}, nil, 0, "destination \"filtered\" skipped: noclear\nalarm-notify: delivery summary: 0 succeeded, 0 failed, 1 skipped\n"},
		"warning history":              {"WARNING", "true", []string{"eligible"}, []string{"eligible"}, 0, "destination \"eligible\" sent\nalarm-notify: delivery summary: 1 succeeded, 0 failed, 0 skipped\n"},
		"clear history":                {"CLEAR", "true", []string{"eligible"}, []string{"eligible"}, 0, "destination \"eligible\" sent\nalarm-notify: delivery summary: 1 succeeded, 0 failed, 0 skipped\n"},
		"critical unknown":             {"CRITICAL", "", []string{"eligible"}, []string{"eligible"}, 0, "destination \"eligible\" sent\nalarm-notify: delivery summary: 1 succeeded, 0 failed, 0 skipped\n"},
	} {
		t.Run(name, func(t *testing.T) {
			requests := make(chan string, 4)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- strings.TrimPrefix(r.URL.Path, "/")
				if r.URL.Path == "/fail" {
					w.WriteHeader(503)
				} else {
					w.WriteHeader(204)
				}
			}))
			defer server.Close()
			cfg := testConfig{Version: 1, Destinations: map[string]map[string]any{
				"critical": {"type": "webhook", "url": "${env:NOTIFIER_CRITICAL_MISSING}"}, "filtered": {"type": "webhook", "url": "${env:NOTIFIER_CRITICAL_MISSING}"},
				"eligible": {"type": "webhook", "url": server.URL + "/eligible"}, "ok": {"type": "webhook", "url": server.URL + "/ok"}, "fail": {"type": "webhook", "url": server.URL + "/fail"},
			}, Routing: notifier.Routing{Roles: map[string][]string{"ops": test.targets}, Policies: map[string]*notifier.DestinationPolicy{"ok": {Critical: false}, "critical": {Critical: true}, "eligible": {Critical: true}, "filtered": {Critical: true, NoWarn: true, NoClear: true}}}}
			data, err := yaml.Marshal(cfg)
			require.NoError(t, err)
			var stdout, stderr bytes.Buffer
			assert.Equal(t, test.code, Run(context.Background(), []string{"send", "--config", writeConfig(t, string(data)), "--role", "ops", "--role", "ops"}, bytes.NewReader(criticalHistoryInput(t, test.status, test.history)), &stdout, &stderr), stderr.String())
			assert.Empty(t, stdout.String())
			assert.Equal(t, "alarm-notify: "+test.logs, stderr.String())
			require.Len(t, requests, len(test.calls))
			for _, call := range test.calls {
				assert.Equal(t, call, <-requests)
			}
		})
	}
}

func TestRunInputOnlyFactsPublicPayload(t *testing.T) {
	for name, test := range map[string]struct {
		destination map[string]any
		path        []string
		encoded     bool
		version     int64
	}{
		"webhook":      {map[string]any{"type": "webhook"}, nil, false, 0},
		"alerta":       {map[string]any{"type": "alerta", "environment": "Production"}, []string{"rawData"}, true, 0},
		"opsgenie":     {opsgenieTestDestination(), []string{"details", "event"}, true, 0},
		"ilert":        {map[string]any{"type": "ilert", "integration_key": "synthetic-key"}, []string{"customDetails"}, false, 0},
		"pagerduty v1": {pagerDutyTestDestination(1), []string{"details"}, false, 1},
		"pagerduty v2": {pagerDutyTestDestination(2), []string{"payload", "custom_details"}, false, 2},
	} {
		t.Run(name, func(t *testing.T) {
			requests := make(chan []byte, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				data, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				requests <- data
				switch test.destination["type"] {
				case "pagerduty":
					pagerDutyTestAck(w, test.version, pagerDutyTestIncidentKey)
				case "alerta":
					w.WriteHeader(201)
					_, _ = io.WriteString(w, alertaTestAck)
				case "opsgenie":
					w.WriteHeader(202)
					_, _ = io.WriteString(w, opsgenieTestAck)
				default:
					w.WriteHeader(202)
				}
			}))
			defer server.Close()
			endpoint := "api_url"
			if name == "webhook" {
				endpoint = "url"
			}
			test.destination[endpoint] = server.URL
			cfg := testConfig{Version: 1, Destinations: map[string]map[string]any{"target": test.destination}, Routing: notifier.Routing{Policies: map[string]*notifier.DestinationPolicy{"target": {Critical: true}}}}
			data, err := yaml.Marshal(cfg)
			require.NoError(t, err)
			var stdout, stderr bytes.Buffer
			require.Zero(t, Run(context.Background(), []string{"send", "--config", writeConfig(t, string(data)), "--destination", "target"}, strings.NewReader(testutil.WithProducerContext(string(criticalHistoryInput(t, "WARNING", "true")), testutil.ProducerContextJSON)), &stdout, &stderr), stderr.String())
			assert.Empty(t, stdout.String())
			assert.Equal(t, "alarm-notify: destination \"target\" sent\nalarm-notify: delivery summary: 1 succeeded, 0 failed, 0 skipped\n", stderr.String())
			require.Len(t, requests, 1)
			body := <-requests
			assert.NotContains(t, string(body), "critical_seen_since_clear")
			var raw any
			require.NoError(t, json.Unmarshal(body, &raw))
			for _, key := range test.path {
				object, ok := raw.(map[string]any)
				require.True(t, ok)
				raw = object[key]
			}
			var public []byte
			if test.encoded {
				value, ok := raw.(string)
				require.True(t, ok)
				public = []byte(value)
			} else {
				public, err = json.Marshal(raw)
				require.NoError(t, err)
			}
			assert.JSONEq(t, string(criticalHistoryInput(t, "WARNING", "")), string(public), fmt.Sprintf("%s public event", name))
		})
	}
}
