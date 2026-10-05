// SPDX-License-Identifier: GPL-3.0-or-later

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadProducerContext(t *testing.T) {
	for name, tt := range map[string]struct {
		input string
		want  notifier.ProducerContext
		bad   bool
	}{
		"omitted": {}, "null": {input: "null"}, "empty": {input: "{}"},
		"single case aliases":   {input: `{"SRC":"source","UNIQUE_ID":42}`, want: notifier.ProducerContext{Source: "source", UniqueID: new(uint32(42))}},
		"single Unicode alias":  {input: `{"ſrc":"source"}`, want: notifier.ProducerContext{Source: "source"}},
		"single escaped member": {input: `{"s\u0072c":"source"}`, want: notifier.ProducerContext{Source: "source"}},
		"complete": {input: testutil.ProducerContextJSON, want: notifier.ProducerContext{
			UniqueID: new(uint32(42)), AlarmID: new(uint32(7)), EventID: new(uint32(3)),
			Source:      "line=12,file=/etc/netdata/health.d/example.conf",
			ValueString: new("42.50 °C"), OldValueString: new("0.00 °C"),
			CalcExpression: "$this > 40", CalcParamValues: "$this = 42.5",
			TotalWarnings: new(uint32(2)), TotalCritical: new(uint32(0)),
			TotalWarnAlarms: "other_alert=123,another_alert=456",
			Classification:  "System", EditCommandLine: "edit-config health.d/example.conf",
			ChildMachineGUID: "00000000-0000-0000-0000-000000000001",
			TransitionID:     "00000000-0000-0000-0000-000000000002", Component: "Sensors", Type: "Environment",
		}},
		"unknown field": {input: `{"synthetic-private-value": 1}`, bad: true},
		"array":         {input: `[]`, bad: true}, "boolean": {input: `true`, bad: true},
		"number": {input: `0`, bad: true}, "string": {input: `"synthetic-private-value"`, bad: true},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := readNotification(strings.NewReader(testutil.WithProducerContext(testutil.ValidEvent, tt.input)))
			if tt.bad {
				require.ErrorContains(t, err, "invalid JSON event")
				assert.NotContains(t, err.Error(), "synthetic-private-value")
				assert.Equal(t, notifier.Notification{}, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, notifier.Notification{Event: testutil.ExpectedEvent(), ProducerContext: tt.want}, got)
			public, err := json.Marshal(got.Event)
			require.NoError(t, err)
			assert.JSONEq(t, testutil.ValidEvent, string(public))
		})
	}
}

func TestReadProducerContextNumbers(t *testing.T) {
	for field, set := range map[string]func(*notifier.ProducerContext, *uint32){
		"unique_id":      func(p *notifier.ProducerContext, v *uint32) { p.UniqueID = v },
		"alarm_id":       func(p *notifier.ProducerContext, v *uint32) { p.AlarmID = v },
		"event_id":       func(p *notifier.ProducerContext, v *uint32) { p.EventID = v },
		"total_warnings": func(p *notifier.ProducerContext, v *uint32) { p.TotalWarnings = v },
		"total_critical": func(p *notifier.ProducerContext, v *uint32) { p.TotalCritical = v },
	} {
		for name, tt := range map[string]struct {
			input string
			want  *uint32
			bad   bool
		}{
			"omitted": {}, "null": {input: "null"}, "zero": {input: "0", want: new(uint32(0))},
			"positive": {input: "42", want: new(uint32(42))}, "maximum": {input: "4294967295", want: new(uint32(4294967295))},
			"overflow": {input: "4294967296", bad: true}, "negative": {input: "-1", bad: true},
			"negative zero": {input: "-0", bad: true}, "fraction": {input: "1.5", bad: true},
			"decimal": {input: "1.0", bad: true}, "exponent": {input: "1e3", bad: true},
			"string": {input: `"synthetic-private-value"`, bad: true}, "boolean": {input: "true", bad: true},
			"array": {input: "[]", bad: true}, "object": {input: "{}", bad: true},
		} {
			t.Run(field+"/"+name, func(t *testing.T) {
				input := "{}"
				if tt.input != "" {
					input = `{"` + field + `":` + tt.input + `}`
				}
				got, err := readNotification(strings.NewReader(testutil.WithProducerContext(testutil.ValidEvent, input)))
				if tt.bad {
					require.ErrorContains(t, err, "invalid JSON event")
					assert.NotContains(t, err.Error(), "synthetic-private-value")
					assert.Equal(t, notifier.Notification{}, got)
					return
				}
				require.NoError(t, err)
				want := notifier.Notification{Event: testutil.ExpectedEvent()}
				set(&want.ProducerContext, tt.want)
				assert.Equal(t, want, got)
			})
		}
	}
}

