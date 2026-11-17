// SPDX-License-Identifier: GPL-3.0-or-later

package synthetic

// Availability says whether the runtime agreed to report an optional output.
type Availability string

const (
	// FeatureUnavailable means the output was requested but never acknowledged.
	FeatureUnavailable Availability = "unavailable"
	// FeatureUnsupported means the runtime acknowledged and declined it.
	FeatureUnsupported Availability = "unsupported"
	FeatureSupported   Availability = "supported"
	// FeatureInvalid means its records or the shared framing were malformed;
	// records accepted before that remain.
	FeatureInvalid Availability = "invalid"
)

// NotRecorded is the reporting status of an output absent from a run record.
const NotRecorded = "not_recorded"

// FeatureStatus is the negotiation and reporting state of one optional output.
// Truncation and outcome are independent of it.
type FeatureStatus struct {
	Availability     Availability `json:"availability"`
	Reason           string       `json:"reason,omitempty"`
	ReporterComplete bool         `json:"reporter_complete"`
}

// ReportingStatus distinguishes unavailable, declined or invalid output from a
// partial or complete reporter stream.
func (s FeatureStatus) ReportingStatus() string {
	switch {
	case s.Availability != FeatureSupported:
		return string(s.Availability)
	case s.ReporterComplete:
		return "complete"
	default:
		return "partial"
	}
}
