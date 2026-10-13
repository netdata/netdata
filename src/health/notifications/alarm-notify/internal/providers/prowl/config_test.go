// SPDX-License-Identifier: GPL-3.0-or-later

package prowl

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"

	"github.com/stretchr/testify/require"
)

func TestFormProviderConfig(t *testing.T) {
	for provider, host := range map[string]string{"prowl": "api.prowlapp.com"} {
		for name, test := range map[string]struct{ field, value, err string }{
			"defaults": {}, "local API": {"api_url", "http://localhost:8080/proxy/", ""},
			"official HTTPS": {"api_url", "https://" + host, ""}, "official HTTP": {"api_url", "http://" + strings.ToUpper(host) + ".:80/", "HTTPS"},
			"API env": {"api_url", "${env:UNREAD_FORM_URL}", ""}, "API file": {"api_url", "${file:" + filepath.Join(t.TempDir(), "unread") + "}", ""},
			"query": {"api_url", "https://example.com/?synthetic-private-value", "query"}, "empty query": {"api_url", "https://example.com/?", "query"},
			"fragment": {"api_url", "https://example.com/#synthetic-private-value", "fragment"}, "empty fragment": {"api_url", "https://example.com/#", "fragment"},
			"userinfo": {"api_url", "https://user:synthetic-private-value@example.com", "user information"}, "relative API": {"api_url", "/synthetic-private-value", "absolute HTTP(S)"},
			"key env": {"api_key", "${env:UNREAD_FORM_KEY}", ""}, "key file": {"api_key", "${file:" + filepath.Join(t.TempDir(), "unread") + "}", ""},
			"empty key": {"api_key", "", "nonempty"}, "key whitespace": {"api_key", "synthetic-private-value ", "without whitespace"},
			"key Unicode": {"api_key", "synthetic-private-value界", "ASCII"}, "key controls": {"api_key", "synthetic-private-value\n", "without whitespace"},
			"key interpolation": {"api_key", "synthetic-private-value${env:KEY}", "secret reference"}, "relative file": {"api_key", "${file:relative}", "absolute path"},
		} {
			t.Run(provider+"/"+name, func(t *testing.T) {
				dst := formTestConfig(provider)
				if test.field == "api_url" {
					dst.APIURL = test.value
				}
				if test.field == "api_key" {
					dst.APIKey = test.value
				}
				checkFormConfig(t, dst, test.err)
			})
		}
	}
	for name, test := range map[string]struct{ key, err string }{
		"single": {prowlTestKey, ""}, "batch": {prowlTestKeys, ""}, "short": {strings.Repeat("a", 39), "40-character"},
		"long": {strings.Repeat("a", 41), "40-character"}, "nonhex": {strings.Repeat("z", 40), "hexadecimal"},
		"empty member": {prowlTestKey + ",", "40-character"}, "invalid member": {prowlTestKey + ",bad", "40-character"},
		"spaced list": {prowlTestKey + ", " + prowlTestKey, "without whitespace"},
	} {
		t.Run(
			"prowl/"+name,
			func(t *testing.T) { checkFormConfig(t, Config{APIKey: test.key}, test.err) },
		)
	}

}

func checkFormConfig(t *testing.T, dst Config, wantErr string) {
	t.Helper()
	want := testutil.Document[Config]{Version: 1, Destinations: map[string]Config{"target": dst}}
	data, err := marshalConfig(want)
	require.NoError(t, err)
	testutil.CheckConfig(t, data, want, wantErr, readConfig)
}
