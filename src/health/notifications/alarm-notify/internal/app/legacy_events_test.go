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
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunLegacyEventMappings(t *testing.T) {
	for name, tt := range map[string]struct {
		method, settings, path, auth, ack string
		code                              int
	}{
		"ilert": {
			method: "ilert", settings: `ILERT_INTEGRATION_KEY=synthetic-key; ILERT_API_URL='ENDPOINT/proxy/'`,
			path: "/proxy/events", code: 202,
		},
		"opsgenie": {
			method: "opsgenie", settings: `OPSGENIE_API_KEY=synthetic-key; OPSGENIE_API_URL='ENDPOINT/proxy/'`,
			path: "/proxy/v2/alerts", auth: "GenieKey synthetic-key", code: 202, ack: opsgenieTestAck,
		},
		"dynatrace": {
			method: "dynatrace", settings: `DYNATRACE_SERVER='ENDPOINT/proxy/'; DYNATRACE_SPACE='space ?#'; DYNATRACE_TOKEN=synthetic-key; DYNATRACE_TAG_VALUE='ops: east,(温度)'; DYNATRACE_EVENT=CUSTOM_ANNOTATION; DYNATRACE_ANNOTATION_TYPE='Custom source'`,
			path: "/proxy/e/space%20%3F%23/api/v2/events/ingest", auth: "Api-Token synthetic-key", code: 201, ack: dynatraceTestAck,
		},
		"ilert literal base": {
			method: "ilert", settings: `ILERT_INTEGRATION_KEY=synthetic-key; ILERT_API_URL='ENDPOINT/${env:EVENT_MAPPING_URL}'`,
			path: "/$%7Benv:EVENT_MAPPING_URL%7D/events", code: 202,
		},
		"opsgenie literal base": {
			method: "opsgenie", settings: `OPSGENIE_API_KEY=synthetic-key; OPSGENIE_API_URL='ENDPOINT/${file:/missing}'`,
			path: "/$%7Bfile:/missing%7D/v2/alerts", auth: "GenieKey synthetic-key", code: 202, ack: opsgenieTestAck,
		},
		"dynatrace literal base and native defaults": {
			method: "dynatrace", settings: `DYNATRACE_SERVER='ENDPOINT/${env:EVENT_MAPPING_URL}'; DYNATRACE_SPACE=space; DYNATRACE_TOKEN=synthetic-key; DYNATRACE_TAG_VALUE=netdata; DYNATRACE_EVENT=CUSTOM_INFO; DYNATRACE_ANNOTATION_TYPE=''`,
			path: "/$%7Benv:EVENT_MAPPING_URL%7D/e/space/api/v2/events/ingest", auth: "Api-Token synthetic-key", code: 201, ack: dynatraceTestAck,
		},
	} {
		for status := range map[string]struct{}{"WARNING": {}, "CRITICAL": {}, "CLEAR": {}} {
			t.Run(name+"/"+status, func(t *testing.T) {
				t.Setenv("PATH", t.TempDir())
				t.Setenv("EVENT_MAPPING_URL", "must-not-be-resolved")
				type request struct{ method, path, query, contentType, auth, body string }
				requests := make(chan request, 2)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					assert.NoError(t, err)
					select {
					case requests <- request{r.Method, r.URL.EscapedPath(), r.URL.RawQuery, r.Header.Get("Content-Type"), r.Header.Get("Authorization"), string(body)}:
					default:
						t.Error("unexpected extra request")
					}
					w.WriteHeader(tt.code)
					_, _ = io.WriteString(w, tt.ack)
				}))
				defer server.Close()
				args := []string{"send-legacy", "--config", filepath.Join("..", "..", "..", "health_alarm_notify.conf"),
					"--config", writeConfig(t, strings.ReplaceAll(tt.settings, "ENDPOINT", server.URL)),
					"--role", "silent", "--role", "disabled", "--method", tt.method, "--method", tt.method}
				input, err := json.Marshal(testutil.EventForStatus(status, "full"))
				require.NoError(t, err)
				var stdout, stderr bytes.Buffer
				require.Equal(t, 0, Run(context.Background(), args, bytes.NewReader(input), &stdout, &stderr), stderr.String())
				assert.Empty(t, stdout.String())
				assert.Contains(t, stderr.String(), "1 succeeded, 0 failed")
				require.Len(t, requests, 1)
				got := <-requests
				path, query := tt.path, ""
				var want map[string]any
				if tt.method == "opsgenie" {
					want = opsgenieTestFixture(t, "full", status)
					if status == "CLEAR" {
						path += "/" + opsgenieTestAlias + "/close"
						query = "identifierType=alias"
					}
				} else {
					data, err := os.ReadFile(fixturePath(tt.method + "-full.json"))
					require.NoError(t, err)
					if status == "CRITICAL" {
						data = []byte(strings.NewReplacer("WARNING", "CRITICAL", "CLEAR", "WARNING").Replace(string(data)))
					} else if status == "CLEAR" {
						data = []byte(strings.NewReplacer("WARNING", "CLEAR", "CLEAR", "CRITICAL").Replace(string(data)))
					}
					require.NoError(t, json.Unmarshal(data, &want))
					if tt.method == "ilert" && status == "CLEAR" {
						want["eventType"] = "RESOLVE"
					}
					if name == "dynatrace" {
						want["entitySelector"] = `type(HOST),tag("ops\: east,(温度)")`
						want["eventType"] = "CUSTOM_ANNOTATION"
						want["properties"].(map[string]any)["dt.event.source"] = "Custom source"
					}
				}
				payload, err := json.Marshal(want)
				require.NoError(t, err)
				assert.JSONEq(t, string(payload), got.body)
				got.body = ""
				assert.Equal(t, request{method: "POST", path: path, query: query, contentType: "application/json", auth: tt.auth}, got)
				for _, secret := range []string{"synthetic-key", server.URL, "must-not-be-resolved"} {
					assert.NotContains(t, stderr.String(), secret)
				}
			})
		}
	}
}

