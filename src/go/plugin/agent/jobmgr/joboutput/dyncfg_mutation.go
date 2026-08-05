// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

func (dcjc *DynCfgJobController) prepareMutation(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	successor lifecycle.PreparedResource,
	unusedPermit lifecycle.LongLivedPermit,
	disposition lifecycle.ResourceTransactionDisposition,
	postimage *dyncfg.GraphConfig,
	result lifecycle.SealedResult,
	cleanup lifecycle.TaskCleanup,
) (lifecycle.PreparedResourceTransaction, error) {
	return dcjc.prepareMutationWithRetry(
		scope,
		current,
		successor,
		unusedPermit,
		disposition,
		postimage,
		result,
		cleanup,
		autoDetectionRetryToken{},
	)
}

func (dcjc *DynCfgJobController) prepareMutationWithRetry(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	successor lifecycle.PreparedResource,
	unusedPermit lifecycle.LongLivedPermit,
	disposition lifecycle.ResourceTransactionDisposition,
	postimage *dyncfg.GraphConfig,
	result lifecycle.SealedResult,
	cleanup lifecycle.TaskCleanup,
	retry autoDetectionRetryToken,
) (lifecycle.PreparedResourceTransaction, error) {
	return dcjc.prepareMutationWithRetryAfterApply(
		scope,
		current,
		successor,
		unusedPermit,
		disposition,
		postimage,
		result,
		cleanup,
		retry,
		nil,
	)
}

func (dcjc *DynCfgJobController) prepareMutationWithRetryAfterApply(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	successor lifecycle.PreparedResource,
	unusedPermit lifecycle.LongLivedPermit,
	disposition lifecycle.ResourceTransactionDisposition,
	postimage *dyncfg.GraphConfig,
	result lifecycle.SealedResult,
	cleanup lifecycle.TaskCleanup,
	retry autoDetectionRetryToken,
	afterApply func(),
) (lifecycle.PreparedResourceTransaction, error) {
	return dcjc.prepareMutationWithRetryAfterApplyAndFallback(
		scope,
		current,
		successor,
		unusedPermit,
		disposition,
		postimage,
		result,
		cleanup,
		retry,
		afterApply,
		nil,
		nil,
	)
}

func (dcjc *DynCfgJobController) prepareMutationWithRetryAfterApplyAndFallback(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	successor lifecycle.PreparedResource,
	unusedPermit lifecycle.LongLivedPermit,
	disposition lifecycle.ResourceTransactionDisposition,
	postimage *dyncfg.GraphConfig,
	result lifecycle.SealedResult,
	cleanup lifecycle.TaskCleanup,
	retry autoDetectionRetryToken,
	afterApply func(),
	busyFallback *ResourceActivationFallback,
	quarantinedFallback *ResourceActivationFallback,
) (lifecycle.PreparedResourceTransaction, error) {
	afterApply = composeAfterApply(dcjc.retrySettlement(scope.ID, retry), afterApply)
	var dependencyCommit func()
	if dcjc.dependencies != nil {
		var err error
		dependencyCommit, err = dcjc.dependencies.PrepareJobChange(scope.ID, postimage)
		if err != nil {
			if successor != nil {
				err = rollbackSuccessorMutation(successor, err)
			}
			return nil, err
		}
	}
	mutation, err := dcjc.graph.PrepareMutation([]dyncfg.GraphChange{{ID: scope.ID, Config: postimage}})
	if errors.Is(err, dyncfg.ErrGraphNoChange) {
		if successor != nil {
			return dcjc.prepareResourceTransaction(
				ResourceTransactionSpec{
					Scope:                         scope,
					Disposition:                   disposition,
					Current:                       current,
					Successor:                     successor,
					Graph:                         dcjc.graph,
					AfterGraphCommit:              dependencyCommit,
					AfterApply:                    afterApply,
					ActivationBusyFallback:        busyFallback,
					ActivationQuarantinedFallback: quarantinedFallback,
					Result:                        result,
					Cleanup:                       cleanup,
				},
			)
		}
		return dcjc.noopWithAfterApply(
			scope,
			current,
			unusedPermit,
			result,
			afterApply,
			cleanup,
		)
	}
	if err != nil {
		if successor != nil {
			err = rollbackSuccessorMutation(successor, err)
		}
		return nil, err
	}
	return dcjc.prepareResourceTransaction(
		ResourceTransactionSpec{
			Scope:                         scope,
			Disposition:                   disposition,
			Current:                       current,
			Successor:                     successor,
			UnusedPermit:                  unusedPermit,
			Graph:                         dcjc.graph,
			Mutation:                      mutation,
			MutationPrepared:              true,
			AfterGraphCommit:              dependencyCommit,
			AfterApply:                    afterApply,
			ActivationBusyFallback:        busyFallback,
			ActivationQuarantinedFallback: quarantinedFallback,
			Result:                        result,
			Cleanup:                       cleanup,
		},
	)
}

