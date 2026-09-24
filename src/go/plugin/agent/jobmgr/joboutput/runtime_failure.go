// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
)

// Only this positive classification may take the operational startup fallback.
// Arbitrary runtime/ownership errors must still fail the transaction closed.
type runtimeStartupFailure struct{ failure *autoDetectionFailure }

func (e *runtimeStartupFailure) Error() string { return e.failure.Error() }
func (e *runtimeStartupFailure) Unwrap() error { return e.failure }

func runtimeFailureFor(resources ConstructedJob, err *jobruntime.RunFailure, stage string) *autoDetectionFailure {
	failure := autoDetectionFailureFor(resources, err)
	failure.class = err.Class()
	failure.retry = failure.retry && err.Retryable()
	failure.runtime = true
	failure.diagnosticFailure = jobConfigFailure(err, stage)
	failure.diagnosticFailure.Reason = err.Reason()
	failure.jobConfigLifecycle = resources.jobConfigSnapshot
	return failure
}

func (dcjc *DynCfgJobController) bindRuntimeFailures(commands jobmgr.PreparedCommandPort, run uint64, fail func(error)) {
	dcjc.factory.notifyRunReady = func(identity lifecycle.ResourceIdentity) {
		err := commands.SubmitPrepared(context.Background(), jobmgr.Request{
			UID:     fmt.Sprintf("jobmgr-run-ready-%d-%x-%d", run, jobAttemptIdentity(jobmgr.ProcessAttemptJobRuntime, identity.ID).Key, identity.Generation),
			LaneKey: identity.ID,
			Source:  lifecycle.SourceJobManager,
			Route:   "internal/jobs/runtime-ready",
		}, dcjc.planRuntimeReady(identity))
		if err != nil && !lifecycle.ContainsOnlyCurrentStoppingRejections(err, run) {
			fail(err)
		}
	}
	dcjc.factory.notifyRunFailure = func(identity lifecycle.ResourceIdentity, failure *jobruntime.RunFailure) {
		plan := dcjc.planRuntimeFailure(identity, failure)
		err := commands.SubmitPrepared(context.Background(), jobmgr.Request{
			UID:     fmt.Sprintf("jobmgr-run-failure-%d-%x-%d", run, jobAttemptIdentity(jobmgr.ProcessAttemptJobRuntime, identity.ID).Key, identity.Generation),
			LaneKey: identity.ID,
			Source:  lifecycle.SourceJobManager,
			Route:   "internal/jobs/runtime-failure",
		}, plan)
		if err != nil && !lifecycle.ContainsOnlyCurrentStoppingRejections(err, run) {
			fail(err)
		}
	}
}

func (dcjc *DynCfgJobController) planRuntimeFailure(identity lifecycle.ResourceIdentity, _ *jobruntime.RunFailure) jobmgr.WorkPlan {
	// The event wakes settlement; the current generation supplies the outcome.
	return dcjc.planRuntimeSettlement(identity, true)
}

// Do not absorb a joined structural or cleanup error merely because errors.As
// can also find an operational startup failure elsewhere in the tree.
func onlyRuntimeStartupFailure(err error) (*runtimeStartupFailure, bool) {
	if failure, ok := err.(*runtimeStartupFailure); ok {
		return failure, true
	}
	if many, ok := err.(interface{ Unwrap() []error }); ok {
		var found *runtimeStartupFailure
		for _, child := range many.Unwrap() {
			failure, ok := onlyRuntimeStartupFailure(child)
			if !ok || (found != nil && found != failure) {
				return nil, false
			}
			found = failure
		}
		return found, found != nil
	}
	if one, ok := err.(interface{ Unwrap() error }); ok {
		return onlyRuntimeStartupFailure(one.Unwrap())
	}
	return nil, false
}
