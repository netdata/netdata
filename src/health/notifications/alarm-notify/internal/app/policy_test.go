// SPDX-License-Identifier: GPL-3.0-or-later

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/webhook"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestReadStatusPolicies(t *testing.T) {
	for name, test := range map[string]struct {
		policies string
		want     *notifier.DestinationPolicy
		err      string
	}{
		"empty":              {policies: "dev: {}", want: &notifier.DestinationPolicy{}},
		"false":              {policies: "dev: {nowarn: false, noclear: false}", want: &notifier.DestinationPolicy{}},
		"nowarn":             {policies: "dev: {nowarn: true}", want: &notifier.DestinationPolicy{NoWarn: true}},
		"noclear":            {policies: "dev: {noclear: true}", want: &notifier.DestinationPolicy{NoClear: true}},
		"both":               {policies: "dev: {nowarn: true, noclear: true}", want: &notifier.DestinationPolicy{NoWarn: true, NoClear: true}},
		"boolean alias":      {policies: "dev: {nowarn: &flag true, noclear: *flag}", want: &notifier.DestinationPolicy{NoWarn: true, NoClear: true}},
		"mapping merge":      {policies: "dev: {<<: {nowarn: true}, noclear: true}", want: &notifier.DestinationPolicy{NoWarn: true, NoClear: true}},
		"unknown target":     {policies: "synthetic-private-value: {}", err: "unconfigured destination"},
		"null policy":        {policies: "dev: null", err: "requires a mapping"},
		"missing policy":     {policies: "dev:", err: "requires a mapping"},
		"scalar policy":      {policies: "dev: false", err: "invalid YAML"},
		"list policy":        {policies: "dev: []", err: "invalid YAML"},
		"list policies":      {policies: "[]", err: "invalid YAML"},
		"unknown flag":       {policies: "dev: {synthetic-private-value: true}", err: "invalid YAML"},
		"critical flag":      {policies: "dev: {critical: true}", want: &notifier.DestinationPolicy{Critical: true}},
		"critical false":     {policies: "dev: {critical: false}", want: &notifier.DestinationPolicy{}},
		"all flags":          {policies: "dev: {critical: true, nowarn: true, noclear: true}", want: &notifier.DestinationPolicy{Critical: true, NoWarn: true, NoClear: true}},
		"critical alias":     {policies: "dev: {critical: &flag true, noclear: *flag}", want: &notifier.DestinationPolicy{Critical: true, NoClear: true}},
		"critical merge":     {policies: "dev: {<<: {critical: true}, nowarn: true}", want: &notifier.DestinationPolicy{Critical: true, NoWarn: true}},
		"critical duplicate": {policies: "dev: {critical: false, critical: true}", err: "invalid YAML"},
		"critical null":      {policies: "dev: {critical: null}", err: "invalid YAML"},
		"critical missing":   {policies: "dev: {critical:}", err: "invalid YAML"},
		"critical string":    {policies: `dev: {critical: "true"}`, err: "invalid YAML"},
		"critical legacy":    {policies: "dev: {critical: yes}", err: "invalid YAML"},
		"critical integer":   {policies: "dev: {critical: 1}", err: "invalid YAML"},
		"critical list":      {policies: "dev: {critical: [true]}", err: "invalid YAML"},
		"critical object":    {policies: "dev: {critical: {value: true}}", err: "invalid YAML"},
		"duplicate flag":     {policies: "dev: {nowarn: false, nowarn: true}", err: "invalid YAML"},
		"duplicate name":     {policies: "dev: {}\n    dev: {}", err: "invalid YAML"},
		"null flag":          {policies: "dev: {nowarn: null}", err: "invalid YAML"},
		"missing flag":       {policies: "dev: {noclear:}", err: "invalid YAML"},
		"string flag":        {policies: `dev: {nowarn: "true"}`, err: "invalid YAML"},
		"legacy boolean":     {policies: "dev: {nowarn: yes}", err: "invalid YAML"},
		"integer flag":       {policies: "dev: {noclear: 1}", err: "invalid YAML"},
		"list flag":          {policies: "dev: {noclear: [true]}", err: "invalid YAML"},
		"object flag":        {policies: "dev: {noclear: {value: true}}", err: "invalid YAML"},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := readWebhookConfig(strings.NewReader(validConfig + "routing:\n  policies:\n    " + test.policies + "\n"))
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				assert.NotContains(t, err.Error(), "synthetic-private-value")
				assert.Equal(t, testutil.Document[webhook.Config]{}, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, testutil.Document[webhook.Config]{
				Version:      1,
				Destinations: map[string]webhook.Config{"dev": {URL: "https://example.com/notify"}},
				Routing:      notifier.Routing{Policies: map[string]*notifier.DestinationPolicy{"dev": test.want}},
			}, got)
		})
	}
}

