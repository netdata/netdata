// SPDX-License-Identifier: GPL-3.0-or-later

package collectorapi

import "time"

// JobConfigFailure describes an observed preparation failure using safe categories.
// It must never contain error text, configuration values or source paths.
type JobConfigFailure struct {
	Stage       string    `json:"stage,omitempty"`
	Reason      string    `json:"reason,omitempty"`
	CompletedAt time.Time `json:"completed_at,omitzero"`
}

func (f JobConfigFailure) Valid() bool {
	switch f.Stage {
	case "configuration", "secret_resolution", "vnode", "construction", "autodetection", "functions", "activation", "secret_restart":
	default:
		return false
	}
	switch f.Reason {
	case "unknown",
		"invalid_configuration",
		"result_limit",
		"panic",
		"unavailable",
		"missing_vnode",
		"cycle",
		"depth",
		"reference",
		"provider",
		"scope",
		"shape",
		"busy",
		"deadline",
		"cancelled",
		"quarantined",
		"superseded",
		"stale_secret_generation",
		"dependent_stopped":
	default:
		return false
	}
	return !f.CompletedAt.IsZero()
}

// JobConfigFailureProjector optionally enriches an already detached lifecycle
// snapshot. The returned snapshot must preserve its identity and existing
// collector evidence. Errors before construction enrich Project's snapshot;
// failed probes enrich Capture's snapshot. Publication still follows commit.
// A panic, nil result or changed identity leaves the original snapshot intact.
type JobConfigFailureProjector interface {
	ProjectFailure(JobConfigLifecycleSnapshot, JobConfigFailure) JobConfigLifecycleSnapshot
}