func TestRunLegacyDynatraceServerPrefix(t *testing.T) {
	for name, tt := range map[string]struct{ prefix, path string }{
		"no trailing slash":       {"/proxy", "/proxy/e/space/api/v2/events/ingest"},
		"one trailing slash":      {"/proxy/", "/proxy/e/space/api/v2/events/ingest"},
		"two trailing slashes":    {"/proxy//", "/proxy//e/space/api/v2/events/ingest"},
		"three trailing slashes":  {"/proxy///", "/proxy///e/space/api/v2/events/ingest"},
		"encoded prefix retained": {"/proxy%2Ftenant//", "/proxy%2Ftenant//e/space/api/v2/events/ingest"},
	} {
		t.Run(name, func(t *testing.T) {
			requests := make(chan string, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path := r.URL.EscapedPath()
				select {
				case requests <- path:
				default:
					t.Error("unexpected extra request")
				}
				if path != tt.path {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.WriteHeader(http.StatusCreated)
				_, _ = io.WriteString(w, dynatraceTestAck)
			}))
			defer server.Close()
			settings := fmt.Sprintf(`SEND_DYNATRACE=YES; DYNATRACE_SERVER='%s%s'; DYNATRACE_SPACE=space; DYNATRACE_TOKEN=synthetic-key; DYNATRACE_TAG_VALUE=netdata; DYNATRACE_EVENT=CUSTOM_INFO`, server.URL, tt.prefix)
			args := []string{"send-legacy", "--config", writeConfig(t, settings), "--role", "silent", "--method", "dynatrace"}
			var stdout, stderr bytes.Buffer
			assert.Equal(t, 0, Run(context.Background(), args, strings.NewReader(testutil.ValidEvent), &stdout, &stderr), stderr.String())
			assert.Contains(t, stderr.String(), "1 succeeded, 0 failed")
			assert.Empty(t, stdout.String())
			require.Len(t, requests, 1)
			assert.Equal(t, tt.path, <-requests)
		})
	}
}

