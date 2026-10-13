// SPDX-License-Identifier: GPL-3.0-or-later

package app

import (
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"

	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRunIncidentDelivery(t *testing.T) {
	for provider, p := range map[string]struct{ status int }{"ilert": {202}, "signl4": {201}} {
		t.Run(provider, func(t *testing.T) {
			for name, test := range map[string]struct {
				status               int
				secret, err          string
				explicit, skip, drop bool
			}{
				"mixed fanout and role dedup": {}, "explicit": {explicit: true},
				"environment": {secret: "env"}, "files": {secret: "file"},
				"empty secret":         {secret: "empty", skip: true, err: "empty value"},
				"nested reference":     {secret: "nested", skip: true, err: "absolute HTTP(S)"},
				"invalid resolved URL": {secret: "relative", skip: true, err: "absolute HTTP(S)"},
				"resolved user info":   {secret: "userinfo", skip: true, err: "user information"},
				"resolved fragment":    {secret: "fragment", skip: true, err: "fragment"},
				"HTTP 200":             {status: 200}, "HTTP 201": {status: 201}, "HTTP 202": {status: 202},
				"HTTP 204": {status: 204}, "HTTP 206": {status: 206},
				"unauthorized": {status: 401}, "redirect": {status: 307}, "rate limit": {status: 429},
				"server error": {status: 503}, "all fail": {explicit: true, status: 403},
				"transport": {drop: true, err: "transport failed"},
			} {
				t.Run(name, func(t *testing.T) {
					type received struct {
						path, query, method, contentType, authorization, agent, body string
						err                                                          error
					}
					requests := make(chan received, 8)
					path := "/" + provider + "/synthetic-private-value"
					if provider == "ilert" {
						path += "/events"
					}
					status := test.status
					if status == 0 {
						status = p.status
					}
					accepted := status == 202 || provider == "signl4" && (status == 200 || status == 201)
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						body, err := io.ReadAll(r.Body)
						requests <- received{r.URL.Path, r.URL.RawQuery, r.Method, r.Header.Get("Content-Type"), r.Header.Get("Authorization"), r.UserAgent(), string(body), err}
						if r.URL.Path == path {
							if test.drop {
								conn, _, err := w.(http.Hijacker).Hijack()
								if err == nil {
									_ = conn.Close()
								}
								return
							}
							w.Header().Set("Location", "/unexpected")
							w.Header().Set("Retry-After", "0")
							w.WriteHeader(status)
						} else if strings.HasSuffix(r.URL.Path, "/events") {
							w.WriteHeader(202)
						} else {
							w.WriteHeader(200)
						}
						_, _ = io.WriteString(w, "synthetic-private-value")
					}))
					defer server.Close()
					cfg := testConfig{Version: 1, Destinations: map[string]map[string]any{
						"ilert":         {"type": "ilert", "api_url": server.URL + "/ilert/synthetic-private-value/", "integration_key": "synthetic-key"},
						"signl4":        {"type": "signl4", "url": server.URL + "/signl4/synthetic-private-value?keep=a%26b&keep=second"},
						"archive":       {"type": "webhook", "url": server.URL + "/archive"},
						"unused_ilert":  {"type": "ilert", "integration_key": "${env:UNUSED_INCIDENT_KEY}", "api_url": "${env:UNUSED_INCIDENT_API}"},
						"unused_signl4": {"type": "signl4", "url": "${env:UNUSED_INCIDENT_URL}"},
					}, Routing: notifier.Routing{Roles: map[string][]string{"ops": {"ilert", "signl4", "archive"}, "dba": {"signl4", "ilert"}}}}
					dst := cfg.Destinations[provider]
					if test.secret != "" {
						endpoint, _ := dst["url"].(string)
						if provider == "ilert" {
							endpoint = dst["api_url"].(string)
						}
						switch test.secret {
						case "empty":
							endpoint = ""
						case "nested":
							endpoint = "${env:NESTED_URL}"
						case "relative":
							endpoint = "/synthetic-private-value"
						case "userinfo":
							endpoint = "https://user:synthetic-private-value@example.com"
						case "fragment":
							endpoint += "#synthetic-private-value"
						}
						t.Setenv("INCIDENT_TEST_URL", " "+endpoint+"\n")
						t.Setenv("INCIDENT_TEST_KEY", " synthetic-key\n")
						urlRef, keyRef := "${env:INCIDENT_TEST_URL}", "${env:INCIDENT_TEST_KEY}"
						if test.secret == "file" {
							urlPath, keyPath := filepath.Join(t.TempDir(), "url"), filepath.Join(t.TempDir(), "key")
							require.NoError(t, os.WriteFile(urlPath, []byte(" "+endpoint+"\n"), 0600))
							require.NoError(t, os.WriteFile(keyPath, []byte(" synthetic-key\n"), 0600))
							urlRef, keyRef = "${file:"+urlPath+"}", "${file:"+keyPath+"}"
						}
						if provider == "ilert" {
							dst["api_url"], dst["integration_key"] = urlRef, keyRef
						} else {
							dst["url"] = urlRef
						}
					}
					cfg.Destinations[provider] = dst
					config, err := yaml.Marshal(cfg)
					require.NoError(t, err)
					event := testutil.ExpectedEvent()
					event.URL = "https://example.com/alert?id=1&view=chart#details"
					input, err := json.Marshal(event)
					require.NoError(t, err)
					args := []string{"send", "--config", writeConfig(t, string(config))}
					if test.explicit {
						args = append(args, "--destination", provider)
					} else {
						args = append(args, "--role", "ops", "--role", "dba")
					}
					var stdout, stderr bytes.Buffer
					code := Run(context.Background(), args, bytes.NewReader(input), &stdout, &stderr)
					failed := !accepted || test.err != ""
					wantCode := 0
					if failed && test.explicit {
						wantCode = 1
					}
					assert.Equal(t, wantCode, code, stderr.String())
					assert.Empty(t, stdout.String())
					for _, secret := range []string{"synthetic-key", "synthetic-private-value", server.URL} {
						assert.NotContains(t, stderr.String(), secret)
					}
					if test.err != "" {
						assert.Contains(t, stderr.String(), test.err)
					} else if !accepted {
						assert.Contains(t, stderr.String(), "returned HTTP")
					} else {
						assert.Contains(t, stderr.String(), `destination "`+provider+`" sent`)
					}
					if failed && test.explicit {
						assert.Contains(t, stderr.String(), "all attempted destinations failed")
					}
					selected := []string{"ilert", "signl4", "archive"}
					if test.explicit {
						selected = []string{provider}
					}
					var want []received
					for _, target := range selected {
						if target == provider && test.skip {
							continue
						}
						request := received{
							path:        "/" + target + "/synthetic-private-value",
							method:      "POST",
							contentType: "application/json",
							agent:       "netdata-alarm-notify",
						}
						payload := input
						if target == "archive" {
							request.path = "/archive"
						} else {
							payload, err = os.ReadFile(fixturePath(target + "-full.json"))
							require.NoError(t, err)
							if target == "ilert" {
								request.path += "/events"
							} else {
								request.query = "keep=a%26b&keep=second"
							}
						}
						request.body = string(payload)
						want = append(want, request)
					}
					assert.Len(t, requests, len(want), "one request per selected destination; no retries or redirects")
					for _, request := range want {
						select {
						case got := <-requests:
							assert.JSONEq(t, request.body, got.body)
							request.body, got.body = "", ""
							assert.Equal(t, request, got)
						default:
							t.Fatal("missing expected request")
						}
					}
				})
			}
		})
	}
}

