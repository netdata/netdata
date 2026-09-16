// SPDX-License-Identifier: GPL-3.0-or-later

package msteams

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
)

func readConfig(r io.Reader) (testutil.Document[Config], error) {
	return testutil.ReadConfig(r, "msteams", func(cfg Config) error { _, err := New(cfg, http.DefaultClient); return err })
}

func testDestination() Config {
	return Config{URL: "https://example.com/teams?sig=synthetic-url-secret"}
}

func configFields(dst Config) map[string]any {
	data, _ := yaml.Marshal(dst)
	fields := map[string]any{}
	_ = yaml.Unmarshal(data, &fields)
	fields["type"] = "msteams"
	return fields
}
func checkFormConfig(t *testing.T, dst Config, wantErr string) {
	t.Helper()
	data, err := yaml.Marshal(map[string]any{"version": 1, "destinations": map[string]any{"test": configFields(dst)}})
	require.NoError(t, err)
	got, err := readConfig(strings.NewReader(string(data)))
	if wantErr != "" {
		require.ErrorContains(t, err, wantErr)
		assert.NotContains(t, err.Error(), "synthetic-private-value")
		assert.Equal(t, testutil.Document[Config]{}, got)
	} else {
		require.NoError(t, err)
		assert.Equal(t, dst, got.Destinations["test"])
	}
}
