// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSMSEagleConfig(t *testing.T) {
	for name, test := range map[string]struct {
		field string
		value any
		err   string
	}{
		"defaults": {}, "SMS": {"message_type", "sms", ""}, "MMS": {"message_type", "mms", ""},
		"ring": {"message_type", "ring", ""}, "TTS": {"message_type", "tts", ""}, "advanced": {"message_type", "tts_advanced", ""},
		"unknown mode":     {"message_type", "synthetic-private-value", "message_type must"},
		"empty recipients": {"recipients", []string{}, "at least one"}, "null recipients": {"recipients", nil, "at least one"},
		"duplicate": {"recipients", []string{"123", "123"}, "duplicates"}, "scalar recipients": {"recipients", "123", "invalid YAML"},
		"invalid phone":  {"recipients", []string{"synthetic-private-value"}, "phone number"},
		"phone controls": {"recipients", []string{"123\n"}, "phone number"}, "plus only": {"recipients", []string{"+"}, "phone number"},
		"phone reference": {"recipients", []string{"${env:PHONE}"}, "phone number"},
		"no API":          {"api_url", "", "required"}, "relative API": {"api_url", "/synthetic-private-value", "absolute HTTP(S)"},
		"HTTP appliance": {"api_url", "http://localhost:8080/proxy/", ""}, "API env": {"api_url", "${env:UNREAD_API}", ""},
		"API file": {"api_url", "${file:/unread/synthetic-url}", ""}, "query": {"api_url", "https://example.com/?synthetic-private-value", "query"},
		"fragment": {"api_url", "https://example.com/#", "fragment"}, "userinfo": {"api_url", "https://user:synthetic-private-value@example.com", "user information"},
		"empty token": {"access_token", "", "nonempty"}, "token spaces": {"access_token", "synthetic-private-value ", "without whitespace"},
		"token Unicode": {"access_token", "synthetic-private-value界", "ASCII"}, "token controls": {"access_token", "synthetic-private-value\n", "without whitespace"},
		"token env": {"access_token", "${env:UNREAD_TOKEN}", ""}, "token file": {"access_token", "${file:/unread/synthetic-token}", ""},
		"interpolation": {"access_token", "synthetic-private-value${env:TOKEN}", "secret reference"},
		"relative file": {"access_token", "${file:relative}", "absolute path"},
		"SMS duration":  {"call_duration", 10, "requires a call"}, "SMS voice": {"voice_id", 1, "requires message_type"},
		"manual encoding": {"encoding", "unicode", "invalid YAML"},
	} {
		t.Run(name, func(t *testing.T) {
			dst := map[string]any{
				"type":         "smseagle",
				"api_url":      "https://example.com",
				"access_token": "synthetic-token",
				"recipients":   []string{"+15005550009", "05005550009"},
			}
			if test.field != "" {
				dst[test.field] = test.value
			}
			data, err := yaml.Marshal(map[string]any{"version": 1, "destinations": map[string]any{"target": dst}})
			require.NoError(t, err)
			got, err := readConfig(strings.NewReader(string(data)))
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				assert.NotContains(t, err.Error(), "synthetic-private-value")
				assert.Equal(t, Config{}, got)
			} else {
				require.NoError(t, err)
				var want Config
				require.NoError(t, yaml.Unmarshal(data, &want))
				assert.Equal(t, want, got)
			}
		})
	}
	for mode := range map[string]struct{}{"sms": {}, "mms": {}, "ring": {}, "tts": {}, "tts_advanced": {}} {
		for field := range map[string]struct{}{"call_duration": {}, "voice_id": {}} {
			for name, test := range map[string]struct {
				value    string
				positive bool
			}{"positive": {"7", true}, "zero": {"0", false}, "negative": {"-1", false}, "fraction": {"1.5", false}, "quoted": {"'7'", false}, "overflow": {"9223372036854775808", false}} {
				t.Run(mode+"/"+field+"/"+name, func(t *testing.T) {
					data := "version: 1\ndestinations:\n  target:\n    type: smseagle\n    api_url: https://example.com\n    access_token: synthetic-token\n    recipients: ['15005550009']\n    message_type: " + mode + "\n    " + field + ": " + test.value + "\n"
					got, err := readConfig(strings.NewReader(data))
					allowed := (field == "call_duration" && (mode == "ring" || mode == "tts" || mode == "tts_advanced")) ||
						(field == "voice_id" && mode == "tts_advanced")
					if allowed && test.positive {
						require.NoError(t, err)
						want := smseagleTestDestination()
						want.Recipients, want.MessageType = []string{"15005550009"}, mode
						n := configInteger(7)
						if field == "call_duration" {
							want.CallDuration = &n
						} else {
							want.VoiceID = &n
						}
						assert.Equal(t, Config{Version: 1, Destinations: map[string]Destination{"target": want}}, got)
					} else {
						require.Error(t, err)
						assert.Equal(t, Config{}, got)
					}
				})
			}
		}
	}
}

func TestSMSEagleFieldIsolation(t *testing.T) {
	fields := reflect.TypeFor[Destination]()
	for i := 0; i < fields.NumField(); i++ {
		field := fields.Field(i)
		switch field.Name {
		case "Type", "APIURL", "AccessToken", "Recipients", "MessageType", "CallDuration", "VoiceID":
			continue
		}
		t.Run(field.Name, func(t *testing.T) {
			dst := smseagleTestDestination()
			v := reflect.ValueOf(&dst).Elem().Field(i)
			if v.Kind() == reflect.Map {
				v.Set(reflect.ValueOf(map[string]string{"warning": "synthetic-private-value"}))
			} else if v.Kind() == reflect.Slice {
				v.Set(reflect.ValueOf([]string{"synthetic-private-value"}))
			} else if v.Kind() == reflect.String {
				v.SetString("synthetic-private-value")
			} else {
				v.Set(reflect.New(v.Type().Elem()))
			}
			wantErr := "fields for another provider"
			if field.Name == "APIVersion" {
				wantErr = "api_version requires type: pagerduty"
			}
			checkFormConfig(t, dst, wantErr)
		})
	}
	for provider := range map[string]struct{}{"webhook": {}, "slack": {}, "discord": {}, "telegram": {}, "pushover": {}, "pushbullet": {}, "twilio": {}, "messagebird": {}, "gotify": {}, "ntfy": {}, "rocketchat": {}, "flock": {}, "fleep": {}, "ilert": {}, "signl4": {}, "alerta": {}, "dynatrace": {}, "prowl": {}, "kavenegar": {}} {
		for name, dst := range map[string]Destination{"recipients": {Recipients: []string{"123"}}, "empty recipients": {Recipients: []string{}}, "message_type": {MessageType: "sms"}, "call_duration": {CallDuration: new(configInteger)}, "voice_id": {VoiceID: new(configInteger)}} {
			t.Run(provider+"/"+name, func(t *testing.T) {
				dst.Type = provider
				require.ErrorContains(t, dst.validate(), "require type: smseagle")
			})
		}
	}
}
