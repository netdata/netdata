// SPDX-License-Identifier: GPL-3.0-or-later

package providers

import (
	"bytes"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/commandexec"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// This independent schema inventory replaces union-specific field-denial tests.
// Even explicitly empty fields belong only to the provider declaring them.
var providerFields = map[string]string{
	"webhook": "url bearer_token", "slack": "url", "discord": "url",
	"telegram": "bot_token chat_id message_thread_id api_url retries_on_limit",
	"pushover": "app_token user_key api_url", "pushbullet": "access_token email channel_tag source_device_id api_url",
	"twilio": "account_sid auth_token from to api_url", "messagebird": "access_key originator recipient api_url",
	"gotify": "app_token api_url", "ntfy": "url access_token username password",
	"rocketchat": "url channel", "flock": "url", "fleep": "url sender",
	"ilert": "integration_key api_url", "signl4": "url", "alerta": "api_url api_key environment",
	"dynatrace": "api_url api_token entity_selector event_type source", "prowl": "api_key api_url",
	"kavenegar": "api_key api_url sender recipient", "smseagle": "api_url access_token recipients message_type call_duration voice_id",
	"pagerduty": "integration_key api_url api_version", "opsgenie": "api_key api_url",
	"msteams": "url icons colors", "matrix": "api_url access_token room_id",
	"command": "executable args env", "smstools3": "executable env to",
	"syslog": "executable args env facility level prefix host port",
	"awssns": "executable env target_arn credential_source message_template", "kafka": "url sender_ip",
	"email": "executable env recipients from plain_text_only threading", "irc": "executable env host port nickname realname channel",
}

func validDestinations(t *testing.T) map[string]map[string]any {
	t.Helper()
	data, err := os.ReadFile("../../examples/notify.yaml")
	require.NoError(t, err)
	var document struct {
		Destinations map[string]map[string]any `yaml:"destinations"`
	}
	require.NoError(t, yaml.Unmarshal(data, &document))
	result := make(map[string]map[string]any)
	for _, dst := range document.Destinations {
		result[dst["type"].(string)] = dst
	}
	executable := filepath.Join(t.TempDir(), "unused-tool")
	result["command"] = map[string]any{"type": "command", "executable": executable}
	result["smstools3"] = map[string]any{"type": "smstools3", "executable": executable, "to": "+15005550009"}
	result["syslog"] = map[string]any{"type": "syslog", "executable": executable}
	result["awssns"] = map[string]any{"type": "awssns", "executable": executable, "credential_source": "imds", "target_arn": "arn:aws:sns:us-east-1:123456789012:alerts"}
	result["kafka"] = map[string]any{"type": "kafka", "url": "https://example.com/bridge", "sender_ip": "192.0.2.1"}
	result["email"] = map[string]any{"type": "email", "executable": executable, "recipients": []string{"ops@example.com"}}
	result["irc"] = map[string]any{"type": "irc", "executable": executable, "host": "irc.example.com", "nickname": "netdata", "realname": "Netdata alerts", "channel": "#alerts"}
	return result
}

func encodeDestination(t *testing.T, dst map[string]any) []byte {
	t.Helper()
	data, err := yaml.Marshal(map[string]any{"version": 1, "destinations": map[string]any{"target": dst}})
	require.NoError(t, err)
	return data
}

func TestBuiltinConfigurationAndFieldIsolation(t *testing.T) {
	registry := Builtin(http.DefaultClient, &commandexec.Runner{})
	destinations := validDestinations(t)
	require.Len(t, registry, 31)
	require.ElementsMatch(t, strings.Fields("webhook slack discord telegram pushover pushbullet twilio messagebird gotify ntfy rocketchat flock fleep ilert signl4 alerta dynatrace prowl kavenegar smseagle pagerduty opsgenie msteams matrix command smstools3 syslog awssns kafka email irc"), keys(registry))
	require.ElementsMatch(t, keys(registry), keys(destinations), "destination fixtures must cover every provider")
	require.ElementsMatch(t, keys(registry), keys(providerFields), "field inventory must cover every provider")
	fields := map[string]bool{}
	for _, names := range providerFields {
		for _, name := range strings.Fields(names) {
			fields[name] = true
		}
	}
	for provider, base := range destinations {
		t.Run(provider, func(t *testing.T) {
			plan, err := config.Read(bytes.NewReader(encodeDestination(t, base)), registry)
			require.NoError(t, err)
			require.NotNil(t, plan.Destinations["target"])
			allowed := map[string]bool{}
			for _, name := range strings.Fields(providerFields[provider]) {
				allowed[name] = true
			}
			for field := range fields {
				if allowed[field] {
					continue
				}
				for label, value := range map[string]any{"null": nil, "empty": "", "zero": 0, "nonempty": "synthetic-private-value"} {
					t.Run(field+"/"+label, func(t *testing.T) {
						dst := maps.Clone(base)
						dst[field] = value
						got, err := config.Read(bytes.NewReader(encodeDestination(t, dst)), registry)
						require.ErrorContains(t, err, "invalid YAML")
						assert.NotContains(t, err.Error(), "synthetic-private-value")
						assert.Equal(t, notifier.Plan{}, got)
					})
				}
			}
		})
	}
}

func keys[V any](values map[string]V) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	return result
}
