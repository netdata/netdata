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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSendPushbullet(t *testing.T) {
	for name, test := range map[string]struct {
		status, calls, code           int
		body, secret, err             string
		explicit, channel, note, drop bool
	}{
		"all providers and role deduplication": {calls: 1},
		"email and channel":                    {calls: 1, channel: true},
		"explicit email":                       {calls: 1, explicit: true},
		"no URL is a note":                     {calls: 1, explicit: true, note: true},
		"environment secrets":                  {calls: 1, secret: "env"},
		"file secrets":                         {calls: 1, secret: "file"},
		"resolved token newline":               {secret: "bad-token", err: "access_token must be"},
		"resolved token empty":                 {secret: "empty-token", err: "empty value"},
		"nested secret":                        {secret: "nested-token", err: "access_token must be"},
		"resolved API credentials":             {secret: "bad-api", err: "user information"},
		"resolved API fragment":                {secret: "fragment-api", err: "fragment"},
		"unconfirmed":                          {calls: 1, status: 204, err: "HTTP 204"},
		"redirect not followed":                {calls: 1, status: 307, err: "HTTP 307"},
		"rate limit not retried":               {calls: 1, status: 429, err: "HTTP 429"},
		"server error not retried":             {calls: 1, status: 503, err: "HTTP 503"},
		"API error":                            {calls: 1, body: `{"error":{"message":"synthetic-private-value"}}`, err: "API rejected"},
		"missing acknowledgment":               {calls: 1, body: `{}`, err: "invalid pushbullet response"},
		"invalid acknowledgment":               {calls: 1, body: `synthetic-private-value`, err: "invalid pushbullet response"},
		"oversized acknowledgment":             {calls: 1, body: strings.Repeat(" ", notificationResponseLimit+1), err: "256 KiB"},
		"explicit failure":                     {calls: 1, status: 403, explicit: true, code: 1, err: "all selected destinations failed"},
		"transport failure":                    {calls: 1, drop: true, err: "transport failed"},
	} {
		t.Run(name, func(t *testing.T) {
			type received struct {
				path, query, method, contentType, auth, accessToken, agent, body string
				err                                                              error
			}
			requests := make(chan received, 12)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				requests <- received{r.URL.Path, r.URL.RawQuery, r.Method, r.Header.Get("Content-Type"), r.Header.Get("Authorization"), r.Header.Get("Access-Token"), r.UserAgent(), string(body), err}
				if r.URL.Path != "/proxy/v2/pushes" {
					_, _ = io.WriteString(w, `{"ok":true,"status":1}`)
					return
				}
				if test.drop {
					connection, _, err := w.(http.Hijacker).Hijack()
					if err == nil {
						_ = connection.Close()
					}
					return
				}
				status, response := test.status, test.body
				if status == 0 {
					status = 200
				}
				if response == "" {
					response = `{"iden":"test-push"}`
				}
				w.Header().Set("Location", "/unexpected")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, response)
			}))
			defer server.Close()
			dst := Destination{Type: "pushbullet", AccessToken: "synthetic-private-value", Email: "ops@example.com",
				SourceDeviceID: "test-source-device", APIURL: server.URL + "/proxy/"}
			if test.secret != "" {
				token, api := dst.AccessToken, dst.APIURL
				switch test.secret {
				case "bad-token":
					token += "\nextra"
				case "empty-token":
					token = ""
				case "nested-token":
					token = "${env:NOTIFIER_NESTED_SECRET}"
				case "bad-api":
					api = "https://user:synthetic-private-value@example.com"
				case "fragment-api":
					api += "#"
				}
				for _, field := range []struct {
					name, value string
					target      *string
				}{
					{"TOKEN", token, &dst.AccessToken}, {"API", api, &dst.APIURL},
				} {
					if test.secret == "file" {
						path := filepath.Join(t.TempDir(), field.name)
						require.NoError(t, os.WriteFile(path, []byte(" "+field.value+"\n"), 0600))
						*field.target = "${file:" + path + "}"
					} else {
						variable := "NOTIFIER_PUSHBULLET_" + field.name
						t.Setenv(variable, " "+field.value+"\n")
						*field.target = "${env:" + variable + "}"
					}
				}
			}
			other := dst
			other.Email, other.ChannelTag, other.SourceDeviceID = "", "test-alerts", ""
			topic := configInteger(7)
			cfg := Config{Version: 1, Destinations: map[string]Destination{
				"push": dst, "channel": other,
				"slack":   {Type: "slack", URL: server.URL + "/slack"},
				"discord": {Type: "discord", URL: server.URL + "/discord"},
				"telegram": {
					Type:            "telegram",
					APIURL:          server.URL,
					BotToken:        "123:synthetic-private-value",
					ChatID:          "-100123",
					MessageThreadID: &topic,
				},
				"pushover": {
					Type:     "pushover",
					APIURL:   server.URL,
					AppToken: pushoverTestToken,
					UserKey:  pushoverTestUser,
				},
				"archive": {Type: "webhook", URL: server.URL + "/archive", BearerToken: "synthetic-private-value"},
				"unused": {
					Type:        "pushbullet",
					AccessToken: "${env:NOTIFIER_UNUSED_TOKEN}",
					Email:       "unused@example.com",
				},
			}, Routing: Routing{Roles: map[string][]string{"ops": {"push", "slack", "discord", "telegram", "pushover", "archive"}, "dba": {"push"}}}}
			if test.channel {
				cfg.Routing.Roles["dba"] = append(cfg.Routing.Roles["dba"], "channel")
			}
			config, err := yaml.Marshal(cfg)
			require.NoError(t, err)
			event := expectedEvent()
			if !test.note {
				event.URL = "https://example.com/alert?id=1&view=chart#details"
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
			for _, private := range []string{"synthetic-private-value", "ops@example.com", "test-alerts", "test-source-device", server.URL} {
				assert.NotContains(t, stderr.String(), private)
			}
			if test.err != "" {
				assert.Contains(t, stderr.String(), test.err)
			}
			var expected []received
			for range test.calls {
				expected = append(expected, received{path: "/proxy/v2/pushes", accessToken: "synthetic-private-value"})
			}
			if !test.explicit {
				expected = append(expected, received{path: "/slack"}, received{path: "/discord", query: "wait=true"},
					received{path: "/bot123:synthetic-private-value/sendMessage"}, received{path: "/1/messages.json"},
					received{path: "/archive", auth: "Bearer synthetic-private-value"})
			}
			if test.channel {
				expected = append(expected, received{path: "/proxy/v2/pushes", accessToken: "synthetic-private-value"})
			}
			require.Len(t, requests, len(expected), "only selected destinations without retries or redirects")
			for i, want := range expected {
				got := <-requests
				fixture := "pushbullet-full.json"
				switch want.path {
				case "/slack":
					fixture = "slack-full.json"
				case "/discord":
					fixture = "discord-full.json"
				case "/bot123:synthetic-private-value/sendMessage":
					fixture = "telegram-full.json"
				case "/1/messages.json":
					fixture = "pushover-full.json"
				case "/archive":
					fixture = ""
				}
				payload := input
				if fixture != "" {
					payload, err = os.ReadFile(filepath.Join("testdata", fixture))
					require.NoError(t, err)
				}
				if test.note || test.channel && i == len(expected)-1 {
					var value map[string]any
					require.NoError(t, json.Unmarshal(payload, &value))
					if test.note {
						value["type"] = "note"
						delete(value, "url")
					}
					if test.channel {
						value["channel_tag"] = "test-alerts"
						delete(value, "email")
						delete(value, "source_device_iden")
					}
					payload, err = json.Marshal(value)
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
