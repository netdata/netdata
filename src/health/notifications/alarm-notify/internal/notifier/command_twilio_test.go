// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
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

func TestSendTwilio(t *testing.T) {
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
		"resolved SID path injection":          {secret: "bad-sid", err: "account_sid must be"},
		"resolved token newline":               {secret: "bad-token", err: "auth_token must be"},
		"resolved token empty":                 {secret: "empty-token", err: "empty value"},
		"nested secret":                        {secret: "nested-token", err: "auth_token must be"},
		"resolved API credentials":             {secret: "bad-api", err: "user information"},
		"resolved API fragment":                {secret: "fragment-api", err: "fragment"},
		"wrong success code":                   {calls: 1, status: 200, err: "HTTP 200"},
		"redirect not followed":                {calls: 1, status: 307, err: "HTTP 307"},
		"rate limit not retried":               {calls: 1, status: 429, err: "HTTP 429"},
		"server error not retried":             {calls: 1, status: 503, err: "HTTP 503"},
		"missing acknowledgment":               {calls: 1, body: `{}`, err: "invalid twilio response"},
		"invalid acknowledgment":               {calls: 1, body: `synthetic-private-value`, err: "invalid twilio response"},
		"oversized acknowledgment":             {calls: 1, body: strings.Repeat(" ", httpclient.ResponseLimit+1), err: "256 KiB"},
		"explicit failure":                     {calls: 1, status: 401, explicit: true, code: 1, err: "all attempted destinations failed"},
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
				if r.URL.Path != "/proxy"+twilioTestPath {
					_, _ = io.WriteString(w, `{"ok":true,"status":1,"iden":"test-push"}`)
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
					response = `{"sid":"SM00000000000000000000000000000000","status":"queued"}`
				}
				w.Header().Set("Location", "/unexpected")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, response)
			}))
			defer server.Close()
			dst := Destination{
				Type:       "twilio",
				AccountSID: twilioTestSID,
				AuthToken:  "synthetic-private-value",
				From:       "+15005550006",
				To:         "+15005550009",
				APIURL:     server.URL + "/proxy/",
			}
			if test.secret != "" {
				sid, token, api := dst.AccountSID, dst.AuthToken, dst.APIURL
				switch test.secret {
				case "bad-sid":
					sid += "/../secret"
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
					{"SID", sid, &dst.AccountSID}, {"TOKEN", token, &dst.AuthToken}, {"API", api, &dst.APIURL},
				} {
					if test.secret == "file" {
						path := filepath.Join(t.TempDir(), field.name)
						require.NoError(t, os.WriteFile(path, []byte(" "+field.value+"\n"), 0600))
						*field.target = "${file:" + path + "}"
					} else {
						variable := "NOTIFIER_TWILIO_" + field.name
						t.Setenv(variable, " "+field.value+"\n")
						*field.target = "${env:" + variable + "}"
					}
				}
			}
			other := dst
			other.From, other.To = "12345", "+15005550008"
			topic := configInteger(7)
			cfg := Config{Version: 1, Destinations: map[string]Destination{
				"sms": dst, "other": other,
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
				"pushbullet": {
					Type:           "pushbullet",
					APIURL:         server.URL,
					AccessToken:    "synthetic-private-value",
					Email:          "ops@example.com",
					SourceDeviceID: "test-source-device",
				},
				"archive": {Type: "webhook", URL: server.URL + "/archive", BearerToken: "synthetic-private-value"},
				"unused": {
					Type:       "twilio",
					AccountSID: "${env:NOTIFIER_UNUSED_SID}",
					AuthToken:  "${env:NOTIFIER_UNUSED_TOKEN}",
					From:       "12345",
					To:         "+15005550009",
				},
			}, Routing: Routing{Roles: map[string][]string{"ops": {"sms", "slack", "discord", "telegram", "pushover", "pushbullet", "archive"}, "dba": {"sms"}}}}
			if test.multiple {
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
			for _, private := range []string{twilioTestSID, "synthetic-private-value", "+15005550006", "+15005550009", server.URL, base64.StdEncoding.EncodeToString([]byte(twilioTestSID + ":synthetic-private-value"))} {
				assert.NotContains(t, stderr.String(), private)
			}
			if test.err != "" {
				assert.Contains(t, stderr.String(), test.err)
			}
			var expected []received
			basic := "Basic " + base64.StdEncoding.EncodeToString([]byte(twilioTestSID+":synthetic-private-value"))
			for range test.calls {
				expected = append(expected, received{path: "/proxy" + twilioTestPath, auth: basic})
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
					received{path: "/archive", auth: "Bearer synthetic-private-value"},
				)
			}
			if test.multiple {
				expected = append(expected, received{path: "/proxy" + twilioTestPath, auth: basic})
			}
			require.Len(t, requests, len(expected), "only selected destinations without retries or redirects")
			for i, want := range expected {
				got := <-requests
				fixture := map[string]string{"/slack": "slack-full.json", "/discord": "discord-full.json", "/bot123:synthetic-private-value/sendMessage": "telegram-full.json", "/1/messages.json": "pushover-full.json", "/v2/pushes": "pushbullet-full.json", "/proxy" + twilioTestPath: "twilio-full.json"}[want.path]
				payload := input
				if fixture != "" {
					payload, err = os.ReadFile(filepath.Join("testdata", fixture))
					require.NoError(t, err)
				}
				want.contentType = "application/json"
				if fixture == "twilio-full.json" {
					var form url.Values
					require.NoError(t, json.Unmarshal(payload, &form))
					if test.multiple && i == len(expected)-1 {
						form.Set("From", "12345")
						form.Set("To", "+15005550008")
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
