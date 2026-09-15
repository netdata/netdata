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
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func pagerDutyTestAck(w http.ResponseWriter, version int64, key string) {
	field, status := "incident_key", 200
	if version == 2 {
		field, status = "dedup_key", 202
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "success", field: key})
}

func TestRunPagerDuty(t *testing.T) {
	for version, endpoint := range map[int64]string{1: "/generic/2010-04-15/create_event.json", 2: "/v2/enqueue"} {
		for name, test := range map[string]struct{ source, status, previous string }{
			"literal warning": {"literal", "WARNING", "CLEAR"}, "env critical": {"env", "CRITICAL", "WARNING"}, "file clear": {"file", "CLEAR", "CRITICAL"},
		} {
			t.Run(fmt.Sprintf("v%d/%s", version, name), func(t *testing.T) {
				type request struct {
					Method, Path, Query, ContentType, Accept, UserAgent, Authorization string
					Body                                                               map[string]any
				}
				requests := make(chan request, 4)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					requests <- request{r.Method, r.URL.EscapedPath(), r.URL.RawQuery, r.Header.Get("Content-Type"), r.Header.Get("Accept"), r.Header.Get("User-Agent"), r.Header.Get("Authorization"), body}
					pagerDutyTestAck(w, version, pagerDutyTestIncidentKey)
				}))
				defer server.Close()
				dst := pagerDutyTestDestination(version)
				dst.APIURL = server.URL + "/proxy/"
				if version == 1 && test.source == "literal" {
					dst.APIVersion = nil
				}
				for _, field := range []struct {
					name  string
					value *string
				}{{"key", &dst.IntegrationKey}, {"url", &dst.APIURL}} {
					if test.source == "env" {
						key := "PAGERDUTY_TEST_" + strings.ToUpper(field.name)
						t.Setenv(key, " \n"+*field.value+"\n")
						*field.value = "${env:" + key + "}"
					}
					if test.source == "file" {
						path := filepath.Join(t.TempDir(), field.name)
						require.NoError(t, os.WriteFile(path, []byte(*field.value+"\n"), 0600))
						*field.value = "${file:" + path + "}"
					}
				}
				second, unused := dst, dst
				second.IntegrationKey = strings.Repeat("b", 32)
				unused.APIURL = "${file:" + filepath.Join(t.TempDir(), "absent") + "}"
				cfg := Config{
					Version:      1,
					Destinations: map[string]Destination{"target": dst, "second": second, "unused": unused},
					Routing: Routing{
						Roles:   map[string][]string{"ops": {"target", "second", "target"}},
						Default: []string{"target"},
					},
				}
				data, err := yaml.Marshal(cfg)
				require.NoError(t, err)
				event := expectedEvent()
				event.URL, event.Status, event.PreviousStatus = "https://example.com/alert?id=1&view=chart#details", test.status, test.previous
				input, err := json.Marshal(event)
				require.NoError(t, err)
				var stdout, stderr bytes.Buffer
				code := Run(
					context.Background(),
					[]string{"send", "--config", writeConfig(t, string(data)), "--role", "ops", "--role", "unknown"},
					bytes.NewReader(input),
					&stdout,
					&stderr,
				)
				require.Equal(t, 0, code, stderr.String())
				assert.Empty(t, stdout.String())
				require.Len(t, requests, 2)
				for _, key := range []string{pagerDutyTestKey, second.IntegrationKey} {
					want := pagerDutyTestFixture(t, version, "full", test.status, test.previous)
					if version == 1 {
						want["service_key"] = key
					} else {
						want["routing_key"] = key
					}
					assert.Equal(
						t,
						request{
							"POST",
							"/proxy" + endpoint,
							"",
							"application/json",
							"application/json",
							"netdata-alarm-notify",
							"",
							want,
						},
						<-requests,
					)
					assert.NotContains(t, stderr.String(), key)
				}
				assert.Contains(t, stderr.String(), "2 succeeded, 0 failed")
				assert.NotContains(t, stderr.String(), server.URL)
			})
		}
	}
}

