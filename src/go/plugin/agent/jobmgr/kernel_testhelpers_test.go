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

func newNoopRunFinalizer() RunFinalizer {
	return RunFinalizerFunc(func(context.Context, uint64) error { return nil })
}

func newNoopRunShutdownBarrier() RunShutdownBarrier {
	return RunShutdownBarrierFunc(func(context.Context, uint64) error { return nil })
}
