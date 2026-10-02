// SPDX-License-Identifier: GPL-3.0-or-later

package telegram

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTelegramConfiguration(t *testing.T) {
	base := Config{BotToken: "123:synthetic-private-value", ChatID: "-100123"}
	topic, retries := field.Integer(7), field.Integer(2)
	full := base
	full.APIURL, full.MessageThreadID, full.RetriesOnLimit = "http://localhost:8081/api", &topic, &retries
	file := filepath.Join(t.TempDir(), "unread")
	tests := map[string]struct {
		fields string
		want   Config
		err    string
	}{
		"minimal": {
			fields: `bot_token: '123:synthetic-private-value', chat_id: '-100123'`,
			want:   base,
		},
		"unquoted numeric chat": {
			fields: `bot_token: '123:synthetic-private-value', chat_id: -100123`,
			want:   base,
		},
		"full": {
			fields: `bot_token: '123:synthetic-private-value', chat_id: '-100123', api_url: 'http://localhost:8081/api', message_thread_id: 7, retries_on_limit: 2`,
			want:   full,
		},
		"channel name and unresolved token": {
			fields: `bot_token: '${env:NOTIFIER_UNUSED_TOKEN}', chat_id: '@example_alerts'`,
			want:   Config{BotToken: "${env:NOTIFIER_UNUSED_TOKEN}", ChatID: "@example_alerts"},
		},
		"unread file and unresolved API": {
			fields: fmt.Sprintf(
				`bot_token: %q, chat_id: '1', api_url: '${env:NOTIFIER_UNUSED_API}'`,
				"${file:"+file+"}",
			),
			want: Config{
				BotToken: "${file:" + file + "}",
				ChatID:   "1",
				APIURL:   "${env:NOTIFIER_UNUSED_API}",
			},
		},
		"missing token": {fields: `chat_id: '1'`, err: "bot_token"},
		"missing chat":  {fields: `bot_token: '123:synthetic-private-value'`, err: "chat_id"},
		"zero chat": {
			fields: `bot_token: '123:synthetic-private-value', chat_id: '0'`,
			err:    "chat_id",
		},
		"chat overflow": {
			fields: `bot_token: '123:synthetic-private-value', chat_id: '9223372036854775808'`,
			err:    "chat_id",
		},
		"legacy chat topic syntax": {
			fields: `bot_token: '123:synthetic-private-value', chat_id: '-100123:7'`,
			err:    "chat_id",
		},
		"multiple recipients": {
			fields: `bot_token: '123:synthetic-private-value', chat_id: '1 2'`,
			err:    "chat_id",
		},
		"empty username": {
			fields: `bot_token: '123:synthetic-private-value', chat_id: '@'`,
			err:    "chat_id",
		},
		"zero topic": {
			fields: `bot_token: '123:synthetic-private-value', chat_id: '1', message_thread_id: 0`,
			err:    "message_thread_id",
		},
		"negative topic": {
			fields: `bot_token: '123:synthetic-private-value', chat_id: '1', message_thread_id: -1`,
			err:    "message_thread_id",
		},
		"fractional topic": {
			fields: `bot_token: '123:synthetic-private-value', chat_id: '1', message_thread_id: 1.5`,
			err:    "invalid YAML",
		},
		"negative retries": {
			fields: `bot_token: '123:synthetic-private-value', chat_id: '1', retries_on_limit: -1`,
			err:    "retries_on_limit",
		},
		"fractional retries": {
			fields: `bot_token: '123:synthetic-private-value', chat_id: '1', retries_on_limit: 1.5`,
			err:    "invalid YAML",
		},
		"whole float topic": {
			fields: `bot_token: '123:synthetic-private-value', chat_id: '1', message_thread_id: 1.0`,
			err:    "invalid YAML",
		},
		"invalid token characters": {
			fields: `bot_token: '123:synthetic-private-value/path', chat_id: '1'`,
			err:    "bot_token",
		},
		"token interpolation": {
			fields: `bot_token: '123:${env:NOTIFIER_TEST}', chat_id: '1'`,
			err:    "secret reference",
		},
		"API interpolation": {
			fields: `bot_token: '123:synthetic-private-value', chat_id: '1', api_url: 'https://${env:HOST}'`,
			err:    "secret reference",
		},
		"relative API": {
			fields: `bot_token: '123:synthetic-private-value', chat_id: '1', api_url: '/local'`,
			err:    "absolute HTTP(S)",
		},
		"API fragment": {
			fields: `bot_token: '123:synthetic-private-value', chat_id: '1', api_url: 'https://example.com/#fragment'`,
			err:    "fragment",
		},
		"API empty query": {
			fields: `bot_token: '123:synthetic-private-value', chat_id: '1', api_url: 'https://example.com/?'`,
			err:    "query",
		},
		"webhook URL": {
			fields: `bot_token: '123:synthetic-private-value', chat_id: '1', url: 'https://example.com'`,
			err:    "invalid YAML",
		},
		"bearer token": {
			fields: `bot_token: '123:synthetic-private-value', chat_id: '1', bearer_token: 'synthetic-private-value'`,
			err:    "invalid YAML",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			input := "version: 1\ndestinations:\n  chat: {type: telegram, " + test.fields + "}\n"
			got, err := readConfig(strings.NewReader(input))
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				assert.NotContains(t, err.Error(), "synthetic-private-value")
				assert.Equal(t, testutil.Document[Config]{}, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, testutil.Document[Config]{Version: 1, Destinations: map[string]Config{"chat": test.want}}, got)
		})
	}
}
