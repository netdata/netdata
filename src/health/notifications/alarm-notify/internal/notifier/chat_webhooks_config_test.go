// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestChatWebhookConfiguration(t *testing.T) {
	file := "${file:" + filepath.Join(t.TempDir(), "unread") + "}"
	for name, test := range map[string]struct {
		dst Destination
		err string
	}{
		"Rocket.Chat defaults":   {dst: Destination{Type: "rocketchat", URL: "http://localhost:8080/hooks/test"}},
		"Rocket.Chat channel":    {dst: Destination{Type: "rocketchat", URL: "https://example.com/hook?key=synthetic-token", Channel: "#alerts"}},
		"Rocket.Chat user":       {dst: Destination{Type: "rocketchat", URL: "${env:UNREAD_URL}", Channel: "@user"}},
		"Rocket.Chat Unicode":    {dst: Destination{Type: "rocketchat", URL: file, Channel: "#警告"}},
		"Flock":                  {dst: Destination{Type: "flock", URL: "https://example.com/hook?key=synthetic-token"}},
		"Flock file":             {dst: Destination{Type: "flock", URL: file}},
		"Fleep default sender":   {dst: Destination{Type: "fleep", URL: "https://example.com/hook"}},
		"Fleep sender":           {dst: Destination{Type: "fleep", URL: file, Sender: "Netdata's \"警告\" bot"}},
		"missing URL":            {dst: Destination{Type: "flock"}, err: "absolute HTTP(S)"},
		"relative URL":           {dst: Destination{Type: "fleep", URL: "/hook"}, err: "absolute HTTP(S)"},
		"URL user info":          {dst: Destination{Type: "rocketchat", URL: "https://user:synthetic-token@example.com"}, err: "user information"},
		"URL fragment":           {dst: Destination{Type: "flock", URL: "https://example.com/#private"}, err: "fragment"},
		"URL interpolation":      {dst: Destination{Type: "flock", URL: "https://example.com/${env:SECRET}"}, err: "secret reference"},
		"relative file":          {dst: Destination{Type: "fleep", URL: "${file:relative}"}, err: "absolute path"},
		"bad scheme":             {dst: Destination{Type: "rocketchat", URL: "file:///tmp/hook"}, err: "absolute HTTP(S)"},
		"missing channel prefix": {dst: Destination{Type: "rocketchat", URL: file, Channel: "alerts"}, err: "one #channel or @user"},
		"empty channel name":     {dst: Destination{Type: "rocketchat", URL: file, Channel: "#"}, err: "one #channel or @user"},
		"multiple channels":      {dst: Destination{Type: "rocketchat", URL: file, Channel: "#a,#b"}, err: "one #channel or @user"},
		"channel spaces":         {dst: Destination{Type: "rocketchat", URL: file, Channel: "#a b"}, err: "one #channel or @user"},
		"channel controls":       {dst: Destination{Type: "rocketchat", URL: file, Channel: "#a\x00"}, err: "one #channel or @user"},
		"channel reference":      {dst: Destination{Type: "rocketchat", URL: file, Channel: "${env:CHANNEL}"}, err: "one #channel or @user"},
		"blank sender":           {dst: Destination{Type: "fleep", URL: file, Sender: " \t"}, err: "literal name"},
		"sender controls":        {dst: Destination{Type: "fleep", URL: file, Sender: "bot\n"}, err: "literal name"},
		"sender reference":       {dst: Destination{Type: "fleep", URL: file, Sender: "${env:SENDER}"}, err: "literal name"},
	} {
		t.Run(name, func(t *testing.T) {
			want := Config{Version: 1, Destinations: map[string]Destination{"chat": test.dst}}
			data, err := yaml.Marshal(want)
			require.NoError(t, err)
			got, err := readConfig(strings.NewReader(string(data)))
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				assert.NotContains(t, err.Error(), "synthetic-token")
				assert.Equal(t, Config{}, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

func TestChatWebhookFieldIsolation(t *testing.T) {
	foreign := map[string]any{
		"api_url": "https://example.com", "bearer_token": "synthetic-token", "bot_token": "synthetic-token",
		"app_token": "synthetic-token", "user_key": "key", "access_token": "synthetic-token", "username": "user",
		"password": "synthetic-token", "email": "ops@example.com", "channel_tag": "alerts", "source_device_id": "source",
		"chat_id": "1", "message_thread_id": 1, "retries_on_limit": 0, "account_sid": twilioTestSID,
		"auth_token": "synthetic-token", "from": "12345", "to": "12345", "access_key": "synthetic-token",
		"originator": "Netdata", "recipient": "12345", "channel": "#alerts", "sender": "Netdata",
	}
	for provider, test := range map[string]struct{ allowed string }{
		"rocketchat": {allowed: "channel"}, "flock": {}, "fleep": {allowed: "sender"},
	} {
		t.Run(provider, func(t *testing.T) {
			for field, value := range foreign {
				if field == test.allowed {
					continue
				}
				t.Run(field, func(t *testing.T) {
					data, err := yaml.Marshal(map[string]any{"version": 1, "destinations": map[string]any{
						"chat": map[string]any{"type": provider, "url": "https://example.com", field: value},
					}})
					require.NoError(t, err)
					got, err := readConfig(strings.NewReader(string(data)))
					require.ErrorContains(t, err, "fields for another provider")
					assert.Equal(t, Config{}, got)
					assert.NotContains(t, err.Error(), "synthetic-token")
				})
			}
		})
	}
	for name, test := range map[string]struct{ provider string }{
		"webhook": {"webhook"}, "Slack": {"slack"}, "Discord": {"discord"}, "Telegram": {"telegram"},
		"Pushover": {"pushover"}, "Pushbullet": {"pushbullet"}, "Twilio": {"twilio"}, "MessageBird": {"messagebird"},
		"Gotify": {"gotify"}, "ntfy": {"ntfy"},
	} {
		t.Run(name, func(t *testing.T) {
			for field, value := range map[string]string{"channel": "#alerts", "sender": "Netdata"} {
				t.Run(field, func(t *testing.T) {
					data, err := yaml.Marshal(map[string]any{"version": 1, "destinations": map[string]any{
						"chat": map[string]any{"type": test.provider, "url": "https://example.com", field: value},
					}})
					require.NoError(t, err)
					got, err := readConfig(strings.NewReader(string(data)))
					require.ErrorContains(t, err, "channel requires type: rocketchat; sender requires type: fleep")
					assert.Equal(t, Config{}, got)
				})
			}
		})
	}
}
