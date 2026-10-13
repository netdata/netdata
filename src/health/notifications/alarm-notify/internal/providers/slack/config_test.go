// SPDX-License-Identifier: GPL-3.0-or-later

package slack

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestURLProviderConfiguration(t *testing.T) {
	for _, provider := range []string{"slack"} {
		t.Run(provider, func(t *testing.T) {
			testURLProviderConfiguration(t, provider)
		})
	}
}

func testURLProviderConfiguration(t *testing.T, provider string) {
	t.Helper()
	tests := map[string]struct {
		url   string
		extra string
		err   string
	}{
		"literal":               {url: "https://example.com/slack"},
		"environment reference": {url: "${env:NOTIFIER_SLACK_MISSING}"},
		"file reference":        {url: "${file:" + filepath.Join(t.TempDir(), "not-created") + "}"},
		"missing URL":           {err: "absolute HTTP(S)"},
		"bearer token refused": {
			url:   "https://example.com/slack",
			extra: "    bearer_token: synthetic-private-value\n",
			err:   "invalid YAML",
		},
		"legacy channel refused": {
			url:   "https://example.com/slack",
			extra: "    channel: synthetic-private-value\n",
			err:   "invalid YAML",
		},
		"legacy username refused": {
			url:   "https://example.com/slack",
			extra: "    username: synthetic-private-value\n",
			err:   "invalid YAML",
		},
		"legacy icon refused": {
			url:   "https://example.com/slack",
			extra: "    icon_url: https://example.com/icon.png\n",
			err:   "invalid YAML",
		},
		"internal legacy overrides refused": {
			url:   "https://example.com/slack",
			extra: "    legacy: {channel: '#synthetic-private-value'}\n",
			err:   "invalid YAML",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			config := strings.Replace(configForURL(test.url), "type: webhook", "type: "+provider, 1) + test.extra
			got, err := readConfig(strings.NewReader(config))
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				assert.NotContains(t, err.Error(), "synthetic-private-value")
				assert.Equal(t, testutil.Document[Config]{}, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(
				t,
				testutil.Document[Config]{Version: 1, Destinations: map[string]Config{"dev": {URL: test.url}}},
				got,
			)
		})
	}
}
