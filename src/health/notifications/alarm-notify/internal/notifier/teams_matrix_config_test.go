// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func teamsMatrixTestDestination(provider string) Destination {
	if provider == "msteams" {
		return Destination{Type: provider, URL: "https://example.com/teams?sig=synthetic-url-secret"}
	}
	return Destination{
		Type:        provider,
		APIURL:      "https://example.com/matrix/",
		AccessToken: "synthetic-token",
		RoomID:      "!room:example.org",
	}
}

func TestTeamsMatrixConfig(t *testing.T) {
	for provider := range map[string]struct{}{"msteams": {}, "matrix": {}} {
		for name, test := range map[string]struct{ field, value, err string }{
			"defaults": {}, "local": {"endpoint", "http://localhost:8080/prefix/", ""},
			"URL env": {"endpoint", "${env:UNREAD_CHAT_URL}", ""}, "URL file": {"endpoint", "${file:" + filepath.Join(t.TempDir(), "unread") + "}", ""},
			"empty": {"endpoint", "", ""}, "relative": {"endpoint", "/synthetic-private-value", "absolute HTTP(S)"},
			"fragment":      {"endpoint", "https://example.com/#fragment", "fragment"},
			"userinfo":      {"endpoint", "https://user:synthetic-private-value@example.com", "user information"},
			"relative file": {"endpoint", "${file:relative}", "absolute path"},
		} {
			t.Run(provider+"/"+name, func(t *testing.T) {
				dst := teamsMatrixTestDestination(provider)
				if test.field == "endpoint" {
					if provider == "matrix" {
						dst.APIURL = test.value
					} else {
						dst.URL = test.value
					}
				}
				want := test.err
				if name == "empty" {
					want = "absolute HTTP(S)"
					if provider == "matrix" {
						want = "required"
					}
				}
				checkFormConfig(t, dst, want)
			})
		}
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
			dst := teamsMatrixTestDestination("matrix")
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
	for name, test := range map[string]struct {
		icons, colors map[string]string
		err           string
	}{
		"custom":              {icons: map[string]string{"warning": "⚡", "critical": "Alarm", "clear": ""}, colors: map[string]string{"warning": "aBc123", "critical": "000000", "clear": ""}},
		"unknown icon status": {icons: map[string]string{"WARNING": "x"}, err: "keys"}, "unknown color status": {colors: map[string]string{"default": "123456"}, err: "keys"},
		"color hash": {colors: map[string]string{"warning": "#123456"}, err: "hexadecimal"}, "short color": {colors: map[string]string{"warning": "fff"}, err: "hexadecimal"},
		"bad color": {colors: map[string]string{"clear": "xxxxxx"}, err: "hexadecimal"}, "icon reference": {icons: map[string]string{"clear": "${env:ICON}"}, err: "literal"},
		"icon controls": {icons: map[string]string{"critical": "a\nb"}, err: "controls"},
	} {
		t.Run("msteams/"+name, func(t *testing.T) {
			dst := teamsMatrixTestDestination("msteams")
			dst.Icons, dst.Colors = test.icons, test.colors
			checkFormConfig(t, dst, test.err)
		})
	}
	for name, field := range map[string]string{"unknown nested key": "icons: {warning: x, bogus: x}", "non-map icons": "icons: x", "duplicate status": "colors: {warning: '123456', warning: 'abcdef'}"} {
		t.Run(name, func(t *testing.T) {
			got, err := readConfig(
				strings.NewReader(
					"version: 1\ndestinations:\n  target:\n    type: msteams\n    url: https://example.com\n    " + field + "\n",
				),
			)
			require.Error(t, err)
			assert.Equal(t, Config{}, got)
		})
	}
}

func TestTeamsMatrixFieldIsolation(t *testing.T) {
	for provider := range map[string]struct{}{"msteams": {}, "matrix": {}} {
		fields := reflect.TypeFor[Destination]()
		for i := 0; i < fields.NumField(); i++ {
			field := fields.Field(i)
			if field.Name == "Type" ||
				provider == "msteams" && (field.Name == "URL" || field.Name == "Icons" || field.Name == "Colors") ||
				provider == "matrix" &&
					(field.Name == "APIURL" || field.Name == "AccessToken" || field.Name == "RoomID") {
				continue
			}
			t.Run(provider+"/"+field.Name, func(t *testing.T) {
				dst := teamsMatrixTestDestination(provider)
				v := reflect.ValueOf(&dst).Elem().Field(i)
				switch v.Kind() {
				case reflect.String:
					v.SetString("synthetic-private-value")
				case reflect.Slice:
					v.Set(reflect.ValueOf([]string{"123"}))
				case reflect.Map:
					v.Set(reflect.ValueOf(map[string]string{"warning": "x"}))
				default:
					v.Set(reflect.New(v.Type().Elem()))
				}
				checkFormConfig(t, dst, "fields for another provider")
			})
		}
	}
	for provider := range map[string]struct{}{"webhook": {}, "slack": {}, "discord": {}, "telegram": {}, "pushover": {}, "pushbullet": {}, "twilio": {}, "messagebird": {}, "gotify": {}, "ntfy": {}, "rocketchat": {}, "flock": {}, "fleep": {}, "ilert": {}, "signl4": {}, "alerta": {}, "dynatrace": {}, "prowl": {}, "kavenegar": {}, "smseagle": {}, "pagerduty": {}, "opsgenie": {}} {
		for name, dst := range map[string]Destination{"room": {RoomID: "!room"}, "icons": {Icons: map[string]string{"warning": "x"}}, "empty icons": {Icons: map[string]string{}}, "colors": {Colors: map[string]string{"warning": "abcdef"}}, "empty colors": {Colors: map[string]string{}}} {
			t.Run(provider+"/"+name, func(t *testing.T) {
				dst.Type = provider
				require.ErrorContains(t, dst.validate(), "fields for another provider")
			})
		}
	}
}
