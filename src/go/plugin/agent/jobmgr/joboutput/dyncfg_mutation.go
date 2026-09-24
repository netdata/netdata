// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"gopkg.in/yaml.v2"
)

func (dcjc *DynCfgJobController) prepareMutation(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	successor lifecycle.PreparedResource,
	unusedPermit lifecycle.LongLivedPermit,
	disposition lifecycle.ResourceTransactionDisposition,
	postimage *dyncfg.GraphConfig,
	reply jobReply,
	cleanup lifecycle.TaskCleanup,
) (lifecycle.PreparedResourceTransaction, error) {
	return dcjc.prepareMutationWithRetry(
		scope,
		current,
		successor,
		unusedPermit,
		disposition,
		postimage,
		reply,
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
	reply jobReply,
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
		reply,
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
	reply jobReply,
	cleanup lifecycle.TaskCleanup,
	retry autoDetectionRetryToken,
	afterApply func(),
	failures ...collectorapi.JobConfigFailure,
) (lifecycle.PreparedResourceTransaction, error) {
	jobConfig := preparedJobConfigLifecycleState(successor)
	if len(failures) != 0 && postimage != nil && (postimage.Status == dyncfg.StatusFailed.String() || postimage.Status == dyncfg.StatusAccepted.String()) {
		jobConfig.identity = dcjc.postimageJobConfigLifecycleGraphState(postimage).identity
		jobConfig.failure = failures[0]
	}
	return dcjc.prepareMutationWithRetryAfterApplyAndFallback(
		scope,
		current,
		successor,
		unusedPermit,
		disposition,
		postimage,
		reply,
		cleanup,
		retry,
		afterApply,
		jobConfig,
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
	reply jobReply,
	cleanup lifecycle.TaskCleanup,
	retry autoDetectionRetryToken,
	afterApply func(),
	jobConfig preparedJobConfigLifecycle,
	busyFallback *ResourceActivationFallback,
	quarantinedFallback *ResourceActivationFallback,
) (lifecycle.PreparedResourceTransaction, error) {
	if successor != nil && postimage != nil && postimage.Status == dyncfg.StatusAccepted.String() {
		var config confgroup.Config
		if err := yaml.Unmarshal(postimage.Payload, &config); err != nil {
			return nil, err
		}
		spec, err := newAcceptedActivationSpec(config)
		if err != nil {
			return nil, err
		}
		afterApply = composeAfterApply(afterApply, func() { dcjc.scheduler.accepted.trackInstalled(spec) })
	}
	jobConfigReconcile := dcjc.prepareJobConfigLifecycleReconcile(scope.ID, postimage, jobConfig)
	afterApply = composeAfterApply(dcjc.retrySettlement(scope.ID, retry), afterApply)
	acceptedAfterApply, err := dcjc.acceptedActivationAfterApply(scope.ID, postimage)
	if err != nil {
		if successor != nil {
			err = rollbackSuccessorMutation(successor, err)
		}
		return nil, err
	}
	afterApply = composeAfterApply(afterApply, acceptedAfterApply)
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
	dependencyCommit = composeAfterApply(dependencyCommit, jobConfigReconcile)
	mutation, err := dcjc.graph.PrepareMutation([]dyncfg.GraphChange{{ID: scope.ID, Config: postimage}})
	if errors.Is(err, dyncfg.ErrGraphNoChange) {
		afterApply = composeAfterApply(afterApply, jobConfigReconcile)
		if successor != nil || disposition == lifecycle.ResourceTransactionRemoved {
			return dcjc.prepareResourceTransaction(
				ResourceTransactionSpec{
					Scope:                         scope,
					Disposition:                   disposition,
					Current:                       current,
					Successor:                     successor,
					UnusedPermit:                  unusedPermit,
					Graph:                         dcjc.graph,
					AfterGraphCommit:              dependencyCommit,
					AfterApply:                    afterApply,
					ActivationBusyFallback:        busyFallback,
					ActivationQuarantinedFallback: quarantinedFallback,
					Cleanup:                       cleanup,
					reply:                         &reply,
				},
			)
		}
		// The graph already holds the adopted postimage.
		return dcjc.noopWithAfterApply(
			scope,
			current,
			unusedPermit,
			reply.seal(transactionOutcome{}),
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
			Cleanup:                       cleanup,
			reply:                         &reply,
		},
	)
}

func (dcjc *DynCfgJobController) newActivationFallback(
	id string,
	postimage *dyncfg.GraphConfig,
	replyFailure jobFailure,
	cleanup lifecycle.TaskCleanup,
	afterApply func(),
	jobConfigSnapshot collectorapi.JobConfigLifecycleSnapshot,
	failure collectorapi.JobConfigFailure,
) (*ResourceActivationFallback, error) {
	if dcjc == nil || id == "" || cleanup == nil {
		return nil, errors.New("job output: invalid activation fallback")
	}
	acceptedAfterApply, err := dcjc.acceptedActivationAfterApply(id, postimage)
	if err != nil {
		return nil, err
	}
	afterApply = composeAfterApply(afterApply, acceptedAfterApply)
	var dependencyCommit func()
	if dcjc.dependencies != nil {
		var err error
		dependencyCommit, err = dcjc.dependencies.PrepareJobChange(id, postimage)
		if err != nil {
			return nil, err
		}
	}
	dependencyCommit = composeAfterApply(
		dependencyCommit,
		dcjc.prepareJobConfigLifecycleReconcile(
			id,
			postimage,
			preparedJobConfigLifecycle{
				identity: dcjc.postimageJobConfigLifecycleGraphState(postimage).identity,
				snapshot: jobConfigSnapshot,
				failure:  failure,
			},
		),
	)
	return &ResourceActivationFallback{
		Change: dyncfg.GraphChange{
			ID:     id,
			Config: postimage,
		},
		AfterGraphReconcile: dependencyCommit,
		AfterApply:          afterApply,
		Cleanup:             cleanup,
		failure:             replyFailure,
	}, nil
}

type activationFallbackPlan struct {
	postimage  *dyncfg.GraphConfig
	failure    jobFailure // why the adopted change ends Failed; unused for internal work
	cleanup    lifecycle.TaskCleanup
	afterApply func()
}

func (dcjc *DynCfgJobController) prepareMutationWithActivationFallbacks(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	successor lifecycle.PreparedResource,
	disposition lifecycle.ResourceTransactionDisposition,
	postimage *dyncfg.GraphConfig,
	reply jobReply,
	cleanup lifecycle.TaskCleanup,
	retry autoDetectionRetryToken,
	afterApply func(),
	busy activationFallbackPlan,
	quarantined activationFallbackPlan,
) (lifecycle.PreparedResourceTransaction, error) {
	jobConfig := preparedJobConfigLifecycleState(successor)
	busyFallback, err := dcjc.newActivationFallback(
		scope.ID,
		busy.postimage,
		busy.failure,
		busy.cleanup,
		busy.afterApply,
		jobConfig.snapshot,
		jobConfigFailure(jobmgr.ErrProcessAttemptBusy, "activation"),
	)
	if err != nil {
		return nil, rollbackSuccessorMutation(successor, err)
	}
	quarantinedFallback, err := dcjc.newActivationFallback(
		scope.ID,
		quarantined.postimage,
		quarantined.failure,
		quarantined.cleanup,
		quarantined.afterApply,
		jobConfig.snapshot,
		jobConfigFailure(jobmgr.ErrProcessAttemptQuarantined, "activation"),
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
		reply,
		cleanup,
		retry,
		afterApply,
		jobConfig,
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
	if spec.Current != nil && (spec.Disposition == lifecycle.ResourceTransactionRemoved || spec.Disposition == lifecycle.ResourceTransactionReplaced) {
		identity := spec.Current.Identity()
		retire := func() { dcjc.retireRestart(identity) }
		spec.AfterApply = composeAfterApply(spec.AfterApply, retire)
		if fallback := spec.ActivationBusyFallback; fallback != nil {
			fallback.AfterApply = composeAfterApply(fallback.AfterApply, retire)
		}
		if fallback := spec.ActivationQuarantinedFallback; fallback != nil {
			fallback.AfterApply = composeAfterApply(fallback.AfterApply, retire)
		}
	}
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
	reply jobReply,
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
		reply,
		cleanup,
		retry,
		func() {
			dcjc.scheduleAutoDetectionRetry(config, failure)
		},
		jobConfigFailure(err, "construction"),
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
	removed := plan.removePlainStock && !failure.keepsFailedStockListed()
	var postimage *dyncfg.GraphConfig
	cleanup := plan.failedCleanup
	reply := plan.reply(failure)
	if removed {
		cleanup = plan.removedCleanup
		reply.status = ""
	} else {
		postimage = &plan.postimage
	}
	var afterApply func()
	if plan.afterApply != nil {
		afterApply = func() {
			plan.afterApply(failure)
		}
	}
	return dcjc.prepareMutationWithRetryAfterApplyAndFallback(
		scope,
		current,
		nil,
		permit,
		resourceRemovalDisposition(current),
		postimage,
		reply,
		cleanup,
		retry,
		afterApply,
		preparedJobConfigLifecycle{
			identity: dcjc.postimageJobConfigLifecycleGraphState(postimage).identity,
			snapshot: failure.jobConfigLifecycle,
			failure:  failure.diagnosticFailure,
		},
		nil,
		nil,
	)
}

// probeFailurePlan supplies source-specific preparation failure handling.
type probeFailurePlan struct {
	postimage        dyncfg.GraphConfig                   // graph postimage to commit as StatusFailed
	failedCleanup    lifecycle.TaskCleanup                // protocol cleanup for the failed status
	removedCleanup   lifecycle.TaskCleanup                // preparation only: cleanup when a plain stock job is removed
	reply            func(*autoDetectionFailure) jobReply // reply intent of the adopted failure
	afterApply       func(*autoDetectionFailure)          // side effect (retry scheduling) after apply
	removePlainStock bool                                 // preparation only: remove a stock job on an unclassified failure
}

// internalFailureReply is the probeFailurePlan.reply of response-free work.
func internalFailureReply(*autoDetectionFailure) jobReply {
	return internalReply()
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
