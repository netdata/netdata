// SPDX-License-Identifier: GPL-3.0-or-later

package alerta

import (
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMonitoringConfiguration(t *testing.T) {
	for provider, base := range map[string]Config{
		"alerta": {APIURL: "https://example.com/api/", Environment: "Production"},
	} {
		t.Run(provider, func(t *testing.T) {
			for name, test := range map[string]struct{ field, value, err string }{
				"defaults":          {},
				"local":             {"api_url", "http://localhost:8080/proxy/", ""},
				"env URL":           {"api_url", "${env:UNREAD_MONITORING_URL}", ""},
				"file URL":          {"api_url", "${file:" + filepath.Join(t.TempDir(), "unread") + "}", ""},
				"missing URL":       {"api_url", "", "api_url is required"},
				"relative URL":      {"api_url", "/synthetic-key", "absolute HTTP(S)"},
				"query":             {"api_url", "https://example.com?synthetic-key", "query"},
				"empty query":       {"api_url", "https://example.com?", "query"},
				"fragment":          {"api_url", "https://example.com#synthetic-key", "fragment"},
				"empty fragment":    {"api_url", "https://example.com#", "fragment"},
				"user info":         {"api_url", "https://user:synthetic-key@example.com", "user information"},
				"interpolation":     {"api_url", "https://example.com/${env:KEY}", "secret reference"},
				"relative file":     {"api_url", "${file:relative}", "absolute path"},
				"literal key":       {"key", "synthetic-key", ""},
				"env key":           {"key", "${env:UNREAD_MONITORING_KEY}", ""},
				"file key":          {"key", "${file:" + filepath.Join(t.TempDir(), "unread") + "}", ""},
				"key whitespace":    {"key", "synthetic-key ", "without whitespace"},
				"key controls":      {"key", "synthetic-key\n", "without whitespace"},
				"key Unicode":       {"key", "synthetic-key界", "printable ASCII"},
				"key interpolation": {"key", "synthetic-key${env:KEY}", "secret reference"},
			} {
				t.Run(name, func(t *testing.T) {
					dst := base
					if test.field == "api_url" {
						dst.APIURL = test.value
					}
					if test.field == "key" {

						dst.APIKey = test.value

					}
					checkMonitoringConfig(t, dst, test.err)
				})
			}
		})
	}
	for name, test := range map[string]struct {
		dst Config
		err string
	}{
		"Alerta custom environment":    {dst: Config{APIURL: "https://example.com", Environment: "Env '東京'"}},
		"Alerta no environment":        {Config{APIURL: "https://example.com"}, "environment"},
		"Alerta blank environment":     {Config{APIURL: "https://example.com", Environment: " \n"}, "environment"},
		"Alerta reference environment": {Config{APIURL: "https://example.com", Environment: "${env:ENVIRONMENT}"}, "literal"},
	} {
		t.Run(name, func(t *testing.T) { checkMonitoringConfig(t, test.dst, test.err) })
	}

}

func checkMonitoringConfig(t *testing.T, dst Config, wantError string) {
	t.Helper()
	want := testutil.Document[Config]{Version: 1, Destinations: map[string]Config{"monitoring": dst}}
	data, err := marshalConfig(want)
	require.NoError(t, err)
	got, err := readConfig(strings.NewReader(string(data)))
	if wantError != "" {
		require.ErrorContains(t, err, wantError)
		assert.NotContains(t, err.Error(), "synthetic-key")
		assert.Equal(t, testutil.Document[Config]{}, got)
		return
	}
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestAlertaPublicAPIHTTPS(t *testing.T) {
	for host := range map[string]struct{}{
		"api.alerta.io": {}, "api.alerta.dev": {}, "alerta-api.fly.dev": {},
	} {
		for name, test := range map[string]struct{ endpoint, err string }{
			"HTTP":             {"http://" + host, "requires HTTPS"},
			"normalized HTTP":  {"http://" + strings.ToUpper(host) + ".:80/api/", "requires HTTPS"},
			"HTTPS":            {"https://" + host + "/api/", ""},
			"normalized HTTPS": {"https://" + strings.ToUpper(host) + ".:443/api/", ""},
			"custom suffix":    {"http://" + host + ".example.com/api/", ""},
		} {
			t.Run(host+"/"+name, func(t *testing.T) {
				checkMonitoringConfig(
					t,
					Config{
						APIURL:      test.endpoint,
						APIKey:      "synthetic-key",
						Environment: "Production",
					},
					test.err,
				)
			})
		}
	}
}
