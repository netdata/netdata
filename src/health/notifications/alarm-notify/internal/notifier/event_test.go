// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventDurations(t *testing.T) {
	for field := range map[string]struct{}{"duration": {}, "non_clear_duration": {}} {
		for name, test := range map[string]struct {
			input string
			want  *uint32
			err   bool
		}{
			"omitted": {}, "null": {input: "null"},
			"zero":     {input: "0", want: new(uint32(0))},
			"seconds":  {input: "123", want: new(uint32(123))},
			"maximum":  {input: "4294967295", want: new(uint32(4294967295))},
			"overflow": {input: "4294967296", err: true},
			"negative": {input: "-1", err: true}, "negative zero": {input: "-0", err: true},
			"fraction": {input: "1.5", err: true}, "decimal": {input: "1.0", err: true},
			"exponent": {input: "1e3", err: true}, "boolean": {input: "true", err: true},
			"string": {input: `"synthetic-private-value"`, err: true}, "array": {input: "[]", err: true},
		} {
			t.Run(field+"/"+name, func(t *testing.T) {
				input := validEvent
				if test.input != "" {
					input = strings.Replace(input, `"version": 1`, `"version": 1, "`+field+`": `+test.input, 1)
				}
				got, err := readEvent(strings.NewReader(input))
				if test.err {
					require.ErrorContains(t, err, "invalid JSON event")
					assert.NotContains(t, err.Error(), "synthetic-private-value")
					assert.Equal(t, Event{}, got)
					return
				}
				require.NoError(t, err)
				want := expectedEvent()
				if field == "duration" {
					want.Duration = test.want
				} else {
					want.NonClearDuration = test.want
				}
				assert.Equal(t, want, got)
				// Public Event consumers omit unknown durations and retain explicit zero.
				encoded, err := json.Marshal(got)
				require.NoError(t, err)
				wantJSON := validEvent
				if test.want != nil {
					wantJSON = input
				}
				assert.JSONEq(t, wantJSON, string(encoded))
			})
		}
	}
}

const validEvent = `{
  "version": 1,
  "incident_id": "test-incident",
  "timestamp": "2026-09-14T12:00:00Z",
  "node": "test-node",
  "alert": "test_alert",
  "chart": "test.chart",
  "context": "test.context",
  "status": "WARNING",
  "previous_status": "CLEAR",
  "summary": "Temperature is high",
  "info": "A quote: \"hot\"\nUnicode: θερμοκρασία",
  "value": 42.5,
  "previous_value": 0,
  "units": "C"
}`

func expectedEvent() Event {
	value, previous := 42.5, 0.0
	return Event{
		Version:        1,
		IncidentID:     "test-incident",
		Timestamp:      time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC),
		Node:           "test-node",
		Alert:          "test_alert",
		Chart:          "test.chart",
		Context:        "test.context",
		Status:         "WARNING",
		PreviousStatus: "CLEAR",
		Summary:        "Temperature is high",
		Info:           "A quote: \"hot\"\nUnicode: θερμοκρασία",
		Value:          &value,
		PreviousValue:  &previous,
		Units:          "C",
	}
}

func TestReadEventURL(t *testing.T) {
	tests := map[string]struct {
		url   string
		valid bool
	}{
		"HTTPS":         {url: "https://example.com/alert?id=1#chart", valid: true},
		"HTTP":          {url: "http://localhost/alert", valid: true},
		"omitted":       {valid: true},
		"relative":      {url: "/synthetic-private-value"},
		"unsafe scheme": {url: "javascript:synthetic-private-value"},
		"userinfo":      {url: "https://user:synthetic-private-value@example.com/alert"},
		"missing host":  {url: "https:///synthetic-private-value"},
		"malformed":     {url: "https://example.com/%synthetic-private-value"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			input := strings.Replace(validEvent, `"version": 1`, fmt.Sprintf(`"version": 1, "url": %q`, test.url), 1)
			got, err := readEvent(strings.NewReader(input))
			if !test.valid {
				require.ErrorContains(t, err, "event url must be")
				assert.Equal(t, Event{}, got)
				assert.NotContains(t, err.Error(), "synthetic-private-value")
				return
			}
			require.NoError(t, err)
			want := expectedEvent()
			want.URL = test.url
			assert.Equal(t, want, got)
		})
	}
}

func TestReadEvent(t *testing.T) {
	nullValues := expectedEvent()
	nullValues.Value, nullValues.PreviousValue = nil, nil
	tests := map[string]struct {
		input string
		want  Event
		err   string
	}{
		"complete event": {input: validEvent, want: expectedEvent()},
		"unknown values": {
			input: strings.ReplaceAll(
				strings.Replace(validEvent, "42.5", "null", 1),
				"\"previous_value\": 0",
				"\"previous_value\": null",
			),
			want: nullValues,
		},
		"empty": {err: "invalid JSON"},
		"unknown field": {
			input: strings.Replace(validEvent, "\"units\"", "\"synthetic-private-value\"", 1),
			err:   "invalid JSON",
		},
		"invalid timestamp": {
			input: strings.Replace(validEvent, "2026-09-14T12:00:00Z", "synthetic-private-value", 1),
			err:   "invalid JSON",
		},
		"nonfinite number": {input: strings.Replace(validEvent, "42.5", "NaN", 1), err: "invalid JSON"},
		"overflow number":  {input: strings.Replace(validEvent, "42.5", "1e999", 1), err: "invalid JSON"},
		"unknown version": {
			input: strings.Replace(validEvent, "\"version\": 1", "\"version\": 2", 1),
			err:   "version must be 1",
		},
		"missing identity": {
			input: strings.Replace(validEvent, "test-incident", "", 1),
			err:   "requires incident_id",
		},
		"missing summary": {
			input: strings.Replace(validEvent, "Temperature is high", " ", 1),
			err:   "requires incident_id",
		},
		"unsupported status": {
			input: strings.Replace(validEvent, "WARNING", "UNDEFINED", 1),
			err:   "status must be",
		},
		"unsupported previous status": {
			input: strings.Replace(validEvent, "CLEAR", "synthetic-private-value", 1),
			err:   "previous_status",
		},
		"second event":     {input: validEvent + validEvent, err: "exactly one JSON event"},
		"trailing garbage": {input: validEvent + " synthetic-private-value", err: "exactly one JSON event"},
		"null":             {input: "null", err: "version must be 1"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := readEvent(strings.NewReader(test.input))
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				assert.NotContains(t, err.Error(), "synthetic-private-value")
				assert.Equal(t, Event{}, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}
