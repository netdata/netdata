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

func TestRunTelegram(t *testing.T) {
	type reply struct {
		status int
		body   string
	}
	success := reply{200, `{"ok":true,"result":{"message_id":1}}`}
	limited := reply{429, `{"ok":false,"error_code":429,"parameters":{"retry_after":0}}`}
	tests := map[string]struct {
		replies                           []reply
		retries                           configInteger
		calls, code                       int
		secret, err                       string
		explicit, second, drop, oversized bool
		minDelay                          time.Duration
	}{
		"mixed providers":           {calls: 1},
		"two chats":                 {calls: 1, second: true},
		"explicit":                  {calls: 1, explicit: true},
		"environment token and API": {calls: 1, secret: "env"},
		"file token":                {calls: 1, secret: "file"},
		"invalid resolved token":    {secret: "bad-token", err: "bot_token"},
		"empty token":               {secret: "empty-token", err: "empty value"},
		"invalid resolved API":      {secret: "bad-api", err: "api_url"},
		"retries off by default":    {replies: []reply{limited}, calls: 1, err: "HTTP 429"},
		"rate limit retry succeeds": {replies: []reply{limited, success}, retries: 2, calls: 2},
		"exhausted retries":         {replies: []reply{limited}, retries: 2, calls: 3, err: "HTTP 429"},
		"server requested two seconds": {
			replies:  []reply{{429, `{"ok":false,"parameters":{"retry_after":2}}`}, success},
			retries:  1,
			calls:    2,
			minDelay: 2 * time.Second,
		},
		"missing delay falls back to one second": {
			replies:  []reply{{429, `{"ok":false}`}, success},
			retries:  1,
			calls:    2,
			minDelay: time.Second,
		},
		"long delay permits later destinations": {
			replies: []reply{{429, `{"ok":false,"parameters":{"retry_after":30}}`}},
			retries: 1,
			calls:   1,
			err:     "remaining deadline",
		},
		"401 not retried": {
			replies: []reply{{401, `synthetic-private-value`}},
			retries: 2,
			calls:   1,
			err:     "HTTP 401",
		},
		"503 not retried": {
			replies: []reply{{503, `synthetic-private-value`}},
			retries: 2,
			calls:   1,
			err:     "HTTP 503",
		},
		"redirect not followed": {replies: []reply{{302, ``}}, retries: 2, calls: 1, err: "HTTP 302"},
		"204 not accepted":      {replies: []reply{{204, ``}}, calls: 1, err: "HTTP 204"},
		"API error under HTTP 200": {
			replies: []reply{{200, `{"ok":false,"error_code":400,"description":"synthetic-private-value"}`}},
			calls:   1,
			err:     "error code 400",
		},
		"invalid acknowledgment": {
			replies: []reply{{200, `{"result":true}`}},
			calls:   1,
			err:     "invalid telegram response",
		},
		"invalid rate-limit response": {
			replies: []reply{{429, `synthetic-private-value`}},
			retries: 2,
			calls:   1,
			err:     "invalid telegram response",
		},
		"inconsistent rate-limit response": {
			replies: []reply{{429, `{"ok":true}`}},
			retries: 2,
			calls:   1,
			err:     "HTTP 429",
		},
		"transport failure not retried": {drop: true, retries: 2, calls: 1, err: "transport failed"},
		"explicit failure": {
			replies:  []reply{{401, ``}},
			calls:    1,
			explicit: true,
			code:     1,
			err:      "all selected destinations failed",
		},
		"oversize skips HTTP": {oversized: true, explicit: true, code: 1, err: "4096-character"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			type received struct {
				path, query, method, contentType, auth, agent, body string
				err                                                 error
				at                                                  time.Time
			}
			requests := make(chan received, 12)
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				requests <- received{r.URL.Path, r.URL.RawQuery, r.Method, r.Header.Get("Content-Type"), r.Header.Get("Authorization"), r.UserAgent(), string(body), err, time.Now()}
				if !strings.HasPrefix(r.URL.Path, "/proxy/bot") {
					if r.URL.Path == "/archive" {
						w.WriteHeader(204)
					}
					return
				}
				if test.drop {
					connection, _, err := w.(http.Hijacker).Hijack()
					if err == nil {
						_ = connection.Close()
					}
					return
				}
				response := success
				if len(test.replies) > 0 {
					response = test.replies[min(calls, len(test.replies)-1)]
				}
				calls++
				w.Header().Set("Location", "/unexpected")
				w.WriteHeader(response.status)
				_, _ = io.WriteString(w, response.body)
			}))
			defer server.Close()
			topic := configInteger(7)
			dst := Destination{
				Type:            "telegram",
				BotToken:        "123:synthetic-private-value",
				ChatID:          "-100123",
				MessageThreadID: &topic,
				APIURL:          server.URL + "/proxy/",
			}
			if test.retries != 0 {
				dst.RetriesOnLimit = &test.retries
			}
			switch test.secret {
			case "env", "bad-token", "empty-token", "bad-api":
				token, api := dst.BotToken, dst.APIURL
				if test.secret == "bad-token" {
					token += "/other"
				}
				if test.secret == "empty-token" {
					token = ""
				}
				if test.secret == "bad-api" {
					api = "https://user:synthetic-private-value@example.com"
				}
				t.Setenv("NOTIFIER_TELEGRAM_TOKEN", " "+token+"\n")
				t.Setenv("NOTIFIER_TELEGRAM_API", " "+api+"\n")
				dst.BotToken, dst.APIURL = "${env:NOTIFIER_TELEGRAM_TOKEN}", "${env:NOTIFIER_TELEGRAM_API}"
			case "file":
				path := filepath.Join(t.TempDir(), "token")
				require.NoError(t, os.WriteFile(path, []byte(" "+dst.BotToken+"\n"), 0600))
				dst.BotToken = "${file:" + path + "}"
			}
			other := dst
			other.ChatID, other.MessageThreadID = "@example_alerts", nil
			cfg := Config{Version: 1, Destinations: map[string]Destination{
				"chat": dst, "other": other,
				"slack":   {Type: "slack", URL: server.URL + "/slack"},
				"discord": {Type: "discord", URL: server.URL + "/discord"},
				"archive": {Type: "webhook", URL: server.URL + "/archive", BearerToken: "synthetic-private-value"},
				"unused":  {Type: "telegram", BotToken: "${env:NOTIFIER_TELEGRAM_UNUSED}", ChatID: "1"},
			}, Routing: Routing{Roles: map[string][]string{"ops": {"chat", "slack", "discord", "archive"}, "dba": {"chat"}}}}
			if test.second {
				cfg.Routing.Roles["dba"] = append(cfg.Routing.Roles["dba"], "other")
			}
			config, err := yaml.Marshal(cfg)
			require.NoError(t, err)
			event := expectedEvent()
			event.URL = "https://example.com/alert?id=1&view=chart#details"
			if test.oversized {
				event.Summary = strings.Repeat("x", 4097)
			}
			input, err := json.Marshal(event)
			require.NoError(t, err)
			args := []string{"send", "--config", writeConfig(t, string(config))}
			if test.explicit {
				args = append(args, "--destination", "chat")
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
			assert.NotContains(t, stderr.String(), "synthetic-private-value")
			assert.NotContains(t, stderr.String(), server.URL)
			if test.err != "" {
				assert.Contains(t, stderr.String(), test.err)
			}
			var expected []received
			for range test.calls {
				expected = append(expected, received{path: "/proxy/bot123:synthetic-private-value/sendMessage"})
			}
			if !test.explicit {
				expected = append(
					expected,
					received{path: "/slack"},
					received{path: "/discord", query: "wait=true"},
					received{path: "/archive", auth: "Bearer synthetic-private-value"},
				)
			}
			if test.second {
				expected = append(expected, received{path: "/proxy/bot123:synthetic-private-value/sendMessage"})
			}
			require.Len(t, requests, len(expected), "only selected destinations and configured rate-limit attempts")
			var first time.Time
			for i, want := range expected {
				got := <-requests
				if i == 0 {
					first = got.at
				}
				if i == 1 && test.minDelay > 0 {
					assert.GreaterOrEqual(t, got.at.Sub(first), test.minDelay)
				}
				fixture := "telegram-full.json"
				switch want.path {
				case "/slack":
					fixture = "slack-full.json"
				case "/discord":
					fixture = "discord-full.json"
				case "/archive":
					fixture = ""
				}
				payload := input
				if fixture != "" {
					payload, err = os.ReadFile(filepath.Join("testdata", fixture))
					require.NoError(t, err)
				}
				if test.second && i == len(expected)-1 {
					var value map[string]any
					require.NoError(t, json.Unmarshal(payload, &value))
					value["chat_id"] = "@example_alerts"
					delete(value, "message_thread_id")
					payload, err = json.Marshal(value)
					require.NoError(t, err)
				}
				assert.JSONEq(t, string(payload), got.body)
				got.body, got.at = "", time.Time{}
				want.method, want.contentType, want.agent = "POST", "application/json", "netdata-alarm-notify"
				assert.Equal(t, want, got)
			}
		})
	}
}

