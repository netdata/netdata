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

func TestIncidentConfiguration(t *testing.T) {
	file := "${file:" + filepath.Join(t.TempDir(), "unread") + "}"
	for name, test := range map[string]struct {
		dst Destination
		err string
	}{
		"ilert defaults":          {dst: Destination{Type: "ilert", IntegrationKey: "synthetic-key"}},
		"ilert official":          {dst: Destination{Type: "ilert", IntegrationKey: "synthetic-key", APIURL: "https://api.ilert.com/api"}},
		"ilert proxy prefix":      {dst: Destination{Type: "ilert", IntegrationKey: "synthetic-key", APIURL: "http://localhost:8080/proxy/api/"}},
		"ilert environment":       {dst: Destination{Type: "ilert", IntegrationKey: "${env:UNREAD_KEY}", APIURL: "${env:UNREAD_API}"}},
		"ilert files":             {dst: Destination{Type: "ilert", IntegrationKey: file, APIURL: file}},
		"ilert missing key":       {dst: Destination{Type: "ilert"}, err: "integration_key must be nonempty"},
		"ilert key spaces":        {dst: Destination{Type: "ilert", IntegrationKey: "synthetic-key "}, err: "without whitespace"},
		"ilert key controls":      {dst: Destination{Type: "ilert", IntegrationKey: "synthetic-key\n"}, err: "without whitespace"},
		"ilert key Unicode":       {dst: Destination{Type: "ilert", IntegrationKey: "synthetic-key界"}, err: "printable ASCII"},
		"ilert key interpolation": {dst: Destination{Type: "ilert", IntegrationKey: "synthetic-key${env:KEY}"}, err: "secret reference"},
		"ilert relative file":     {dst: Destination{Type: "ilert", IntegrationKey: "${file:relative}"}, err: "absolute path"},
		"ilert insecure official": {dst: Destination{Type: "ilert", IntegrationKey: "synthetic-key", APIURL: "http://API.ILERT.COM.:80/api"}, err: "requires HTTPS"},
		"ilert query":             {dst: Destination{Type: "ilert", IntegrationKey: "synthetic-key", APIURL: "https://example.com/api?key=synthetic-key"}, err: "query"},
		"ilert empty query":       {dst: Destination{Type: "ilert", IntegrationKey: "synthetic-key", APIURL: "https://example.com/api?"}, err: "query"},
		"ilert empty fragment":    {dst: Destination{Type: "ilert", IntegrationKey: "synthetic-key", APIURL: "https://example.com/api#"}, err: "fragment"},
		"ilert fragment":          {dst: Destination{Type: "ilert", IntegrationKey: "synthetic-key", APIURL: "https://example.com/api#synthetic-key"}, err: "fragment"},
		"ilert user info":         {dst: Destination{Type: "ilert", IntegrationKey: "synthetic-key", APIURL: "https://user:synthetic-key@example.com/api"}, err: "user information"},
		"ilert relative API":      {dst: Destination{Type: "ilert", IntegrationKey: "synthetic-key", APIURL: "/api"}, err: "absolute HTTP(S)"},
		"ilert API interpolation": {dst: Destination{Type: "ilert", IntegrationKey: "synthetic-key", APIURL: "https://example.com/${env:API}"}, err: "secret reference"},
		"SIGNL4 URL":              {dst: Destination{Type: "signl4", URL: "https://example.com/hook?keep=a%26b"}},
		"SIGNL4 local":            {dst: Destination{Type: "signl4", URL: "http://localhost:8080/hook"}},
		"SIGNL4 environment":      {dst: Destination{Type: "signl4", URL: "${env:UNREAD_URL}"}},
		"SIGNL4 file":             {dst: Destination{Type: "signl4", URL: file}},
		"SIGNL4 missing URL":      {dst: Destination{Type: "signl4"}, err: "absolute HTTP(S)"},
		"SIGNL4 relative URL":     {dst: Destination{Type: "signl4", URL: "/synthetic-key"}, err: "absolute HTTP(S)"},
		"SIGNL4 user info":        {dst: Destination{Type: "signl4", URL: "https://user:synthetic-key@example.com"}, err: "user information"},
		"SIGNL4 fragment":         {dst: Destination{Type: "signl4", URL: "https://example.com/#synthetic-key"}, err: "fragment"},
		"SIGNL4 interpolation":    {dst: Destination{Type: "signl4", URL: "https://example.com/${env:KEY}"}, err: "secret reference"},
	} {
		t.Run(name, func(t *testing.T) {
			want := Config{Version: 1, Destinations: map[string]Destination{"incident": test.dst}}
			data, err := yaml.Marshal(want)
			require.NoError(t, err)
			got, err := readConfig(strings.NewReader(string(data)))
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				assert.NotContains(t, err.Error(), "synthetic-key")
				assert.Equal(t, Config{}, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

func TestIncidentFieldIsolation(t *testing.T) {
	fields := map[string]any{
		"url": "https://example.com", "api_url": "https://example.com", "integration_key": "synthetic-key",
		"bearer_token": "synthetic-key", "bot_token": "synthetic-key", "app_token": "synthetic-key",
		"user_key": "synthetic-key", "access_token": "synthetic-key", "username": "user", "password": "synthetic-key",
		"email": "ops@example.com", "channel_tag": "alerts", "source_device_id": "source", "chat_id": "1",
		"message_thread_id": 1, "retries_on_limit": 0, "account_sid": twilioTestSID, "auth_token": "synthetic-key",
		"from": "12345", "to": "12345", "access_key": "synthetic-key", "originator": "Netdata",
		"recipient": "12345", "channel": "#alerts", "sender": "Netdata",
	}
	for provider, test := range map[string]struct{ allowed map[string]any }{
		"ilert":  {map[string]any{"integration_key": "synthetic-key", "api_url": "https://example.com/api"}},
		"signl4": {map[string]any{"url": "https://example.com/hook"}},
	} {
		t.Run(provider, func(t *testing.T) {
			for field, value := range fields {
				if _, ok := test.allowed[field]; ok {
					continue
				}
				t.Run(field, func(t *testing.T) {
					dst := map[string]any{"type": provider, field: value}
					for name, allowed := range test.allowed {
						dst[name] = allowed
					}
					data, err := yaml.Marshal(
						map[string]any{"version": 1, "destinations": map[string]any{"incident": dst}},
					)
					require.NoError(t, err)
					got, err := readConfig(strings.NewReader(string(data)))
					require.Error(t, err)
					assert.Equal(t, Config{}, got)
					assert.NotContains(t, err.Error(), "synthetic-key")
				})
			}
		})
	}
	for provider := range map[string]struct{}{
		"webhook": {}, "slack": {}, "discord": {}, "telegram": {}, "pushover": {}, "pushbullet": {},
		"twilio": {}, "messagebird": {}, "gotify": {}, "ntfy": {}, "rocketchat": {}, "flock": {}, "fleep": {},
	} {
		t.Run(provider, func(t *testing.T) {
			dst := Destination{Type: provider, IntegrationKey: "synthetic-key"}
			require.EqualError(t, dst.validate(), "integration_key requires type: ilert or pagerduty")
		})
	}
}
