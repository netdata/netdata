// SPDX-License-Identifier: GPL-3.0-or-later

package app

import (
	"encoding/json"
	"errors"
	"io"
	"strings"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
)

func readNotification(r io.Reader) (notifier.Notification, error) {
	var input struct {
		notifyevent.Event
		CriticalSeenSinceClear *bool `json:"critical_seen_since_clear"`
	}
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return notifier.Notification{}, errors.New("invalid JSON event: check syntax, field names, types, and timestamp")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return notifier.Notification{}, errors.New("input must contain exactly one JSON event")
	}
	event := input.Event
	if event.Version != 1 {
		return notifier.Notification{}, errors.New("event version must be 1")
	}
	if event.URL != "" && !httpclient.ValidURL(event.URL, true) {
		return notifier.Notification{}, errors.New("event url must be an absolute HTTP(S) URL without user information")
	}
	if strings.TrimSpace(event.IncidentID) == "" || strings.TrimSpace(event.Node) == "" ||
		strings.TrimSpace(event.Alert) == "" || strings.TrimSpace(event.Summary) == "" || event.Timestamp.IsZero() {
		return notifier.Notification{}, errors.New("event requires incident_id, timestamp, node, alert, and summary")
	}
	switch event.Status {
	case "WARNING", "CRITICAL", "CLEAR":
	default:
		return notifier.Notification{}, errors.New("event status must be WARNING, CRITICAL, or CLEAR")
	}
	switch event.PreviousStatus {
	case "", "UNINITIALIZED", "UNDEFINED", "REMOVED", "CLEAR", "WARNING", "CRITICAL":
	default:
		return notifier.Notification{}, errors.New("event previous_status is not a recognized alert status")
	}
	return notifier.Notification{Event: event, CriticalSeenSinceClear: input.CriticalSeenSinceClear}, nil
}
