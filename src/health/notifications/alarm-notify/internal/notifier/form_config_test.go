// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestFormProviderConfig(t *testing.T) {
	for provider, host := range map[string]string{"prowl": "api.prowlapp.com", "kavenegar": "api.kavenegar.com"} {
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
				dst := formTestDestination(provider)
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
			func(t *testing.T) { checkFormConfig(t, Destination{Type: "prowl", APIKey: test.key}, test.err) },
		)
	}
	for name, test := range map[string]struct{ key, err string }{"dot": {".", "dot path"}, "parent": {"..", "dot path"}, "escaped punctuation": {"synthetic/key?%&#", ""}} {
		t.Run("kavenegar/"+name, func(t *testing.T) {
			dst := formTestDestination("kavenegar")
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
				dst := formTestDestination("kavenegar")
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

func checkFormConfig(t *testing.T, dst Destination, wantErr string) {
	t.Helper()
	want := Config{Version: 1, Destinations: map[string]Destination{"target": dst}}
	data, err := yaml.Marshal(want)
	require.NoError(t, err)
	got, err := readConfig(strings.NewReader(string(data)))
	if wantErr != "" {
		require.ErrorContains(t, err, wantErr)
		assert.NotContains(t, err.Error(), "synthetic-private-value")
		assert.Equal(t, Config{}, got)
	} else {
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}
}

func TestFormProviderFieldIsolation(t *testing.T) {
	// Cover every current destination field, including fields added by future providers.
	for provider := range map[string]struct{}{"prowl": {}, "kavenegar": {}} {
		base := formTestDestination(provider)
		base.APIURL = "https://example.com"
		fields := reflect.TypeFor[Destination]()
		for i := 0; i < fields.NumField(); i++ {
			field := fields.Field(i)
			if field.Name == "Type" || field.Name == "APIKey" || field.Name == "APIURL" ||
				provider == "kavenegar" && (field.Name == "Sender" || field.Name == "Recipient") {
				continue
			}
			t.Run(provider+"/"+field.Name, func(t *testing.T) {
				dst := base
				v := reflect.ValueOf(&dst).Elem().Field(i)
				if v.Kind() == reflect.Map {
					v.Set(reflect.ValueOf(map[string]string{"warning": "synthetic-private-value"}))
				} else if v.Kind() == reflect.Slice {
					v.Set(reflect.ValueOf([]string{"15005550009"}))
				} else if v.Kind() == reflect.String {
					v.SetString("synthetic-private-value")
				} else {
					n := configInteger(1)
					v.Set(reflect.ValueOf(&n))
				}
				wantErr := "fields for another provider"
				if field.Name == "Recipients" || field.Name == "MessageType" || field.Name == "CallDuration" ||
					field.Name == "VoiceID" {
					wantErr = "require type: smseagle"
				}
				if field.Name == "APIVersion" {
					wantErr = "api_version requires type: pagerduty"
				}
				checkFormConfig(t, dst, wantErr)
			})
		}
	}
}
