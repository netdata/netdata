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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRunMonitoringDelivery(t *testing.T) {
	for provider, p := range map[string]struct{ path string }{
		"alerta":    {"/alerta/private-endpoint/alert"},
		"dynatrace": {"/dynatrace/private-endpoint/api/v2/events/ingest"},
	} {
		t.Run(provider, func(t *testing.T) {
			for name, test := range map[string]struct {
				status                                  int
				secret, err                             string
				explicit, skip, drop, badAck, anonymous bool
			}{
				"mixed fanout and dedup": {}, "explicit": {explicit: true},
				"env": {secret: "env"}, "file": {secret: "file"},
				"empty URL":     {secret: "empty", skip: true, err: "empty value"},
				"nested URL":    {secret: "nested", skip: true, err: "absolute HTTP(S)"},
				"relative URL":  {secret: "relative", skip: true, err: "absolute HTTP(S)"},
				"URL user info": {secret: "userinfo", skip: true, err: "user information"},
				"URL fragment":  {secret: "fragment", skip: true, err: "fragment"},
				"URL query":     {secret: "query", skip: true, err: "query"},
				"empty key":     {secret: "empty-key", skip: true, err: "empty value"},
				"invalid key":   {secret: "invalid-key", skip: true, err: "without whitespace"},
				"nested key":    {secret: "nested-key", skip: true, err: "without whitespace"},
				"Unicode key":   {secret: "unicode-key", skip: true, err: "printable ASCII"},
				"HTTP 200":      {status: 200}, "HTTP 201": {status: 201}, "HTTP 202": {status: 202},
				"HTTP 204": {status: 204}, "HTTP 206": {status: 206},
				"redirect": {status: 307}, "rate limit": {status: 429}, "unauthorized": {status: 401}, "server error": {status: 503},
				"all fail": {status: 403, explicit: true}, "all suppressed or unaccepted": {status: 202, explicit: true},
				"bad acknowledgment": {badAck: true, err: "acknowledge", explicit: true},
				"transport":          {drop: true, err: "transport failed"},
				"anonymous Alerta":   {anonymous: true},
			} {
				if test.anonymous && provider != "alerta" {
					continue
				}
				t.Run(name, func(t *testing.T) {
					type received struct {
						path, query, method, contentType, authorization, agent, body string
						err                                                          error
					}
					requests := make(chan received, 8)
					status := test.status
					if status == 0 {
						status = 201
					}
					accepted := status == 201 || provider == "alerta" && status == 200
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						body, err := io.ReadAll(r.Body)
						requests <- received{r.URL.Path, r.URL.RawQuery, r.Method, r.Header.Get("Content-Type"), r.Header.Get("Authorization"), r.UserAgent(), string(body), err}
						ack := alertaTestAck
						if strings.HasSuffix(r.URL.Path, "/ingest") {
							ack = dynatraceTestAck
						}
						if r.URL.Path == p.path {
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
							if test.badAck {
								ack = `{}`
							}
						} else {
							w.WriteHeader(201)
						}
						_, _ = io.WriteString(w, ack)
					}))
					defer server.Close()
					cfg := testConfig{Version: 1, Destinations: map[string]map[string]any{
						"alerta":           {"type": "alerta", "api_url": server.URL + "/alerta/private-endpoint/", "api_key": "synthetic-key", "environment": "Production"},
						"dynatrace":        {"type": "dynatrace", "api_url": server.URL + "/dynatrace/private-endpoint/", "api_token": "synthetic-key", "entity_selector": `type(HOST),tag("netdata")`},
						"archive":          {"type": "webhook", "url": server.URL + "/archive"},
						"unused_alerta":    {"type": "alerta", "api_url": "${env:UNUSED_MONITORING_URL}", "api_key": "${env:UNUSED_MONITORING_KEY}", "environment": "Production"},
						"unused_dynatrace": {"type": "dynatrace", "api_url": "${env:UNUSED_MONITORING_URL}", "api_token": "${env:UNUSED_MONITORING_KEY}", "entity_selector": "type(HOST)"},
					}, Routing: notifier.Routing{Roles: map[string][]string{"ops": {"alerta", "dynatrace", "archive"}, "dba": {"dynatrace", "alerta"}}}}
					dst := cfg.Destinations[provider]
					if test.anonymous {
						dst["api_key"] = ""
					}
					if test.secret != "" {
						endpoint, key := dst["api_url"].(string), "synthetic-key"
						switch test.secret {
						case "empty":
							endpoint = ""
						case "nested":
							endpoint = "${env:NESTED_URL}"
						case "relative":
							endpoint = "/private-endpoint"
						case "userinfo":
							endpoint = "https://user:synthetic-key@example.com"
						case "fragment":
							endpoint += "#synthetic-key"
						case "query":
							endpoint += "?synthetic-key"
						case "empty-key":
							key = ""
						case "invalid-key":
							key = "synthetic-key\ninvalid"
						case "nested-key":
							key = "${env:NESTED_KEY}"
						case "unicode-key":
							key = "synthetic-key界"
						}
						t.Setenv("MONITORING_TEST_URL", " "+endpoint+"\n")
						t.Setenv("MONITORING_TEST_KEY", " "+key+"\n")
						urlRef, keyRef := "${env:MONITORING_TEST_URL}", "${env:MONITORING_TEST_KEY}"
						if test.secret == "file" {
							urlPath, keyPath := filepath.Join(t.TempDir(), "url"), filepath.Join(t.TempDir(), "key")
							require.NoError(t, os.WriteFile(urlPath, []byte(" "+endpoint+"\n"), 0600))
							require.NoError(t, os.WriteFile(keyPath, []byte(" "+key+"\n"), 0600))
							urlRef, keyRef = "${file:"+urlPath+"}", "${file:"+keyPath+"}"
						}
						dst["api_url"] = urlRef
						if provider == "alerta" {
							dst["api_key"] = keyRef
						} else {
							dst["api_token"] = keyRef
						}
					}
					cfg.Destinations[provider] = dst
					data, err := yaml.Marshal(cfg)
					require.NoError(t, err)
					event := testutil.ExpectedEvent()
					event.URL = "https://example.com/alert?id=1&view=chart#details"
					input, err := json.Marshal(event)
					require.NoError(t, err)
					args := []string{"send", "--config", writeConfig(t, string(data))}
					selected := []string{"alerta", "dynatrace", "archive"}
					if test.explicit {
						args = append(args, "--destination", provider)
						selected = []string{provider}
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
					for _, secret := range []string{"synthetic-key", "private-endpoint", server.URL, "NESTED_KEY"} {
						assert.NotContains(t, stderr.String(), secret)
					}
					if test.err != "" {
						assert.Contains(t, stderr.String(), test.err)
					} else if provider == "alerta" && status == 202 {
						assert.Contains(t, stderr.String(), "suppressed")
					} else if !accepted {
						assert.Contains(t, stderr.String(), "returned HTTP")
					} else {
						assert.Contains(t, stderr.String(), `destination "`+provider+`" sent`)
					}
					wantRequests := len(selected)
					if test.skip {
						wantRequests--
					}
					require.Len(t, requests, wantRequests)
					for _, target := range selected {
						if target == provider && test.skip {
							continue
						}
						got := <-requests
						want := received{
							path:        "/archive",
							method:      "POST",
							contentType: "application/json",
							agent:       "netdata-alarm-notify",
						}
						if target == "archive" {
							assert.JSONEq(t, string(input), got.body)
						} else {
							want.path = "/" + target + "/private-endpoint/alert"
							want.authorization = "Key synthetic-key"
							if target == "dynatrace" {
								want.path = "/dynatrace/private-endpoint/api/v2/events/ingest"
								want.authorization = "Api-Token synthetic-key"
							}
							if target == "alerta" && test.anonymous {
								want.authorization = ""
							}
							fixture, err := os.ReadFile(fixturePath(target + "-full.json"))
							require.NoError(t, err)
							assertMonitoringPayload(t, target, string(fixture), got.body)
						}
						got.body = ""
						assert.Equal(t, want, got)
					}
				})
			}
		})
	}
}