func TestRunIlertResolvedCredentials(t *testing.T) {
	for name, test := range map[string]struct{ field, value, err string }{
		"empty key":          {"integration_key", "", "empty value"},
		"key whitespace":     {"integration_key", "synthetic key", "without whitespace"},
		"key controls":       {"integration_key", "synthetic\x01key", "printable ASCII"},
		"nested key":         {"integration_key", "${env:UNRESOLVED}", "printable ASCII"},
		"insecure official":  {"api_url", "http://api.ilert.com/api", "requires HTTPS"},
		"API query":          {"api_url", "https://example.com/?key=synthetic-key", "query"},
		"API empty fragment": {"api_url", "https://example.com/#", "fragment"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("ILERT_INVALID_FIELD", test.value)
			dst := map[string]string{
				"type":            "ilert",
				"integration_key": "synthetic-key",
				"api_url":         "https://example.com/api",
				test.field:        "${env:ILERT_INVALID_FIELD}",
			}
			cfg, err := yaml.Marshal(map[string]any{"version": 1, "destinations": map[string]any{"ilert": dst}})
			require.NoError(t, err)
			var stdout, stderr bytes.Buffer
			code := Run(
				context.Background(),
				[]string{"send", "--config", writeConfig(t, string(cfg)), "--destination", "ilert"},
				strings.NewReader(testutil.ValidEvent),
				&stdout,
				&stderr,
			)
			assert.Equal(t, 1, code)
			assert.Contains(t, stderr.String(), test.err)
			assert.NotContains(t, stderr.String(), "synthetic")
			assert.NotContains(t, stderr.String(), "example.com")
			assert.Empty(t, stdout.String())
		})
	}
}

