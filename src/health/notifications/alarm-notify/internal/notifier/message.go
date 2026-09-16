// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"strconv"
	"strings"
	"time"
)

type notificationField struct {
	name  string
	value string
}

// notificationFields supplies shared content; each provider owns escaping and presentation.
func notificationFields(event Event) []notificationField {
	status := event.Status
	if event.PreviousStatus != "" && event.PreviousStatus != event.Status {
		status = event.PreviousStatus + " → " + event.Status
	}
	fields := []notificationField{{"Node", event.Node}, {"Alert", event.Alert}, {"Status", status}}
	if event.Chart != "" {
		fields = append(fields, notificationField{"Chart", event.Chart})
	}
	if event.Context != "" {
		fields = append(fields, notificationField{"Context", event.Context})
	}
	for _, value := range []struct {
		label string
		value *float64
	}{{"Value", event.Value}, {"Previous value", event.PreviousValue}} {
		if value.value != nil {
			formatted := strconv.FormatFloat(*value.value, 'g', -1, 64)
			if event.Units != "" {
				formatted += " " + event.Units
			}
			fields = append(fields, notificationField{value.label, formatted})
		}
	}
	return fields
}

func notificationPlainText(event Event, includeURL bool) string {
	lines := []string{event.Summary}
	if event.Info != "" {
		lines = append(lines, event.Info)
	}
	for _, field := range notificationFields(event) {
		lines = append(lines, field.name+": "+field.value)
	}
	lines = append(lines, "Time: "+event.Timestamp.Format(time.RFC3339))
	if includeURL && event.URL != "" {
		lines = append(lines, event.URL)
	}
	return strings.Join(lines, "\n")
}