func TestRunMonitoringLifecycle(t *testing.T) {
	for name, test := range map[string]struct{ chart, resource, alert string }{
		"regular":   {"system.cpu", "test-node", "system.cpu.test_alert"},
		"httpcheck": {"httpcheck.website", "httpcheck.website", "test_alert"},
	} {
		t.Run(name, func(t *testing.T) {
			type received struct {
				path string
				body []byte
			}
			requests := make(chan received, 12)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				requests <- received{r.URL.Path, body}
				w.WriteHeader(201)
				ack := alertaTestAck
				if strings.HasSuffix(r.URL.Path, "/ingest") {
					ack = dynatraceTestAck
				}
				_, _ = io.WriteString(w, ack)
			}))
			defer server.Close()
			data, err := yaml.Marshal(testConfig{Version: 1, Destinations: map[string]map[string]any{
				"production":  {"type": "alerta", "api_url": server.URL, "environment": "Production"},
				"development": {"type": "alerta", "api_url": server.URL, "environment": "Development"},
				"dynatrace":   {"type": "dynatrace", "api_url": server.URL, "api_token": "synthetic-key", "entity_selector": `type(HOST),tag("a,b")`, "event_type": "CUSTOM_ALERT", "source": "Synthetic source"},
			}, Routing: notifier.Routing{Roles: map[string][]string{"ops": {"production", "development", "dynatrace"}}}})
			require.NoError(t, err)
			path := writeConfig(t, string(data))
			for _, status := range []string{"WARNING", "CRITICAL", "CLEAR"} {
				event := testutil.ExpectedEvent()
				event.Status, event.Chart, event.Summary = status, test.chart, status+" summary"
				input, err := json.Marshal(event)
				require.NoError(t, err)
				var stdout, stderr bytes.Buffer
				require.Equal(
					t,
					0,
					Run(
						context.Background(),
						[]string{"send", "--config", path, "--role", "ops"},
						bytes.NewReader(input),
						&stdout,
						&stderr,
					),
					stderr.String(),
				)
				for _, environment := range []string{"Production", "Development"} {
					received := <-requests
					assert.Equal(t, "/alert", received.path)
					var got struct {
						Environment string `json:"environment"`
						Resource    string `json:"resource"`
						Event       string `json:"event"`
						Severity    string `json:"severity"`
						Text        string `json:"text"`
					}
					require.NoError(t, json.Unmarshal(received.body, &got))
					severity := strings.ToLower(status)
					if status == "CLEAR" {
						severity = "cleared"
					}
					assert.Equal(
						t,
						[]string{environment, test.resource, test.alert, severity},
						[]string{got.Environment, got.Resource, got.Event, got.Severity},
					)
					assert.Contains(t, got.Text, status+" summary")
				}
				received := <-requests
				assert.Equal(t, "/api/v2/events/ingest", received.path)
				var got map[string]any
				require.NoError(t, json.Unmarshal(received.body, &got))
				assert.Equal(t, "CUSTOM_ALERT", got["eventType"], "CLEAR retains configured type")
				assert.Equal(t, `type(HOST),tag("a,b")`, got["entitySelector"])
				assert.Equal(t, "test-node "+status+": "+status+" summary", got["title"])
				assert.NotContains(t, got, "startTime")
				assert.NotContains(t, got, "endTime")
				assert.NotContains(t, got, "timeout")
			}
			assert.Empty(t, requests)
		})
	}
}

