// SPDX-License-Identifier: GPL-3.0-or-later

package msteams

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMSTeamsConfig(t *testing.T) {
	for name, test := range map[string]struct{ field, value, err string }{
		"defaults": {}, "local": {"endpoint", "http://localhost:8080/prefix/", ""},
		"URL env": {"endpoint", "${env:UNREAD_CHAT_URL}", ""}, "URL file": {"endpoint", "${file:" + filepath.Join(t.TempDir(), "unread") + "}", ""},
		"empty": {"endpoint", "", "absolute HTTP(S)"}, "relative": {"endpoint", "/synthetic-private-value", "absolute HTTP(S)"},
		"fragment":      {"endpoint", "https://example.com/#fragment", "fragment"},
		"userinfo":      {"endpoint", "https://user:synthetic-private-value@example.com", "user information"},
		"relative file": {"endpoint", "${file:relative}", "absolute path"},
	} {
		t.Run("msteams/"+name, func(t *testing.T) {
			dst := testDestination()
			if test.field == "endpoint" {
				dst.URL = test.value
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
			dst := testDestination()
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
			assert.Equal(t, testutil.Document[Config]{}, got)
		})
	}
}
