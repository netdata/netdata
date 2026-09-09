// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"

func (ck *CommandKernel) observe(event DiagnosticEvent) {
	if ck == nil {
		return
	}
	if event.Generation == 0 {
		event.Generation = ck.run.Generation()
	}
	ObserveDiagnostic(ck.diagnosticObserver, event)
}

func (ck *CommandKernel) recordAbandonment(
	resource string,
	ref lifecycle.TaskRef,
	abandonment lifecycle.TaskAbandonment,
) {
	ck.abandoned.Record(abandonment)
	state := abandonment.Outcome.String()
	if abandonment.Outcome == lifecycle.TaskOutcomeNone && abandonment.Cleanup {
		state = "cleanup"
	}
	if abandonment.Outcome == lifecycle.TaskOutcomeNone &&
		!abandonment.Cleanup &&
		abandonment.LongLivedPermits != 0 {
		state = "long-lived permit"
	}
	ck.observe(DiagnosticEvent{
		Level:    DiagnosticError,
		Name:     "job manager task ownership abandoned",
		Resource: resource,
		State:    state,
		Task:     ref,
		Count:    abandonment.OwnershipCount(),
	})
}