func TestRunDynatracePropertyLimit(t *testing.T) {
	for field := range map[string]struct{}{"incident_id": {}, "info": {}} {
		t.Run(field, func(t *testing.T) {
			requests := make(chan string, 4)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- r.URL.Path
				w.WriteHeader(204)
			}))
			defer server.Close()
			config, err := yaml.Marshal(testConfig{Version: 1, Destinations: map[string]map[string]any{
				"dynatrace": {"type": "dynatrace", "api_url": server.URL, "api_token": "synthetic-key", "entity_selector": "type(HOST)"},
				"archive":   {"type": "webhook", "url": server.URL + "/archive"},
			}, Routing: notifier.Routing{Roles: map[string][]string{"ops": {"dynatrace", "archive"}}}})
			require.NoError(t, err)
			event := testutil.ExpectedEvent()
			if field == "incident_id" {
				event.IncidentID = strings.Repeat("界", 4097)
			} else {
				event.Info = strings.Repeat("界", 4097)
			}
			input, err := json.Marshal(event)
			require.NoError(t, err)
			var stdout, stderr bytes.Buffer
			code := Run(
				context.Background(),
				[]string{"send", "--config", writeConfig(t, string(config)), "--role", "ops"},
				bytes.NewReader(input),
				&stdout,
				&stderr,
			)
			assert.Equal(t, 0, code)
			assert.Contains(t, stderr.String(), "dynatrace event property exceeds 4096 characters")
			assert.NotContains(t, stderr.String(), "synthetic-key")
			require.Len(t, requests, 1)
			assert.Equal(t, "/archive", <-requests)
		})
	}
}

func TestRunAlertaPublicAPIHTTPS(t *testing.T) {
	for host := range map[string]struct{}{
		"api.alerta.io": {}, "api.alerta.dev": {}, "alerta-api.fly.dev": {},
	} {
		for mode := range map[string]struct{}{"env": {}, "file": {}} {
			t.Run(host+"/"+mode, func(t *testing.T) {
				endpoint := " http://" + strings.ToUpper(host) + ".:80/api/\n"
				t.Setenv("ALERTA_TEST_PUBLIC_API", endpoint)
				reference := "${env:ALERTA_TEST_PUBLIC_API}"
				if mode == "file" {
					path := filepath.Join(t.TempDir(), "api-url")
					require.NoError(t, os.WriteFile(path, []byte(endpoint), 0600))
					reference = "${file:" + path + "}"
				}
				// A missing key also prevents a live request if URL validation regresses.
				missingKey := "${file:" + filepath.Join(t.TempDir(), "missing-key") + "}"
				config, err := yaml.Marshal(testConfig{Version: 1, Destinations: map[string]map[string]any{
					"alerta": {"type": "alerta", "api_url": reference, "api_key": missingKey, "environment": "Production"},
				}})
				require.NoError(t, err)
				path := writeConfig(t, string(config))
				var stdout, stderr bytes.Buffer
				require.Equal(
					t,
					0,
					Run(context.Background(), []string{"validate", "--config", path}, nil, &stdout, &stderr),
				)
				stdout.Reset()
				stderr.Reset()
				code := Run(
					context.Background(),
					[]string{"send", "--config", path, "--destination", "alerta"},
					strings.NewReader(testutil.ValidEvent),
					&stdout,
					&stderr,
				)
				assert.Equal(t, 1, code)
				assert.Empty(t, stdout.String())
				assert.Contains(t, stderr.String(), "requires HTTPS")
				assert.NotContains(t, stderr.String(), "could not read secret file")
				assert.NotContains(t, stderr.String(), host)
			})
		}
	}
}
