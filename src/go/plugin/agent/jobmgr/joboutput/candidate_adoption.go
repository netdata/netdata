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
		return nil, err
	}
	spec, err := newAcceptedActivationSpec(config)
	if err != nil {
		adoption.release()
		return nil, err
	}
	handoff := func() {
		stage := adoption.stage
		adoption.stage = nil
		dcjc.scheduler.accepted.armWithGate(spec,
			dcjc.activationRelease(scope.ID, jobmgr.ProcessAttemptJobRuntime), nil, stage)
	}
	prepared, err := dcjc.prepareMutationWithRetryAfterApplyAndFallback(
		scope, current, nil, permit, resourceRemovalDisposition(current), postimage, reply, cleanup,
		autoDetectionRetryToken{}, composeAfterApply(afterApply, handoff),
		preparedJobConfigLifecycle{
			identity: result.candidate.jobConfigIdentity,
			snapshot: result.candidate.jobConfigSnapshot,
		},
		nil, nil,
	)
	if err != nil {
		adoption.release()
		return nil, err
	}
	adoption.PreparedResourceTransaction = prepared
	return adoption, nil
}
