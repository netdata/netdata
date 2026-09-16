// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

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
	"time"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestReadStatusPolicies(t *testing.T) {
	for name, test := range map[string]struct {
		policies string
		want     *DestinationPolicy
		err      string
	}{
		"empty":          {policies: "dev: {}", want: &DestinationPolicy{}},
		"false":          {policies: "dev: {nowarn: false, noclear: false}", want: &DestinationPolicy{}},
		"nowarn":         {policies: "dev: {nowarn: true}", want: &DestinationPolicy{NoWarn: true}},
		"noclear":        {policies: "dev: {noclear: true}", want: &DestinationPolicy{NoClear: true}},
		"both":           {policies: "dev: {nowarn: true, noclear: true}", want: &DestinationPolicy{NoWarn: true, NoClear: true}},
		"boolean alias":  {policies: "dev: {nowarn: &flag true, noclear: *flag}", want: &DestinationPolicy{NoWarn: true, NoClear: true}},
		"mapping merge":  {policies: "dev: {<<: {nowarn: true}, noclear: true}", want: &DestinationPolicy{NoWarn: true, NoClear: true}},
		"unknown target": {policies: "synthetic-private-value: {}", err: "unconfigured destination"},
		"null policy":    {policies: "dev: null", err: "requires a mapping"},
		"missing policy": {policies: "dev:", err: "requires a mapping"},
		"scalar policy":  {policies: "dev: false", err: "invalid YAML"},
		"list policy":    {policies: "dev: []", err: "invalid YAML"},
		"list policies":  {policies: "[]", err: "invalid YAML"},
		"unknown flag":   {policies: "dev: {synthetic-private-value: true}", err: "invalid YAML"},
		"critical flag":  {policies: "dev: {critical: true}", err: "invalid YAML"},
		"duplicate flag": {policies: "dev: {nowarn: false, nowarn: true}", err: "invalid YAML"},
		"duplicate name": {policies: "dev: {}\n    dev: {}", err: "invalid YAML"},
		"null flag":      {policies: "dev: {nowarn: null}", err: "invalid YAML"},
		"missing flag":   {policies: "dev: {noclear:}", err: "invalid YAML"},
		"string flag":    {policies: `dev: {nowarn: "true"}`, err: "invalid YAML"},
		"legacy boolean": {policies: "dev: {nowarn: yes}", err: "invalid YAML"},
		"integer flag":   {policies: "dev: {noclear: 1}", err: "invalid YAML"},
		"list flag":      {policies: "dev: {noclear: [true]}", err: "invalid YAML"},
		"object flag":    {policies: "dev: {noclear: {value: true}}", err: "invalid YAML"},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := readConfig(strings.NewReader(validConfig + "routing:\n  policies:\n    " + test.policies + "\n"))
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				assert.NotContains(t, err.Error(), "synthetic-private-value")
				assert.Equal(t, Config{}, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, Config{
				Version:      1,
				Destinations: map[string]Destination{"dev": {Type: "webhook", URL: "https://example.com/notify"}},
				Routing:      Routing{Policies: map[string]*DestinationPolicy{"dev": test.want}},
			}, got)
		})
	}
}

func TestRunStatusPolicyMatrix(t *testing.T) {
	for name, test := range map[string]struct {
		policy *DestinationPolicy
		skips  map[string]string
	}{
		"omitted": {},
		"false":   {policy: &DestinationPolicy{}},
		"nowarn":  {policy: &DestinationPolicy{NoWarn: true}, skips: map[string]string{"WARNING": "nowarn"}},
		"noclear": {policy: &DestinationPolicy{NoClear: true}, skips: map[string]string{"CLEAR": "noclear"}},
		"both": {policy: &DestinationPolicy{NoWarn: true, NoClear: true},
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
							cfg := Config{Version: 1,
								Destinations: map[string]Destination{"dev\nquoted": {Type: "webhook", URL: server.URL}},
								Routing: Routing{Roles: map[string][]string{
									"sysadmin": {"dev\nquoted", "dev\nquoted"}, "dba": {"dev\nquoted"},
								}},
							}
							if test.policy != nil {
								cfg.Routing.Policies = map[string]*DestinationPolicy{"dev\nquoted": test.policy}
							}
							data, err := yaml.Marshal(cfg)
							require.NoError(t, err)
							args := append([]string{"send", "--config", writeConfig(t, string(data))}, options...)
							event := expectedEvent()
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
			cfg := Config{Version: 1, Destinations: map[string]Destination{
				"skip": {Type: "webhook", URL: "${env:NOTIFIER_POLICY_MISSING}"},
				"fail": {Type: "webhook", URL: server.URL + "/fail"},
				"ok":   {Type: "webhook", URL: server.URL + "/ok"},
			}, Routing: Routing{Default: test.targets, Policies: map[string]*DestinationPolicy{"skip": {NoWarn: true}}}}
			data, err := yaml.Marshal(cfg)
			require.NoError(t, err)
			input := test.input
			if input == "" {
				input = validEvent
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

func TestDispatchStatusPolicyCancellation(t *testing.T) {
	for name, test := range map[string]struct {
		before  bool
		targets []string
		want    []deliveryResult
	}{
		"before filtering":           {before: true, targets: []string{"dev"}},
		"last skipped result":        {targets: []string{"dev"}, want: []deliveryResult{{destination: "dev", skipReason: "nowarn"}}},
		"remaining skips unreported": {targets: []string{"dev", "after"}, want: []deliveryResult{{destination: "dev", skipReason: "nowarn"}}},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if test.before {
				cancel()
			}
			cfg := Config{Routing: Routing{Policies: map[string]*DestinationPolicy{
				"dev": {NoWarn: true}, "after": {NoWarn: true},
			}}}
			var got []deliveryResult
			err := dispatch(ctx, nil, cfg, test.targets, expectedEvent(), time.Second, func(result deliveryResult) {
				got = append(got, result)
				cancel()
			})
			require.ErrorIs(t, err, context.Canceled)
			assert.Equal(t, test.want, got)
		})
	}
}
