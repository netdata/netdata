// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestMonitoringConfiguration(t *testing.T) {
	for provider, base := range map[string]Destination{
		"alerta":    {Type: "alerta", APIURL: "https://example.com/api/", Environment: "Production"},
		"dynatrace": {Type: "dynatrace", APIURL: "https://example.com/e/test", APIToken: "synthetic-key", EntitySelector: `type(HOST),tag("netdata")`},
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
						if provider == "alerta" {
							dst.APIKey = test.value
						} else {
							dst.APIToken = test.value
						}
					}
					checkMonitoringConfig(t, dst, test.err)
				})
			}
		})
	}
	for name, test := range map[string]struct {
		dst Destination
		err string
	}{
		"Alerta custom environment":    {dst: Destination{Type: "alerta", APIURL: "https://example.com", Environment: "Env '東京'"}},
		"Alerta no environment":        {Destination{Type: "alerta", APIURL: "https://example.com"}, "environment"},
		"Alerta blank environment":     {Destination{Type: "alerta", APIURL: "https://example.com", Environment: " \n"}, "environment"},
		"Alerta reference environment": {Destination{Type: "alerta", APIURL: "https://example.com", Environment: "${env:ENVIRONMENT}"}, "literal"},
		"Dynatrace no token":           {Destination{Type: "dynatrace", APIURL: "https://example.com", EntitySelector: "type(HOST)"}, "api_token"},
		"Dynatrace no selector":        {Destination{Type: "dynatrace", APIURL: "https://example.com", APIToken: "synthetic-key"}, "entity_selector"},
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
				Destination{
					Type:           "dynatrace",
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
				Destination{
					Type:           "dynatrace",
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

func checkMonitoringConfig(t *testing.T, dst Destination, wantError string) {
	t.Helper()
	want := Config{Version: 1, Destinations: map[string]Destination{"monitoring": dst}}
	data, err := yaml.Marshal(want)
	require.NoError(t, err)
	got, err := readConfig(strings.NewReader(string(data)))
	if wantError != "" {
		require.ErrorContains(t, err, wantError)
		assert.NotContains(t, err.Error(), "synthetic-key")
		assert.Equal(t, Config{}, got)
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
					Destination{
						Type:        "alerta",
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

func TestMonitoringFieldIsolation(t *testing.T) {
	fields := map[string]any{
		"url": "https://example.com", "api_url": "https://example.com", "integration_key": "synthetic-key",
		"bearer_token": "synthetic-key", "bot_token": "synthetic-key", "app_token": "synthetic-key",
		"user_key": "synthetic-key", "access_token": "synthetic-key", "username": "user", "password": "synthetic-key",
		"email": "ops@example.com", "channel_tag": "alerts", "source_device_id": "source", "chat_id": "1",
		"message_thread_id": 1, "retries_on_limit": 0, "account_sid": twilioTestSID, "auth_token": "synthetic-key",
		"from": "12345", "to": "12345", "access_key": "synthetic-key", "originator": "Netdata",
		"recipient": "12345", "channel": "#alerts", "sender": "Netdata", "api_key": "synthetic-key",
		"environment": "Production", "api_token": "synthetic-key", "entity_selector": "type(HOST)", "source": "Netdata", "event_type": "CUSTOM_INFO",
	}
	for provider, allowed := range map[string]map[string]any{
		"alerta":    {"api_url": "https://example.com", "api_key": "synthetic-key", "environment": "Production"},
		"dynatrace": {"api_url": "https://example.com", "api_token": "synthetic-key", "entity_selector": "type(HOST)", "source": "Netdata", "event_type": "CUSTOM_INFO"},
	} {
		t.Run(provider, func(t *testing.T) {
			for field, value := range fields {
				if _, ok := allowed[field]; ok {
					continue
				}
				t.Run(field, func(t *testing.T) {
					dst := map[string]any{"type": provider, field: value}
					for key, val := range allowed {
						dst[key] = val
					}
					data, err := yaml.Marshal(
						map[string]any{"version": 1, "destinations": map[string]any{"target": dst}},
					)
					require.NoError(t, err)
					got, err := readConfig(strings.NewReader(string(data)))
					require.Error(t, err)
					assert.Equal(t, Config{}, got)
				})
			}
		})
	}
	for provider := range map[string]struct{}{
		"webhook": {}, "slack": {}, "discord": {}, "telegram": {}, "pushover": {}, "pushbullet": {},
		"twilio": {}, "messagebird": {}, "gotify": {}, "ntfy": {}, "rocketchat": {}, "flock": {}, "fleep": {}, "ilert": {}, "signl4": {},
	} {
		for field, dst := range map[string]Destination{
			"api_key": {APIKey: "synthetic-key"}, "environment": {Environment: "Production"}, "api_token": {APIToken: "synthetic-key"},
			"entity_selector": {EntitySelector: "type(HOST)"}, "event_type": {EventType: "CUSTOM_INFO"}, "source": {Source: "Netdata"},
		} {
			t.Run(provider+"/"+field, func(t *testing.T) {
				dst.Type = provider
				require.ErrorContains(t, dst.validate(), "require type:")
			})
		}
	}
}
