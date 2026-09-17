// SPDX-License-Identifier: GPL-3.0-or-later

package gotify

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
	gotify := Config{APIURL: "http://localhost:8081/prefix/", AppToken: "synthetic-token"}
	file := "${file:" + filepath.Join(t.TempDir(), "unread") + "}"
	for name, test := range map[string]struct {
		base    Config
		changes map[string]any
		want    Config
		err     string
	}{
		"gotify":                 {base: gotify, want: gotify},
		"gotify references":      {base: gotify, changes: map[string]any{"app_token": file, "api_url": "${env:UNUSED_API}"}, want: Config{APIURL: "${env:UNUSED_API}", AppToken: file}},
		"gotify missing API":     {base: gotify, changes: map[string]any{"api_url": nil}, err: "api_url is required"},
		"gotify missing token":   {base: gotify, changes: map[string]any{"app_token": nil}, err: "app_token must be"},
		"gotify API query":       {base: gotify, changes: map[string]any{"api_url": "https://example.com/?"}, err: "query"},
		"gotify API fragment":    {base: gotify, changes: map[string]any{"api_url": "https://example.com/#"}, err: "fragment"},
		"gotify API credentials": {base: gotify, changes: map[string]any{"api_url": "https://user:synthetic-token@example.com"}, err: "user information"},
		"gotify token controls":  {base: gotify, changes: map[string]any{"app_token": "synthetic-token\n"}, err: "app_token must be"},
		"gotify token unicode":   {base: gotify, changes: map[string]any{"app_token": "界"}, err: "app_token must be"},
		"gotify interpolation":   {base: gotify, changes: map[string]any{"app_token": "prefix${env:KEY}"}, err: "secret reference"},
	} {
		t.Run(name, func(t *testing.T) {
			data, err := yaml.Marshal(test.base)
			require.NoError(t, err)
			fields := map[string]any{}
			require.NoError(t, yaml.Unmarshal(data, &fields))
			fields["type"] = "gotify"
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
