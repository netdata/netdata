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
	failurePlans ...successorFailurePlan,
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
		failurePlans...,
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
	failurePlans ...successorFailurePlan,
) (lifecycle.PreparedResourceTransaction, error) {
	if len(failurePlans) > 1 || len(failurePlans) == 1 && successor == nil {
		return nil, errors.New("job output: invalid successor-failure plan")
	}
	var failurePlan *successorFailurePlan
	if len(failurePlans) == 1 {
		failurePlan = &failurePlans[0]
	}
	var dependencyCommit func()
	var failedDependencyCommit func()
	var removedDependencyCommit func()
	if dcjc.dependencies != nil {
		var err error
		dependencyCommit, err = dcjc.dependencies.PrepareJobChange(scope.ID, postimage)
		if err != nil {
			if successor != nil {
				err = rollbackSuccessorMutation(successor, err)
			}
			return nil, err
		}
		if failurePlan != nil {
			failedDependencyCommit, err = dcjc.dependencies.PrepareJobChange(scope.ID, &failurePlan.postimage)
			if err == nil && failurePlan.removePlainStock {
				removedDependencyCommit, err = dcjc.dependencies.PrepareJobChange(scope.ID, nil)
			}
			if err != nil {
				return nil, rollbackSuccessorMutation(successor, err)
			}
		}
	}
	mutation, err := dcjc.graph.PrepareMutation([]dyncfg.GraphChange{{ID: scope.ID, Config: postimage}})
	if errors.Is(err, dyncfg.ErrGraphNoChange) {
		if successor != nil {
			return dcjc.prepareResourceTransaction(
				ResourceTransactionSpec{
					Scope:            scope,
					Disposition:      disposition,
					Current:          current,
					Successor:        successor,
					Graph:            dcjc.graph,
					AfterGraphCommit: dependencyCommit,
					AfterApply:       dcjc.retrySettlement(scope.ID, retry),
					Result:           result,
					Cleanup:          cleanup,
					SuccessorFailure: successorFailureResolver(
						failurePlan,
						failedDependencyCommit,
						removedDependencyCommit,
					),
				},
			)
		}
		return dcjc.noopWithAfterApply(
			scope,
			current,
			unusedPermit,
			result,
			dcjc.retrySettlement(scope.ID, retry),
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
			Scope:            scope,
			Disposition:      disposition,
			Current:          current,
			Successor:        successor,
			UnusedPermit:     unusedPermit,
			Graph:            dcjc.graph,
			Mutation:         mutation,
			MutationPrepared: true,
			AfterGraphCommit: dependencyCommit,
			AfterApply:       dcjc.retrySettlement(scope.ID, retry),
			Result:           result,
			Cleanup:          cleanup,
			SuccessorFailure: successorFailureResolver(failurePlan, failedDependencyCommit, removedDependencyCommit),
		},
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

type successorFailurePlan struct {
	postimage        dyncfg.GraphConfig                                 // graph postimage to commit as StatusFailed
	failedCleanup    lifecycle.TaskCleanup                              // protocol cleanup for the failed status
	removedCleanup   lifecycle.TaskCleanup                              // protocol cleanup when a plain stock job is removed instead
	result           func(*autoDetectionFailure) lifecycle.SealedResult // builds the dyncfg response from the failure
	afterApply       func(*autoDetectionFailure)                        // side effect (retry scheduling) after apply
	removePlainStock bool                                               // remove instead of fail for a stock + non-coded failure
}

func successorFailureResolver(
	plan *successorFailurePlan,
	failedDependencyCommit func(),
	removedDependencyCommit func(),
) func(*autoDetectionFailure) (SuccessorFailureResolution, error) {
	if plan == nil {
		return nil
	}
	return func(failure *autoDetectionFailure) (SuccessorFailureResolution, error) {
		if failure == nil {
			return SuccessorFailureResolution{}, errors.New("job output: nil autodetection failure")
		}
		removed := plan.removePlainStock && !failure.coded
		postimage := plan.postimage
		resolution := SuccessorFailureResolution{
			Postimage:        &postimage,
			AfterGraphCommit: failedDependencyCommit,
			Cleanup:          plan.failedCleanup,
		}
		if removed {
			resolution.Postimage = nil
			resolution.AfterGraphCommit = removedDependencyCommit
			resolution.Cleanup = plan.removedCleanup
		}
		if plan.result != nil {
			resolution.Result = plan.result(failure)
		}
		if plan.afterApply != nil {
			resolution.AfterApply = func() {
				plan.afterApply(failure)
			}
		}
		return resolution, nil
	}
}

// autoDetectionFailureResultFunc builds a successorFailurePlan.result closure
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
// successorFailurePlan.afterApply closure that reschedules the given config.
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
