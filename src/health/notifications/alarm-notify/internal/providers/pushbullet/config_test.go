// SPDX-License-Identifier: GPL-3.0-or-later

package pushbullet

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestPushbulletConfiguration(t *testing.T) {
	base := Config{AccessToken: "synthetic-private-value", Email: "ops@example.com"}
	full := base
	full.APIURL, full.SourceDeviceID = "http://localhost:8081/proxy/", "test-source-device"
	file := "${file:" + filepath.Join(t.TempDir(), "unread") + "}"
	for name, test := range map[string]struct {
		changes map[string]any
		want    Config
		err     string
	}{
		"email":               {want: base},
		"full":                {changes: map[string]any{"api_url": full.APIURL, "source_device_id": full.SourceDeviceID}, want: full},
		"channel":             {changes: map[string]any{"email": nil, "channel_tag": "test-alerts"}, want: Config{AccessToken: base.AccessToken, ChannelTag: "test-alerts"}},
		"unresolved secrets":  {changes: map[string]any{"access_token": file, "api_url": "${env:NOTIFIER_UNUSED_API}"}, want: Config{AccessToken: file, Email: base.Email, APIURL: "${env:NOTIFIER_UNUSED_API}"}},
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
		"webhook URL":         {changes: map[string]any{"url": "https://example.com"}, err: "invalid YAML"},
		"bearer token":        {changes: map[string]any{"bearer_token": "synthetic-private-value"}, err: "invalid YAML"},
		"bot token":           {changes: map[string]any{"bot_token": "123:synthetic-private-value"}, err: "invalid YAML"},
		"app token":           {changes: map[string]any{"app_token": strings.Repeat("a", 30)}, err: "invalid YAML"},
		"user key":            {changes: map[string]any{"user_key": strings.Repeat("u", 30)}, err: "invalid YAML"},
		"chat":                {changes: map[string]any{"chat_id": "1"}, err: "invalid YAML"},
		"topic":               {changes: map[string]any{"message_thread_id": 0}, err: "invalid YAML"},
		"retries":             {changes: map[string]any{"retries_on_limit": 0}, err: "invalid YAML"},
	} {
		t.Run(name, func(t *testing.T) {
			fields := map[string]any{"type": "pushbullet", "access_token": base.AccessToken, "email": base.Email}
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
				assert.Equal(t, testutil.Document[Config]{}, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, testutil.Document[Config]{Version: 1, Destinations: map[string]Config{"push": test.want}}, got)
		})
	}
}
