// SPDX-License-Identifier: GPL-3.0-or-later

package msteams

import (
	"io"
	"net/http"
	"testing"

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
	want := testutil.Document[Config]{Version: 1, Destinations: map[string]Config{"test": dst}}
	testutil.CheckConfig(t, data, want, wantErr, readConfig)
}
