// SPDX-License-Identifier: GPL-3.0-or-later

package dynatrace

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
		"dynatrace": {APIURL: "https://example.com/e/test", APIToken: "synthetic-key", EntitySelector: `type(HOST),tag("netdata")`},
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

						dst.APIToken = test.value

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
		"Dynatrace no token":    {Config{APIURL: "https://example.com", EntitySelector: "type(HOST)"}, "api_token"},
		"Dynatrace no selector": {Config{APIURL: "https://example.com", APIToken: "synthetic-key"}, "entity_selector"},
	} {
		t.Run(name, func(t *testing.T) { checkMonitoringConfig(t, test.dst, test.err) })
	}
	for name, test := range map[string]struct{ selector, source, eventType, err string }{
		"entity ID":            {selector: `entityId("HOST-0123456789ABCDEF")`},
		"selector max":         {selector: strings.Repeat("界", 2000)},
		"selector too long":    {selector: strings.Repeat("界", 2001), err: "2000"},
		"selector whitespace":  {selector: " \n", err: "nonempty literal"},
		"selector reference":   {selector: "${env:SELECTOR}", err: "literal"},
		"source custom":        {source: "Netdata's \"source\" 😀"},
		"source max":           {source: strings.Repeat("界", 4096)},
		"source too long":      {source: strings.Repeat("界", 4097), err: "4096"},
		"source whitespace":    {source: " \n", err: "nonempty literal"},
		"source reference":     {source: "${env:SOURCE}", err: "literal"},
		"unknown event type":   {eventType: "synthetic-key", err: "event_type"},
		"lowercase event type": {eventType: "custom_info", err: "event_type"},
	} {
		t.Run(name, func(t *testing.T) {
			selector := test.selector
			if selector == "" {
				selector = "type(HOST)"
			}
			checkMonitoringConfig(
				t,
				Config{
					APIURL:         "https://example.com",
					APIToken:       "synthetic-key",
					EntitySelector: selector,
					Source:         test.source,
					EventType:      test.eventType,
				},
				test.err,
			)
		})
	}
	for eventType := range map[string]struct{}{
		"AVAILABILITY_EVENT": {}, "CUSTOM_ALERT": {}, "CUSTOM_ANNOTATION": {}, "CUSTOM_CONFIGURATION": {},
		"CUSTOM_DEPLOYMENT": {}, "CUSTOM_INFO": {}, "ERROR_EVENT": {}, "MARKED_FOR_TERMINATION": {},
		"PERFORMANCE_EVENT": {}, "RESOURCE_CONTENTION_EVENT": {}, "WARNING": {},
	} {
		t.Run(eventType, func(t *testing.T) {
			checkMonitoringConfig(
				t,
				Config{
					APIURL:         "https://example.com",
					APIToken:       "synthetic-key",
					EntitySelector: "type(HOST)",
					EventType:      eventType,
				},
				"",
			)
		})
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