func TestRunStatusPolicyMatrix(t *testing.T) {
	for name, test := range map[string]struct {
		policy *notifier.DestinationPolicy
		skips  map[string]string
	}{
		"omitted": {},
		"false":   {policy: &notifier.DestinationPolicy{}},
		"nowarn":  {policy: &notifier.DestinationPolicy{NoWarn: true}, skips: map[string]string{"WARNING": "nowarn"}},
		"noclear": {policy: &notifier.DestinationPolicy{NoClear: true}, skips: map[string]string{"CLEAR": "noclear"}},
		"both": {policy: &notifier.DestinationPolicy{NoWarn: true, NoClear: true},
			skips: map[string]string{"WARNING": "nowarn", "CLEAR": "noclear"}},
	} {
		t.Run(name, func(t *testing.T) {
			for _, status := range []string{"WARNING", "CRITICAL", "CLEAR"} {
				t.Run(status, func(t *testing.T) {
					for selection, options := range map[string][]string{
						"direct": {"--destination", "dev\nquoted"},
						"roles":  {"--role", "sysadmin", "--role", "dba", "--role", "sysadmin"},
					} {
						t.Run(selection, func(t *testing.T) {
							requests := make(chan notifyevent.Event, 4)
							server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
								var event notifyevent.Event
								assert.NoError(t, json.NewDecoder(r.Body).Decode(&event))
								requests <- event
								w.WriteHeader(http.StatusNoContent)
							}))
							defer server.Close()
							cfg := testConfig{Version: 1,
								Destinations: map[string]map[string]any{"dev\nquoted": {"type": "webhook", "url": server.URL}},
								Routing: notifier.Routing{Roles: map[string][]string{
									"sysadmin": {"dev\nquoted", "dev\nquoted"}, "dba": {"dev\nquoted"},
								}},
							}
							if test.policy != nil {
								cfg.Routing.Policies = map[string]*notifier.DestinationPolicy{"dev\nquoted": test.policy}
							}
							data, err := yaml.Marshal(cfg)
							require.NoError(t, err)
							args := append([]string{"send", "--config", writeConfig(t, string(data))}, options...)
							event := testutil.ExpectedEvent()
							event.Status = status
							// CLEAR eligibility remains the producer's responsibility, even without a previous state.
							event.PreviousStatus = ""
							input, err := json.Marshal(event)
							require.NoError(t, err)
							var stdout, stderr bytes.Buffer
							assert.Zero(t, Run(context.Background(), args, bytes.NewReader(input), &stdout, &stderr))
							assert.Empty(t, stdout.String())
							if reason := test.skips[status]; reason != "" {
								assert.Empty(t, requests)
								assert.Equal(t, fmt.Sprintf("alarm-notify: destination %q skipped: %s\nalarm-notify: delivery summary: 0 succeeded, 0 failed, 1 skipped\n", "dev\nquoted", reason), stderr.String())
							} else {
								require.Len(t, requests, 1)
								assert.Equal(t, event, <-requests)
								assert.Equal(t, "alarm-notify: destination \"dev\\nquoted\" sent\nalarm-notify: delivery summary: 1 succeeded, 0 failed, 0 skipped\n", stderr.String())
							}
						})
					}
				})
			}
		})
	}
}

