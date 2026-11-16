// SPDX-License-Identifier: GPL-3.0-or-later

package event

import "time"

// Event is the experimental webhook's public document. Add internal-only facts separately.
type Event struct {
	Version          int       `json:"version"`
	IncidentID       string    `json:"incident_id"`
	Timestamp        time.Time `json:"timestamp"`
	Node             string    `json:"node"`
	Alert            string    `json:"alert"`
	Chart            string    `json:"chart,omitempty"`
	Context          string    `json:"context,omitempty"`
	Status           string    `json:"status"`
	PreviousStatus   string    `json:"previous_status,omitempty"`
	Summary          string    `json:"summary"`
	Info             string    `json:"info,omitempty"`
	Value            *float64  `json:"value"`
	PreviousValue    *float64  `json:"previous_value"`
	Duration         *uint32   `json:"duration,omitempty"`
	NonClearDuration *uint32   `json:"non_clear_duration,omitempty"`
	Units            string    `json:"units,omitempty"`
	URL              string    `json:"url,omitempty"`
}
