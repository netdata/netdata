// SPDX-License-Identifier: GPL-3.0-or-later

package message

import (
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
)

// Field is unescaped notification content; providers own its presentation.
type Field struct {
	Name  string
	Value string
}

// Fields supplies shared content; each provider owns escaping and presentation.
func Fields(event event.Event) []Field {
	status := event.Status
	if event.PreviousStatus != "" && event.PreviousStatus != event.Status {
		status = event.PreviousStatus + " → " + event.Status
	}
	fields := []Field{{"Node", event.Node}, {"Alert", event.Alert}, {"Status", status}}
	if event.Chart != "" {
		fields = append(fields, Field{"Chart", event.Chart})
	}
	if event.Context != "" {
		fields = append(fields, Field{"Context", event.Context})
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
			fields = append(fields, Field{value.label, formatted})
		}
	}
	return fields
}

// PlainText renders the shared plain-text notification content.
func PlainText(event event.Event, includeURL bool) string {
	lines := []string{event.Summary}
	if event.Info != "" {
		lines = append(lines, event.Info)
	}
	for _, field := range Fields(event) {
		lines = append(lines, field.Name+": "+field.Value)
	}
	lines = append(lines, "Time: "+event.Timestamp.Format(time.RFC3339))
	if includeURL && event.URL != "" {
		lines = append(lines, event.URL)
	}
	return strings.Join(lines, "\n")
}

// EscapeControls keeps controls out of newline-framed logs and terminal displays.
func EscapeControls(text string) string {
	var escaped strings.Builder
	escaped.Grow(len(text))
	for _, r := range text {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			quoted := strconv.QuoteRune(r)
			escaped.WriteString(quoted[1 : len(quoted)-1])
		} else {
			escaped.WriteRune(r)
		}
	}
	return escaped.String()
}

// StatusColor is the severity palette used by chat webhook messages.
func StatusColor(status string) string {
	switch status {
	case "WARNING":
		return "#f0ad4e"
	case "CRITICAL":
		return "#d9534f"
	default:
		return "#5cb85c"
	}
}
