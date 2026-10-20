// SPDX-License-Identifier: GPL-3.0-or-later

package opsgenie

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"math"
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

const opsgenieTestKey = "synthetic-opsgenie-key"
const opsgenieTestAlias = "1532eea2c9f6a386140992fc18e52fbde95854d42a4bfa2e46d1c0c5d280c456"
const opsgenieTestAck = `{"result":"Request will be processed","requestId":"test-request","took":0.1}`

func opsgenieTestConfig() Config {
	return Config{APIKey: opsgenieTestKey}
}

func opsgenieTestFixture(t *testing.T, variant, status string) map[string]any {
	t.Helper()
	action := "create"
	if status == "CLEAR" {
		action = "close"
	}
	data, err := os.ReadFile(fmt.Sprintf("testdata/opsgenie-%s-%s.json", action, variant))
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

func TestRenderOpsgenie(t *testing.T) {
	full := testutil.ExpectedEvent()
	full.URL = "https://example.com/alert?id=1&view=chart#details"
	minimal := notifyevent.Event{
		Version:    1,
		IncidentID: "test-incident",
		Timestamp:  full.Timestamp,
		Node:       "node",
		Alert:      "alert",
		Summary:    "summary",
	}
	for name, test := range map[string]struct{ variant, status, previous string }{
		"warning": {"full", "WARNING", "CLEAR"}, "critical": {"full", "CRITICAL", "WARNING"}, "clear": {"full", "CLEAR", "CRITICAL"},
		"minimal warning": {"minimal", "WARNING", ""}, "minimal clear": {"minimal", "CLEAR", ""},
	} {
		t.Run(name, func(t *testing.T) {
			event := full
			if test.variant == "minimal" {
				event = minimal
			}
			event.Status, event.PreviousStatus = test.status, test.previous
			message, err := renderOpsgenie(event)
			require.NoError(t, err)
			data, err := json.Marshal(message)
			require.NoError(t, err)
			var got map[string]any
			require.NoError(t, json.Unmarshal(data, &got))
			assert.Equal(t, opsgenieTestFixture(t, test.variant, test.status), got)
		})
	}
}

func TestOpsgenieIdentity(t *testing.T) {
	for name, test := range map[string]struct{ id, want string }{
		"fixture": {"test-incident", opsgenieTestAlias},
		"second":  {"incident", "d4191834714542dcf3e5d8a6ab386c9b72259430730157b6e5c76469cbb6a622"},
	} {
		t.Run(name, func(t *testing.T) { assert.Equal(t, test.want, opsgenieAlias(test.id)) })
	}
	for name, id := range map[string]string{"case": "TEST-incident", "space": " test-incident", "URL characters": "test-incident/?#%", "Unicode": "test-incident界", "long": "test-incident" + strings.Repeat("x", 600)} {
		t.Run(name, func(t *testing.T) {
			key := opsgenieAlias(id)
			assert.Len(t, key, 64)
			assert.NotEqual(t, opsgenieTestAlias, key)
			assert.NotEqual(t, opsgenieAlias(id+"y"), key)
			assert.NotContains(t, key, "/")
		})
	}
	for status, offset := range map[string]time.Duration{"WARNING": 0, "CRITICAL": time.Hour} {
		t.Run(status, func(t *testing.T) {
			event := testutil.ExpectedEvent()
			event.Status = status
			event.Timestamp = event.Timestamp.Add(offset)
			message, err := renderOpsgenie(event)
			require.NoError(t, err)
			assert.Equal(t, opsgenieTestAlias, message.(opsgenieCreate).Alias)
		})
	}
}

func TestOpsgenieTitleLimits(t *testing.T) {
	for name, test := range map[string]struct {
		symbol string
		size   int
	}{"ASCII boundary": {"x", 130}, "ASCII over": {"x", 131}, "Unicode boundary": {"界", 130}, "emoji over": {"😀", 131}, "escaping": {"<", 130}} {
		t.Run(name, func(t *testing.T) {
			event := testutil.ExpectedEvent()
			const prefix = "test-node WARNING: "
			event.Summary = strings.Repeat(test.symbol, test.size-len(prefix))
			want := prefix + event.Summary
			if test.size > 130 {
				want = prefix + strings.Repeat(test.symbol, 127-len(prefix)) + "..."
			}
			message, err := renderOpsgenie(event)
			require.NoError(t, err)
			got := message.(opsgenieCreate)
			assert.Equal(t, want, got.Message)
			assert.True(t, utf8.ValidString(got.Message))
			var details notifyevent.Event
			require.NoError(t, json.Unmarshal([]byte(got.Details["event"]), &details))
			assert.Equal(t, event, details)
		})
	}
}

func TestOpsgenieContentLimits(t *testing.T) {
	for name, test := range map[string]struct {
		field, status string
		size          int
		err           string
	}{
		"source boundary": {"node", "WARNING", 100, ""}, "source over": {"node", "WARNING", 101, "source"},
		"clear source over": {"node", "CLEAR", 101, "source"}, "entity boundary": {"alert", "WARNING", 512, ""},
		"entity over": {"alert", "WARNING", 513, "entity"}, "clear long entity": {"alert", "CLEAR", 513, ""},
		"description over": {"info", "WARNING", 15001, "description"},
	} {
		t.Run(name, func(t *testing.T) {
			event := testutil.ExpectedEvent()
			event.Status = test.status
			value := strings.Repeat("界", test.size)
			switch test.field {
			case "node":
				event.Node = value
			case "alert":
				event.Alert = value
			case "info":
				event.Info = value
			}
			message, err := renderOpsgenie(event)
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				assert.Nil(t, message)
			} else {
				require.NoError(t, err)
			}
		})
	}
	for name, test := range map[string]struct {
		symbol string
		extra  int
	}{"ASCII boundary": {"x", 0}, "ASCII over": {"x", 1}, "Unicode boundary": {"界", 0}, "Unicode over": {"界", 1}, "escaped boundary": {"<", 0}, "escaped over": {"<", 1}} {
		t.Run("details/"+name, func(t *testing.T) {
			event := testutil.ExpectedEvent()
			event.Info = "x"
			data, err := json.Marshal(event)
			require.NoError(t, err)
			remaining := 8000 - utf8.RuneCount(data) - len("event") + 1
			width := 1
			if test.symbol == "<" {
				width = 6
			}
			event.Info = strings.Repeat(test.symbol, remaining/width) + strings.Repeat("x", remaining%width+test.extra)
			message, err := renderOpsgenie(event)
			if test.extra != 0 {
				require.ErrorContains(t, err, "details")
				assert.Nil(t, message)
			} else {
				require.NoError(t, err)
				assert.Equal(t, 8000, len("event")+utf8.RuneCountInString(message.(opsgenieCreate).Details["event"]))
			}
		})
		t.Run("note/"+name, func(t *testing.T) {
			event := testutil.ExpectedEvent()
			event.Status = "CLEAR"
			event.Info = "x"
			message, err := renderOpsgenie(event)
			require.NoError(t, err)
			remaining := 25000 - utf8.RuneCountInString(message.(opsgenieClose).Note)
			width := 2
			if test.symbol == "<" {
				width = 7
			}
			// IncidentID appears once in the note, allowing exact boundary adjustment.
			event.Info += strings.Repeat(test.symbol, remaining/width)
			event.IncidentID += strings.Repeat("x", remaining%width+test.extra)
			message, err = renderOpsgenie(event)
			if test.extra != 0 {
				require.ErrorContains(t, err, "note")
				assert.Nil(t, message)
			} else {
				require.NoError(t, err)
				assert.Equal(t, 25000, utf8.RuneCountInString(message.(opsgenieClose).Note))
			}
		})
	}
	for status := range map[string]struct{}{"WARNING": {}, "CLEAR": {}} {
		t.Run("encoding/"+status, func(t *testing.T) {
			event := testutil.ExpectedEvent()
			event.Status = status
			event.Value = new(math.NaN())
			_, err := renderOpsgenie(event)
			require.ErrorContains(t, err, "could not encode opsgenie event")
		})
	}
}