func TestRunStatusPolicyResults(t *testing.T) {
	t.Setenv("NOTIFIER_POLICY_MISSING", "")
	require.NoError(t, os.Unsetenv("NOTIFIER_POLICY_MISSING"))
	for name, test := range map[string]struct {
		targets []string
		input   string
		code    int
		calls   []string
		logs    string
	}{
		"all skipped": {targets: []string{"skip", "skip"}, logs: "destination \"skip\" skipped: nowarn\nalarm-notify: delivery summary: 0 succeeded, 0 failed, 1 skipped\n"},
		"skip before failure": {targets: []string{"skip", "fail"}, code: 1, calls: []string{"fail"},
			logs: "destination \"skip\" skipped: nowarn\nalarm-notify: destination \"fail\" failed: webhook returned HTTP 503\nalarm-notify: delivery summary: 0 succeeded, 1 failed, 1 skipped\nalarm-notify: all attempted destinations failed\n"},
		"skip after failure": {targets: []string{"fail", "skip"}, code: 1, calls: []string{"fail"},
			logs: "destination \"fail\" failed: webhook returned HTTP 503\nalarm-notify: destination \"skip\" skipped: nowarn\nalarm-notify: delivery summary: 0 succeeded, 1 failed, 1 skipped\nalarm-notify: all attempted destinations failed\n"},
		"mixed": {targets: []string{"skip", "fail", "ok"}, calls: []string{"fail", "ok"},
			logs: "destination \"skip\" skipped: nowarn\nalarm-notify: destination \"fail\" failed: webhook returned HTTP 503\nalarm-notify: destination \"ok\" sent\nalarm-notify: delivery summary: 1 succeeded, 1 failed, 1 skipped\n"},
		"unselected policy": {targets: []string{"ok"}, calls: []string{"ok"},
			logs: "destination \"ok\" sent\nalarm-notify: delivery summary: 1 succeeded, 0 failed, 0 skipped\n"},
		"invalid event still fails": {targets: []string{"skip"}, input: "invalid", code: 1,
			logs: "delivery summary: 0 succeeded, 0 failed, 0 skipped\nalarm-notify: invalid JSON event: check syntax, field names, types, and timestamp\n"},
	} {
		t.Run(name, func(t *testing.T) {
			requests := make(chan string, 4)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- strings.TrimPrefix(r.URL.Path, "/")
				if r.URL.Path == "/fail" {
					w.WriteHeader(http.StatusServiceUnavailable)
				} else {
					w.WriteHeader(http.StatusNoContent)
				}
			}))
			defer server.Close()
			cfg := testConfig{Version: 1, Destinations: map[string]map[string]any{
				"skip": {"type": "webhook", "url": "${env:NOTIFIER_POLICY_MISSING}"},
				"fail": {"type": "webhook", "url": server.URL + "/fail"},
				"ok":   {"type": "webhook", "url": server.URL + "/ok"},
			}, Routing: notifier.Routing{Default: test.targets, Policies: map[string]*notifier.DestinationPolicy{"skip": {NoWarn: true}}}}
			data, err := yaml.Marshal(cfg)
			require.NoError(t, err)
			input := test.input
			if input == "" {
				input = testutil.ValidEvent
			}
			var stdout, stderr bytes.Buffer
			assert.Equal(t, test.code, Run(context.Background(), []string{"send", "--config", writeConfig(t, string(data)), "--role", "unknown"}, strings.NewReader(input), &stdout, &stderr))
			assert.Empty(t, stdout.String())
			assert.Equal(t, "alarm-notify: "+test.logs, stderr.String())
			require.Len(t, requests, len(test.calls))
			for _, path := range test.calls {
				assert.Equal(t, path, <-requests)
			}
		})
	}
}