func TestReadProducerContextStrings(t *testing.T) {
	for field, set := range map[string]func(*notifier.ProducerContext, *string){
		"src": func(p *notifier.ProducerContext, v *string) {
			if v != nil {
				p.Source = *v
			}
		},
		"value_string":     func(p *notifier.ProducerContext, v *string) { p.ValueString = v },
		"old_value_string": func(p *notifier.ProducerContext, v *string) { p.OldValueString = v },
		"calc_expression": func(p *notifier.ProducerContext, v *string) {
			if v != nil {
				p.CalcExpression = *v
			}
		},
		"calc_param_values": func(p *notifier.ProducerContext, v *string) {
			if v != nil {
				p.CalcParamValues = *v
			}
		},
		"total_warn_alarms": func(p *notifier.ProducerContext, v *string) {
			if v != nil {
				p.TotalWarnAlarms = *v
			}
		},
		"total_crit_alarms": func(p *notifier.ProducerContext, v *string) {
			if v != nil {
				p.TotalCritAlarms = *v
			}
		},
		"classification": func(p *notifier.ProducerContext, v *string) {
			if v != nil {
				p.Classification = *v
			}
		},
		"edit_command_line": func(p *notifier.ProducerContext, v *string) {
			if v != nil {
				p.EditCommandLine = *v
			}
		},
		"child_machine_guid": func(p *notifier.ProducerContext, v *string) {
			if v != nil {
				p.ChildMachineGUID = *v
			}
		},
		"transition_id": func(p *notifier.ProducerContext, v *string) {
			if v != nil {
				p.TransitionID = *v
			}
		},
		"component": func(p *notifier.ProducerContext, v *string) {
			if v != nil {
				p.Component = *v
			}
		},
		"type": func(p *notifier.ProducerContext, v *string) {
			if v != nil {
				p.Type = *v
			}
		},
	} {
		for name, tt := range map[string]struct {
			input string
			want  *string
			err   string
		}{
			"omitted": {}, "null": {input: "null"}, "empty": {input: `""`, want: new("")},
			"literal": {input: `"  synthetic-private-value λ\n'$value' $(literal) {{unknown}} ${env:UNREAD}  "`, want: new("  synthetic-private-value λ\n'$value' $(literal) {{unknown}} ${env:UNREAD}  ")},
			"NUL":     {input: `"synthetic-private-value\u0000suffix"`, err: "must not contain NUL"},
			"number":  {input: "1", err: "invalid JSON event"}, "boolean": {input: "true", err: "invalid JSON event"},
			"array": {input: "[]", err: "invalid JSON event"}, "object": {input: "{}", err: "invalid JSON event"},
		} {
			t.Run(field+"/"+name, func(t *testing.T) {
				input := "{}"
				if tt.input != "" {
					input = `{"` + field + `":` + tt.input + `}`
				}
				got, err := readNotification(strings.NewReader(testutil.WithProducerContext(testutil.ValidEvent, input)))
				if tt.err != "" {
					require.ErrorContains(t, err, tt.err)
					assert.NotContains(t, err.Error(), "synthetic-private-value")
					assert.Equal(t, notifier.Notification{}, got)
					return
				}
				require.NoError(t, err)
				want := notifier.Notification{Event: testutil.ExpectedEvent()}
				set(&want.ProducerContext, tt.want)
				assert.Equal(t, want, got)
			})
		}
	}
}

