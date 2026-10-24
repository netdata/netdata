// SPDX-License-Identifier: GPL-3.0-or-later
package app

import (
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/url"
	"os"
	"strings"
	"testing"
)

const pagerDutyTestKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

const pagerDutyTestIncidentKey = "1532eea2c9f6a386140992fc18e52fbde95854d42a4bfa2e46d1c0c5d280c456"

func pagerDutyTestDestination(version int64) map[string]any {
	return map[string]any{"type": "pagerduty", "integration_key": pagerDutyTestKey, "api_version": version}
}

func pagerDutyTestFixture(t *testing.T, version int64, variant, status, previous string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(fixturePath(fmt.Sprintf("pagerduty-v%d-%s.json", version, variant)))
	require.NoError(t, err)
	var want map[string]any
	require.NoError(t, json.Unmarshal(data, &want))
	var details map[string]any
	if version == 1 {
		details = want["details"].(map[string]any)
		want["description"] = strings.Replace(want["description"].(string), " WARNING:", " "+status+":", 1)
		if status == "CLEAR" {
			want["event_type"] = "resolve"
			delete(want, "client")
			delete(want, "client_url")
		}
	} else {
		payload := want["payload"].(map[string]any)
		details = payload["custom_details"].(map[string]any)
		payload["summary"] = strings.Replace(payload["summary"].(string), " WARNING:", " "+status+":", 1)
		payload["severity"] = strings.ToLower(status)
		if status == "CLEAR" {
			want["event_action"] = "resolve"
			payload["severity"] = "info"
		}
	}
	details["status"] = status
	if previous != "" {
		details["previous_status"] = previous
	}
	return want
}

const opsgenieTestKey = "synthetic-opsgenie-key"

const opsgenieTestAlias = "1532eea2c9f6a386140992fc18e52fbde95854d42a4bfa2e46d1c0c5d280c456"

const opsgenieTestAck = `{"result":"Request will be processed","requestId":"test-request","took":0.1}`

func opsgenieTestDestination() map[string]any {
	return map[string]any{"type": "opsgenie", "api_key": opsgenieTestKey}
}

func opsgenieTestFixture(t *testing.T, variant, status string) map[string]any {
	t.Helper()
	action := "create"
	if status == "CLEAR" {
		action = "close"
	}
	data, err := os.ReadFile(fixturePath(fmt.Sprintf("opsgenie-%s-%s.json", action, variant)))
	require.NoError(t, err)
	if status == "CRITICAL" {
		data = []byte(
			strings.ReplaceAll(
				strings.ReplaceAll(strings.ReplaceAll(string(data), "WARNING", "CRITICAL"), "CLEAR", "WARNING"),
				`"P3"`,
				`"P1"`,
			),
		)
	}
	var want map[string]any
	require.NoError(t, json.Unmarshal(data, &want))
	return want
}

const smseagleTestResponse = `[{"status":"queued","message":"OK","number":"+15005550009","id":1},{"status":"queued","message":"OK","number":"05005550009","id":2}]`

func smseagleTestDestination() map[string]any {
	return map[string]any{"type": "smseagle", "api_url": "https://example.com", "access_token": "synthetic-token", "recipients": []string{"+15005550009", "05005550009"}}
}

func smseagleTestFixture(t *testing.T, variant, mode, status string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(fixturePath("smseagle-" + variant + ".json"))
	require.NoError(t, err)
	var want map[string]any
	require.NoError(t, json.Unmarshal(data, &want))
	text := want["text"].(string)
	if status == "CRITICAL" {
		text = strings.ReplaceAll(text, "WARNING", "CRITICAL")
	}
	if status == "CLEAR" {
		text = strings.ReplaceAll(text, "CLEAR → WARNING", "CRITICAL → CLEAR")
	}
	want["text"] = text
	if mode == "ring" || mode == "tts" || mode == "tts_advanced" {
		delete(want, "encoding")
		want["duration"] = float64(10)
		want["text"] = strings.TrimSuffix(text, "\nhttps://example.com/alert?id=1&view=chart#details")
		if mode == "ring" {
			delete(want, "text")
		}
		if mode == "tts_advanced" {
			want["voice_id"] = float64(1)
		}
	}
	return want
}

const twilioTestSID = "AC00000000000000000000000000000000"

const twilioTestPath = "/2010-04-01/Accounts/" + twilioTestSID + "/Messages.json"

const messagebirdTestPath = "/messages"

const prowlTestKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

const prowlTestKeys = prowlTestKey + ",BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"

const prowlTestResponse = `<?xml version="1.0" encoding="UTF-8"?><prowl><success code="200" remaining="999" resetdate="1234567890"/></prowl>`

const kavenegarTestResponse = `{"return":{"status":200,"message":"تایید شد"},"entries":[{"messageid":8792343,"status":1,"receptor":"+15005550009"}]}`

func formTestFixture(t *testing.T, provider, variant string) url.Values {
	t.Helper()
	data, err := os.ReadFile(fixturePath(provider + "-" + variant + ".json"))
	require.NoError(t, err)
	var want url.Values
	require.NoError(t, json.Unmarshal(data, &want))
	return want
}

const alertaTestAck = `{"status":"ok","id":"synthetic-alert"}`

const dynatraceTestAck = `{"reportCount":1,"eventIngestResults":[{"status":"OK","correlationId":"synthetic-event"}]}`

func assertMonitoringPayload(t *testing.T, provider, want, got string) {
	t.Helper()
	var wantObject, gotObject map[string]any
	require.NoError(t, json.Unmarshal([]byte(want), &wantObject))
	require.NoError(t, json.Unmarshal([]byte(got), &gotObject))
	if provider == "alerta" {
		wantRaw, ok := wantObject["rawData"].(string)
		require.True(t, ok)
		gotRaw, ok := gotObject["rawData"].(string)
		require.True(t, ok)
		assert.JSONEq(t, wantRaw, gotRaw)
		delete(wantObject, "rawData")
		delete(gotObject, "rawData")
	}
	assert.Equal(t, wantObject, gotObject)
}

func formTestDestination(provider string) map[string]any {
	if provider == "prowl" {
		return map[string]any{"type": provider, "api_key": prowlTestKeys}
	}
	return map[string]any{"type": provider, "api_key": "synthetic-key", "sender": "+15005550006", "recipient": "+15005550009"}
}

const kafkaFullJSON = `{
  "host_ip": "192.0.2.1", "when": 1789387200, "name": "test_alert", "chart": "test.chart",
  "status": "WARNING", "old_status": "CLEAR", "value": 42.5, "old_value": 0,
  "duration": 0, "non_clear_duration": 123, "units": "C",
  "info": "A quote: \"hot\"\nUnicode: θερμοκρασία"
}`
