// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

// A late preflight failure follows the same source-specific rejection/retry
// policy as an invalid candidate returned from Check.
type candidatePreflightError struct{ cause error }

func (e *candidatePreflightError) Error() string { return e.cause.Error() }
func (e *candidatePreflightError) Unwrap() error { return e.cause }

// candidateAdoption retains the checked candidate until the graph transaction
// transfers it to accepted activation. Dispose and a rolled-back Apply release
// it; successful acceptance does not wait for the previous runtime's cleanup.
type candidateAdoption struct {
	lifecycle.PreparedResourceTransaction
	mu       sync.Mutex
	consumed bool
	stage    *preparedJobCandidate
}

func (a *candidateAdoption) consume() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.consumed {
		return errors.New("job output: candidate adoption consumed")
	}
	a.consumed = true
	return nil
}

func (a *candidateAdoption) release() {
	a.stage.Release()
	a.stage = nil
}

func (a *candidateAdoption) Apply(ctx context.Context) (lifecycle.AppliedResourceTransaction, error) {
	if err := a.consume(); err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	defer a.release()
	return a.PreparedResourceTransaction.Apply(ctx)
}

func (a *candidateAdoption) Dispose(ctx context.Context) (lifecycle.ReadyResource, error) {
	if err := a.consume(); err != nil {
		return nil, err
	}
	defer a.release()
	return a.PreparedResourceTransaction.Dispose(ctx)
}

func (dcjc *DynCfgJobController) prepareCandidateAdoption(
	ctx context.Context,
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	permit lifecycle.LongLivedPermit,
	stage *preparedJobCandidate,
	config confgroup.Config,
	postimage *dyncfg.GraphConfig,
	reply jobReply,
	cleanup lifecycle.TaskCleanup,
	afterApply func(),
) (lifecycle.PreparedResourceTransaction, error) {
	adoption := &candidateAdoption{
		stage: stage,
	}
	result, err := stage.inspect()
	if err != nil {
		adoption.release()
		return nil, &candidatePreflightError{cause: err}
	}
	if reply.candidate == nil {
		reply.candidate, err = dcjc.candidates.prepareAdoptExact(config, true, dyncfg.StatusAccepted, true, 0, dcjc.scheduler.retries.now())
		if err != nil {
			adoption.release()
			return nil, err
		}
	}
	reply.retainedCandidate = true
	spec, err := dcjc.configModules.newAcceptedActivationSpec(config)
	if err != nil {
		adoption.release()
		return nil, err
	}
	spec.work = reply.candidate.after.Active
	handoff := func() {
		stage := adoption.stage
		adoption.stage = nil
		dcjc.scheduler.accepted.startExecution(spec,
			dcjc.activationRelease(scope.ID, jobmgr.ProcessAttemptJobRuntime), stage)
	}
	prepared, err := dcjc.prepareMutationAfterApplyAndFallback(
		ctx,
		scope, current, nil, permit, resourceRemovalDisposition(current), postimage, reply, cleanup,
		composeAfterApply(afterApply, handoff),
		preparedJobConfigLifecycle{
			identity: result.candidate.jobConfigIdentity,
			snapshot: result.candidate.jobConfigSnapshot,
		}, jobConfigIntentEnable,
		nil, nil, stage,
	)
	if err != nil {
		adoption.release()
		return nil, err
	}
	adoption.PreparedResourceTransaction = prepared
	return adoption, nil
}
