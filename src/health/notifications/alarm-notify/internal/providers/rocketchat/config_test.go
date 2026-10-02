// SPDX-License-Identifier: GPL-3.0-or-later

package rocketchat

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestChatWebhookConfiguration(t *testing.T) {
	file := "${file:" + filepath.Join(t.TempDir(), "unread") + "}"
	for name, test := range map[string]struct {
		dst Config
		err string
	}{
		"Rocket.Chat defaults":   {dst: Config{URL: "http://localhost:8080/hooks/test"}},
		"Rocket.Chat channel":    {dst: Config{URL: "https://example.com/hook?key=synthetic-token", Channel: "#alerts"}},
		"Rocket.Chat user":       {dst: Config{URL: "${env:UNREAD_URL}", Channel: "@user"}},
		"Rocket.Chat Unicode":    {dst: Config{URL: file, Channel: "#警告"}},
		"URL user info":          {dst: Config{URL: "https://user:synthetic-token@example.com"}, err: "user information"},
		"bad scheme":             {dst: Config{URL: "file:///tmp/hook"}, err: "absolute HTTP(S)"},
		"missing channel prefix": {dst: Config{URL: file, Channel: "alerts"}, err: "one #channel or @user"},
		"empty channel name":     {dst: Config{URL: file, Channel: "#"}, err: "one #channel or @user"},
		"multiple channels":      {dst: Config{URL: file, Channel: "#a,#b"}, err: "one #channel or @user"},
		"channel spaces":         {dst: Config{URL: file, Channel: "#a b"}, err: "one #channel or @user"},
		"channel controls":       {dst: Config{URL: file, Channel: "#a\x00"}, err: "one #channel or @user"},
		"channel reference":      {dst: Config{URL: file, Channel: "${env:CHANNEL}"}, err: "one #channel or @user"},
	} {
		t.Run(name, func(t *testing.T) {
			want := testutil.Document[Config]{Version: 1, Destinations: map[string]Config{"chat": test.dst}}
			data, err := yaml.Marshal(map[string]any{"version": 1, "destinations": map[string]any{"chat": configFields(test.dst)}})
			require.NoError(t, err)
			got, err := readConfig(strings.NewReader(string(data)))
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				assert.NotContains(t, err.Error(), "synthetic-token")
				assert.Equal(t, testutil.Document[Config]{}, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}
