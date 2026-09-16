// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSendPush(t *testing.T) {
	for _, provider := range []string{"gotify", "ntfy"} {
		t.Run(provider, func(t *testing.T) {
			for name, test := range map[string]struct {
				status                  int
				body, secret, auth, err string
				explicit, drop, content bool
			}{
				"mixed fanout and dedup": {}, "explicit": {explicit: true},
				"anonymous": {auth: "anonymous"}, "Basic": {auth: "basic"},
				"environment": {secret: "env"}, "file": {secret: "file"},
				"Basic environment": {auth: "basic", secret: "env"}, "Basic file": {auth: "basic", secret: "file"},
				"secret empty":             {secret: "empty", err: "empty value"},
				"secret nested":            {secret: "nested", err: "must"},
				"secret controls":          {secret: "controls", err: "must"},
				"resolved URL credentials": {secret: "url-credentials", err: "user information"},
				"resolved URL fragment":    {secret: "url-fragment", err: "fragment"},
				"no redirect":              {status: 307, err: "HTTP 307"}, "no rate limit retry": {status: 429, err: "HTTP 429"},
				"no server retry": {status: 503, err: "HTTP 503"}, "all fail": {status: 403, explicit: true, err: "all attempted destinations failed"},
				"missing ack": {body: `{}`, err: "invalid"}, "bad ack": {body: "synthetic-private-value", err: "invalid"},
				"large ack": {body: strings.Repeat(" ", httpclient.ResponseLimit+1), err: "256 KiB"},
				"transport": {drop: true, err: "transport failed"}, "arbitrary content": {content: true},
			} {
				t.Run(name, func(t *testing.T) {
					type received struct {
						path, query, method string
						headers             http.Header
						body                string
						err                 error
					}
					requests := make(chan received, 10)
					paths := map[string]string{"gotify": "/gotify/message", "ntfy": "/ntfy/alerts"}
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						body, err := io.ReadAll(r.Body)
						headers := http.Header{}
						for _, key := range []string{"Content-Type", "User-Agent", "Authorization", "X-Gotify-Key", "Title", "Priority", "Tags", "Actions"} {
							if value := r.Header.Get(key); value != "" {
								headers.Set(key, value)
							}
						}
						requests <- received{r.URL.Path, r.URL.RawQuery, r.Method, headers, string(body), err}
						status := 200
						response := `{"id":"test-message","event":"message"}`
						if r.URL.Path == paths["gotify"] {
							response = `{"id":25,"appid":5}`
						}
						if r.URL.Path == paths[provider] {
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
							if test.body != "" {
								response = test.body
							}
						}
						w.Header().Set("Location", "/unexpected")
						w.WriteHeader(status)
						_, _ = io.WriteString(w, response)
					}))
					defer server.Close()
					gotify := Destination{
						Type:     "gotify",
						APIURL:   server.URL + "/gotify/",
						AppToken: "synthetic-private-value",
					}
					ntfy := Destination{
						Type:        "ntfy",
						URL:         server.URL + "/ntfy/alerts?cache=no",
						AccessToken: "synthetic-private-value",
					}
					if test.auth == "anonymous" {
						ntfy.AccessToken = ""
					}
					if test.auth == "basic" {
						ntfy.AccessToken = ""
						ntfy.Username = "synthetic-user"
						ntfy.Password = "synthetic-password:界"
					}
					cfg := Config{Version: 1, Destinations: map[string]Destination{
						"gotify": gotify, "ntfy": ntfy, "archive": {Type: "webhook", URL: server.URL + "/archive"},
						"unused-gotify": {
							Type:     "gotify",
							APIURL:   "${env:UNUSED_GOTIFY_URL}",
							AppToken: "${env:UNUSED_GOTIFY_TOKEN}",
						},
						"unused-ntfy": {
							Type:        "ntfy",
							URL:         "${env:UNUSED_NTFY_URL}",
							AccessToken: "${env:UNUSED_NTFY_TOKEN}",
						},
					}, Routing: Routing{Roles: map[string][]string{"ops": {"gotify", "ntfy", "archive"}, "dba": {"gotify", "ntfy"}}}}
					if test.secret != "" {
						dst := cfg.Destinations[provider]
						fields := map[string]*string{"TOKEN": &dst.AppToken, "URL": &dst.APIURL}
						if provider == "ntfy" {
							fields = map[string]*string{"TOKEN": &dst.AccessToken, "URL": &dst.URL}
							if test.auth == "basic" {
								fields = map[string]*string{
									"USER":     &dst.Username,
									"PASSWORD": &dst.Password,
									"URL":      &dst.URL,
								}
							}
						}
						for field, target := range fields {
							value := *target
							if field == "TOKEN" {
								switch test.secret {
								case "empty":
									value = ""
								case "nested":
									value = "${env:NESTED_SECRET}"
								case "controls":
									value += "\nextra"
								}
							}
							if field == "URL" {
								switch test.secret {
								case "url-credentials":
									value = "https://user:synthetic-private-value@example.com"
								case "url-fragment":
									value += "#"
								}
							}
							if test.secret == "file" {
								path := filepath.Join(t.TempDir(), field)
								require.NoError(t, os.WriteFile(path, []byte(" "+value+"\n"), 0600))
								*target = "${file:" + path + "}"
							} else {
								variable := "NOTIFY_PUSH_" + field
								t.Setenv(variable, " "+value+"\n")
								*target = "${env:" + variable + "}"
							}
						}
						cfg.Destinations[provider] = dst
					}
					config, err := yaml.Marshal(cfg)
					require.NoError(t, err)
					event := expectedEvent()
					event.URL = "https://example.com/alert?id=1&view=chart#details"
					if test.content {
						event.Node = "節点\r\nInjected: value"
						event.Alert = strings.Repeat("界😀", 100)
						event.Summary = strings.Repeat("界😀\"&", 1700)
						event.URL = "https://example.com/a,b;c?x=\"quoted\"&y=1#fragment"
					}
					input, err := json.Marshal(event)
					require.NoError(t, err)
					args := []string{"send", "--config", writeConfig(t, string(config))}
					if test.explicit {
						args = append(args, "--destination", provider)
					} else {
						args = append(args, "--role", "ops", "--role", "dba")
					}
					var stdout, stderr bytes.Buffer
					wantCode := 0
					if test.explicit && test.err != "" {
						wantCode = 1
					}
					assert.Equal(
						t,
						wantCode,
						Run(context.Background(), args, bytes.NewReader(input), &stdout, &stderr),
						stderr.String(),
					)
					assert.Empty(t, stdout.String())
					if test.err != "" {
						assert.Contains(t, stderr.String(), test.err)
					}
					for _, private := range []string{"synthetic-private-value", "synthetic-user", "synthetic-password", server.URL} {
						assert.NotContains(t, stderr.String(), private)
					}
					names := []string{"gotify", "ntfy", "archive"}
					if test.explicit {
						names = []string{provider}
					}
					if test.secret != "" && test.secret != "env" && test.secret != "file" {
						var selected []string
						for _, n := range names {
							if n != provider {
								selected = append(selected, n)
							}
						}
						names = selected
					}
					require.Len(
						t,
						requests,
						len(names),
						"one request per selected valid destination, no retries or redirects",
					)
					for _, name := range names {
						got := <-requests
						want := received{
							method: "POST",
							headers: http.Header{
								"Content-Type": {"application/json"},
								"User-Agent":   {"netdata-alarm-notify"},
							},
						}
						if name == "archive" {
							want.path = "/archive"
							assert.JSONEq(t, string(input), got.body)
						}
						if name == "gotify" {
							want.path = paths[name]
							want.headers.Set("X-Gotify-Key", "synthetic-private-value")
							payload, err := os.ReadFile(filepath.Join("testdata", "gotify-full.json"))
							require.NoError(t, err)
							if test.content {
								payload, err = json.Marshal(
									gotifyMessage{
										Title:    event.Node + " WARNING: " + event.Summary,
										Message:  event.Summary + "\n" + event.Info + "\nNode: " + event.Node + "\nAlert: " + event.Alert + "\nStatus: CLEAR → WARNING\nChart: test.chart\nContext: test.context\nValue: 42.5 C\nPrevious value: 0 C\nTime: 2026-09-14T12:00:00Z\n" + event.URL,
										Priority: 4,
									},
								)
								require.NoError(t, err)
							}
							assert.JSONEq(t, string(payload), got.body)
						}
						if name == "ntfy" {
							want.path, want.query = paths[name], "cache=no"
							want.headers.Set("Content-Type", "text/plain; charset=utf-8")
							payload, err := os.ReadFile(filepath.Join("testdata", "ntfy-full.json"))
							require.NoError(t, err)
							var fixture struct {
								Headers http.Header
								Body    string
							}
							require.NoError(t, json.Unmarshal(payload, &fixture))
							if test.content {
								title, err := new(mime.WordDecoder).DecodeHeader(got.headers.Get("Title"))
								require.NoError(t, err)
								assert.Equal(t, event.Node+": "+event.Alert, title)
								fixture.Headers.Set("Title", got.headers.Get("Title"))
								actions, err := json.Marshal(
									[]ntfyAction{{Action: "view", Label: "View node", URL: event.URL, Clear: true}},
								)
								require.NoError(t, err)
								fixture.Headers.Set("Actions", string(actions))
								fixture.Body = event.Summary + "\n" + event.Info + "\nNode: " + event.Node + "\nAlert: " + event.Alert + "\nStatus: CLEAR → WARNING\nChart: test.chart\nContext: test.context\nValue: 42.5 C\nPrevious value: 0 C\nTime: 2026-09-14T12:00:00Z"
							}
							for key, values := range fixture.Headers {
								want.headers[key] = values
							}
							if test.auth != "anonymous" {
								auth := "Bearer synthetic-private-value"
								if test.auth == "basic" {
									auth = "Basic " + base64.StdEncoding.EncodeToString(
										[]byte("synthetic-user:synthetic-password:界"),
									)
								}
								want.headers.Set("Authorization", auth)
							}
							assert.Equal(t, fixture.Body, got.body)
						}
						got.body = ""
						assert.Equal(t, want, got)
					}
				})
			}
		})
	}
}
