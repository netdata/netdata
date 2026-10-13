// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

// Readiness is a notification, not permission to publish. The current resource
// and live runtime outcome are checked again in the serialized transaction.
func (dcjc *DynCfgJobController) planRuntimeReady(identity lifecycle.ResourceIdentity) jobmgr.WorkPlan {
	return dcjc.planRuntimeSettlement(identity, false)
}

func (dcjc *DynCfgJobController) planRuntimeSettlement(identity lifecycle.ResourceIdentity, terminal bool) jobmgr.WorkPlan {
	return jobmgr.WorkPlan{
		Claims:     []string{DynCfgJobGraphClaim},
		NoResponse: true,
		Transaction: &jobmgr.ResourceTransactionPlan{
			ID: identity.ID,
			Prepare: func(_ context.Context, current lifecycle.ReadyResource, scope lifecycle.ResourceTransactionScope, permit lifecycle.LongLivedPermit) (lifecycle.PreparedResourceTransaction, error) {
				record, exists := dcjc.graph.Lookup(identity.ID)
				generation, ok := current.(*JobGeneration)
				if !ok || scope.Current != identity || current.Identity() != identity || !exists ||
					(record.Status != dyncfg.StatusAccepted.String() && !(terminal && record.Status == dyncfg.StatusRunning.String())) {
					return dcjc.noop(scope, current, permit, noResponseResult())
				}
				return &runtimeSettlementTransaction{controller: dcjc, scope: scope, current: generation}, nil
			},
		},
	}
}

type runtimeSettlementTransaction struct {
	mu         sync.Mutex
	consumed   bool
	controller *DynCfgJobController
	scope      lifecycle.ResourceTransactionScope
	current    *JobGeneration
}

func (t *runtimeSettlementTransaction) Scope() lifecycle.ResourceTransactionScope {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.consumed {
		return lifecycle.ResourceTransactionScope{}
	}
	return t.scope
}

func (t *runtimeSettlementTransaction) take() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.consumed {
		return errors.New("job output: runtime settlement transaction already consumed")
	}
	t.consumed = true
	return nil
}

func (t *runtimeSettlementTransaction) Dispose(context.Context) (lifecycle.ReadyResource, error) {
	if err := t.take(); err != nil {
		return nil, err
	}
	return t.current, nil
}

func (t *runtimeSettlementTransaction) Apply(ctx context.Context) (applied lifecycle.AppliedResourceTransaction, resultErr error) {
	if err := t.take(); err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	delegated := false
	defer func() {
		if delegated || resultErr == nil {
			return
		}
		// Until Apply is delegated, the graph is uncommitted and this
		// transaction still owns current, including after partial publication.
		unchanged, ownershipErr := lifecycle.NewAppliedResourceTransaction(
			t.scope, lifecycle.ResourceTransactionUnchanged, t.current, noResponseResult(), func() error { return nil },
		)
		if ownershipErr == nil {
			applied = unchanged
		}
		expectedRetirement := jobmgr.ContainsOnlyErrorLeaves(resultErr, jobmgr.ErrProcessAttemptRetired, jobmgr.ErrProcessAttemptStopped)
		resultErr = errors.Join(resultErr, ownershipErr)
		if expectedRetirement && ownershipErr == nil {
			// Run retirement will detach and finalize the returned generation.
			resultErr = nil
		}
	}()
	dcjc := t.controller
	record, exists := dcjc.graph.Lookup(t.scope.ID)
	if !exists || (record.Status != dyncfg.StatusAccepted.String() && record.Status != dyncfg.StatusRunning.String()) {
		return lifecycle.AppliedResourceTransaction{}, errors.New("job output: runtime settlement graph changed inside transaction")
	}
	startupErr := t.current.StartupResult()
	if errors.Is(startupErr, ErrJobStartupPending) {
		return lifecycle.NewAppliedResourceTransaction(t.scope, lifecycle.ResourceTransactionUnchanged, t.current, noResponseResult(), func() error { return nil })
	}
	if startupErr == nil && record.Status == dyncfg.StatusRunning.String() {
		return lifecycle.NewAppliedResourceTransaction(t.scope, lifecycle.ResourceTransactionUnchanged, t.current, noResponseResult(), func() error { return nil })
	}
	if startupErr == nil {
		startupErr = t.current.Publish()
		if startupErr == nil {
			startupErr = t.current.StartupResult()
		}
	}
	status := dyncfg.StatusRunning
	disposition := lifecycle.ResourceTransactionUnchanged
	var failure *autoDetectionFailure
	if startupErr != nil {
		operational, ok := onlyRuntimeStartupFailure(startupErr)
		if !ok {
			return lifecycle.AppliedResourceTransaction{}, startupErr
		}
		status = dyncfg.StatusFailed
		disposition = lifecycle.ResourceTransactionRemoved
		failure = operational.failure
	}
	postimage := graphConfig(record, status)
	replyFailure := jobFailure{}
	if failure != nil {
		replyFailure = collectorFailure(failure, "job restart failed: %v")
	}
	afterApply := func() {
		dcjc.completeRestart(t.current.Identity(), adoptedResult(dyncfg.CommandRestart, status, replyFailure))
	}
	if failure != nil {
		config, err := graphRecordConfig(record)
		if err != nil {
			return lifecycle.AppliedResourceTransaction{}, err
		}
		afterApply = composeAfterApply(afterApply, func() { dcjc.scheduleAutoDetectionRetry(config, failure) })
	}
	jobConfig := preparedJobConfigLifecycle{
		identity: t.current.resources.jobConfigIdentity,
		snapshot: t.current.resources.jobConfigSnapshot,
	}
	if failure != nil {
		jobConfig.failure = failure.diagnosticFailure
	} else {
		jobConfig.runtime = t.current.resources.candidateJob
	}
	prepared, err := dcjc.prepareMutationWithRetryAfterApplyAndFallback(
		t.scope, t.current, nil, lifecycle.LongLivedPermit{}, disposition, &postimage, internalReply(),
		dcjc.configStatusCleanup(t.scope.ID, status), autoDetectionRetryToken{}, afterApply,
		jobConfig, nil, nil,
	)
	if err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	delegated = true
	return prepared.Apply(ctx)
}
