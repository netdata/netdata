// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

// Stable categories let the production sink bound repeated poll diagnostics
// without keeping per-job or per-error logging state.
const (
	DiagnosticAvailabilityCallbackFailed       = "Function availability callback failed"
	DiagnosticAvailabilityAttemptFailed        = "Function availability attempt failed"
	DiagnosticAvailabilityReconciliationFailed = "Function availability reconciliation failed"
)

func (c *Controller) observeAvailabilityFailure(name string, bundle *functionBundle, err error) {
	if c == nil || c.diagnostics == nil || err == nil {
		return
	}
	level := jobmgr.DiagnosticWarning
	if name == DiagnosticAvailabilityReconciliationFailed {
		level = jobmgr.DiagnosticError
	}
	state := "failed"
	if errors.Is(err, lifecycle.ErrTaskPanic) {
		state = "panic"
	}
	// Callback errors can contain recovered values or credentials. Keep only
	// a fixed classification and the bundle's already-sanitized identity.
	jobmgr.ObserveDiagnostic(c.diagnostics, jobmgr.DiagnosticEvent{
		Name:       name,
		Resource:   bundle.resource,
		Generation: c.epoch,
		State:      state,
		Level:      level,
	})
}
