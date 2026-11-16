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
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"

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
		retries                           field.Integer
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
			err:      "all attempted destinations failed",
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
			topic := field.Integer(7)
			dst := map[string]any{
				"type":              "telegram",
				"bot_token":         "123:synthetic-private-value",
				"chat_id":           "-100123",
				"message_thread_id": &topic,
				"api_url":           server.URL + "/proxy/",
			}
			if test.retries != 0 {
				dst["retries_on_limit"] = &test.retries
			}
			switch test.secret {
			case "env", "bad-token", "empty-token", "bad-api":
				token, api := dst["bot_token"].(string), dst["api_url"].(string)
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
				dst["bot_token"], dst["api_url"] = "${env:NOTIFIER_TELEGRAM_TOKEN}", "${env:NOTIFIER_TELEGRAM_API}"
			case "file":
				path := filepath.Join(t.TempDir(), "token")
				require.NoError(t, os.WriteFile(path, []byte(" "+dst["bot_token"].(string)+"\n"), 0600))
				dst["bot_token"] = "${file:" + path + "}"
			}
			other := maps.Clone(dst)
			other["chat_id"], other["message_thread_id"] = "@example_alerts", nil
			cfg := testConfig{Version: 1, Destinations: map[string]map[string]any{
				"chat": dst, "other": other,
				"slack":   {"type": "slack", "url": server.URL + "/slack"},
				"discord": {"type": "discord", "url": server.URL + "/discord"},
				"archive": {"type": "webhook", "url": server.URL + "/archive", "bearer_token": "synthetic-private-value"},
				"unused":  {"type": "telegram", "bot_token": "${env:NOTIFIER_TELEGRAM_UNUSED}", "chat_id": "1"},
			}, Routing: notifier.Routing{Roles: map[string][]string{"ops": {"chat", "slack", "discord", "archive"}, "dba": {"chat"}}}}
			if test.second {
				cfg.Routing.Roles["dba"] = append(cfg.Routing.Roles["dba"], "other")
			}
			config, err := yaml.Marshal(cfg)
			require.NoError(t, err)
			event := testutil.ExpectedEvent()
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
					payload, err = os.ReadFile(fixturePath(fixture))
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
		"matrix cancel": {provider: "matrix", cancel: true}, "matrix deadline": {provider: "matrix"},
		"opsgenie create deadline": {provider: "opsgenie"}, "opsgenie create cancel": {provider: "opsgenie", cancel: true},
		"opsgenie close deadline": {provider: "opsgenie-close"}, "opsgenie close cancel": {provider: "opsgenie-close", cancel: true},
		"pagerduty v1 deadline": {provider: "pagerduty-v1"}, "pagerduty v1 cancel": {provider: "pagerduty-v1", cancel: true},
		"pagerduty v2 deadline": {provider: "pagerduty-v2"}, "pagerduty v2 cancel": {provider: "pagerduty-v2", cancel: true},
		"smseagle cancel": {provider: "smseagle", cancel: true}, "smseagle deadline": {provider: "smseagle"},
		"prowl cancel": {provider: "prowl", cancel: true}, "prowl deadline": {provider: "prowl"},
		"kavenegar cancel": {provider: "kavenegar", cancel: true}, "kavenegar deadline": {provider: "kavenegar"},
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
		"alerta cancel":        {provider: "alerta", cancel: true}, "alerta deadline": {provider: "alerta"},
		"dynatrace cancel": {provider: "dynatrace", cancel: true}, "dynatrace deadline": {provider: "dynatrace"},
	} {
		t.Run(name, func(t *testing.T) {
			started, stopped, cleanup := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var after atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path := r.URL.Path
				if strings.HasPrefix(path, "/_matrix/client/v3/rooms/") {
					path = "/matrix"
				}
				switch path {
				case "/matrix", "/v2/alerts", "/v2/alerts/" + opsgenieTestAlias + "/close",
					"/generic/2010-04-15/create_event.json", "/v2/enqueue", "/api/v2/messages/sms",
					"/add",
					"/synthetic-key/sms/send.json",
					"/bot123:synthetic-private-value/sendMessage",
					"/1/messages.json",
					"/v2/pushes",
					twilioTestPath,
					messagebirdTestPath,
					"/message",
					"/topic",
					"/rocket-hook",
					"/alert",
					"/api/v2/events/ingest":
					_, _ = io.Copy(io.Discard, r.Body)
					if r.URL.Path == "/v2/enqueue" || strings.HasPrefix(r.URL.Path, "/v2/alerts") {
						w.WriteHeader(202)
					} else if r.URL.Path == twilioTestPath || r.URL.Path == messagebirdTestPath ||
						r.URL.Path == "/api/v2/events/ingest" {
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
			dst := map[string]any{
				"type":      "telegram",
				"api_url":   server.URL,
				"bot_token": "123:synthetic-private-value",
				"chat_id":   "1",
			}
			if test.provider == "matrix" {
				dst = map[string]any{
					"type":         "matrix",
					"api_url":      server.URL,
					"access_token": "synthetic-token",
					"room_id":      "!room:example.org",
				}
			}
			if test.provider == "pushover" {
				dst = map[string]any{
					"type":      "pushover",
					"api_url":   server.URL,
					"app_token": pushoverTestToken,
					"user_key":  pushoverTestUser,
				}
			}
			if test.provider == "pushbullet" {
				dst = map[string]any{
					"type":         "pushbullet",
					"api_url":      server.URL,
					"access_token": "synthetic-private-value",
					"email":        "ops@example.com",
				}
			}
			if test.provider == "twilio" {
				dst = map[string]any{
					"type":        "twilio",
					"api_url":     server.URL,
					"account_sid": twilioTestSID,
					"auth_token":  "synthetic-private-value",
					"from":        "+15005550006",
					"to":          "+15005550009",
				}
			}
			if test.provider == "messagebird" {
				dst = map[string]any{
					"type":       "messagebird",
					"api_url":    server.URL,
					"access_key": "synthetic-private-value",
					"originator": "Netdata",
					"recipient":  "+15005550009",
				}
			}
			if test.provider == "gotify" {
				dst = map[string]any{"type": "gotify", "api_url": server.URL, "app_token": "synthetic-token"}
			}
			if test.provider == "ntfy" {
				dst = map[string]any{"type": "ntfy", "url": server.URL + "/topic"}
			}
			if test.provider == "rocketchat" {
				dst = map[string]any{"type": "rocketchat", "url": server.URL + "/rocket-hook"}
			}
			if test.provider == "alerta" {
				dst = map[string]any{"type": "alerta", "api_url": server.URL, "environment": "Production"}
			}
			if test.provider == "dynatrace" {
				dst = map[string]any{
					"type":            "dynatrace",
					"api_url":         server.URL,
					"api_token":       "synthetic-key",
					"entity_selector": "type(HOST)",
				}
			}
			if strings.HasPrefix(test.provider, "pagerduty-v") {
				dst = pagerDutyTestDestination(1)
				if test.provider == "pagerduty-v2" {
					dst = pagerDutyTestDestination(2)
				}
				dst["api_url"] = server.URL
			}
			if strings.HasPrefix(test.provider, "opsgenie") {
				dst = opsgenieTestDestination()
				dst["api_url"] = server.URL
			}
			if test.provider == "smseagle" {
				dst = smseagleTestDestination()
				dst["api_url"] = server.URL
			}
			if test.provider == "prowl" || test.provider == "kavenegar" {
				dst = formTestDestination(test.provider)
				dst["api_url"] = server.URL
			}

			config, err := yaml.Marshal(testConfig{Version: 1, Destinations: map[string]map[string]any{
				"first":   {"type": "webhook", "url": server.URL + "/first"},
				"blocked": dst,
				"after":   {"type": "webhook", "url": server.URL + "/after"},
			}, Routing: notifier.Routing{Roles: map[string][]string{"ops": {"first", "blocked", "after"}}}})
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			path := writeConfig(t, string(config))
			var stdout, stderr bytes.Buffer
			done := make(chan int, 1)
			input := testutil.ValidEvent
			if test.provider == "opsgenie-close" {
				input = strings.ReplaceAll(input, "WARNING", "CLEAR")
			}
			go func() {
				done <- Run(ctx, []string{"send", "--config", path, "--role", "ops", "--timeout", "500ms"}, strings.NewReader(input), &stdout, &stderr)
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
