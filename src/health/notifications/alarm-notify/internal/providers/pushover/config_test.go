// SPDX-License-Identifier: GPL-3.0-or-later

package pushover

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestPushoverConfiguration(t *testing.T) {
	base := Config{AppToken: pushoverTestToken, UserKey: pushoverTestUser}
	custom := base
	custom.APIURL = "http://localhost:8081/proxy/"
	file := "${file:" + filepath.Join(t.TempDir(), "unread") + "}"
	for name, test := range map[string]struct {
		changes map[string]any
		want    Config
		err     string
	}{
		"minimal":    {want: base},
		"custom API": {changes: map[string]any{"api_url": custom.APIURL}, want: custom},
		"unresolved secrets": {changes: map[string]any{"app_token": file, "user_key": "${env:NOTIFIER_UNUSED_USER}", "api_url": "${env:NOTIFIER_UNUSED_API}"},
			want: Config{AppToken: file, UserKey: "${env:NOTIFIER_UNUSED_USER}", APIURL: "${env:NOTIFIER_UNUSED_API}"}},
		"missing app":          {changes: map[string]any{"app_token": nil}, err: "app_token"},
		"missing user":         {changes: map[string]any{"user_key": nil}, err: "user_key"},
		"short app":            {changes: map[string]any{"app_token": strings.Repeat("a", 29)}, err: "app_token"},
		"long user":            {changes: map[string]any{"user_key": strings.Repeat("u", 31)}, err: "user_key"},
		"app punctuation":      {changes: map[string]any{"app_token": strings.Repeat("a", 29) + "-"}, err: "app_token"},
		"user unicode":         {changes: map[string]any{"user_key": strings.Repeat("界", 30)}, err: "user_key"},
		"multiple users":       {changes: map[string]any{"user_key": pushoverTestUser + "," + pushoverTestUser}, err: "user_key"},
		"interpolation":        {changes: map[string]any{"app_token": "prefix${env:TOKEN}"}, err: "secret reference"},
		"relative secret file": {changes: map[string]any{"user_key": "${file:relative}"}, err: "absolute"},
		"relative API":         {changes: map[string]any{"api_url": "/proxy"}, err: "absolute HTTP(S)"},
		"API query":            {changes: map[string]any{"api_url": "https://example.com/?token=synthetic-private-value"}, err: "query"},
		"empty API query":      {changes: map[string]any{"api_url": "https://example.com/?"}, err: "query"},
		"empty API fragment":   {changes: map[string]any{"api_url": "https://example.com/#"}, err: "fragment"},
		"API credentials":      {changes: map[string]any{"api_url": "https://user:synthetic-private-value@example.com"}, err: "user information"},
		"API fragment":         {changes: map[string]any{"api_url": "https://example.com/#synthetic-private-value"}, err: "fragment"},
		"official plaintext":   {changes: map[string]any{"api_url": "http://API.PUSHOVER.NET./"}, err: "requires HTTPS"},
		"webhook URL":          {changes: map[string]any{"url": "https://example.com"}, err: "invalid YAML"},
		"bearer token":         {changes: map[string]any{"bearer_token": "synthetic-private-value"}, err: "invalid YAML"},
		"bot token":            {changes: map[string]any{"bot_token": "123:synthetic-private-value"}, err: "invalid YAML"},
		"chat":                 {changes: map[string]any{"chat_id": "1"}, err: "invalid YAML"},
		"topic":                {changes: map[string]any{"message_thread_id": 0}, err: "invalid YAML"},
		"retries":              {changes: map[string]any{"retries_on_limit": 0}, err: "invalid YAML"},
	} {
		t.Run(name, func(t *testing.T) {
			fields := map[string]any{"type": "pushover", "app_token": pushoverTestToken, "user_key": pushoverTestUser}
			for key, value := range test.changes {
				fields[key] = value
			}
			input, err := yaml.Marshal(map[string]any{"version": 1, "destinations": map[string]any{"push": fields}})
			require.NoError(t, err)
			got, err := readConfig(strings.NewReader(string(input)))
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				assert.NotContains(t, err.Error(), "synthetic-private-value")
				assert.NotContains(t, err.Error(), pushoverTestToken)
				assert.NotContains(t, err.Error(), pushoverTestUser)
				assert.Equal(t, testutil.Document[Config]{}, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, testutil.Document[Config]{Version: 1, Destinations: map[string]Config{"push": test.want}}, got)
		})
	}
}
