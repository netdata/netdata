// SPDX-License-Identifier: GPL-3.0-or-later

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSendPushover(t *testing.T) {
	for name, test := range map[string]struct {
		status                            int
		body, secret, err                 string
		explicit, second, oversized, drop bool
		calls, code                       int
	}{
		"mixed providers and role deduplication": {calls: 1},
		"two recipients":                         {calls: 1, second: true},
		"explicit":                               {calls: 1, explicit: true},
		"environment secrets":                    {calls: 1, secret: "env"},
		"file secrets":                           {calls: 1, secret: "file"},
		"invalid resolved app":                   {secret: "bad-app", err: "app_token must contain"},
		"invalid resolved user":                  {secret: "bad-user", err: "user_key must contain"},
		"nested secret rejected":                 {secret: "nested-app", err: "app_token must contain"},
		"empty resolved user":                    {secret: "empty-user", err: "empty value"},
		"invalid resolved API":                   {secret: "bad-api", err: "without user information"},
		"resolved API empty fragment":            {secret: "fragment-api", err: "fragment"},
		"shortens and sends":                     {calls: 1, explicit: true, oversized: true},
		"unconfirmed":                            {calls: 1, status: 204, err: "HTTP 204"},
		"redirect not followed":                  {calls: 1, status: 307, err: "HTTP 307"},
		"quota not retried":                      {calls: 1, status: 429, err: "HTTP 429"},
		"server error not retried":               {calls: 1, status: 503, err: "HTTP 503"},
		"API rejection":                          {calls: 1, body: `{"status":0,"errors":["synthetic-private-value"]}`, err: "API rejected"},
		"invalid acknowledgment":                 {calls: 1, body: `synthetic-private-value`, err: "invalid pushover response"},
		"explicit failure":                       {calls: 1, status: 403, explicit: true, code: 1, err: "all attempted destinations failed"},
		"transport failure not retried":          {calls: 1, drop: true, err: "transport failed"},
	} {
		t.Run(name, func(t *testing.T) {
			type received struct {
				path, query, method, contentType, auth, agent, body string
				err                                                 error
			}
			requests := make(chan received, 10)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				requests <- received{r.URL.Path, r.URL.RawQuery, r.Method, r.Header.Get("Content-Type"), r.Header.Get("Authorization"), r.UserAgent(), string(body), err}
				if r.URL.Path != "/proxy/1/messages.json" {
					_, _ = io.WriteString(w, `{"ok":true}`)
					return
				}
				if test.drop {
					connection, _, err := w.(http.Hijacker).Hijack()
					if err == nil {
						_ = connection.Close()
					}
					return
				}
				status := test.status
				if status == 0 {
					status = 200
				}
				response := test.body
				if response == "" {
					response = `{"status":1}`
				}
				w.Header().Set("Location", "/unexpected")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, response)
			}))
			defer server.Close()
			dst := map[string]any{
				"type":      "pushover",
				"app_token": pushoverTestToken,
				"user_key":  pushoverTestUser,
				"api_url":   server.URL + "/proxy/",
			}
			if test.secret != "" {
				app, user, api := dst["app_token"].(string), dst["user_key"].(string), dst["api_url"].(string)
				switch test.secret {
				case "bad-app":
					app = "synthetic-private-value"
				case "bad-user":
					user = "synthetic-private-value"
				case "nested-app":
					app = "${env:NOTIFIER_NESTED_SECRET}"
				case "empty-user":
					user = ""
				case "bad-api":
					api = "https://user:synthetic-private-value@example.com"
				case "fragment-api":
					api += "#"
				}
				for _, field := range []struct {
					name, value string
					target      string
				}{
					{"APP", app, "app_token"}, {"USER", user, "user_key"}, {"API", api, "api_url"},
				} {
					if test.secret == "file" {
						path := filepath.Join(t.TempDir(), field.name)
						require.NoError(t, os.WriteFile(path, []byte(" "+field.value+"\n"), 0600))
						dst[field.target] = "${file:" + path + "}"
					} else {
						variable := "NOTIFIER_PUSHOVER_" + field.name
						t.Setenv(variable, " "+field.value+"\n")
						dst[field.target] = "${env:" + variable + "}"
					}
				}
			}
			other := maps.Clone(dst)
			other["user_key"] = strings.Repeat("V", 30)
			topic := field.Integer(7)
			cfg := testConfig{Version: 1, Destinations: map[string]map[string]any{
				"push": dst, "other": other,
				"slack":   {"type": "slack", "url": server.URL + "/slack"},
				"discord": {"type": "discord", "url": server.URL + "/discord"},
				"telegram": {
					"type":              "telegram",
					"bot_token":         "123:synthetic-private-value",
					"chat_id":           "-100123",
					"message_thread_id": &topic,
					"api_url":           server.URL,
				},
				"archive": {"type": "webhook", "url": server.URL + "/archive", "bearer_token": "synthetic-private-value"},
				"unused": {
					"type":      "pushover",
					"app_token": "${env:NOTIFIER_UNUSED_APP}",
					"user_key":  "${env:NOTIFIER_UNUSED_USER}",
				},
			}, Routing: notifier.Routing{Roles: map[string][]string{"ops": {"push", "slack", "discord", "telegram", "archive"}, "dba": {"push"}}}}
			if test.second {
				cfg.Routing.Roles["dba"] = append(cfg.Routing.Roles["dba"], "other")
			}
			config, err := yaml.Marshal(cfg)
			require.NoError(t, err)
			event := testutil.ExpectedEvent()
			event.URL = "https://example.com/alert?id=1&view=chart#details"
			if test.oversized {
				event.Summary = strings.Repeat("😀", 1100)
				event.URL = "https://example.com/" + strings.Repeat("x", 513)
			}
			input, err := json.Marshal(event)
			require.NoError(t, err)
			args := []string{"send", "--config", writeConfig(t, string(config))}
			if test.explicit {
				args = append(args, "--destination", "push")
			} else {
				args = append(args, "--role", "ops", "--role", "dba")
			}
			var stdout, stderr bytes.Buffer
			assert.Equal(
				t,
				test.code,
				Run(context.Background(), args, bytes.NewReader(input), &stdout, &stderr),
				stderr.String(),
			)
			assert.Empty(t, stdout.String())
			for _, secret := range []string{pushoverTestToken, pushoverTestUser, "synthetic-private-value", server.URL} {
				assert.NotContains(t, stderr.String(), secret)
			}
			if test.err != "" {
				assert.Contains(t, stderr.String(), test.err)
			}
			var expected []received
			for range test.calls {
				expected = append(expected, received{path: "/proxy/1/messages.json"})
			}
			if !test.explicit {
				expected = append(
					expected,
					received{path: "/slack"},
					received{path: "/discord", query: "wait=true"},
					received{
						path: "/bot123:synthetic-private-value/sendMessage",
					},
					received{path: "/archive", auth: "Bearer synthetic-private-value"},
				)
			}
			if test.second {
				expected = append(expected, received{path: "/proxy/1/messages.json"})
			}
			require.Len(t, requests, len(expected), "only selected destinations, without retries or redirects")
			for i, want := range expected {
				got := <-requests
				fixture := "pushover-full.json"
				switch want.path {
				case "/slack":
					fixture = "slack-full.json"
				case "/discord":
					fixture = "discord-full.json"
				case "/bot123:synthetic-private-value/sendMessage":
					fixture = "telegram-full.json"
				case "/archive":
					fixture = ""
				}
				payload := input
				if fixture != "" {
					payload, err = os.ReadFile(fixturePath(fixture))
					require.NoError(t, err)
				}
				if test.second && i == len(expected)-1 {
					payload = bytes.ReplaceAll(payload, []byte(pushoverTestUser), []byte(other["user_key"].(string)))
				}
				if test.oversized {
					payload, err = json.Marshal(
						pushoverMessage{
							Token:     pushoverTestToken,
							User:      pushoverTestUser,
							HTML:      1,
							Timestamp: 1789387200,
							Title: "test-node WARNING: " + strings.Repeat(
								"😀",
								228,
							) + "...",
							Message: "<b>" + strings.Repeat("😀", 1014) + "...</b>",
						},
					)
					require.NoError(t, err)
				}
				assert.JSONEq(t, string(payload), got.body)
				got.body = ""
				want.method, want.contentType, want.agent = "POST", "application/json", "netdata-alarm-notify"
				assert.Equal(t, want, got)
			}
		})
	}
}
