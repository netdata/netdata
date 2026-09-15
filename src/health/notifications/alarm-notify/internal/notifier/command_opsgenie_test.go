// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"bytes"
	"context"
	"encoding/json"
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

func TestRunOpsgenie(t *testing.T) {
	for source := range map[string]struct{}{"literal": {}, "env": {}, "file": {}} {
		for status, previous := range map[string]string{"WARNING": "CLEAR", "CRITICAL": "WARNING", "CLEAR": "CRITICAL"} {
			t.Run(source+"/"+status, func(t *testing.T) {
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
					w.WriteHeader(202)
					_, _ = io.WriteString(w, opsgenieTestAck)
				}))
				defer server.Close()
				dst := opsgenieTestDestination()
				dst.APIURL = server.URL + "/proxy/"
				for _, field := range []struct {
					name  string
					value *string
				}{{"key", &dst.APIKey}, {"url", &dst.APIURL}} {
					if source == "env" {
						name := "OPSGENIE_TEST_" + strings.ToUpper(field.name)
						t.Setenv(name, " \n"+*field.value+"\n")
						*field.value = "${env:" + name + "}"
					}
					if source == "file" {
						path := filepath.Join(t.TempDir(), field.name)
						require.NoError(t, os.WriteFile(path, []byte(*field.value+"\n"), 0600))
						*field.value = "${file:" + path + "}"
					}
				}
				second, unused := dst, dst
				second.APIKey = "second-synthetic-key"
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
				event.URL = "https://example.com/alert?id=1&view=chart#details"
				event.Status, event.PreviousStatus = status, previous
				if status == "CRITICAL" {
					event.Timestamp = event.Timestamp.Add(time.Hour)
				}
				if status == "CLEAR" {
					event.Timestamp = event.Timestamp.Add(2 * time.Hour)
				}
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
				want := opsgenieTestFixture(t, "full", status)
				fixture, err := json.Marshal(want)
				require.NoError(t, err)
				require.NoError(
					t,
					json.Unmarshal(
						[]byte(
							strings.ReplaceAll(
								string(fixture),
								"2026-09-14T12:00:00Z",
								event.Timestamp.Format(time.RFC3339),
							),
						),
						&want,
					),
				)
				path, query := "/proxy/v2/alerts", ""
				if status == "CLEAR" {
					path += "/" + opsgenieTestAlias + "/close"
					query = "identifierType=alias"
				}
				for _, key := range []string{opsgenieTestKey, second.APIKey} {
					assert.Equal(
						t,
						request{
							"POST",
							path,
							query,
							"application/json",
							"application/json",
							"netdata-alarm-notify",
							"GenieKey " + key,
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

func TestRunOpsgenieDistinctIncidents(t *testing.T) {
	for id, want := range map[string]string{"test-incident": opsgenieTestAlias, "incident": "d4191834714542dcf3e5d8a6ab386c9b72259430730157b6e5c76469cbb6a622"} {
		t.Run(id, func(t *testing.T) {
			paths := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				paths <- r.URL.RequestURI()
				w.WriteHeader(202)
				_, _ = io.WriteString(w, opsgenieTestAck)
			}))
			defer server.Close()
			dst := opsgenieTestDestination()
			dst.APIURL = server.URL
			data, err := yaml.Marshal(Config{Version: 1, Destinations: map[string]Destination{"target": dst}})
			require.NoError(t, err)
			event := expectedEvent()
			event.IncidentID = id
			event.Status = "CLEAR"
			event.Timestamp = event.Timestamp.Add(3 * time.Hour)
			input, err := json.Marshal(event)
			require.NoError(t, err)
			var stdout, stderr bytes.Buffer
			code := Run(
				context.Background(),
				[]string{"send", "--config", writeConfig(t, string(data)), "--destination", "target"},
				bytes.NewReader(input),
				&stdout,
				&stderr,
			)
			require.Equal(t, 0, code, stderr.String())
			assert.Equal(t, "/v2/alerts/"+want+"/close?identifierType=alias", <-paths)
		})
	}
}

func TestRunOpsgenieFailures(t *testing.T) {
	for status := range map[string]struct{}{"WARNING": {}, "CLEAR": {}} {
		for name, test := range map[string]struct {
			status      int
			body, err   string
			withSuccess bool
		}{
			"HTTP": {401, "synthetic-private-value", "HTTP 401", false}, "rate limit": {429, "", "HTTP 429", false}, "server error": {500, "", "HTTP 500", false},
			"redirect": {307, "", "HTTP 307", false}, "invalid ack": {202, "synthetic-private-value", "invalid", false},
			"missing id": {202, `{"result":"Request will be processed"}`, "acknowledgment", false}, "mixed": {400, "synthetic-private-value", "HTTP 400", true},
		} {
			t.Run(status+"/"+name, func(t *testing.T) {
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
				dst := opsgenieTestDestination()
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
				code := Run(
					context.Background(),
					args,
					strings.NewReader(strings.ReplaceAll(validEvent, "WARNING", status)),
					&stdout,
					&stderr,
				)
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
				for _, secret := range []string{opsgenieTestKey, "synthetic-private-value", server.URL} {
					assert.NotContains(t, stderr.String(), secret)
				}
			})
		}
	}
}

func TestRunOpsgenieSecretsAndLimits(t *testing.T) {
	for name, test := range map[string]struct{ field, value, status, err string }{
		"empty key": {"api_key", " \n", "WARNING", "empty value"}, "control key": {"api_key", "synthetic-private-value\nother", "CLEAR", "without whitespace"},
		"nested key": {"api_key", "${env:NESTED}", "WARNING", "without whitespace"},
		"URL query":  {"api_url", "https://example.com/?synthetic-private-value", "WARNING", "query"},
		"US HTTP":    {"api_url", "http://API.OPSGENIE.COM./", "CLEAR", "HTTPS"}, "EU HTTP": {"api_url", "http://API.EU.OPSGENIE.COM./", "WARNING", "HTTPS"},
		"oversized details": {"info", strings.Repeat("界", 8001), "WARNING", "details"}, "oversized note": {"info", strings.Repeat("界", 25001), "CLEAR", "note"},
	} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }),
			)
			defer server.Close()
			dst := opsgenieTestDestination()
			dst.APIURL = server.URL
			event := expectedEvent()
			event.Status = test.status
			t.Setenv("OPSGENIE_INVALID_SECRET", test.value)
			switch test.field {
			case "api_key":
				dst.APIKey = "${env:OPSGENIE_INVALID_SECRET}"
			case "api_url":
				dst.APIURL = "${env:OPSGENIE_INVALID_SECRET}"
				dst.APIKey = "${file:" + filepath.Join(t.TempDir(), "unread-key") + "}"
			case "info":
				event.Info = test.value
			}
			data, err := yaml.Marshal(Config{Version: 1, Destinations: map[string]Destination{"target": dst}})
			require.NoError(t, err)
			input, err := json.Marshal(event)
			require.NoError(t, err)
			var stdout, stderr bytes.Buffer
			code := Run(
				context.Background(),
				[]string{"send", "--config", writeConfig(t, string(data)), "--destination", "target"},
				bytes.NewReader(input),
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
