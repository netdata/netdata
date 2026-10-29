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
	ctx context.Context,
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	successor lifecycle.PreparedResource,
	unusedPermit lifecycle.LongLivedPermit,
	disposition lifecycle.ResourceTransactionDisposition,
	postimage *dyncfg.GraphConfig,
	reply jobReply,
	cleanup lifecycle.TaskCleanup,
) (lifecycle.PreparedResourceTransaction, error) {
	return dcjc.prepareMutationAfterApply(
		ctx,
		scope,
		current,
		successor,
		unusedPermit,
		disposition,
		postimage,
		reply,
		cleanup,
		nil,
	)
}

func (dcjc *DynCfgJobController) prepareMutationAfterApply(
	ctx context.Context,
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	successor lifecycle.PreparedResource,
	unusedPermit lifecycle.LongLivedPermit,
	disposition lifecycle.ResourceTransactionDisposition,
	postimage *dyncfg.GraphConfig,
	reply jobReply,
	cleanup lifecycle.TaskCleanup,
	afterApply func(),
	failures ...collectorapi.JobConfigFailure,
) (lifecycle.PreparedResourceTransaction, error) {
	jobConfig := preparedJobConfigLifecycleState(successor)
	if len(failures) != 0 && postimage != nil &&
		(postimage.Status == dyncfg.StatusFailed.String() || postimage.Status == dyncfg.StatusAccepted.String()) {
		jobConfig.identity = dcjc.postimageJobConfigLifecycleGraphState(postimage).identity
		jobConfig.failure = failures[0]
	}
	return dcjc.prepareMutationAfterApplyAndFallback(
		ctx,
		scope,
		current,
		successor,
		unusedPermit,
		disposition,
		postimage,
		reply,
		cleanup,
		afterApply,
		jobConfig,
		nil,
		nil,
		nil,
	)
}

