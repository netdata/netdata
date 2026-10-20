// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
)

// Notification separates input-only facts from the public provider event.
type Notification struct {
	Event                  event.Event
	CriticalSeenSinceClear *bool
	ProducerContext        ProducerContext
}

// ProducerContext carries facts for legacy settings and custom functions, not provider payloads.
// Nil numbers mean unknown. Nil formatted values use the Event's numeric value and units.
type ProducerContext struct {
	UniqueID         *uint32 `json:"unique_id,omitempty"`
	AlarmID          *uint32 `json:"alarm_id,omitempty"`
	EventID          *uint32 `json:"event_id,omitempty"`
	Source           string  `json:"src,omitempty"`
	ValueString      *string `json:"value_string,omitempty"`
	OldValueString   *string `json:"old_value_string,omitempty"`
	CalcExpression   string  `json:"calc_expression,omitempty"`
	CalcParamValues  string  `json:"calc_param_values,omitempty"`
	TotalWarnings    *uint32 `json:"total_warnings,omitempty"`
	TotalCritical    *uint32 `json:"total_critical,omitempty"`
	TotalWarnAlarms  string  `json:"total_warn_alarms,omitempty"`
	TotalCritAlarms  string  `json:"total_crit_alarms,omitempty"`
	Classification   string  `json:"classification,omitempty"`
	EditCommandLine  string  `json:"edit_command_line,omitempty"`
	ChildMachineGUID string  `json:"child_machine_guid,omitempty"`
	TransitionID     string  `json:"transition_id,omitempty"`
	Component        string  `json:"component,omitempty"`
	Type             string  `json:"type,omitempty"`
}

// Validate rejects bytes that cannot be represented in legacy shell variables.
func (p ProducerContext) Validate() error {
	for name, value := range map[string]*string{
		"src": &p.Source, "value_string": p.ValueString, "old_value_string": p.OldValueString,
		"calc_expression": &p.CalcExpression, "calc_param_values": &p.CalcParamValues,
		"total_warn_alarms": &p.TotalWarnAlarms, "total_crit_alarms": &p.TotalCritAlarms,
		"classification": &p.Classification, "edit_command_line": &p.EditCommandLine,
		"child_machine_guid": &p.ChildMachineGUID, "transition_id": &p.TransitionID,
		"component": &p.Component, "type": &p.Type,
	} {
		if value != nil && strings.ContainsRune(*value, 0) {
			return fmt.Errorf("producer_context.%s must not contain NUL", name)
		}
	}
	return nil
}