func TestReadOpsgenieResponse(t *testing.T) {
	for name, test := range map[string]struct {
		status    int
		body, err string
	}{
		"accepted": {202, opsgenieTestAck, ""}, "wrong success": {200, opsgenieTestAck, "HTTP 200"}, "unauthorized": {401, "synthetic-private-value", "HTTP 401"},
		"limit": {429, "synthetic-private-value", "HTTP 429"}, "redirect": {307, "", "HTTP 307"},
		"missing id":    {202, `{"result":"Request will be processed"}`, "acknowledgment"},
		"empty id":      {202, `{"result":"Request will be processed","requestId":""}`, "acknowledgment"},
		"whitespace id": {202, `{"result":"Request will be processed","requestId":" \n"}`, "acknowledgment"},
		"wrong id type": {202, `{"result":"Request will be processed","requestId":1}`, "invalid"},
		"wrong result":  {202, `{"result":"synthetic-private-value","requestId":"test"}`, "acknowledgment"},
		"empty":         {202, "", "invalid"}, "null": {202, "null", "acknowledgment"}, "array": {202, "[]", "invalid"}, "object": {202, "{}", "acknowledgment"},
		"malformed": {202, "synthetic-private-value", "invalid"}, "trailing": {202, opsgenieTestAck + opsgenieTestAck, "invalid"},
		"body boundary": {202, opsgenieTestAck + strings.Repeat(" ", httpclient.ResponseLimit-len(opsgenieTestAck)), ""},
		"body over":     {202, opsgenieTestAck + strings.Repeat(" ", httpclient.ResponseLimit-len(opsgenieTestAck)+1), "256 KiB"},
	} {
		t.Run(name, func(t *testing.T) {
			reader := strings.NewReader(test.body)
			body := &testBody{Reader: reader}
			err := readOpsgenieResponse(&http.Response{StatusCode: test.status, Body: body})
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				assert.NotContains(t, err.Error(), "synthetic-private-value")
			} else {
				require.NoError(t, err)
			}
			assert.True(t, body.closed)
			if test.status != 202 {
				assert.Equal(t, len(test.body), reader.Len())
			}
		})
	}
	for name, test := range map[string]struct {
		err  error
		want string
	}{"read": {errors.New("synthetic-private-value"), "transport failed"}, "cancel": {context.Canceled, "canceled"}, "deadline": {context.DeadlineExceeded, "timed out"}} {
		t.Run(name, func(t *testing.T) {
			body := &testBody{Reader: formErrorReader{test.err}}
			err := readOpsgenieResponse(&http.Response{StatusCode: 202, Body: body})
			require.ErrorContains(t, err, test.want)
			assert.NotContains(t, err.Error(), "synthetic-private-value")
			assert.True(t, body.closed)
		})
	}
}