func TestAcknowledgmentCancellation(t *testing.T) {
	for name, test := range map[string]struct {
		provider string
		cancel   bool
	}{
		"telegram cancel":      {provider: "telegram", cancel: true},
		"telegram deadline":    {provider: "telegram"},
		"pushover cancel":      {provider: "pushover", cancel: true},
		"pushover deadline":    {provider: "pushover"},
		"pushbullet cancel":    {provider: "pushbullet", cancel: true},
		"pushbullet deadline":  {provider: "pushbullet"},
		"twilio cancel":        {provider: "twilio", cancel: true},
		"twilio deadline":      {provider: "twilio"},
		"messagebird cancel":   {provider: "messagebird", cancel: true},
		"messagebird deadline": {provider: "messagebird"},
		"gotify cancel":        {provider: "gotify", cancel: true},
		"gotify deadline":      {provider: "gotify"},
		"ntfy cancel":          {provider: "ntfy", cancel: true},
		"ntfy deadline":        {provider: "ntfy"},
		"rocketchat cancel":    {provider: "rocketchat", cancel: true},
		"rocketchat deadline":  {provider: "rocketchat"},
	} {
		t.Run(name, func(t *testing.T) {
			started, stopped, cleanup := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var after atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/bot123:synthetic-private-value/sendMessage",
					"/1/messages.json",
					"/v2/pushes",
					twilioTestPath,
					messagebirdTestPath, "/message", "/topic", "/rocket-hook":
					_, _ = io.Copy(io.Discard, r.Body)
					if r.URL.Path == twilioTestPath || r.URL.Path == messagebirdTestPath {
						w.WriteHeader(201)
					} else {
						w.WriteHeader(200)
					}
					_, _ = io.WriteString(w, `{`)
					w.(http.Flusher).Flush()
					close(started)
					select {
					case <-r.Context().Done():
						close(stopped)
					case <-cleanup:
					}
				case "/after":
					after.Add(1)
				default:
					w.WriteHeader(204)
				}
			}))
			defer server.Close()
			defer close(cleanup)
			dst := Destination{
				Type:     "telegram",
				APIURL:   server.URL,
				BotToken: "123:synthetic-private-value",
				ChatID:   "1",
			}
			if test.provider == "pushover" {
				dst = Destination{
					Type:     "pushover",
					APIURL:   server.URL,
					AppToken: pushoverTestToken,
					UserKey:  pushoverTestUser,
				}
			}
			if test.provider == "pushbullet" {
				dst = Destination{
					Type:        "pushbullet",
					APIURL:      server.URL,
					AccessToken: "synthetic-private-value",
					Email:       "ops@example.com",
				}
			}
			if test.provider == "twilio" {
				dst = Destination{
					Type:       "twilio",
					APIURL:     server.URL,
					AccountSID: twilioTestSID,
					AuthToken:  "synthetic-private-value",
					From:       "+15005550006",
					To:         "+15005550009",
				}
			}
			if test.provider == "messagebird" {
				dst = Destination{
					Type:       "messagebird",
					APIURL:     server.URL,
					AccessKey:  "synthetic-private-value",
					Originator: "Netdata",
					Recipient:  "+15005550009",
				}
			}
			if test.provider == "gotify" {
				dst = Destination{Type: "gotify", APIURL: server.URL, AppToken: "synthetic-token"}
			}
			if test.provider == "ntfy" {
				dst = Destination{Type: "ntfy", URL: server.URL + "/topic"}
			}
			if test.provider == "rocketchat" {
				dst = Destination{Type: "rocketchat", URL: server.URL + "/rocket-hook"}
			}
			config, err := yaml.Marshal(Config{Version: 1, Destinations: map[string]Destination{
				"first":   {Type: "webhook", URL: server.URL + "/first"},
				"blocked": dst,
				"after":   {Type: "webhook", URL: server.URL + "/after"},
			}, Routing: Routing{Roles: map[string][]string{"ops": {"first", "blocked", "after"}}}})
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			path := writeConfig(t, string(config))
			var stdout, stderr bytes.Buffer
			done := make(chan int, 1)
			go func() {
				done <- Run(ctx, []string{"send", "--config", path, "--role", "ops", "--timeout", "500ms"}, strings.NewReader(validEvent), &stdout, &stderr)
			}()
			select {
			case <-started:
			case <-time.After(2 * time.Second):
				t.Fatal("provider request not started")
			}
			if test.cancel {
				cancel()
			}
			select {
			case code := <-done:
				assert.Equal(t, 1, code)
			case <-time.After(2 * time.Second):
				t.Fatal("command did not stop")
			}
			select {
			case <-stopped:
			case <-time.After(time.Second):
				t.Fatal("receiver did not see cancellation")
			}
			assert.Zero(t, after.Load())
			assert.Empty(t, stdout.String())
			assert.Contains(t, stderr.String(), `destination "first" sent`)
			assert.NotContains(t, stderr.String(), "synthetic-private-value")
			assert.NotContains(t, stderr.String(), server.URL)
		})
	}
}
