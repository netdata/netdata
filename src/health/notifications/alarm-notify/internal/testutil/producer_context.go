// SPDX-License-Identifier: GPL-3.0-or-later

package testutil

import "strings"

const ProducerContextJSON = `{
 "unique_id": 42, "alarm_id": 7, "event_id": 3,
 "src": "line=12,file=/etc/netdata/health.d/example.conf",
 "value_string": "42.50 °C", "old_value_string": "0.00 °C",
 "calc_expression": "$this > 40", "calc_param_values": "$this = 42.5",
 "total_warnings": 2, "total_critical": 0,
 "total_warn_alarms": "other_alert=123,another_alert=456", "total_crit_alarms": "",
 "classification": "System", "edit_command_line": "edit-config health.d/example.conf",
 "child_machine_guid": "00000000-0000-0000-0000-000000000001",
 "transition_id": "00000000-0000-0000-0000-000000000002",
 "component": "Sensors", "type": "Environment"
}`

func WithProducerContext(input, contextJSON string) string {
	if contextJSON == "" {
		return input
	}
	return strings.Replace(input, "{", `{"producer_context":`+contextJSON+`,`, 1)
}