func (dcjc *DynCfgJobController) newActivationFallback(
	id string,
	postimage *dyncfg.GraphConfig,
	result lifecycle.SealedResult,
	cleanup lifecycle.TaskCleanup,
	afterApply func(),
) (*ResourceActivationFallback, error) {
	if dcjc == nil || id == "" || cleanup == nil {
		return nil, errors.New("job output: invalid activation fallback")
	}
	var dependencyCommit func()
	if dcjc.dependencies != nil {
		var err error
		dependencyCommit, err = dcjc.dependencies.PrepareJobChange(id, postimage)
		if err != nil {
			return nil, err
		}
	}
	return &ResourceActivationFallback{
		Change: dyncfg.GraphChange{
			ID:     id,
			Config: postimage,
		},
		AfterGraphCommit: dependencyCommit,
		AfterApply:       afterApply,
		Result:           result,
		Cleanup:          cleanup,
	}, nil
}

type activationFallbackPlan struct {
	postimage  *dyncfg.GraphConfig
	result     lifecycle.SealedResult
	cleanup    lifecycle.TaskCleanup
	afterApply func()
}

func (dcjc *DynCfgJobController) prepareMutationWithActivationFallbacks(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	successor lifecycle.PreparedResource,
	disposition lifecycle.ResourceTransactionDisposition,
	postimage *dyncfg.GraphConfig,
	result lifecycle.SealedResult,
	cleanup lifecycle.TaskCleanup,
	retry autoDetectionRetryToken,
	afterApply func(),
	busy activationFallbackPlan,
	quarantined activationFallbackPlan,
) (lifecycle.PreparedResourceTransaction, error) {
	busyFallback, err := dcjc.newActivationFallback(
		scope.ID,
		busy.postimage,
		busy.result,
		busy.cleanup,
		busy.afterApply,
	)
	if err != nil {
		return nil, rollbackSuccessorMutation(successor, err)
	}
	quarantinedFallback, err := dcjc.newActivationFallback(
		scope.ID,
		quarantined.postimage,
		quarantined.result,
		quarantined.cleanup,
		quarantined.afterApply,
	)
	if err != nil {
		return nil, rollbackSuccessorMutation(successor, err)
	}
	return dcjc.prepareMutationWithRetryAfterApplyAndFallback(
		scope,
		current,
		successor,
		lifecycle.LongLivedPermit{},
		disposition,
		postimage,
		result,
		cleanup,
		retry,
		afterApply,
		busyFallback,
		quarantinedFallback,
	)
}

func (dcjc *DynCfgJobController) retrySettlement(id string, token autoDetectionRetryToken) func() {
	if token.generation == 0 {
		return func() {
			dcjc.scheduler.retries.cancel(id)
		}
	}
	return func() {
		dcjc.scheduler.retries.cancelToken(id, token)
	}
}

func (dcjc *DynCfgJobController) prepareResourceTransaction(
	spec ResourceTransactionSpec,
) (lifecycle.PreparedResourceTransaction, error) {
	transaction, err := PrepareResourceTransaction(spec)
	if err == nil {
		return transaction, nil
	}
	var rollbackErr error
	if spec.Graph != nil && spec.MutationPrepared {
		rollbackErr = spec.Graph.Abort(spec.Mutation)
	}
	if spec.Successor != nil {
		rollbackErr = errors.Join(rollbackErr, rejectPreparedSuccessor(context.Background(), spec.Successor))
	}
	err = errors.Join(err, rollbackErr)
	if rollbackErr != nil {
		err = lifecycle.RetainOwnership(err)
	}
	return nil, err
}

func (dcjc *DynCfgJobController) scheduleAutoDetectionRetry(config confgroup.Config, failure *autoDetectionFailure) {
	if dcjc == nil || dcjc.scheduler == nil || failure == nil || !failure.retry {
		return
	}
	dcjc.scheduler.retries.schedule(config, failure.retryAfter)
}

