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

func TestPushbulletConfiguration(t *testing.T) {
	base := Destination{Type: "pushbullet", AccessToken: "synthetic-private-value", Email: "ops@example.com"}
	full := base
	full.APIURL, full.SourceDeviceID = "http://localhost:8081/proxy/", "test-source-device"
	file := "${file:" + filepath.Join(t.TempDir(), "unread") + "}"
	for name, test := range map[string]struct {
		changes map[string]any
		want    Destination
		err     string
	}{
		"email":               {want: base},
		"full":                {changes: map[string]any{"api_url": full.APIURL, "source_device_id": full.SourceDeviceID}, want: full},
		"channel":             {changes: map[string]any{"email": nil, "channel_tag": "test-alerts"}, want: Destination{Type: "pushbullet", AccessToken: base.AccessToken, ChannelTag: "test-alerts"}},
		"unresolved secrets":  {changes: map[string]any{"access_token": file, "api_url": "${env:NOTIFIER_UNUSED_API}"}, want: Destination{Type: "pushbullet", AccessToken: file, Email: base.Email, APIURL: "${env:NOTIFIER_UNUSED_API}"}},
		"missing token":       {changes: map[string]any{"access_token": nil}, err: "access_token"},
		"token line break":    {changes: map[string]any{"access_token": "synthetic-private-value\n"}, err: "access_token"},
		"token whitespace":    {changes: map[string]any{"access_token": "synthetic-private-value other"}, err: "access_token"},
		"token control":       {changes: map[string]any{"access_token": "synthetic-private-value\x00"}, err: "access_token"},
		"token unicode":       {changes: map[string]any{"access_token": "界"}, err: "access_token"},
		"interpolation":       {changes: map[string]any{"access_token": "prefix${env:TOKEN}"}, err: "secret reference"},
		"relative file":       {changes: map[string]any{"access_token": "${file:relative}"}, err: "absolute"},
		"missing recipient":   {changes: map[string]any{"email": nil}, err: "exactly one"},
		"two recipient types": {changes: map[string]any{"channel_tag": "test-alerts"}, err: "exactly one"},
		"multiple emails":     {changes: map[string]any{"email": "a@example.com,b@example.com"}, err: "single address"},
		"invalid email":       {changes: map[string]any{"email": "synthetic-private-value"}, err: "single address"},
		"display name":        {changes: map[string]any{"email": "Example <ops@example.com>"}, err: "single address"},
		"legacy hash":         {changes: map[string]any{"email": nil, "channel_tag": "#test-alerts"}, err: "channel_tag"},
		"multiple tags":       {changes: map[string]any{"email": nil, "channel_tag": "test-alerts other"}, err: "channel_tag"},
		"blank tag":           {changes: map[string]any{"email": nil, "channel_tag": " "}, err: "channel_tag"},
		"source whitespace":   {changes: map[string]any{"source_device_id": " "}, err: "source_device_id"},
		"source control":      {changes: map[string]any{"source_device_id": "test\x00"}, err: "source_device_id"},
		"source reference":    {changes: map[string]any{"source_device_id": "${env:SOURCE}"}, err: "source_device_id"},
		"API query":           {changes: map[string]any{"api_url": "https://example.com/?token=synthetic-private-value"}, err: "query"},
		"API empty query":     {changes: map[string]any{"api_url": "https://example.com/?"}, err: "query"},
		"API fragment":        {changes: map[string]any{"api_url": "https://example.com/#"}, err: "fragment"},
		"API userinfo":        {changes: map[string]any{"api_url": "https://user:synthetic-private-value@example.com"}, err: "user information"},
		"relative API":        {changes: map[string]any{"api_url": "/proxy"}, err: "absolute HTTP(S)"},
		"official plaintext":  {changes: map[string]any{"api_url": "http://API.PUSHBULLET.COM./"}, err: "requires HTTPS"},
		"webhook URL":         {changes: map[string]any{"url": "https://example.com"}, err: "support access_token"},
		"bearer token":        {changes: map[string]any{"bearer_token": "synthetic-private-value"}, err: "support access_token"},
		"bot token":           {changes: map[string]any{"bot_token": "123:synthetic-private-value"}, err: "support access_token"},
		"app token":           {changes: map[string]any{"app_token": pushoverTestToken}, err: "support access_token"},
		"user key":            {changes: map[string]any{"user_key": pushoverTestUser}, err: "support access_token"},
		"chat":                {changes: map[string]any{"chat_id": "1"}, err: "support access_token"},
		"topic":               {changes: map[string]any{"message_thread_id": 0}, err: "support access_token"},
		"retries":             {changes: map[string]any{"retries_on_limit": 0}, err: "support access_token"},
	} {
		t.Run(name, func(t *testing.T) {
			fields := map[string]any{"type": base.Type, "access_token": base.AccessToken, "email": base.Email}
			for key, value := range test.changes {
				fields[key] = value
			}
			input, err := yaml.Marshal(map[string]any{"version": 1, "destinations": map[string]any{"push": fields}})
			require.NoError(t, err)
			got, err := readConfig(strings.NewReader(string(input)))
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				assert.NotContains(t, err.Error(), "synthetic-private-value")
				assert.NotContains(t, err.Error(), base.Email)
				assert.Equal(t, Config{}, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, Config{Version: 1, Destinations: map[string]Destination{"push": test.want}}, got)
		})
	}
}

func TestPushbulletFieldsRejectOtherProviders(t *testing.T) {
	for name, dst := range map[string]Destination{
		"webhook":  {Type: "webhook", URL: "https://example.com"},
		"slack":    {Type: "slack", URL: "https://example.com"},
		"discord":  {Type: "discord", URL: "https://example.com"},
		"telegram": {Type: "telegram", BotToken: "123:token", ChatID: "1"},
		"pushover": {Type: "pushover", AppToken: pushoverTestToken, UserKey: pushoverTestUser},
	} {
		t.Run(name, func(t *testing.T) {
			for _, field := range []string{"access_token", "email", "channel_tag", "source_device_id"} {
				t.Run(field, func(t *testing.T) {
					data, err := yaml.Marshal(dst)
					require.NoError(t, err)
					var fields map[string]any
					require.NoError(t, yaml.Unmarshal(data, &fields))
					fields[field] = "synthetic-private-value"
					data, err = yaml.Marshal(fields)
					require.NoError(t, err)
					var value Destination
					require.NoError(t, yaml.Unmarshal(data, &value))
					require.ErrorContains(t, value.validate(), "require type: pushbullet")
				})
			}
		})
	}
}
