// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import "context"

// admit is a test-only convenience that submits an already-prepared command
// through the kernel admission path with no input-body reservation.
func (ck *CommandKernel) admit(
	request Request,
	plan WorkPlan,
	submissionContext context.Context,
	submissionResult,
	terminalResult chan error,
) error {
	return ck.admitSubmission(
		request,
		plan,
		submissionContext,
		submissionResult,
		terminalResult,
		nil,
		false,
	)
}

// newNoopRunFinalizer and newNoopRunShutdownBarrier build inert run-lifecycle
// callbacks used only by tests. Production wires real callbacks and detects the
// inert forms with isNoopRunFinalizer / isNoopRunShutdownBarrier.
func newNoopRunFinalizer() RunFinalizer             { return noopRunFinalizer{} }
func newNoopRunShutdownBarrier() RunShutdownBarrier { return noopRunShutdownBarrier{} }
