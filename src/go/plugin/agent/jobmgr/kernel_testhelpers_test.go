// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import "context"

type runShutdownBarrierFunc func(context.Context, uint64) error

func (fn runShutdownBarrierFunc) BeforeFunctionCatalogClose(
	ctx context.Context,
	generation uint64,
) error {
	return fn(ctx, generation)
}

type runFinalizerFunc func(context.Context, uint64) error

func (fn runFinalizerFunc) FinalizeRun(ctx context.Context, generation uint64) error {
	return fn(ctx, generation)
}

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
	return runFinalizerFunc(func(context.Context, uint64) error { return nil })
}

func newNoopRunShutdownBarrier() RunShutdownBarrier {
	return runShutdownBarrierFunc(func(context.Context, uint64) error { return nil })
}
