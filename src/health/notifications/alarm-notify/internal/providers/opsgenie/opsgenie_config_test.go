// SPDX-License-Identifier: GPL-3.0-or-later

package opsgenie

import (
	"path/filepath"
	"testing"
)

func TestOpsgenieConfig(t *testing.T) {
	for name, test := range map[string]struct{ field, value, err string }{
		"defaults": {}, "custom API": {"api_url", "http://localhost:8080/proxy/", ""},
		"US HTTPS": {"api_url", "https://api.opsgenie.com", ""}, "EU HTTPS": {"api_url", "https://api.eu.opsgenie.com", ""},
		"US HTTP": {"api_url", "http://API.OPSGENIE.COM.:80/", "HTTPS"}, "EU HTTP": {"api_url", "http://API.EU.OPSGENIE.COM./", "HTTPS"},
		"API env": {"api_url", "${env:UNREAD_OPSGENIE_URL}", ""}, "API file": {"api_url", "${file:" + filepath.Join(t.TempDir(), "unread") + "}", ""},
		"query": {"api_url", "https://example.com/?synthetic-private-value", "query"}, "empty query": {"api_url", "https://example.com/?", "query"},
		"fragment": {"api_url", "https://example.com/#", "fragment"}, "userinfo": {"api_url", "https://user:synthetic-private-value@example.com", "user information"},
		"relative": {"api_url", "/synthetic-private-value", "absolute HTTP(S)"},
		"key env":  {"api_key", "${env:UNREAD_OPSGENIE_KEY}", ""}, "key file": {"api_key", "${file:" + filepath.Join(t.TempDir(), "unread-key") + "}", ""},
		"empty key": {"api_key", "", "nonempty"}, "key whitespace": {"api_key", "synthetic-private-value ", "without whitespace"},
		"key Unicode": {"api_key", "synthetic-private-value界", "ASCII"}, "key controls": {"api_key", "synthetic-private-value\n", "without whitespace"},
		"key interpolation": {"api_key", "synthetic-private-value${env:KEY}", "secret reference"}, "relative file": {"api_key", "${file:relative}", "absolute path"},
	} {
		t.Run(name, func(t *testing.T) {
			dst := opsgenieTestConfig()
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
