// SPDX-License-Identifier: GPL-3.0-or-later

package ilert

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
		"ilert defaults":          {dst: Config{IntegrationKey: "synthetic-key"}},
		"ilert official":          {dst: Config{IntegrationKey: "synthetic-key", APIURL: "https://api.ilert.com/api"}},
		"ilert proxy prefix":      {dst: Config{IntegrationKey: "synthetic-key", APIURL: "http://localhost:8080/proxy/api/"}},
		"ilert environment":       {dst: Config{IntegrationKey: "${env:UNREAD_KEY}", APIURL: "${env:UNREAD_API}"}},
		"ilert files":             {dst: Config{IntegrationKey: file, APIURL: file}},
		"ilert missing key":       {dst: Config{}, err: "integration_key must be nonempty"},
		"ilert key spaces":        {dst: Config{IntegrationKey: "synthetic-key "}, err: "without whitespace"},
		"ilert key controls":      {dst: Config{IntegrationKey: "synthetic-key\n"}, err: "without whitespace"},
		"ilert key Unicode":       {dst: Config{IntegrationKey: "synthetic-key界"}, err: "printable ASCII"},
		"ilert key interpolation": {dst: Config{IntegrationKey: "synthetic-key${env:KEY}"}, err: "secret reference"},
		"ilert relative file":     {dst: Config{IntegrationKey: "${file:relative}"}, err: "absolute path"},
		"ilert insecure official": {dst: Config{IntegrationKey: "synthetic-key", APIURL: "http://API.ILERT.COM.:80/api"}, err: "requires HTTPS"},
		"ilert query":             {dst: Config{IntegrationKey: "synthetic-key", APIURL: "https://example.com/api?key=synthetic-key"}, err: "query"},
		"ilert empty query":       {dst: Config{IntegrationKey: "synthetic-key", APIURL: "https://example.com/api?"}, err: "query"},
		"ilert empty fragment":    {dst: Config{IntegrationKey: "synthetic-key", APIURL: "https://example.com/api#"}, err: "fragment"},
		"ilert fragment":          {dst: Config{IntegrationKey: "synthetic-key", APIURL: "https://example.com/api#synthetic-key"}, err: "fragment"},
		"ilert user info":         {dst: Config{IntegrationKey: "synthetic-key", APIURL: "https://user:synthetic-key@example.com/api"}, err: "user information"},
		"ilert relative API":      {dst: Config{IntegrationKey: "synthetic-key", APIURL: "/api"}, err: "absolute HTTP(S)"},
		"ilert API interpolation": {dst: Config{IntegrationKey: "synthetic-key", APIURL: "https://example.com/${env:API}"}, err: "secret reference"},
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
