// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"encoding/json"
	"errors"
	"io"
	"strings"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
)

func readEvent(r io.Reader) (notifyevent.Event, error) {
	var event notifyevent.Event
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return notifyevent.Event{}, errors.New("invalid JSON event: check syntax, field names, types, and timestamp")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return notifyevent.Event{}, errors.New("input must contain exactly one JSON event")
	}
	if event.Version != 1 {
		return notifyevent.Event{}, errors.New("event version must be 1")
	}
	if event.URL != "" && !httpclient.ValidURL(event.URL, true) {
		return notifyevent.Event{}, errors.New("event url must be an absolute HTTP(S) URL without user information")
	}
	if strings.TrimSpace(event.IncidentID) == "" || strings.TrimSpace(event.Node) == "" ||
		strings.TrimSpace(event.Alert) == "" || strings.TrimSpace(event.Summary) == "" || event.Timestamp.IsZero() {
		return notifyevent.Event{}, errors.New("event requires incident_id, timestamp, node, alert, and summary")
	}
	switch event.Status {
	case "WARNING", "CRITICAL", "CLEAR":
	default:
		return notifyevent.Event{}, errors.New("event status must be WARNING, CRITICAL, or CLEAR")
	}
	switch event.PreviousStatus {
	case "", "UNINITIALIZED", "UNDEFINED", "REMOVED", "CLEAR", "WARNING", "CRITICAL":
	default:
		return notifyevent.Event{}, errors.New("event previous_status is not a recognized alert status")
	}
	return event, nil
}