func TestRunPagerDutyFailures(t *testing.T) {
	for version, accepted := range map[int64]int{1: 200, 2: 202} {
		for name, test := range map[string]struct {
			status      int
			body, err   string
			withSuccess bool
		}{
			"HTTP": {401, "synthetic-private-value", "HTTP 401", false}, "v1 rate limit": {403, "", "HTTP 403", false},
			"v2 rate limit": {429, "", "HTTP 429", false}, "server error": {500, "", "HTTP 500", false},
			"redirect": {307, "", "HTTP 307", false}, "invalid ack": {accepted, "synthetic-private-value", "invalid", false},
			"missing key": {accepted, `{"status":"success"}`, "acknowledgment", false}, "mixed fanout": {400, "synthetic-private-value", "HTTP 400", true},
		} {
			t.Run(fmt.Sprintf("v%d/%s", version, name), func(t *testing.T) {
				var target, after, redirect atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, _ = io.Copy(io.Discard, r.Body)
					switch r.URL.Path {
					case "/after":
						after.Add(1)
						w.WriteHeader(204)
					case "/redirect":
						redirect.Add(1)
						w.WriteHeader(204)
					default:
						target.Add(1)
						w.Header().Set("Location", "/redirect")
						w.WriteHeader(test.status)
						_, _ = io.WriteString(w, test.body)
					}
				}))
				defer server.Close()
				dst := pagerDutyTestDestination(version)
				dst.APIURL = server.URL
				cfg := Config{
					Version: 1,
					Destinations: map[string]Destination{
						"target": dst,
						"after":  {Type: "webhook", URL: server.URL + "/after"},
					},
					Routing: Routing{Roles: map[string][]string{"ops": {"target", "after"}}},
				}
				data, err := yaml.Marshal(cfg)
				require.NoError(t, err)
				args := []string{"send", "--config", writeConfig(t, string(data)), "--destination", "target"}
				if test.withSuccess {
					args[len(args)-2], args[len(args)-1] = "--role", "ops"
				}
				var stdout, stderr bytes.Buffer
				code := Run(context.Background(), args, strings.NewReader(validEvent), &stdout, &stderr)
				if test.withSuccess {
					assert.Equal(t, 0, code)
					assert.EqualValues(t, 1, after.Load())
				} else {
					assert.Equal(t, 1, code)
					assert.Zero(t, after.Load())
				}
				assert.EqualValues(t, 1, target.Load())
				assert.Zero(t, redirect.Load())
				assert.Empty(t, stdout.String())
				assert.Contains(t, stderr.String(), test.err)
				for _, secret := range []string{"synthetic-private-value", dst.IntegrationKey, server.URL} {
					assert.NotContains(t, stderr.String(), secret)
				}
			})
		}
	}
}

func TestRunPagerDutySecrets(t *testing.T) {
	for version := range map[int64]struct{}{1: {}, 2: {}} {
		for name, test := range map[string]struct{ field, value, err string }{
			"empty key": {"integration_key", " \n", "empty value"}, "key controls": {"integration_key", "synthetic-private-value\nother", "without whitespace"},
			"short key": {"integration_key", "synthetic-private-value", "32 characters"}, "nested key": {"integration_key", "${env:NESTED}", "without whitespace"},
			"URL query": {"api_url", "https://example.com/?synthetic-private-value", "query"}, "URL fragment": {"api_url", "https://example.com/#", "fragment"},
			"US HTTP": {"api_url", "http://EVENTS.PAGERDUTY.COM./", "HTTPS"}, "EU HTTP": {"api_url", "http://EVENTS.EU.PAGERDUTY.COM./", "HTTPS"},
		} {
			t.Run(fmt.Sprintf("v%d/%s", version, name), func(t *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }),
				)
				defer server.Close()
				dst := pagerDutyTestDestination(version)
				dst.APIURL = server.URL
				t.Setenv("PAGERDUTY_INVALID_SECRET", test.value)
				if test.field == "integration_key" {
					dst.IntegrationKey = "${env:PAGERDUTY_INVALID_SECRET}"
				} else {
					dst.APIURL = "${env:PAGERDUTY_INVALID_SECRET}"
					dst.IntegrationKey = "${file:" + filepath.Join(t.TempDir(), "unread-key") + "}"
				}
				data, err := yaml.Marshal(Config{Version: 1, Destinations: map[string]Destination{"target": dst}})
				require.NoError(t, err)
				var stdout, stderr bytes.Buffer
				code := Run(
					context.Background(),
					[]string{"send", "--config", writeConfig(t, string(data)), "--destination", "target"},
					strings.NewReader(validEvent),
					&stdout,
					&stderr,
				)
				assert.Equal(t, 1, code)
				assert.Zero(t, calls.Load())
				assert.Empty(t, stdout.String())
				assert.Contains(t, stderr.String(), test.err)
				assert.NotContains(t, stderr.String(), "synthetic-private-value")
				assert.NotContains(t, stderr.String(), "could not read secret file")
			})
		}
	}
}

func TestPagerDutyRequestLimit(t *testing.T) {
	for version := range map[int64]struct{}{1: {}, 2: {}} {
		for name, test := range map[string]struct {
			symbol string
			extra  int
		}{"ASCII boundary": {"x", 0}, "ASCII over": {"x", 1}, "escaped boundary": {"<", 0}, "escaped over": {"<", 1}} {
			t.Run(fmt.Sprintf("v%d/%s", version, name), func(t *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					data, err := io.ReadAll(r.Body)
					assert.NoError(t, err)
					assert.Len(t, data, 512*1024)
					calls.Add(1)
					pagerDutyTestAck(w, version, pagerDutyTestIncidentKey)
				}))
				defer server.Close()
				dst := pagerDutyTestDestination(version)
				dst.APIURL = server.URL
				event := expectedEvent()
				event.Info = "x"
				base, err := json.Marshal(renderPagerDuty(dst, event))
				require.NoError(t, err)
				remaining := 512*1024 - len(base) + 1
				width := 1
				if test.symbol == "<" {
					width = 6
				}
				event.Info = strings.Repeat(
					test.symbol,
					remaining/width,
				) + strings.Repeat(
					"x",
					remaining%width+test.extra,
				)
				err = sendPagerDuty(context.Background(), dst, event, time.Second)
				if test.extra == 0 {
					require.NoError(t, err)
					assert.EqualValues(t, 1, calls.Load())
				} else {
					require.ErrorContains(t, err, "512 KiB")
					assert.Zero(t, calls.Load())
				}
			})
		}
	}
}
