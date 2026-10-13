// SPDX-License-Identifier: GPL-3.0-or-later

package flock

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
		"Flock":             {dst: Config{URL: "https://example.com/hook?key=synthetic-token"}},
		"Flock file":        {dst: Config{URL: file}},
		"missing URL":       {dst: Config{}, err: "absolute HTTP(S)"},
		"URL fragment":      {dst: Config{URL: "https://example.com/#private"}, err: "fragment"},
		"URL interpolation": {dst: Config{URL: "https://example.com/${env:SECRET}"}, err: "secret reference"},
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
