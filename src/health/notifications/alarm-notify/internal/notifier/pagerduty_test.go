// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const pagerDutyTestKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const pagerDutyTestIncidentKey = "1532eea2c9f6a386140992fc18e52fbde95854d42a4bfa2e46d1c0c5d280c456"

func pagerDutyTestDestination(version int64) Destination {
	n := configInteger(version)
	return Destination{Type: "pagerduty", IntegrationKey: pagerDutyTestKey, APIVersion: &n}
}

func pagerDutyTestFixture(t *testing.T, version int64, variant, status, previous string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(fmt.Sprintf("testdata/pagerduty-v%d-%s.json", version, variant))
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

func TestRenderPagerDuty(t *testing.T) {
	full := expectedEvent()
	full.URL = "https://example.com/alert?id=1&view=chart#details"
	minimal := notifyevent.Event{
		Version:    1,
		IncidentID: "test-incident",
		Timestamp:  full.Timestamp,
		Node:       "node",
		Alert:      "alert",
		Summary:    "summary",
		Status:     "WARNING",
	}
	for version := range map[int64]struct{}{1: {}, 2: {}} {
		for name, test := range map[string]struct{ variant, status, previous string }{
			"warning": {"full", "WARNING", "CLEAR"}, "critical": {"full", "CRITICAL", "WARNING"}, "clear": {"full", "CLEAR", "CRITICAL"},
			"minimal warning": {"minimal", "WARNING", ""}, "minimal clear": {"minimal", "CLEAR", ""},
		} {
			t.Run(fmt.Sprintf("v%d/%s", version, name), func(t *testing.T) {
				event := full
				if test.variant == "minimal" {
					event = minimal
				}
				event.Status, event.PreviousStatus = test.status, test.previous
				data, err := json.Marshal(renderPagerDuty(pagerDutyTestDestination(version), event))
				require.NoError(t, err)
				var got map[string]any
				require.NoError(t, json.Unmarshal(data, &got))
				assert.Equal(t, pagerDutyTestFixture(t, version, test.variant, test.status, test.previous), got)
			})
		}
	}
	assert.Equal(
		t,
		renderPagerDuty(pagerDutyTestDestination(1), full),
		renderPagerDuty(Destination{Type: "pagerduty", IntegrationKey: pagerDutyTestKey}, full),
	)
}

func TestPagerDutyIdentity(t *testing.T) {
	for name, test := range map[string]struct{ id, want string }{
		"fixture": {"test-incident", pagerDutyTestIncidentKey},
		"simple":  {"incident", "d4191834714542dcf3e5d8a6ab386c9b72259430730157b6e5c76469cbb6a622"},
	} {
		t.Run(name, func(t *testing.T) { assert.Equal(t, test.want, pagerDutyIncidentKey(test.id)) })
	}
	for name, id := range map[string]string{"case": "TEST-incident", "space": " test-incident", "newline": "test-incident\n", "Unicode": "test-incident界", "long common prefix": strings.Repeat("x", 256) + "a"} {
		t.Run(name, func(t *testing.T) {
			key := pagerDutyIncidentKey(id)
			assert.Len(t, key, 64)
			assert.NotEqual(t, pagerDutyTestIncidentKey, key)
			assert.NotEqual(t, pagerDutyIncidentKey(strings.Repeat("x", 256)+"b"), key)
		})
	}
	for name, test := range map[string]struct {
		status string
		offset time.Duration
	}{"warning": {"WARNING", 0}, "critical later": {"CRITICAL", time.Hour}, "clear later": {"CLEAR", 2 * time.Hour}} {
		t.Run(name, func(t *testing.T) {
			event := expectedEvent()
			event.Status, event.Timestamp = test.status, event.Timestamp.Add(test.offset)
			assert.Equal(
				t,
				pagerDutyTestIncidentKey,
				renderPagerDuty(pagerDutyTestDestination(1), event).(pagerDutyV1Event).IncidentKey,
			)
			assert.Equal(
				t,
				pagerDutyTestIncidentKey,
				renderPagerDuty(pagerDutyTestDestination(2), event).(pagerDutyV2Event).DedupKey,
			)
		})
	}
}

func TestPagerDutyTitleLimits(t *testing.T) {
	for name, test := range map[string]struct {
		symbol string
		size   int
	}{"ASCII boundary": {"x", 1024}, "ASCII over": {"x", 1025}, "Unicode boundary": {"界", 1024}, "emoji over": {"😀", 1025}, "JSON escaping": {"<", 1024}} {
		t.Run(name, func(t *testing.T) {
			event := expectedEvent()
			const prefix = "test-node WARNING: "
			event.Summary = strings.Repeat(test.symbol, test.size-len(prefix))
			want := prefix + event.Summary
			if test.size > 1024 {
				want = prefix + strings.Repeat(test.symbol, 1021-len(prefix)) + "..."
			}
			one := renderPagerDuty(pagerDutyTestDestination(1), event).(pagerDutyV1Event)
			two := renderPagerDuty(pagerDutyTestDestination(2), event).(pagerDutyV2Event)
			assert.Equal(t, want, one.Description)
			assert.Equal(t, want, two.Payload.Summary)
			assert.True(t, utf8.ValidString(one.Description))
			assert.Equal(t, event, one.Details)
			assert.Equal(t, event, two.Payload.CustomDetails)
		})
	}
}

func TestReadPagerDutyResponse(t *testing.T) {
	for version, p := range map[int64]struct {
		status int
		key    string
	}{1: {200, "incident_key"}, 2: {202, "dedup_key"}} {
		success := fmt.Sprintf(
			`{"status":"success","%s":%q,"message":"Event processed"}`,
			p.key,
			pagerDutyTestIncidentKey,
		)
		for name, test := range map[string]struct {
			status    int
			body, err string
		}{
			"success": {p.status, success, ""}, "HTTP error": {400, "synthetic-private-value", "HTTP 400"},
			"rate limit": {429, "synthetic-private-value", "HTTP 429"}, "redirect": {307, "", "HTTP 307"},
			"wrong success": {201, success, "HTTP 201"}, "empty": {p.status, "", "invalid"}, "null": {p.status, "null", "acknowledgment"},
			"empty object": {p.status, `{}`, "acknowledgment"}, "array": {p.status, `[]`, "invalid"},
			"wrong status":   {p.status, strings.Replace(success, "success", "synthetic-private-value", 1), "acknowledgment"},
			"missing key":    {p.status, `{"status":"success"}`, "acknowledgment"},
			"wrong key":      {p.status, strings.Replace(success, pagerDutyTestIncidentKey, "synthetic-private-value", 1), "acknowledgment"},
			"wrong key type": {p.status, fmt.Sprintf(`{"status":"success","%s":1}`, p.key), "invalid"},
			"malformed":      {p.status, "synthetic-private-value", "invalid"}, "trailing": {p.status, success + success, "invalid"},
			"limit":      {p.status, success + strings.Repeat(" ", httpclient.ResponseLimit-len(success)), ""},
			"over limit": {p.status, success + strings.Repeat(" ", httpclient.ResponseLimit-len(success)+1), "256 KiB"},
		} {
			t.Run(fmt.Sprintf("v%d/%s", version, name), func(t *testing.T) {
				reader := strings.NewReader(test.body)
				body := &telegramTestBody{Reader: reader}
				err := readPagerDutyResponse(
					&http.Response{StatusCode: test.status, Body: body},
					version,
					pagerDutyTestIncidentKey,
				)
				if test.err == "" {
					require.NoError(t, err)
				} else {
					require.ErrorContains(t, err, test.err)
					assert.NotContains(t, err.Error(), "synthetic-private-value")
				}
				assert.True(t, body.closed)
				if test.status != p.status {
					assert.Equal(t, len(test.body), reader.Len())
				}
			})
		}
		for name, test := range map[string]struct {
			err  error
			want string
		}{"read": {errors.New("synthetic-private-value"), "transport failed"}, "cancel": {context.Canceled, "canceled"}, "deadline": {context.DeadlineExceeded, "timed out"}} {
			t.Run(fmt.Sprintf("v%d/%s", version, name), func(t *testing.T) {
				body := &telegramTestBody{Reader: formErrorReader{test.err}}
				err := readPagerDutyResponse(
					&http.Response{StatusCode: p.status, Body: body},
					version,
					pagerDutyTestIncidentKey,
				)
				require.ErrorContains(t, err, test.want)
				assert.NotContains(t, err.Error(), "synthetic-private-value")
				assert.True(t, body.closed)
			})
		}
	}
}
