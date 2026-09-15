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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRunChatWebhooks(t *testing.T) {
	for provider, p := range map[string]struct{ channel, sender string }{
		"rocketchat": {channel: "#alerts"}, "flock": {}, "fleep": {sender: "Netdata"},
	} {
		t.Run(provider, func(t *testing.T) {
			for name, test := range map[string]struct {
				status                                 int
				secret, response, err                  string
				explicit, skip, second, defaults, drop bool
			}{
				"mixed fanout": {}, "explicit": {explicit: true}, "second target": {second: true},
				"webhook defaults": {defaults: true}, "environment URL": {secret: "env"}, "file URL": {secret: "file"},
				"empty secret":         {secret: "empty", skip: true, err: "empty value"},
				"nested reference":     {secret: "nested", skip: true, err: "absolute HTTP(S)"},
				"bad resolved URL":     {secret: "bad-url", skip: true, err: "absolute HTTP(S)"},
				"resolved user info":   {secret: "user-info", skip: true, err: "user information"},
				"resolved fragment":    {secret: "fragment", skip: true, err: "fragment"},
				"wrong success status": {status: 201, err: "HTTP 201"}, "empty success status": {status: 204, err: "HTTP 204"},
				"unauthorized": {status: 401, err: "HTTP 401"}, "redirect refused": {status: 307, err: "HTTP 307"},
				"no rate limit retry": {status: 429, err: "HTTP 429"}, "no server retry": {status: 503, err: "HTTP 503"},
				"all fail":           {explicit: true, status: 403, err: "all selected destinations failed"},
				"transport":          {drop: true, err: "transport failed"},
				"arbitrary response": {response: "synthetic-private-value"},
			} {
				t.Run(name, func(t *testing.T) {
					type received struct {
						path, query, method, contentType, authorization, agent, body string
						err                                                          error
					}
					requests := make(chan received, 8)
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						body, err := io.ReadAll(r.Body)
						requests <- received{r.URL.Path, r.URL.RawQuery, r.Method, r.Header.Get("Content-Type"), r.Header.Get("Authorization"), r.UserAgent(), string(body), err}
						status, response := 200, `{"success":true}`
						if r.URL.Path == "/"+provider+"/synthetic-private-value" {
							if test.drop {
								conn, _, err := w.(http.Hijacker).Hijack()
								if err == nil {
									_ = conn.Close()
								}
								return
							}
							if test.status != 0 {
								status = test.status
							}
							if test.response != "" {
								response = test.response
							}
						}
						w.Header().Set("Location", "/unexpected")
						w.Header().Set("Retry-After", "0")
						w.WriteHeader(status)
						_, _ = io.WriteString(w, response)
					}))
					defer server.Close()
					cfg := Config{Version: 1, Destinations: map[string]Destination{
						"rocketchat": {
							Type:    "rocketchat",
							URL:     server.URL + "/rocketchat/synthetic-private-value?keep=a%26b&keep=second",
							Channel: "#alerts",
						},
						"flock": {
							Type: "flock",
							URL:  server.URL + "/flock/synthetic-private-value?keep=a%26b&keep=second",
						},
						"fleep": {
							Type:   "fleep",
							URL:    server.URL + "/fleep/synthetic-private-value?keep=a%26b&keep=second",
							Sender: "Netdata",
						},
						"archive": {
							Type:        "webhook",
							URL:         server.URL + "/archive",
							BearerToken: "synthetic-private-value",
						},
						"unused": {Type: provider, URL: "${env:UNUSED_CHAT_URL}"},
					}, Routing: Routing{Roles: map[string][]string{"ops": {"rocketchat", "flock", "fleep", "archive"}, "dba": {"rocketchat", "flock", "fleep"}}}}
					dst := cfg.Destinations[provider]
					if test.defaults {
						dst.Channel, dst.Sender = "", ""
					}
					if test.secret != "" {
						endpoint := dst.URL
						switch test.secret {
						case "empty":
							endpoint = ""
						case "nested":
							endpoint = "${env:NESTED_URL}"
						case "bad-url":
							endpoint = "/synthetic-private-value"
						case "user-info":
							endpoint = "https://user:synthetic-private-value@example.com/hook"
						case "fragment":
							endpoint += "#synthetic-private-value"
						}
						if test.secret == "file" {
							path := filepath.Join(t.TempDir(), "url")
							require.NoError(t, os.WriteFile(path, []byte(" "+endpoint+"\n"), 0600))
							dst.URL = "${file:" + path + "}"
						} else {
							t.Setenv("CHAT_WEBHOOK_TEST_URL", " "+endpoint+"\n")
							dst.URL = "${env:CHAT_WEBHOOK_TEST_URL}"
						}
					}
					cfg.Destinations[provider] = dst
					if test.second {
						cfg.Destinations["other"] = Destination{
							Type:    provider,
							URL:     server.URL + "/other",
							Channel: p.channel,
							Sender:  p.sender,
						}
						cfg.Routing.Roles["dba"] = append(cfg.Routing.Roles["dba"], "other")
					}
					config, err := yaml.Marshal(cfg)
					require.NoError(t, err)
					event := expectedEvent()
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
					wantCode := 0
					if test.explicit && test.err != "" {
						wantCode = 1
					}
					assert.Equal(t, wantCode, code)
					assert.Empty(t, stdout.String())
					assert.NotContains(t, stderr.String(), "synthetic-private-value")
					assert.NotContains(t, stderr.String(), server.URL)
					if test.err != "" {
						assert.Contains(t, stderr.String(), test.err)
					}
					if test.response != "" && provider == "rocketchat" {
						assert.Contains(t, stderr.String(), "invalid rocketchat response")
					}
					if test.err == "" && !(test.response != "" && provider == "rocketchat") {
						assert.Contains(t, stderr.String(), `destination "`+provider+`" sent`)
					}
					selected := []string{"rocketchat", "flock", "fleep", "archive"}
					if test.explicit {
						selected = []string{provider}
					}
					if test.second {
						selected = append(selected, "other")
					}
					var want []received
					for _, target := range selected {
						if target == provider && test.skip {
							continue
						}
						payload := input
						request := received{
							path:        "/" + target + "/synthetic-private-value",
							query:       "keep=a%26b&keep=second",
							method:      "POST",
							contentType: "application/json",
							agent:       "netdata-alarm-notify",
						}
						fixture := target
						if target == "other" {
							fixture = provider
							request.path, request.query = "/other", ""
						}
						if target == "archive" {
							request.path, request.query, request.authorization = "/archive", "", "Bearer synthetic-private-value"
						} else {
							payload, err = os.ReadFile(filepath.Join("testdata", fixture+"-full.json"))
							require.NoError(t, err)
							if test.defaults && target == provider {
								var message map[string]any
								require.NoError(t, json.Unmarshal(payload, &message))
								delete(message, "channel")
								delete(message, "user")
								payload, err = json.Marshal(message)
								require.NoError(t, err)
							}
						}
						request.body = string(payload)
						want = append(want, request)
					}
					assert.Len(
						t,
						requests,
						len(want),
						"one request per selected name, no retries, redirects or channel loop",
					)
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

func TestHTTPStatusAcknowledgment(t *testing.T) {
	for name, test := range map[string]struct {
		provider string
		status   int
	}{
		"Flock": {"flock", 200}, "Fleep": {"fleep", 200}, "ilert": {"ilert", 202},
		"SIGNL4 200": {"signl4", 200}, "SIGNL4 201": {"signl4", 201}, "SIGNL4 202": {"signl4", 202},
	} {
		t.Run(name, func(t *testing.T) {
			cleanup, stopped := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, "synthetic-private-value")
				w.(http.Flusher).Flush()
				select {
				case <-r.Context().Done():
					close(stopped)
				case <-cleanup:
				}
			}))
			defer server.Close()
			defer close(cleanup)
			dst := Destination{Type: test.provider, URL: server.URL}
			if test.provider == "ilert" {
				dst = Destination{Type: "ilert", APIURL: server.URL, IntegrationKey: "synthetic-key"}
			}
			cfg, err := yaml.Marshal(
				Config{
					Version:      1,
					Destinations: map[string]Destination{"chat": dst},
				},
			)
			require.NoError(t, err)
			var stdout, stderr bytes.Buffer
			code := Run(
				context.Background(),
				[]string{"send", "--config", writeConfig(t, string(cfg)), "--destination", "chat", "--timeout", "2s"},
				strings.NewReader(validEvent),
				&stdout,
				&stderr,
			)
			assert.Zero(t, code, "do not wait for or interpret the provider's response body")
			assert.Empty(t, stdout.String())
			assert.NotContains(t, stderr.String(), "synthetic-private-value")
			select {
			case <-stopped:
			case <-time.After(time.Second):
				t.Fatal("response body was not closed")
			}
		})
	}
}
