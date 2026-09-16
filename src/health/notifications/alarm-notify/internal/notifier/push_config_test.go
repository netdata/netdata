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

func TestPushConfiguration(t *testing.T) {
	gotify := Destination{Type: "gotify", APIURL: "http://localhost:8081/prefix/", AppToken: "synthetic-token"}
	ntfy := Destination{Type: "ntfy", URL: "https://example.com/topic"}
	file := "${file:" + filepath.Join(t.TempDir(), "unread") + "}"
	for name, test := range map[string]struct {
		base    Destination
		changes map[string]any
		want    Destination
		err     string
	}{
		"gotify":                  {base: gotify, want: gotify},
		"anonymous ntfy":          {base: ntfy, want: ntfy},
		"ntfy Basic":              {base: ntfy, changes: map[string]any{"username": "user", "password": "a password:界"}, want: Destination{Type: "ntfy", URL: ntfy.URL, Username: "user", Password: "a password:界"}},
		"ntfy token":              {base: ntfy, changes: map[string]any{"access_token": "synthetic-token"}, want: Destination{Type: "ntfy", URL: ntfy.URL, AccessToken: "synthetic-token"}},
		"gotify references":       {base: gotify, changes: map[string]any{"app_token": file, "api_url": "${env:UNUSED_API}"}, want: Destination{Type: "gotify", APIURL: "${env:UNUSED_API}", AppToken: file}},
		"ntfy references":         {base: ntfy, changes: map[string]any{"url": file, "username": "${env:UNUSED_USER}", "password": file}, want: Destination{Type: "ntfy", URL: file, Username: "${env:UNUSED_USER}", Password: file}},
		"ntfy URL query":          {base: ntfy, changes: map[string]any{"url": ntfy.URL + "?cache=no"}, want: Destination{Type: "ntfy", URL: ntfy.URL + "?cache=no"}},
		"gotify missing API":      {base: gotify, changes: map[string]any{"api_url": nil}, err: "api_url is required"},
		"gotify missing token":    {base: gotify, changes: map[string]any{"app_token": nil}, err: "app_token must be"},
		"gotify API query":        {base: gotify, changes: map[string]any{"api_url": "https://example.com/?"}, err: "query"},
		"gotify API fragment":     {base: gotify, changes: map[string]any{"api_url": "https://example.com/#"}, err: "fragment"},
		"gotify API credentials":  {base: gotify, changes: map[string]any{"api_url": "https://user:synthetic-token@example.com"}, err: "user information"},
		"gotify token controls":   {base: gotify, changes: map[string]any{"app_token": "synthetic-token\n"}, err: "app_token must be"},
		"gotify token unicode":    {base: gotify, changes: map[string]any{"app_token": "界"}, err: "app_token must be"},
		"gotify interpolation":    {base: gotify, changes: map[string]any{"app_token": "prefix${env:KEY}"}, err: "secret reference"},
		"ntfy missing URL":        {base: ntfy, changes: map[string]any{"url": nil}, err: "topic URL"},
		"ntfy relative URL":       {base: ntfy, changes: map[string]any{"url": "/topic"}, err: "topic URL"},
		"ntfy fragment":           {base: ntfy, changes: map[string]any{"url": ntfy.URL + "#"}, err: "fragment"},
		"ntfy credentials in URL": {base: ntfy, changes: map[string]any{"url": "https://user:synthetic-token@example.com/topic"}, err: "user information"},
		"ntfy username only":      {base: ntfy, changes: map[string]any{"username": "user"}, err: "configured together"},
		"ntfy password only":      {base: ntfy, changes: map[string]any{"password": "synthetic-token"}, err: "configured together"},
		"ntfy conflicting auth":   {base: ntfy, changes: map[string]any{"username": "user", "password": "password", "access_token": "token"}, err: "not both"},
		"ntfy username colon":     {base: ntfy, changes: map[string]any{"username": "user:extra", "password": "password"}, err: "colon"},
		"ntfy password controls":  {base: ntfy, changes: map[string]any{"username": "user", "password": "synthetic-token\n"}, err: "controls"},
		"ntfy token whitespace":   {base: ntfy, changes: map[string]any{"access_token": "synthetic-token extra"}, err: "access_token must be"},
		"ntfy relative file":      {base: ntfy, changes: map[string]any{"access_token": "${file:relative}"}, err: "absolute"},
	} {
		t.Run(name, func(t *testing.T) {
			data, err := yaml.Marshal(test.base)
			require.NoError(t, err)
			fields := map[string]any{}
			require.NoError(t, yaml.Unmarshal(data, &fields))
			for k, v := range test.changes {
				fields[k] = v
			}
			data, err = yaml.Marshal(map[string]any{"version": 1, "destinations": map[string]any{"push": fields}})
			require.NoError(t, err)
			got, err := readConfig(strings.NewReader(string(data)))
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				assert.NotContains(t, err.Error(), "synthetic-token")
				assert.Equal(t, Config{}, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, Config{Version: 1, Destinations: map[string]Destination{"push": test.want}}, got)
		})
	}
}

func TestPushProviderFieldIsolation(t *testing.T) {
	fields := map[string]any{"url": "https://example.com", "api_url": "https://example.com", "bearer_token": "token",
		"bot_token": "token", "app_token": "token", "user_key": "user", "access_token": "token", "username": "user", "password": "password",
		"email": "ops@example.com", "channel_tag": "alerts", "source_device_id": "source", "chat_id": "1", "message_thread_id": 1,
		"retries_on_limit": 0, "account_sid": twilioTestSID, "auth_token": "token", "from": "12345", "to": "12345",
		"access_key": "token", "originator": "Netdata", "recipient": "12345"}
	for provider, allowed := range map[string]map[string]bool{
		"gotify": {"api_url": true, "app_token": true}, "ntfy": {"url": true, "access_token": true, "username": true, "password": true},
	} {
		t.Run(provider, func(t *testing.T) {
			for field, value := range fields {
				if allowed[field] {
					continue
				}
				t.Run(field, func(t *testing.T) {
					data, err := yaml.Marshal(
						map[string]any{
							"version":      1,
							"destinations": map[string]any{"push": map[string]any{"type": provider, field: value}},
						},
					)
					require.NoError(t, err)
					_, err = readConfig(strings.NewReader(string(data)))
					require.ErrorContains(t, err, provider+" destinations support")
				})
			}
		})
	}
	for _, provider := range []string{"webhook", "slack", "discord", "telegram", "pushover", "pushbullet", "twilio", "messagebird"} {
		t.Run(provider, func(t *testing.T) {
			for _, field := range []string{"username", "password"} {
				data, err := yaml.Marshal(
					map[string]any{
						"version":      1,
						"destinations": map[string]any{"push": map[string]any{"type": provider, field: "secret"}},
					},
				)
				require.NoError(t, err)
				_, err = readConfig(strings.NewReader(string(data)))
				require.ErrorContains(t, err, "require type: ntfy")
			}
		})
	}
}