func TestRunLegacyEventPreflight(t *testing.T) {
	for name, tt := range map[string]struct {
		overlay, err string
		methods      []string
		code, calls  int
	}{
		"all global methods":                {calls: 3},
		"ilert old URL only":                {overlay: `ILERT_INTEGRATION_KEY=''; ILERT_ALERT_SOURCE_URL='ENDPOINT/synthetic-private-value'`, code: 1, err: "remove or clear ILERT_ALERT_SOURCE_URL"},
		"ilert both old and new":            {overlay: `ILERT_ALERT_SOURCE_URL='ENDPOINT/synthetic-private-value'`, code: 1, err: "remove or clear ILERT_ALERT_SOURCE_URL"},
		"ilert disabled ignores old URL":    {overlay: `SEND_ILERT=NO; ILERT_ALERT_SOURCE_URL='ENDPOINT/synthetic-private-value'`, calls: 2},
		"ilert flag is exact":               {overlay: `SEND_ILERT=yes; ILERT_ALERT_SOURCE_URL='ENDPOINT/synthetic-private-value'`, calls: 2},
		"ilert unselected ignores old URL":  {overlay: `ILERT_ALERT_SOURCE_URL='ENDPOINT/synthetic-private-value'`, methods: []string{"opsgenie", "dynatrace"}, calls: 2},
		"ilert URL cleared in overlay":      {overlay: `ILERT_ALERT_SOURCE_URL='ENDPOINT/old'; ILERT_ALERT_SOURCE_URL=''`, calls: 3},
		"ilert absent credential skips":     {overlay: `ILERT_INTEGRATION_KEY=''`, calls: 2},
		"opsgenie absent credential skips":  {overlay: `OPSGENIE_API_KEY=''`, calls: 2},
		"dynatrace absent event skips":      {overlay: `DYNATRACE_EVENT=''`, calls: 2},
		"dynatrace absent space skips":      {overlay: `DYNATRACE_SPACE=''`, calls: 2},
		"dynatrace absent server skips":     {overlay: `DYNATRACE_SERVER=''`, calls: 2},
		"dynatrace absent token skips":      {overlay: `DYNATRACE_TOKEN=''`, calls: 2},
		"dynatrace absent tag skips":        {overlay: `DYNATRACE_TAG_VALUE=''`, calls: 2},
		"opsgenie flag is exact":            {overlay: `SEND_OPSGENIE=yes`, calls: 2},
		"dynatrace flag is exact":           {overlay: `SEND_DYNATRACE=yes`, calls: 2},
		"ilert token is literal":            {overlay: `ILERT_INTEGRATION_KEY='${env:EVENT_MAPPING_KEY}'`, code: 1, err: "integration_key must be"},
		"opsgenie token is literal":         {overlay: `OPSGENIE_API_KEY='${file:/missing}'`, code: 1, err: "api_key must be"},
		"dynatrace token is literal":        {overlay: `DYNATRACE_TOKEN='${env:EVENT_MAPPING_KEY}'`, code: 1, err: "api_token must be"},
		"ilert invalid base":                {overlay: `ILERT_API_URL='/synthetic-private-value'`, code: 1, err: "absolute HTTP(S)"},
		"ilert official HTTP":               {overlay: `ILERT_API_URL='http://api.ilert.com/api'`, code: 1, err: "requires HTTPS"},
		"opsgenie official HTTP":            {overlay: `OPSGENIE_API_URL='http://api.eu.opsgenie.com'`, code: 1, err: "requires HTTPS"},
		"opsgenie query":                    {overlay: `OPSGENIE_API_URL='ENDPOINT?secret=synthetic-private-value'`, code: 1, err: "must not contain a query"},
		"dynatrace query":                   {overlay: `DYNATRACE_SERVER='ENDPOINT?secret=synthetic-private-value'`, code: 1, err: "must not contain a query"},
		"dynatrace empty fragment":          {overlay: `DYNATRACE_SERVER='ENDPOINT#'`, code: 1, err: "without user information or fragment"},
		"dynatrace dot segment":             {overlay: `DYNATRACE_SPACE='..'`, code: 1, err: "DYNATRACE_SPACE must be"},
		"dynatrace path injection":          {overlay: `DYNATRACE_SPACE='../synthetic-private-value'`, code: 1, err: "DYNATRACE_SPACE must be"},
		"dynatrace encoded path injection":  {overlay: `DYNATRACE_SPACE='%2e%2e%2f'`, code: 1, err: "DYNATRACE_SPACE must be"},
		"dynatrace backslash path":          {overlay: `DYNATRACE_SPACE='..\synthetic-private-value'`, code: 1, err: "DYNATRACE_SPACE must be"},
		"dynatrace tag injection":           {overlay: `DYNATRACE_TAG_VALUE='synthetic-private-value"),tag("other'`, code: 1, err: "DYNATRACE_TAG_VALUE cannot be mapped literally"},
		"dynatrace ambiguous tag escape":    {overlay: `DYNATRACE_TAG_VALUE='synthetic-private-value\:tag'`, code: 1, err: "DYNATRACE_TAG_VALUE cannot be mapped literally"},
		"dynatrace ambiguous tilde":         {overlay: `DYNATRACE_TAG_VALUE='synthetic-private-value~'`, code: 1, err: "DYNATRACE_TAG_VALUE cannot be mapped literally"},
		"dynatrace tag context":             {overlay: `DYNATRACE_TAG_VALUE='[context]synthetic-private-value'`, code: 1, err: "DYNATRACE_TAG_VALUE cannot be mapped literally"},
		"dynatrace tag boundary whitespace": {overlay: `DYNATRACE_TAG_VALUE=' synthetic-private-value'`, code: 1, err: "DYNATRACE_TAG_VALUE cannot be mapped literally"},
		"dynatrace tag control":             {overlay: "DYNATRACE_TAG_VALUE='synthetic-private-value\nother'", code: 1, err: "DYNATRACE_TAG_VALUE cannot be mapped literally"},
		"dynatrace selector limit":          {overlay: "DYNATRACE_TAG_VALUE='" + strings.Repeat("x", 2000) + "'", code: 1, err: "at most 2000"},
		"dynatrace invalid event":           {overlay: `DYNATRACE_EVENT=synthetic-private-value`, code: 1, err: "event_type must be"},
		"dynatrace invalid source":          {overlay: `DYNATRACE_ANNOTATION_TYPE='${env:EVENT_MAPPING_KEY}'`, code: 1, err: "source must be"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("PATH", t.TempDir())
			t.Setenv("EVENT_MAPPING_KEY", "synthetic-key")
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				switch {
				case strings.HasSuffix(r.URL.Path, "/ingest"):
					w.WriteHeader(201)
					_, _ = io.WriteString(w, dynatraceTestAck)
				case strings.HasSuffix(r.URL.Path, "/alerts"):
					w.WriteHeader(202)
					_, _ = io.WriteString(w, opsgenieTestAck)
				default:
					w.WriteHeader(202)
				}
			}))
			defer server.Close()
			settings := `SEND_ILERT=YES; ILERT_INTEGRATION_KEY=synthetic-key; ILERT_API_URL='ENDPOINT';
SEND_OPSGENIE=YES; OPSGENIE_API_KEY=synthetic-key; OPSGENIE_API_URL='ENDPOINT';
SEND_DYNATRACE=YES; DYNATRACE_SERVER='ENDPOINT'; DYNATRACE_SPACE=space; DYNATRACE_TOKEN=synthetic-key; DYNATRACE_TAG_VALUE=netdata; DYNATRACE_EVENT=CUSTOM_INFO`
			args := []string{"send-legacy", "--role", "silent", "--config", writeConfig(t, strings.ReplaceAll(settings, "ENDPOINT", server.URL)),
				"--config", writeConfig(t, strings.ReplaceAll(tt.overlay, "ENDPOINT", server.URL))}
			for _, method := range tt.methods {
				args = append(args, "--method", method)
			}
			var stdout, stderr bytes.Buffer
			assert.Equal(t, tt.code, Run(context.Background(), args, strings.NewReader(testutil.ValidEvent), &stdout, &stderr), stderr.String())
			assert.Equal(t, int32(tt.calls), calls.Load())
			assert.Contains(t, stderr.String(), tt.err)
			assert.Contains(t, stderr.String(), fmt.Sprintf("%d succeeded, 0 failed", tt.calls))
			assert.Empty(t, stdout.String())
			for _, secret := range []string{"synthetic-private-value", "synthetic-key", server.URL} {
				assert.NotContains(t, stderr.String(), secret)
			}
		})
	}
}