func (dcjc *DynCfgJobController) prepareTransientConstructionFailure(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	permit lifecycle.LongLivedPermit,
	postimage dyncfg.GraphConfig,
	result lifecycle.SealedResult,
	cleanup lifecycle.TaskCleanup,
	retry autoDetectionRetryToken,
	config confgroup.Config,
	err error,
) (lifecycle.PreparedResourceTransaction, error) {
	failure := transientActivationFailure(config, err)
	return dcjc.prepareMutationWithRetryAfterApply(
		scope,
		current,
		nil,
		permit,
		resourceRemovalDisposition(current),
		&postimage,
		result,
		cleanup,
		retry,
		func() {
			dcjc.scheduleAutoDetectionRetry(config, failure)
		},
	)
}

func (dcjc *DynCfgJobController) prepareProbeFailure(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	permit lifecycle.LongLivedPermit,
	retry autoDetectionRetryToken,
	failure *autoDetectionFailure,
	plan probeFailurePlan,
) (lifecycle.PreparedResourceTransaction, error) {
	if failure == nil {
		return nil, errors.New("job output: nil probe failure")
	}
	removed := plan.removePlainStock && !failure.coded
	var postimage *dyncfg.GraphConfig
	cleanup := plan.failedCleanup
	if removed {
		cleanup = plan.removedCleanup
	} else {
		postimage = &plan.postimage
	}
	result := lifecycle.SealedResult{}
	if plan.result != nil {
		result = plan.result(failure)
	}
	var afterApply func()
	if plan.afterApply != nil {
		afterApply = func() {
			plan.afterApply(failure)
		}
	}
	return dcjc.prepareMutationWithRetryAfterApply(
		scope,
		current,
		nil,
		permit,
		resourceRemovalDisposition(current),
		postimage,
		result,
		cleanup,
		retry,
		afterApply,
	)
}

type probeFailurePlan struct {
	postimage        dyncfg.GraphConfig                                 // graph postimage to commit as StatusFailed
	failedCleanup    lifecycle.TaskCleanup                              // protocol cleanup for the failed status
	removedCleanup   lifecycle.TaskCleanup                              // protocol cleanup when a plain stock job is removed instead
	result           func(*autoDetectionFailure) lifecycle.SealedResult // builds the dyncfg response from the failure
	afterApply       func(*autoDetectionFailure)                        // side effect (retry scheduling) after apply
	removePlainStock bool                                               // remove instead of fail for a stock + non-coded failure
}

// autoDetectionFailureResultFunc builds a probeFailurePlan.result closure
// with a fixed default code and message; the failure's own code overrides the
// default when present.
func autoDetectionFailureResultFunc(
	defaultCode int,
	message string,
) func(*autoDetectionFailure) lifecycle.SealedResult {
	return func(failure *autoDetectionFailure) lifecycle.SealedResult {
		code := defaultCode
		if failure.coded {
			code = failure.code
		}
		return mustDynCfgMessage(code, fmt.Sprintf(message, failure.cause))
	}
}

// scheduleRetryAfterApply adapts scheduleAutoDetectionRetry into a
// probeFailurePlan.afterApply closure that reschedules the given config.
func (dcjc *DynCfgJobController) scheduleRetryAfterApply(config confgroup.Config) func(*autoDetectionFailure) {
	return func(failure *autoDetectionFailure) {
		dcjc.scheduleAutoDetectionRetry(config, failure)
	}
}

func rejectPreparedSuccessor(ctx context.Context, successor lifecycle.PreparedResource) error {
	if prepared, ok := successor.(PreparedJob); ok {
		return prepared.reject(ctx)
	}
	return successor.Dispose(ctx)
}

// rollbackSuccessorMutation rejects a prepared successor after a failed mutation
// prep, joins the rejection error, and retains ownership when the rejection
// itself fails so a leaked resource is never treated as released.
func rollbackSuccessorMutation(successor lifecycle.PreparedResource, err error) error {
	rollbackErr := rejectPreparedSuccessor(context.Background(), successor)
	err = errors.Join(err, rollbackErr)
	if rollbackErr != nil {
		err = lifecycle.RetainOwnership(err)
	}
	return err
}

func (dcjc *DynCfgJobController) noop(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	permit lifecycle.LongLivedPermit,
	result lifecycle.SealedResult,
	cleanups ...lifecycle.TaskCleanup,
) (lifecycle.PreparedResourceTransaction, error) {
	return dcjc.noopWithAfterApply(scope, current, permit, result, nil, cleanups...)
}

func (dcjc *DynCfgJobController) noopWithAfterApply(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	permit lifecycle.LongLivedPermit,
	result lifecycle.SealedResult,
	afterApply func(),
	cleanups ...lifecycle.TaskCleanup,
) (lifecycle.PreparedResourceTransaction, error) {
	cleanup := joinDynCfgCleanups(cleanups...)
	return PrepareNoopResourceTransaction(scope, current, permit, result, cleanup, afterApply)
}