func TestRunIncidentLifecycle(t *testing.T) {
	for name, test := range map[string]struct{ id, ilertKey string }{
		"opaque":                 {"test-incident", "1532eea2c9f6a386140992fc18e52fbde95854d42a4bfa2e46d1c0c5d280c456"},
		"case distinction":       {"Test-Incident", "02e4cf1b2d5e55b0a0074760a3759b70ae186d40700ee728657abbd27f460e7a"},
		"whitespace distinction": {" test-incident ", "7300984238f2b8a59dd1b10483ab47341e23cd30d2274e7c8337cdd1d5b836f8"},
		"Unicode":                {"事件", "c560201b331c54435b445baebe03b8922d6ef6a80065411ed3f379eb8bef9848"},
	} {
		t.Run(name, func(t *testing.T) {
			type received struct {
				path string
				body []byte
			}
			requests := make(chan received, 8)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				requests <- received{r.URL.Path, body}
				w.WriteHeader(202)
			}))
			defer server.Close()
			cfg := testConfig{Version: 1, Destinations: map[string]map[string]any{
				"ilert":  {"type": "ilert", "integration_key": "synthetic-key", "api_url": server.URL + "/api"},
				"signl4": {"type": "signl4", "url": server.URL + "/webhook"},
			}, Routing: notifier.Routing{Roles: map[string][]string{"ops": {"ilert", "signl4"}}}}
			config, err := yaml.Marshal(cfg)
			require.NoError(t, err)
			args := []string{"send", "--config", writeConfig(t, string(config)), "--role", "ops"}
			// Separate invocations change all presentation facts; only incident_id supplies correlation.
			for step, status := range []string{"WARNING", "CRITICAL", "CLEAR"} {
				event := notifyevent.Event{
					Version:    1,
					IncidentID: test.id,
					Timestamp:  testutil.ExpectedEvent().Timestamp.Add(time.Duration(step) * time.Minute),
					Node:       "node-" + status,
					Alert:      "alert-" + status,
					Summary:    "summary-" + status,
					Status:     status,
				}
				input, err := json.Marshal(event)
				require.NoError(t, err)
				var stdout, stderr bytes.Buffer
				assert.Zero(
					t,
					Run(context.Background(), args, bytes.NewReader(input), &stdout, &stderr),
					stderr.String(),
				)
				require.Len(t, requests, 2)
				ilertType, signl4Status := "ALERT", "new"
				if status == "CLEAR" {
					ilertType, signl4Status = "RESOLVE", "resolved"
				}
				text := event.Summary + "\nNode: " + event.Node + "\nAlert: " + event.Alert + "\nStatus: " + status + "\nTime: " + event.Timestamp.Format(
					time.RFC3339,
				)
				title := event.Node + " " + status + ": " + event.Summary
				for _, want := range []struct {
					path    string
					message any
				}{
					{"/api/events", map[string]any{"integrationKey": "synthetic-key", "eventType": ilertType, "alertKey": test.ilertKey, "summary": title, "details": text, "customDetails": event}},
					{"/webhook", map[string]any{"Title": title, "Message": text, "Severity": status, "X-S4-ExternalID": test.id, "X-S4-Status": signl4Status, "X-S4-SourceSystem": "Netdata"}},
				} {
					got := <-requests
					payload, err := json.Marshal(want.message)
					require.NoError(t, err)
					assert.Equal(t, want.path, got.path)
					assert.JSONEq(t, string(payload), string(got.body))
				}
			}
		})
	}
}