func (dcjc *DynCfgJobController) prepareMutationAfterApplyAndFallback(
	ctx context.Context,
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	successor lifecycle.PreparedResource,
	unusedPermit lifecycle.LongLivedPermit,
	disposition lifecycle.ResourceTransactionDisposition,
	postimage *dyncfg.GraphConfig,
	reply jobReply,
	cleanup lifecycle.TaskCleanup,
	afterApply func(),
	jobConfig preparedJobConfigLifecycle,
	busyFallback *ResourceActivationFallback,
	quarantinedFallback *ResourceActivationFallback,
	candidate *preparedJobCandidate,
) (lifecycle.PreparedResourceTransaction, error) {
	delta, err := dcjc.prepareControlCandidate(scope.ID, postimage, reply)
	if err != nil {
		return nil, err
	}
	if delta != nil {
		reply.candidate = delta
		commit, err := dcjc.prepareCandidateCommit(delta, reply.retainedCandidate, reply.restart)
		if err != nil {
			return nil, err
		}
		afterApply = composeAfterApply(commit, afterApply)
		if prepared, ok := successor.(PreparedJob); ok && delta.after.Active != nil {
			prepared.state.continuation = delta.after.Active.Token
		}
	}
	jobConfigReconcile := dcjc.prepareJobConfigLifecycleReconcile(scope.ID, postimage, jobConfig)
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
	if err := dcjc.validateCandidateAdmission(ctx, reply.candidate); err != nil {
		if successor != nil {
			err = rollbackSuccessorMutation(successor, err)
		}
		return nil, err
	}
	// Admission can yield the graph claim after Check. Validate every captured
	// dependency and candidate ownership only after all yielding work has finished.
	if candidate != nil {
		if _, err := candidate.inspect(); err != nil {
			return nil, &candidatePreflightError{cause: err}
		}
	}
	mutation, err := dcjc.graph.PrepareMutation([]dyncfg.GraphChange{{ID: scope.ID, Config: postimage}})
	if errors.Is(err, dyncfg.ErrGraphNoChange) {
		afterApply = composeAfterApply(afterApply, jobConfigReconcile)
		transaction, err := dcjc.prepareResourceTransaction(ResourceTransactionSpec{
			Scope:                         scope,
			Disposition:                   disposition,
			Current:                       current,
			Successor:                     successor,
			UnusedPermit:                  unusedPermit,
			Graph:                         dcjc.graph,
			AfterApply:                    afterApply,
			ActivationBusyFallback:        busyFallback,
			ActivationQuarantinedFallback: quarantinedFallback,
			Cleanup:                       cleanup,
			reply:                         &reply,
		})
		return transaction, err
	}
	if err != nil {
		if successor != nil {
			err = rollbackSuccessorMutation(successor, err)
		}
		return nil, err
	}
	transaction, prepareErr := dcjc.prepareResourceTransaction(
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
	return transaction, prepareErr
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
	ctx context.Context,
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	successor lifecycle.PreparedResource,
	disposition lifecycle.ResourceTransactionDisposition,
	postimage *dyncfg.GraphConfig,
	reply jobReply,
	cleanup lifecycle.TaskCleanup,
	afterApply func(),
	busy activationFallbackPlan,
	quarantined activationFallbackPlan,
) (lifecycle.PreparedResourceTransaction, error) {
	jobConfig := preparedJobConfigLifecycleState(successor)
	if reply.candidate != nil {
		commit, err := dcjc.prepareCandidateCommit(reply.candidate, false, reply.restart)
		if err != nil {
			return nil, rollbackSuccessorMutation(successor, err)
		}
		busy.afterApply = composeAfterApply(commit, busy.afterApply)
		if work := reply.candidate.after.Active; work != nil {
			blocked, err := dcjc.candidates.prepareOutcome(scope.ID, work.Token, candidateOutcome{Kind: candidateFrameworkBlocked, Block: candidateBlockQuarantined, Listed: true}, dcjc.scheduler.retries.now())
			if err != nil {
				return nil, rollbackSuccessorMutation(successor, err)
			}
			postimage, err := candidateGraphBinding(scope.ID, blocked.Decision.Binding)
			if err != nil {
				return nil, rollbackSuccessorMutation(successor, err)
			}
			commit, err := dcjc.prepareCandidateCommit(blocked, false, restartObservationTransition{
				settlement: &activationRestartSettlement{
					work: work.Token, result: rejectedResult(testFailure(jobmgr.ErrProcessAttemptQuarantined)),
				},
			})
			if err != nil {
				return nil, rollbackSuccessorMutation(successor, err)
			}
			quarantined.postimage = postimage
			if postimage != nil {
				config := blocked.Decision.Binding.Config
				quarantined.cleanup = dcjc.configCreateCleanup(*postimage, config.SourceType(), config.Source(), dcjc.configType(dcjc.modules[config.Module()]))
			}
			quarantined.afterApply = composeAfterApply(commit, quarantined.afterApply)
		}
	}
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
	return dcjc.prepareMutationAfterApplyAndFallback(
		ctx,
		scope,
		current,
		successor,
		lifecycle.LongLivedPermit{},
		disposition,
		postimage,
		reply,
		cleanup,
		afterApply,
		jobConfig,
		busyFallback,
		quarantinedFallback,
		nil,
	)
}

func (dcjc *DynCfgJobController) prepareResourceTransaction(
	spec ResourceTransactionSpec,
) (lifecycle.PreparedResourceTransaction, error) {
	if spec.Current != nil &&
		(spec.Disposition == lifecycle.ResourceTransactionRemoved || spec.Disposition == lifecycle.ResourceTransactionReplaced) {
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

func (dcjc *DynCfgJobController) prepareTransientConstructionFailure(
	ctx context.Context,
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	permit lifecycle.LongLivedPermit,
	postimage dyncfg.GraphConfig,
	reply jobReply,
	cleanup lifecycle.TaskCleanup,
	config confgroup.Config,
	err error,
) (lifecycle.PreparedResourceTransaction, error) {
	failure := transientActivationFailure(config, err)
	if reply.command != "" && reply.candidate == nil {
		after := 0
		if failure.retry {
			after = failure.retryAfter
		}
		delta, policyErr := dcjc.candidates.prepareAdoptExact(config, true, dyncfg.StatusFailed, true, after, dcjc.scheduler.retries.now())
		if policyErr != nil {
			return nil, policyErr
		}
		reply.candidate = delta
	}
	return dcjc.prepareMutationAfterApply(
		ctx,
		scope,
		current,
		nil,
		permit,
		resourceRemovalDisposition(current),
		&postimage,
		reply,
		cleanup,
		nil,
		jobConfigFailure(err, "construction"),
	)
}

func (dcjc *DynCfgJobController) prepareProbeFailure(
	ctx context.Context,
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	permit lifecycle.LongLivedPermit,
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
	if reply.command != "" && reply.candidate == nil {
		var config confgroup.Config
		if err := yaml.Unmarshal(plan.postimage.Payload, &config); err != nil {
			return nil, err
		}
		after := 0
		if failure.retry {
			after = failure.retryAfter
		}
		delta, err := dcjc.candidates.prepareAdoptExact(config, true, dyncfg.StatusFailed, !removed, after, dcjc.scheduler.retries.now())
		if err != nil {
			return nil, err
		}
		reply.candidate = delta
	}
	var afterApply func()
	if plan.afterApply != nil {
		afterApply = func() {
			plan.afterApply(failure)
		}
	}
	return dcjc.prepareMutationAfterApplyAndFallback(
		ctx,
		scope,
		current,
		nil,
		permit,
		resourceRemovalDisposition(current),
		postimage,
		reply,
		cleanup,
		afterApply,
		preparedJobConfigLifecycle{
			identity: dcjc.postimageJobConfigLifecycleGraphState(postimage).identity,
			snapshot: failure.jobConfigLifecycle,
			failure:  failure.diagnosticFailure,
		},
		nil,
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
