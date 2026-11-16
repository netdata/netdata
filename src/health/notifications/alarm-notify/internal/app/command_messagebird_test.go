// SPDX-License-Identifier: GPL-3.0-or-later

package app

import (
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"maps"

	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSendMessageBird(t *testing.T) {
	for name, test := range map[string]struct {
		status, calls, code      int
		body, secret, err        string
		explicit, multiple, drop bool
	}{
		"all providers and role deduplication": {calls: 1},
		"multiple recipients":                  {calls: 1, multiple: true},
		"explicit":                             {calls: 1, explicit: true},
		"environment secrets":                  {calls: 1, secret: "env"},
		"file secrets":                         {calls: 1, secret: "file"},
		"resolved token newline":               {secret: "bad-token", err: "access_key must be"},
		"resolved token empty":                 {secret: "empty-token", err: "empty value"},
		"nested secret":                        {secret: "nested-token", err: "access_key must be"},
		"resolved API credentials":             {secret: "bad-api", err: "user information"},
		"resolved API fragment":                {secret: "fragment-api", err: "fragment"},
		"wrong success code":                   {calls: 1, status: 200, err: "HTTP 200"},
		"redirect not followed":                {calls: 1, status: 307, err: "HTTP 307"},
		"rate limit not retried":               {calls: 1, status: 429, err: "HTTP 429"},
		"server error not retried":             {calls: 1, status: 503, err: "HTTP 503"},
		"missing acknowledgment":               {calls: 1, body: `{}`, err: "invalid messagebird response"},
		"invalid acknowledgment":               {calls: 1, body: `synthetic-private-value`, err: "invalid messagebird response"},
		"oversized acknowledgment":             {calls: 1, body: strings.Repeat(" ", httpclient.ResponseLimit+1), err: "256 KiB"},
		"explicit failure":                     {calls: 1, status: 401, explicit: true, code: 1, err: "all attempted destinations failed"},
		"transport failure":                    {calls: 1, drop: true, err: "transport failed"},
	} {
		t.Run(name, func(t *testing.T) {
			type received struct {
				path, query, method, contentType, auth, accessToken, accept, agent, body string
				err                                                                      error
			}
			requests := make(chan received, 12)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				requests <- received{r.URL.Path, r.URL.RawQuery, r.Method, r.Header.Get("Content-Type"), r.Header.Get("Authorization"), r.Header.Get("Access-Token"), r.Header.Get("Accept"), r.UserAgent(), string(body), err}
				if r.URL.Path != "/proxy"+messagebirdTestPath {
					if r.URL.Path == twilioTestPath {
						w.WriteHeader(201)
					}
					_, _ = io.WriteString(w, `{"ok":true,"status":1,"iden":"test-push","sid":"test-message"}`)
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
					status = 201
				}
				if response == "" {
					response = `{"id":"test-message"}`
				}
				w.Header().Set("Location", "/unexpected")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, response)
			}))
			defer server.Close()
			dst := map[string]any{"type": "messagebird", "access_key": "synthetic-private-value", "originator": "+15005550006", "recipient": "+15005550009", "api_url": server.URL + "/proxy/"}
			if test.secret != "" {
				token, api := dst["access_key"].(string), dst["api_url"].(string)
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
					target      string
				}{
					{"TOKEN", token, "access_key"}, {"API", api, "api_url"},
				} {
					if test.secret == "file" {
						path := filepath.Join(t.TempDir(), field.name)
						require.NoError(t, os.WriteFile(path, []byte(" "+field.value+"\n"), 0600))
						dst[field.target] = "${file:" + path + "}"
					} else {
						variable := "NOTIFIER_MESSAGEBIRD_" + field.name
						t.Setenv(variable, " "+field.value+"\n")
						dst[field.target] = "${env:" + variable + "}"
					}
				}
			}
			other := maps.Clone(dst)
			other["originator"], other["recipient"] = "12345", "+15005550008"
			topic := int64(7)
			cfg := testConfig{Version: 1, Destinations: map[string]map[string]any{
				"sms": dst, "other": other,
				"slack":      {"type": "slack", "url": server.URL + "/slack"},
				"discord":    {"type": "discord", "url": server.URL + "/discord"},
				"telegram":   {"type": "telegram", "api_url": server.URL, "bot_token": "123:synthetic-private-value", "chat_id": "-100123", "message_thread_id": &topic},
				"pushover":   {"type": "pushover", "api_url": server.URL, "app_token": pushoverTestToken, "user_key": pushoverTestUser},
				"pushbullet": {"type": "pushbullet", "api_url": server.URL, "access_token": "synthetic-private-value", "email": "ops@example.com", "source_device_id": "test-source-device"},
				"twilio":     {"type": "twilio", "api_url": server.URL, "account_sid": twilioTestSID, "auth_token": "synthetic-private-value", "from": "+15005550006", "to": "+15005550009"},
				"archive":    {"type": "webhook", "url": server.URL + "/archive", "bearer_token": "synthetic-private-value"},
				"unused":     {"type": "messagebird", "access_key": "${env:NOTIFIER_UNUSED_TOKEN}", "originator": "12345", "recipient": "+15005550009"},
			}, Routing: notifier.Routing{Roles: map[string][]string{"ops": {"sms", "slack", "discord", "telegram", "pushover", "pushbullet", "twilio", "archive"}, "dba": {"sms"}}}}
			if test.multiple {
				cfg.Routing.Roles["dba"] = append(cfg.Routing.Roles["dba"], "other")
			}
			config, err := yaml.Marshal(cfg)
			require.NoError(t, err)
			event := testutil.ExpectedEvent()
			event.URL = "https://example.com/alert?id=1&view=chart#details"
			input, err := json.Marshal(event)
			require.NoError(t, err)
			args := []string{"send", "--config", writeConfig(t, string(config))}
			if test.explicit {
				args = append(args, "--destination", "sms")
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
			for _, private := range []string{"synthetic-private-value", "+15005550006", "+15005550009", server.URL} {
				assert.NotContains(t, stderr.String(), private)
			}
			if test.err != "" {
				assert.Contains(t, stderr.String(), test.err)
			}
			var expected []received
			accessKey := "AccessKey synthetic-private-value"
			for range test.calls {
				expected = append(
					expected,
					received{path: "/proxy" + messagebirdTestPath, auth: accessKey, accept: "application/json"},
				)
			}
			if !test.explicit {
				expected = append(
					expected,
					received{path: "/slack"},
					received{path: "/discord", query: "wait=true"},
					received{path: "/bot123:synthetic-private-value/sendMessage"},
					received{path: "/1/messages.json"},
					received{
						path:        "/v2/pushes",
						accessToken: "synthetic-private-value",
					},
					received{
						path: twilioTestPath,
						auth: "Basic " + base64.StdEncoding.EncodeToString(
							[]byte(twilioTestSID+":synthetic-private-value"),
						),
					},
					received{path: "/archive", auth: "Bearer synthetic-private-value"},
				)
			}
			if test.multiple {
				expected = append(
					expected,
					received{path: "/proxy" + messagebirdTestPath, auth: accessKey, accept: "application/json"},
				)
			}
			require.Len(t, requests, len(expected), "only selected destinations without retries or redirects")
			for i, want := range expected {
				got := <-requests
				fixture := map[string]string{"/slack": "slack-full.json", "/discord": "discord-full.json", "/bot123:synthetic-private-value/sendMessage": "telegram-full.json", "/1/messages.json": "pushover-full.json", "/v2/pushes": "pushbullet-full.json", "/proxy" + messagebirdTestPath: "messagebird-full.json", twilioTestPath: "twilio-full.json"}[want.path]
				payload := input
				if fixture != "" {
					payload, err = os.ReadFile(fixturePath(fixture))
					require.NoError(t, err)
				}
				want.contentType = "application/json"
				if fixture == "messagebird-full.json" || fixture == "twilio-full.json" {
					var form url.Values
					require.NoError(t, json.Unmarshal(payload, &form))
					if test.multiple && i == len(expected)-1 {
						form.Set("originator", "12345")
						form.Set("recipients", "+15005550008")
					}
					decoded, err := url.ParseQuery(got.body)
					require.NoError(t, err)
					assert.Equal(t, form, decoded)
					want.contentType = "application/x-www-form-urlencoded"
				} else {
					assert.JSONEq(t, string(payload), got.body)
				}
				got.body = ""
				want.method, want.agent = "POST", "netdata-alarm-notify"
				assert.Equal(t, want, got)
			}
		})
	}
}
