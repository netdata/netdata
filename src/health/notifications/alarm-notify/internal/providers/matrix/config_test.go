// SPDX-License-Identifier: GPL-3.0-or-later

package matrix

import (
	"path/filepath"
	"testing"
)

func TestMatrixConfig(t *testing.T) {
	for name, test := range map[string]struct{ field, value, err string }{
		"defaults": {}, "local": {"endpoint", "http://localhost:8080/prefix/", ""},
		"URL env": {"endpoint", "${env:UNREAD_CHAT_URL}", ""}, "URL file": {"endpoint", "${file:" + filepath.Join(t.TempDir(), "unread") + "}", ""},
		"empty": {"endpoint", "", "required"}, "relative": {"endpoint", "/synthetic-private-value", "absolute HTTP(S)"},
		"fragment":      {"endpoint", "https://example.com/#fragment", "fragment"},
		"userinfo":      {"endpoint", "https://user:synthetic-private-value@example.com", "user information"},
		"relative file": {"endpoint", "${file:relative}", "absolute path"},
	} {
		t.Run("matrix/"+name, func(t *testing.T) {
			dst := testDestination()
			if test.field == "endpoint" {
				dst.APIURL = test.value
			}
			checkFormConfig(t, dst, test.err)
		})
	}
	for name, test := range map[string]struct{ field, value, err string }{
		"opaque room": {"room_id", "!opaque/room+hash", ""}, "room metacharacters": {"room_id", "!room/?#%:example.org", ""},
		"empty room": {"room_id", "", "room_id"}, "sigil only": {"room_id", "!", "room_id"}, "alias": {"room_id", "#room:example.org", "room_id"},
		"room whitespace": {"room_id", "!room\t", "room_id"}, "room control": {"room_id", "!room\x00", "room_id"},
		"room reference": {"room_id", "!${env:ROOM}", "literal"}, "blank token": {"access_token", "", "nonempty"},
		"token whitespace": {"access_token", "synthetic-private-value ", "without whitespace"}, "token Unicode": {"access_token", "κλειδί", "ASCII"},
		"token env": {"access_token", "${env:UNREAD_MATRIX_TOKEN}", ""}, "token file": {"access_token", "${file:" + filepath.Join(t.TempDir(), "unread") + "}", ""},
		"empty query": {"api_url", "https://example.com/?", "query"}, "query": {"api_url", "https://example.com/?access_token=synthetic-private-value", "query"},
		"empty fragment": {"api_url", "https://example.com/#", "fragment"},
		"official HTTP":  {"api_url", "http://MATRIX.ORG.:8448", "HTTPS"}, "client HTTP": {"api_url", "http://MATRIX-CLIENT.MATRIX.ORG.", "HTTPS"},
		"official HTTPS": {"api_url", "https://matrix.org", ""},
	} {
		t.Run("matrix/"+name, func(t *testing.T) {
			dst := testDestination()
			switch test.field {
			case "room_id":
				dst.RoomID = test.value
			case "access_token":
				dst.AccessToken = test.value
			case "api_url":
				dst.APIURL = test.value
			}
			checkFormConfig(t, dst, test.err)
		})
	}
}
