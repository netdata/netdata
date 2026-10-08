// SPDX-License-Identifier: GPL-3.0-or-later

package legacydelivery

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventValues(t *testing.T) {
	base := map[string]string{
		"roles": "ops dba", "host": "test-node", "args_host": "test-node", "when": "1789387200",
		"name": "test_alert", "chart": "test.chart", "context": "test.context", "status": "WARNING", "old_status": "CLEAR",
		"units": "C", "info": "A quote: \"hot\"\nUnicode: θερμοκρασία", "summary": "Temperature is high",
		"value": "42.5", "old_value": "0", "duration": "", "non_clear_duration": "",
		"date": "2026-09-14T12:00:00Z", "status_message": "needs attention", "value_string": "42.5 C", "old_value_string": "0 C",
		"unique_id": "", "alarm_id": "", "event_id": "", "src": "", "calc_expression": "", "calc_param_values": "",
		"total_warnings": "", "total_critical": "", "total_warn_alarms": "", "total_crit_alarms": "",
		"classification": "", "edit_command_line": "", "child_machine_guid": "", "transition_id": "", "component": "", "type": "",
	}
	for name, tt := range map[string]struct {
		input         string
		unknownValues bool
		want          map[string]string
	}{
		"omitted": {}, "null": {input: "null"},
		"full context": {input: testutil.ProducerContextJSON, want: map[string]string{
			"unique_id": "42", "alarm_id": "7", "event_id": "3", "src": "line=12,file=/etc/netdata/health.d/example.conf",
			"value_string": "42.50 °C", "old_value_string": "0.00 °C", "calc_expression": "$this > 40", "calc_param_values": "$this = 42.5",
			"total_warnings": "2", "total_critical": "0", "total_warn_alarms": "other_alert=123,another_alert=456", "total_crit_alarms": "",
			"classification": "System", "edit_command_line": "edit-config health.d/example.conf",
			"child_machine_guid": "00000000-0000-0000-0000-000000000001", "transition_id": "00000000-0000-0000-0000-000000000002",
			"component": "Sensors", "type": "Environment",
		}},
		"zero and maximum": {input: `{"unique_id":0,"alarm_id":4294967295,"event_id":0,"total_warnings":0,"total_critical":4294967295}`,
			want: map[string]string{"unique_id": "0", "alarm_id": "4294967295", "event_id": "0", "total_warnings": "0", "total_critical": "4294967295"}},
		"null formatting falls back":        {input: `{"value_string":null,"old_value_string":null}`},
		"empty formatting preserved":        {input: `{"value_string":"","old_value_string":""}`, want: map[string]string{"value_string": "", "old_value_string": ""}},
		"absent numeric values":             {unknownValues: true, want: map[string]string{"value": "", "old_value": "", "value_string": "", "old_value_string": ""}},
		"formatting without numeric values": {unknownValues: true, input: `{"value_string":"unknown","old_value_string":"not collected"}`, want: map[string]string{"value": "", "old_value": "", "value_string": "unknown", "old_value_string": "not collected"}},
		"literal formatting":                {input: `{"value_string":" ${env:UNREAD} $(literal) {{unknown}}\nλ ","old_value_string":"$value"}`, want: map[string]string{"value_string": " ${env:UNREAD} $(literal) {{unknown}}\nλ ", "old_value_string": "$value"}},
	} {
		t.Run(name, func(t *testing.T) {
			n := notifier.Notification{Event: testutil.ExpectedEvent()}
			if tt.input != "" {
				require.NoError(t, json.Unmarshal([]byte(tt.input), &n.ProducerContext))
			}
			if tt.unknownValues {
				n.Event.Value, n.Event.PreviousValue = nil, nil
			}
			want := maps.Clone(base)
			maps.Copy(want, tt.want)
			assert.Equal(t, want, eventValues(n, []string{"ops", "dba"}))
		})
	}
}

func TestPrepareProducerContextMutation(t *testing.T) {
	for field := range map[string]struct{}{
		"unique_id": {}, "alarm_id": {}, "event_id": {}, "src": {}, "value_string": {}, "old_value_string": {},
		"calc_expression": {}, "calc_param_values": {}, "total_warnings": {}, "total_critical": {},
		"total_warn_alarms": {}, "total_crit_alarms": {}, "classification": {}, "edit_command_line": {},
		"child_machine_guid": {}, "transition_id": {}, "component": {}, "type": {},
	} {
		for name, tt := range map[string]struct {
			input   string
			restore bool
		}{
			"omitted": {}, "supplied": {input: testutil.ProducerContextJSON},
			"restored omitted": {restore: true}, "restored supplied": {input: testutil.ProducerContextJSON, restore: true},
		} {
			t.Run(field+"/"+name, func(t *testing.T) {
				n := notifier.Notification{Event: testutil.ExpectedEvent()}
				if tt.input != "" {
					require.NoError(t, json.Unmarshal([]byte(tt.input), &n.ProducerContext))
				}
				cfg := `DISCORD_WEBHOOK_URL=https://example.org; DEFAULT_RECIPIENT_DISCORD=channel; original="$` + field + `"; ` + field + `=synthetic-private-value`
				if tt.restore {
					cfg += `; ` + field + `="$original"`
				}
				plan, names, err := Prepare(context.Background(), parsedPrograms(t, cfg), []string{"discord"}, []string{"ops"}, n, http.DefaultClient, nil)
				if tt.restore {
					require.NoError(t, err)
					assert.Equal(t, []string{"discord-1"}, names)
					assert.Len(t, plan.Destinations, 1)
					return
				}
				require.ErrorContains(t, err, "legacy setting "+field+" is not supported")
				assert.NotContains(t, err.Error(), "synthetic-private-value")
				assert.Equal(t, notifier.Plan{}, plan)
				assert.Nil(t, names)
			})
		}
	}
}
