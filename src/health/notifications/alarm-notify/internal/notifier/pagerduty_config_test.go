// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPagerDutyConfig(t *testing.T) {
	for version := range map[int64]struct{}{1: {}, 2: {}} {
		for name, test := range map[string]struct{ field, value, err string }{
			"defaults": {}, "custom API": {"api_url", "http://localhost:8080/proxy/", ""},
			"US HTTPS": {"api_url", "https://events.pagerduty.com", ""}, "EU HTTPS": {"api_url", "https://events.eu.pagerduty.com", ""},
			"US HTTP": {"api_url", "http://EVENTS.PAGERDUTY.COM.:80/", "HTTPS"}, "EU HTTP": {"api_url", "http://EVENTS.EU.PAGERDUTY.COM./", "HTTPS"},
			"API env": {"api_url", "${env:UNREAD_PD_API}", ""}, "API file": {"api_url", "${file:/unread/synthetic-api}", ""},
			"query": {"api_url", "https://example.com/?synthetic-private-value", "query"}, "empty query": {"api_url", "https://example.com/?", "query"},
			"fragment": {"api_url", "https://example.com/#", "fragment"}, "userinfo": {"api_url", "https://user:synthetic-private-value@example.com", "user information"},
			"relative": {"api_url", "/synthetic-private-value", "absolute HTTP(S)"},
			"key env":  {"integration_key", "${env:UNREAD_PD_KEY}", ""}, "key file": {"integration_key", "${file:/unread/synthetic-key}", ""},
			"empty key": {"integration_key", "", "nonempty"}, "short key": {"integration_key", strings.Repeat("a", 31), "32 characters"},
			"long key": {"integration_key", strings.Repeat("a", 33), "32 characters"}, "uppercase key": {"integration_key", strings.Repeat("A", 32), ""},
			"key space": {"integration_key", pagerDutyTestKey + " ", "without whitespace"}, "key control": {"integration_key", pagerDutyTestKey + "\n", "without whitespace"},
			"Unicode": {"integration_key", "synthetic-private-value界", "ASCII"}, "key list": {"integration_key", pagerDutyTestKey + "," + pagerDutyTestKey, "32 characters"},
			"interpolation": {"integration_key", "synthetic-private-value${env:KEY}", "secret reference"}, "relative file": {"integration_key", "${file:relative}", "absolute path"},
		} {
			t.Run(fmt.Sprintf("v%d/%s", version, name), func(t *testing.T) {
				dst := pagerDutyTestDestination(version)
				if test.field == "api_url" {
					dst.APIURL = test.value
				}
				if test.field == "integration_key" {
					dst.IntegrationKey = test.value
				}
				checkFormConfig(t, dst, test.err)
			})
		}
		t.Run(fmt.Sprintf("v%d/ruleset", version), func(t *testing.T) {
			dst := pagerDutyTestDestination(version)
			dst.IntegrationKey = "R" + strings.Repeat("z", 31)
			wantErr := ""
			if version == 1 {
				wantErr = "hexadecimal"
			}
			checkFormConfig(t, dst, wantErr)
		})
	}
	for name, test := range map[string]struct {
		value string
		want  *configInteger
		err   string
	}{
		"omitted": {"", nil, ""}, "one": {"1", new(configInteger(1)), ""}, "two": {"2", new(configInteger(2)), ""},
		"zero": {"0", nil, "api_version"}, "negative": {"-1", nil, "api_version"}, "unknown": {"3", nil, "api_version"},
		"fraction": {"1.5", nil, "invalid YAML"}, "quoted": {"'2'", nil, "invalid YAML"}, "overflow": {"9223372036854775808", nil, "invalid YAML"},
	} {
		t.Run(name, func(t *testing.T) {
			data := "version: 1\ndestinations:\n  target:\n    type: pagerduty\n    integration_key: " + pagerDutyTestKey + "\n"
			if test.value != "" {
				data += "    api_version: " + test.value + "\n"
			}
			got, err := readConfig(strings.NewReader(data))
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				assert.Equal(t, Config{}, got)
			} else {
				require.NoError(t, err)
				assert.Equal(t, Config{Version: 1, Destinations: map[string]Destination{"target": {Type: "pagerduty", IntegrationKey: pagerDutyTestKey, APIVersion: test.want}}}, got)
			}
		})
	}
}

func TestPagerDutyFieldIsolation(t *testing.T) {
	fields := reflect.TypeFor[Destination]()
	for i := 0; i < fields.NumField(); i++ {
		field := fields.Field(i)
		switch field.Name {
		case "Type", "IntegrationKey", "APIURL", "APIVersion":
			continue
		}
		t.Run(field.Name, func(t *testing.T) {
			dst := pagerDutyTestDestination(2)
			v := reflect.ValueOf(&dst).Elem().Field(i)
			switch v.Kind() {
			case reflect.Map:
				v.Set(reflect.ValueOf(map[string]string{"warning": "synthetic-private-value"}))
			case reflect.String:
				v.SetString("synthetic-private-value")
			case reflect.Slice:
				v.Set(reflect.ValueOf([]string{"123"}))
			default:
				n := configInteger(1)
				v.Set(reflect.ValueOf(&n))
			}
			checkFormConfig(t, dst, "fields for another provider")
		})
	}
	for provider := range map[string]struct{}{"webhook": {}, "slack": {}, "discord": {}, "telegram": {}, "pushover": {}, "pushbullet": {}, "twilio": {}, "messagebird": {}, "gotify": {}, "ntfy": {}, "rocketchat": {}, "flock": {}, "fleep": {}, "ilert": {}, "signl4": {}, "alerta": {}, "dynatrace": {}, "prowl": {}, "kavenegar": {}, "smseagle": {}, "opsgenie": {}} {
		t.Run(provider, func(t *testing.T) {
			require.ErrorContains(
				t,
				(Destination{Type: provider, APIVersion: new(configInteger(1))}).validate(),
				"api_version requires type: pagerduty",
			)
		})
	}
}
