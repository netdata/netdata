// SPDX-License-Identifier: GPL-3.0-or-later

package signl4

import (
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIncidentConfiguration(t *testing.T) {
	file := "${file:" + filepath.Join(t.TempDir(), "unread") + "}"
	for name, test := range map[string]struct {
		dst Config
		err string
	}{
		"SIGNL4 URL":           {dst: Config{URL: "https://example.com/hook?keep=a%26b"}},
		"SIGNL4 local":         {dst: Config{URL: "http://localhost:8080/hook"}},
		"SIGNL4 environment":   {dst: Config{URL: "${env:UNREAD_URL}"}},
		"SIGNL4 file":          {dst: Config{URL: file}},
		"SIGNL4 missing URL":   {dst: Config{}, err: "absolute HTTP(S)"},
		"SIGNL4 relative URL":  {dst: Config{URL: "/synthetic-key"}, err: "absolute HTTP(S)"},
		"SIGNL4 user info":     {dst: Config{URL: "https://user:synthetic-key@example.com"}, err: "user information"},
		"SIGNL4 fragment":      {dst: Config{URL: "https://example.com/#synthetic-key"}, err: "fragment"},
		"SIGNL4 interpolation": {dst: Config{URL: "https://example.com/${env:KEY}"}, err: "secret reference"},
	} {
		t.Run(name, func(t *testing.T) {
			want := testutil.Document[Config]{Version: 1, Destinations: map[string]Config{"incident": test.dst}}
			data, err := marshalConfig(want)
			require.NoError(t, err)
			got, err := readConfig(strings.NewReader(string(data)))
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				assert.NotContains(t, err.Error(), "synthetic-key")
				assert.Equal(t, testutil.Document[Config]{}, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}
