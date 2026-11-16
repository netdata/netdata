// SPDX-License-Identifier: GPL-3.0-or-later

package ntfy

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestPushConfiguration(t *testing.T) {
	ntfy := Config{URL: "https://example.com/topic"}
	file := "${file:" + filepath.Join(t.TempDir(), "unread") + "}"
	for name, test := range map[string]struct {
		base    Config
		changes map[string]any
		want    Config
		err     string
	}{
		"anonymous ntfy":          {base: ntfy, want: ntfy},
		"ntfy Basic":              {base: ntfy, changes: map[string]any{"username": "user", "password": "a password:界"}, want: Config{URL: ntfy.URL, Username: "user", Password: "a password:界"}},
		"ntfy token":              {base: ntfy, changes: map[string]any{"access_token": "synthetic-token"}, want: Config{URL: ntfy.URL, AccessToken: "synthetic-token"}},
		"ntfy references":         {base: ntfy, changes: map[string]any{"url": file, "username": "${env:UNUSED_USER}", "password": file}, want: Config{URL: file, Username: "${env:UNUSED_USER}", Password: file}},
		"ntfy URL query":          {base: ntfy, changes: map[string]any{"url": ntfy.URL + "?cache=no"}, want: Config{URL: ntfy.URL + "?cache=no"}},
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
			fields["type"] = "ntfy"
			for k, v := range test.changes {
				fields[k] = v
			}
			data, err = yaml.Marshal(map[string]any{"version": 1, "destinations": map[string]any{"push": fields}})
			require.NoError(t, err)
			got, err := readConfig(strings.NewReader(string(data)))
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				assert.NotContains(t, err.Error(), "synthetic-token")
				assert.Equal(t, testutil.Document[Config]{}, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, testutil.Document[Config]{Version: 1, Destinations: map[string]Config{"push": test.want}}, got)
		})
	}
}
