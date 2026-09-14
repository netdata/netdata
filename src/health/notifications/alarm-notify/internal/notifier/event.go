// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

// Event is the experimental webhook's public document. Add internal-only facts separately.
type Event struct {
	Version        int       `json:"version"`
	IncidentID     string    `json:"incident_id"`
	Timestamp      time.Time `json:"timestamp"`
	Node           string    `json:"node"`
	Alert          string    `json:"alert"`
	Chart          string    `json:"chart,omitempty"`
	Context        string    `json:"context,omitempty"`
	Status         string    `json:"status"`
	PreviousStatus string    `json:"previous_status,omitempty"`
	Summary        string    `json:"summary"`
	Info           string    `json:"info,omitempty"`
	Value          *float64  `json:"value"`
	PreviousValue  *float64  `json:"previous_value"`
	Units          string    `json:"units,omitempty"`
	URL            string    `json:"url,omitempty"`
}

func readEvent(r io.Reader) (Event, error) {
	var event Event
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return Event{}, errors.New("invalid JSON event: check syntax, field names, types, and timestamp")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return Event{}, errors.New("input must contain exactly one JSON event")
	}
	if event.Version != 1 {
		return Event{}, errors.New("event version must be 1")
	}
	if event.URL != "" && !validHTTPURL(event.URL, true) {
		return Event{}, errors.New("event url must be an absolute HTTP(S) URL without user information")
	}
	if strings.TrimSpace(event.IncidentID) == "" || strings.TrimSpace(event.Node) == "" ||
		strings.TrimSpace(event.Alert) == "" || strings.TrimSpace(event.Summary) == "" || event.Timestamp.IsZero() {
		return Event{}, errors.New("event requires incident_id, timestamp, node, alert, and summary")
	}
	switch event.Status {
	case "WARNING", "CRITICAL", "CLEAR":
	default:
		return Event{}, errors.New("event status must be WARNING, CRITICAL, or CLEAR")
	}
	switch event.PreviousStatus {
	case "", "UNINITIALIZED", "UNDEFINED", "REMOVED", "CLEAR", "WARNING", "CRITICAL":
	default:
		return Event{}, errors.New("event previous_status is not a recognized alert status")
	}
	return event, nil
}
