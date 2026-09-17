// SPDX-License-Identifier: GPL-3.0-or-later

package legacydelivery

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
)

func eventValues(e event.Event, roles []string) map[string]string {
	values := map[string]string{
		"roles": strings.Join(roles, " "), "host": e.Node, "args_host": e.Node,
		"when": strconv.FormatInt(e.Timestamp.Unix(), 10), "name": e.Alert, "chart": e.Chart,
		"context": e.Context, "status": e.Status, "old_status": e.PreviousStatus,
		"units": e.Units, "info": e.Info, "summary": e.Summary,
		"value": "", "old_value": "", "duration": "", "non_clear_duration": "",
	}
	if e.Value != nil {
		values["value"] = strconv.FormatFloat(*e.Value, 'g', -1, 64)
	}
	if e.PreviousValue != nil {
		values["old_value"] = strconv.FormatFloat(*e.PreviousValue, 'g', -1, 64)
	}
	if e.Duration != nil {
		values["duration"] = strconv.FormatUint(uint64(*e.Duration), 10)
	}
	if e.NonClearDuration != nil {
		values["non_clear_duration"] = strconv.FormatUint(uint64(*e.NonClearDuration), 10)
	}
	return values
}

func initialValues(e event.Event, roles []string) map[string]string {
	values := eventValues(e, roles)
	for _, m := range methods {
		if !m.global {
			values["SEND_"+strings.ToUpper(m.name)] = "YES"
		}
	}
	values["SEND_KAFKA"] = "YES"
	values["SEND_DYNATRACE"] = ""
	values["SMSEAGLE_MSG_TYPE"] = "sms"
	values["SMSEAGLE_CALL_DURATION"] = "10"
	values["SMSEAGLE_VOICE_ID"] = "1"
	values["IRC_PORT"] = "6667"
	values["FLEEP_SENDER"] = e.Node
	return values
}

func unsupportedSettings(values, initial map[string]string, eligible []method) error {
	unsupported := func(key string) error {
		return fmt.Errorf("legacy setting %s is not supported for this delivery; use native event/configuration settings", key)
	}
	// These presentation and event mutations have no mapping to the current event contract.
	for _, key := range []string{"date_format", "images_base_url"} {
		if values[key] != "" {
			return unsupported(key)
		}
	}
	for _, key := range []string{"use_fqdn", "clear_alarm_always"} {
		if values[key] == "YES" {
			return unsupported(key)
		}
	}
	for _, key := range []string{"roles", "host", "args_host", "when", "name", "chart", "context", "status", "old_status", "units", "info", "summary", "value", "old_value", "duration", "non_clear_duration"} {
		if values[key] != initial[key] {
			return unsupported(key)
		}
	}
	for _, m := range eligible {
		if m.tool == "" {
			for _, key := range []string{"curl", "curl_options"} {
				if strings.TrimSpace(values[key]) != "" {
					return unsupported(key)
				}
			}
		}
		if m.name == "email" {
			charset := strings.ToLower(values["EMAIL_CHARSET"])
			if charset != "" && charset != "utf-8" && charset != "utf8" {
				return unsupported("EMAIL_CHARSET")
			}
		}
	}
	return nil
}

func integer(value, name string) (*field.Integer, error) {
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%s must be an integer", name)
	}
	return new(field.Integer(parsed)), nil
}

func shellFields(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' })
}