func TestRunProducerContextPreflight(t *testing.T) {
	tests := map[string]struct{ input, err string }{
		"bad number":    {input: `{"unique_id":"synthetic-private-value"}`, err: "invalid JSON event"},
		"unknown field": {input: `{"synthetic-private-value":true}`, err: "invalid JSON event"},
		"NUL":           {input: `{"src":"synthetic-private-value\u0000"}`, err: "producer_context.src must not contain NUL"},
	}
	for name, tt := range tests {
		tt.input = testutil.WithProducerContext(testutil.ValidEvent, tt.input)
		tests[name] = tt
	}
	for name, input := range duplicateProducerContextInputs(t) {
		tests["duplicate "+name] = struct{ input, err string }{input: input, err: "invalid JSON event"}
	}
	for name, tt := range tests {
		for _, mode := range []string{"send", "send-legacy"} {
			t.Run(mode+"/"+name, func(t *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(200) }))
				defer server.Close()
				cfg := "version: 1\ndestinations:\n  target:\n    type: webhook\n    url: " + server.URL + "\nrouting:\n  default: [target]\n"
				if mode == "send-legacy" {
					cfg = "DISCORD_WEBHOOK_URL='" + server.URL + "'; DEFAULT_RECIPIENT_DISCORD=channel"
				}
				var stdout, stderr bytes.Buffer
				input := tt.input
				assert.Equal(t, 1, Run(context.Background(), []string{mode, "--config", writeConfig(t, cfg), "--role", "ops"}, strings.NewReader(input), &stdout, &stderr))
				assert.Empty(t, stdout.String())
				assert.Contains(t, stderr.String(), tt.err)
				assert.NotContains(t, stderr.String(), "synthetic-private-value")
				assert.NotContains(t, stderr.String(), "destination ")
				assert.Zero(t, calls.Load())
			})
		}
	}
}

func duplicateProducerContextInputs(t *testing.T) map[string]string {
	t.Helper()
	tests := map[string]string{
		"object then null":           `"producer_context":{"src":"synthetic-private-value","unique_id":42,"value_string":"stale"},"producer_context":null`,
		"object then empty":          `"producer_context":{"src":"synthetic-private-value"},"producer_context":{}`,
		"object then object":         `"producer_context":{"src":"synthetic-private-value"},"producer_context":{"unique_id":42}`,
		"null then object":           `"producer_context":null,"producer_context":{"src":"synthetic-private-value"}`,
		"invalid type before null":   `"producer_context":{"unique_id":"synthetic-private-value"},"producer_context":null`,
		"unknown before null":        `"producer_context":{"synthetic-private-value":true},"producer_context":null`,
		"invalid member before null": `"producer_context":{"unique_id":"synthetic-private-value","unique_id":null}`,
		"two nulls":                  `"producer_context":null,"producer_context":null`,
		"context case variant":       `"producer_context":{"src":"synthetic-private-value"},"PRODUCER_CONTEXT":null`,
		"escaped context":            `"producer_context":{"src":"synthetic-private-value"},"producer_\u0063ontext":null`,
		"member case variant":        `"producer_context":{"src":"synthetic-private-value","SRC":null}`,
		"escaped member":             `"producer_context":{"src":"synthetic-private-value","s\u0072c":null}`,
		"member Unicode fold":        `"producer_context":{"src":"synthetic-private-value","ſrc":null}`,
		"member null first":          `"producer_context":{"src":null,"src":"synthetic-private-value"}`,
		"identical member values":    `"producer_context":{"src":"synthetic-private-value","src":"synthetic-private-value"}`,
	}
	var members map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(testutil.ProducerContextJSON), &members))
	for field, value := range members {
		tests[field+" then null"] = `"producer_context":{"` + field + `":` + string(value) + `,"` + field + `":null}`
	}
	for name, fields := range tests {
		tests[name] = strings.TrimSuffix(testutil.ValidEvent, "}") + "," + fields + "}"
	}
	return tests
}

func TestReadProducerContextDuplicates(t *testing.T) {
	for name, input := range duplicateProducerContextInputs(t) {
		t.Run(name, func(t *testing.T) {
			got, err := readNotification(strings.NewReader(input))
			require.ErrorContains(t, err, "invalid JSON event")
			assert.NotContains(t, err.Error(), "synthetic-private-value")
			assert.Equal(t, notifier.Notification{}, got)
		})
	}
}
