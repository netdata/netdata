// SPDX-License-Identifier: GPL-3.0-or-later

package kavenegar

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"

	"github.com/stretchr/testify/require"
)

func TestFormProviderConfig(t *testing.T) {
	for provider, host := range map[string]string{"kavenegar": "api.kavenegar.com"} {
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
	for name, test := range map[string]struct{ key, err string }{"dot": {".", "dot path"}, "parent": {"..", "dot path"}, "escaped punctuation": {"synthetic/key?%&#", ""}} {
		t.Run("kavenegar/"+name, func(t *testing.T) {
			dst := formTestConfig("kavenegar")
			dst.APIKey = test.key
			checkFormConfig(t, dst, test.err)
		})
	}
	for field := range map[string]struct{}{"sender": {}, "recipient": {}} {
		for name, test := range map[string]struct{ value, err string }{
			"digits": {"15005550009", ""}, "plus": {"+15005550009", ""}, "leading zero": {"05005550009", ""},
			"missing": {"", "phone number"}, "whitespace": {" 123", "phone number"}, "reference": {"${env:NUMBER}", "phone number"},
			"list": {"123,456", "phone number"}, "unicode": {"１２３", "phone number"}, "controls": {"123\n", "phone number"}, "plus only": {"+", "phone number"},
		} {
			t.Run("kavenegar/"+field+"/"+name, func(t *testing.T) {
				dst := formTestConfig("kavenegar")
				if field == "sender" {
					dst.Sender = test.value
				} else {
					dst.Recipient = test.value
				}
				checkFormConfig(t, dst, test.err)
			})
		}
	}
}

func checkFormConfig(t *testing.T, dst Config, wantErr string) {
	t.Helper()
	want := testutil.Document[Config]{Version: 1, Destinations: map[string]Config{"target": dst}}
	data, err := marshalConfig(want)
	require.NoError(t, err)
	testutil.CheckConfig(t, data, want, wantErr, readConfig)
}
