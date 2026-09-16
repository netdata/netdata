// SPDX-License-Identifier: GPL-3.0-or-later

package testutil

import (
	"time"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
)

const ValidEvent = `{
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

func ExpectedEvent() notifyevent.Event {
	value, previous := 42.5, 0.0
	return notifyevent.Event{
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
