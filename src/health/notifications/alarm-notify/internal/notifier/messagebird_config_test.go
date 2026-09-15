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

func TestMessageBirdConfiguration(t *testing.T) {
	base := Destination{
		Type:       "messagebird",
		AccessKey:  "synthetic-private-value",
		Originator: "Netdata",
		Recipient:  "+15005550009",
	}
	full, numeric, inbox, digits, secrets := base, base, base, base, base
	full.APIURL = "http://localhost:8081/proxy/"
	numeric.Originator = "+15005550006"
	inbox.Originator = "inbox"
	digits.Recipient = "0015005550009"
	secrets.AccessKey, secrets.APIURL = "${file:"+filepath.Join(t.TempDir(), "unread")+"}", "${env:NOTIFIER_UNUSED_API}"
	for name, test := range map[string]struct {
		changes map[string]any
		want    Destination
		err     string
	}{
		"minimal":                    {want: base},
		"full":                       {changes: map[string]any{"api_url": full.APIURL}, want: full},
		"numeric originator":         {changes: map[string]any{"originator": numeric.Originator}, want: numeric},
		"inbox originator":           {changes: map[string]any{"originator": inbox.Originator}, want: inbox},
		"recipient digits preserved": {changes: map[string]any{"recipient": digits.Recipient}, want: digits},
		"unresolved secrets":         {changes: map[string]any{"access_key": secrets.AccessKey, "api_url": secrets.APIURL}, want: secrets},
		"missing key":                {changes: map[string]any{"access_key": nil}, err: "access_key"},
		"key line break":             {changes: map[string]any{"access_key": "synthetic-private-value\n"}, err: "access_key"},
		"key whitespace":             {changes: map[string]any{"access_key": "synthetic-private-value extra"}, err: "access_key"},
		"key control":                {changes: map[string]any{"access_key": "synthetic-private-value\x00"}, err: "access_key"},
		"key unicode":                {changes: map[string]any{"access_key": "界"}, err: "access_key"},
		"interpolation":              {changes: map[string]any{"access_key": "prefix${env:KEY}"}, err: "secret reference"},
		"relative file":              {changes: map[string]any{"access_key": "${file:relative}"}, err: "absolute"},
		"missing originator":         {changes: map[string]any{"originator": nil}, err: "originator"},
		"originator control":         {changes: map[string]any{"originator": "sender\n"}, err: "originator"},
		"originator whitespace":      {changes: map[string]any{"originator": " sender"}, err: "originator"},
		"originator reference":       {changes: map[string]any{"originator": "${env:FROM}"}, err: "originator"},
		"missing recipient":          {changes: map[string]any{"recipient": nil}, err: "recipient"},
		"recipient reference":        {changes: map[string]any{"recipient": "${env:TO}"}, err: "recipient"},
		"recipient whitespace":       {changes: map[string]any{"recipient": "+15005550009 +15005550006"}, err: "recipient"},
		"recipient list":             {changes: map[string]any{"recipient": "+15005550009,+15005550006"}, err: "recipient"},
		"recipient plus only":        {changes: map[string]any{"recipient": "+"}, err: "recipient"},
		"recipient multiple plus":    {changes: map[string]any{"recipient": "++15005550009"}, err: "recipient"},
		"recipient unicode digits":   {changes: map[string]any{"recipient": "１２３"}, err: "recipient"},
		"API query":                  {changes: map[string]any{"api_url": "https://example.com/?secret=value"}, err: "query"},
		"API empty query":            {changes: map[string]any{"api_url": "https://example.com/?"}, err: "query"},
		"API fragment":               {changes: map[string]any{"api_url": "https://example.com/#"}, err: "fragment"},
		"API userinfo":               {changes: map[string]any{"api_url": "https://user:synthetic-private-value@example.com"}, err: "user information"},
		"relative API":               {changes: map[string]any{"api_url": "/proxy"}, err: "absolute HTTP(S)"},
		"official plaintext":         {changes: map[string]any{"api_url": "http://REST.MESSAGEBIRD.COM./"}, err: "requires HTTPS"},
	} {
		t.Run(name, func(t *testing.T) {
			fields := map[string]any{
				"type":       base.Type,
				"access_key": base.AccessKey,
				"originator": base.Originator,
				"recipient":  base.Recipient,
			}
			for key, value := range test.changes {
				fields[key] = value
			}
			input, err := yaml.Marshal(map[string]any{"version": 1, "destinations": map[string]any{"sms": fields}})
			require.NoError(t, err)
			got, err := readConfig(strings.NewReader(string(input)))
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				for _, private := range []string{base.AccessKey, base.Originator, base.Recipient} {
					assert.NotContains(t, err.Error(), private)
				}
				assert.Equal(t, Config{}, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, Config{Version: 1, Destinations: map[string]Destination{"sms": test.want}}, got)
		})
	}
}

func TestMessageBirdRejectsOtherFields(t *testing.T) {
	for field, value := range map[string]any{
		"url": "https://example.com", "bearer_token": "token", "bot_token": "token", "app_token": "token",
		"user_key": "user", "access_token": "token", "email": "ops@example.com", "channel_tag": "alerts",
		"source_device_id": "source", "chat_id": "1", "message_thread_id": 1, "retries_on_limit": 0,
		"account_sid": twilioTestSID, "auth_token": "token", "from": "12345", "to": "+15005550009",
	} {
		t.Run(field, func(t *testing.T) {
			data, err := yaml.Marshal(map[string]any{"version": 1, "destinations": map[string]any{"sms": map[string]any{
				"type": "messagebird", "access_key": "token", "originator": "Netdata", "recipient": "+15005550009", field: value,
			}}})
			require.NoError(t, err)
			_, err = readConfig(strings.NewReader(string(data)))
			require.ErrorContains(t, err, "support access_key, originator, recipient and api_url only")
		})
	}
}

func TestMessageBirdFieldsRejectOtherProviders(t *testing.T) {
	for _, provider := range []string{"webhook", "slack", "discord", "telegram", "pushover", "pushbullet", "twilio"} {
		t.Run(provider, func(t *testing.T) {
			for field, value := range map[string]string{"access_key": "token", "originator": "Netdata", "recipient": "+15005550009"} {
				t.Run(field, func(t *testing.T) {
					data, err := yaml.Marshal(
						map[string]any{
							"version":      1,
							"destinations": map[string]any{"target": map[string]any{"type": provider, field: value}},
						},
					)
					require.NoError(t, err)
					_, err = readConfig(strings.NewReader(string(data)))
					require.ErrorContains(t, err, "require type: messagebird")
				})
			}
		})
	}
}
