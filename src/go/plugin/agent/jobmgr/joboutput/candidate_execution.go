// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"gopkg.in/yaml.v2"
)

func candidateGraphBinding(id string, binding *candidateBinding) (*dyncfg.GraphConfig, error) {
	if binding == nil || !binding.Listed {
		return nil, nil
	}
	payload, err := yaml.Marshal(binding.Config)
	if err != nil {
		return nil, err
	}
	return &dyncfg.GraphConfig{ID: id, Module: binding.Config.Module(), Name: binding.Config.Name(), Status: binding.Health.String(), Payload: payload}, nil
}

type candidateDecisionOptions struct {
	lifecycle preparedJobConfigLifecycle
	restart   restartObservationTransition
}

func (dcjc *DynCfgJobController) prepareCandidateDecision(ctx context.Context, scope lifecycle.ResourceTransactionScope, current lifecycle.ReadyResource, permit lifecycle.LongLivedPermit, delta *candidateDelta, options ...candidateDecisionOptions) (lifecycle.PreparedResourceTransaction, error) {
	var option candidateDecisionOptions
	if len(options) > 0 {
		option = options[0]
	}
	var resume func()
	if before, after := delta.before, delta.after; current == nil && before != nil && before.Proposal != nil && before.Active != nil &&
		after.Proposal == nil && after.Active != nil && after.Active.Token == before.Active.Token && delta.Decision.Work == nil {
		// The source proposal displaced this executor without superseding its
		// admitted intent. Restore it through an acknowledged lane transaction.
		token := after.Active.Token
		resume = func() { dcjc.retainCandidateResume(scope.ID, token) }
	}
	if delta.Decision.Binding == nil && !delta.Decision.Remove {
		commit, err := dcjc.prepareCandidateCommit(delta, false, option.restart)
		if err != nil {
			return nil, err
		}
		return dcjc.noopWithAfterApply(scope, current, permit, noResponseResult(), composeAfterApply(commit, resume))
	}
	postimage, err := candidateGraphBinding(scope.ID, delta.Decision.Binding)
	if err != nil {
		return nil, err
	}
	var cleanup lifecycle.TaskCleanup = func() error { return nil }
	if id := dcjc.externalID(scope.ID); id != "" {
		cleanup = dcjc.configDeleteCleanup(id)
	}
	if binding := delta.Decision.Binding; binding != nil && !binding.Listed {
		cleanup = dcjc.configDeleteCleanup(dcjc.configID(binding.Config.Module(), binding.Config.Name()))
	}
	if postimage != nil {
		cleanup = dcjc.candidateProjectionCleanup(scope.ID, postimage, delta)
	}
	reply := internalReply()
	reply.candidate = delta
	reply.restart = option.restart
	return dcjc.prepareMutationAfterApplyAndFallback(ctx, scope, current, nil, permit, resourceRemovalDisposition(current), postimage, reply, cleanup, resume, option.lifecycle, jobConfigIntentPreserve, nil, nil, nil)
}

func (dcjc *DynCfgJobController) candidateProjectionCleanup(id string, postimage *dyncfg.GraphConfig, delta *candidateDelta) lifecycle.TaskCleanup {
	if record, exists := dcjc.graph.Lookup(id); exists && delta.before != nil && delta.before.Binding != nil &&
		delta.after.Binding != nil && delta.before.Binding.Key == delta.after.Binding.Key && record.Payload() == string(postimage.Payload) {
		return dcjc.configStatusCleanup(id, dyncfg.Status(postimage.Status))
	}
	config := delta.after.Binding.Config
	return dcjc.configCreateCleanup(*postimage, config.SourceType(), config.Source(), dcjc.configType(dcjc.modules[config.Module()]))
}

// prepareCandidateCommit resolves dependency keys off-loop. Its returned hook
// only installs the prepared state and releases token-bearing executor effects.
func (dcjc *DynCfgJobController) prepareCandidateCommit(delta *candidateDelta, handoff bool, transitions ...restartObservationTransition) (func(), error) {
	if delta == nil {
		return nil, nil
	}
	var transition restartObservationTransition
	if len(transitions) > 0 {
		transition = transitions[0]
	}
	observe := dcjc.prepareRestartObservationCommit(delta, transition)
	var spec acceptedActivationSpec
	if work := delta.Decision.Work; work != nil && !handoff {
		var err error
		spec, err = dcjc.configModules.newAcceptedActivationSpec(work.Config)
		if err != nil {
			// Invalid proposals are classified by their serialized completion, not
			// by the observational inventory transaction.
			spec = acceptedActivationSpec{config: work.Config, preparationErr: err}
		}
		spec.work = work
	}
	return func() {
		if !delta.apply() {
			delta.owner.mu.Lock()
			retired := delta.owner.closed
			delta.owner.mu.Unlock()
			if retired {
				return
			}
			dcjc.scheduler.accepted.fail(errors.New("job output: candidate transition lost its lane predecessor"))
			return
		}
		if delta.Decision.Ignored {
			return
		}
		observe()
		id := delta.name
		if delta.Decision.Revoke {
			dcjc.scheduler.accepted.cancelUnless(id, "")
		}
		if wake := delta.Decision.Wake; wake != nil {
			dcjc.scheduler.retries.schedule(id, *wake)
		} else {
			dcjc.scheduler.retries.cancel(id)
		}
		if spec.work != nil {
			dcjc.scheduler.accepted.startExecution(spec, dcjc.activationRelease(id, jobmgr.ProcessAttemptJob), nil)
		}
	}, nil
}

// prepareControllerActivation accepts only current controller work. The
// executor's object identity and the controller token are independent fences.
func (dcjc *DynCfgJobController) prepareControllerActivation(ctx context.Context, attempt acceptedActivationAttempt, current lifecycle.ReadyResource, scope lifecycle.ResourceTransactionScope, permit lifecycle.LongLivedPermit) (transaction lifecycle.PreparedResourceTransaction, resultErr error) {
	defer func() {
		var admission *configIntentAdmissionError
		if errors.As(resultErr, &admission) && ctx.Err() == nil && !lifecycle.OwnershipRetained(resultErr) {
			transaction, resultErr = dcjc.prepareControllerAdmissionFailure(ctx, attempt, current, scope, permit, resultErr)
		}
	}()
	work := attempt.spec.work
	state := dcjc.candidates.view(scope.ID)
	if work == nil || state == nil || !dcjc.candidates.valid(scope.ID, work.Token) || !dcjc.scheduler.accepted.isCurrent(scope.ID, attempt.token) {
		return dcjc.noop(scope, current, permit, noResponseResult())
	}
	config := work.Config
	if err := dcjc.validateCandidateMetadata(config); err != nil {
		return dcjc.prepareControllerActivationError(ctx, attempt, current, scope, permit, classifyActivationError(jobmgr.RejectProposal(err)))
	}
	if attempt.stage == nil {
		return dcjc.prepareControllerActivationError(ctx, attempt, current, scope, permit, classifyActivationError(attempt.err))
	}
	if work.Tentative || state.Binding == nil || state.Binding.Key != work.Token.Candidate {
		// Every changed binding retains its checked stage across admission.
		// Resource reservation may yield and invalidate its dependency proof.
		result, err := attempt.stage.inspect()
		if err != nil {
			return dcjc.prepareControllerActivationError(ctx, attempt, current, scope, permit, classifyActivationError(err))
		}
		if result.err != nil {
			return dcjc.prepareControllerActivationError(ctx, attempt, current, scope, permit, classifyActivationError(result.err))
		}
		if result.failure != nil {
			return dcjc.prepareControllerProbeFailure(ctx, attempt, current, scope, permit, result.failure)
		}
		// The checked stage is handed off without repeating Init or Check.
		delta, err := dcjc.candidates.prepareOutcome(scope.ID, work.Token, candidateOutcome{Kind: candidateAdmitted, Listed: true}, dcjc.scheduler.retries.now())
		if err != nil {
			return nil, err
		}
		postimage, err := candidateGraphBinding(scope.ID, delta.Decision.Binding)
		if err != nil {
			return nil, err
		}
		reply := internalReply()
		reply.candidate = delta
		prepared, err := dcjc.prepareCandidateAdoption(ctx, scope, current, permit, attempt.stage, config, postimage, reply,
			dcjc.configCreateCleanup(*postimage, config.SourceType(), config.Source(), dcjc.configType(dcjc.modules[config.Module()])), attempt.markApplied())
		if err == nil && attempt.transferred != nil {
			attempt.transferred.Store(true)
		}
		var preflight *candidatePreflightError
		if errors.As(err, &preflight) {
			return dcjc.prepareControllerActivationError(ctx, attempt, current, scope, permit, classifyActivationError(err))
		}
		return prepared, err
	}
	successor, failure, err := dcjc.factory.prepareCandidate(scope.Successor, permit, attempt.stage)
	if err != nil {
		return dcjc.prepareControllerActivationError(ctx, attempt, current, scope, permit, classifyActivationError(err))
	}
	if failure != nil {
		return dcjc.prepareControllerProbeFailure(ctx, attempt, current, scope, permit, failure)
	}
	delta, err := dcjc.candidates.prepareOutcome(scope.ID, work.Token, candidateOutcome{Kind: candidateAdmitted, Listed: true}, dcjc.scheduler.retries.now())
	if err != nil {
		return nil, rollbackSuccessorMutation(successor, err)
	}
	postimage, err := candidateGraphBinding(scope.ID, delta.Decision.Binding)
	if err != nil {
		return nil, rollbackSuccessorMutation(successor, err)
	}
	successor.state.continuation = work.Token
	reply := internalReply()
	reply.candidate = delta
	return dcjc.prepareMutationWithActivationFallbacks(ctx, scope, current, successor, lifecycle.ResourceTransactionInstalled, postimage, reply,
		dcjc.candidateProjectionCleanup(scope.ID, postimage, delta), composeAfterApply(func() { dcjc.installActivationRestart(attempt.token, scope.Successor) }, attempt.markApplied()),
		activationFallbackPlan{postimage: postimage, cleanup: dcjc.configStatusCleanup(scope.ID, dyncfg.StatusAccepted), afterApply: func() {
			dcjc.scheduler.accepted.waitFor(scope.ID, attempt.token, dcjc.activationRelease(scope.ID, jobmgr.ProcessAttemptJobRuntime))
		}},
		activationFallbackPlan{postimage: postimage, cleanup: dcjc.configStatusCleanup(scope.ID, dyncfg.StatusFailed)})
}

// Admission containment uses a separate identity from collector construction.
// Waiting on the job identity could immediately wake and spin while validation
// still owns its physical attempt; quarantine rejects only this exact proposal.
func (dcjc *DynCfgJobController) prepareControllerAdmissionFailure(ctx context.Context, attempt acceptedActivationAttempt, current lifecycle.ReadyResource, scope lifecycle.ResourceTransactionScope, permit lifecycle.LongLivedPermit, cause error) (lifecycle.PreparedResourceTransaction, error) {
	activation := classifyActivationError(cause)
	switch activation.kind {
	case activationFailureBusy, activationFailureDeadline:
		identity := jobTestAttemptIdentity(configOperationValidateIntent, attempt.spec.config)
		release, ok := dcjc.factory.config.Attempts.ProcessAttemptReleased(identity)
		if !ok {
			released := make(chan struct{})
			close(released)
			release = released
		}
		return dcjc.prepareControllerWait(ctx, attempt, current, scope, permit, cause, release)
	case activationFailureQuarantined:
		activation.kind = activationFailureProposal
		return dcjc.prepareControllerActivationError(ctx, attempt, current, scope, permit, activation)
	case activationFailureProposal:
		return dcjc.prepareControllerActivationError(ctx, attempt, current, scope, permit, activation)
	default:
		return nil, cause
	}
}

func (dcjc *DynCfgJobController) validateCandidateMetadata(config confgroup.Config) error {
	if config == nil || config.Module() == "" || config.Name() == "" {
		return errors.New("job output: discovered config has no identity")
	}
	if err := dyncfg.JobNameRuleStrict(config.Name()); err != nil {
		return err
	}
	creator, ok := dcjc.modules.Lookup(config.Module())
	if !ok {
		return errors.New("job output: discovered module is not registered")
	}
	if err := validateFactoryConfigIdentity(config, creator); err != nil {
		return err
	}
	if !netdataapi.ValidSingleQuotedProtocolField(config.Source()) || !netdataapi.ValidBareProtocolField(config.SourceType()) {
		return errors.New("job output: discovered config has invalid protocol metadata")
	}
	return nil
}

func (dcjc *DynCfgJobController) prepareControllerProbeFailure(ctx context.Context, attempt acceptedActivationAttempt, current lifecycle.ReadyResource, scope lifecycle.ResourceTransactionScope, permit lifecycle.LongLivedPermit, failure *autoDetectionFailure) (lifecycle.PreparedResourceTransaction, error) {
	config := attempt.spec.config
	if dcjc.configModules.activationWaitsForDependency(config, failure) {
		return dcjc.prepareControllerWait(ctx, attempt, current, scope, permit, failure, nil)
	}
	retry := 0
	if failure.retry {
		retry = failure.retryAfter
	}
	listed := config.SourceType() != confgroup.TypeStock || failure.keepsFailedStockListed()
	delta, err := dcjc.candidates.prepareOutcome(scope.ID, attempt.spec.work.Token, candidateOutcome{Kind: candidateFailed, RetryAfter: retry, Listed: listed}, dcjc.scheduler.retries.now())
	if err != nil {
		return nil, err
	}
	postimage, err := candidateGraphBinding(scope.ID, delta.Decision.Binding)
	if err != nil {
		return nil, err
	}
	transaction, err := dcjc.prepareCandidateDecision(ctx, scope, current, permit, delta, candidateDecisionOptions{
		lifecycle: preparedJobConfigLifecycle{
			identity: dcjc.postimageJobConfigLifecycleGraphState(postimage).identity,
			snapshot: failure.jobConfigLifecycle,
			failure:  failure.diagnosticFailure,
		},
		restart: restartObservationTransition{settlement: &activationRestartSettlement{
			work:   attempt.spec.work.Token,
			result: adoptedResult(dyncfg.CommandRestart, dyncfg.StatusFailed, collectorFailure(failure, "job restart failed: %v")),
		}},
	})
	if err != nil {
		return nil, err
	}
	return &candidateCompletionTransaction{PreparedResourceTransaction: transaction, applied: attempt.markApplied()}, nil
}

func (dcjc *DynCfgJobController) prepareControllerActivationError(ctx context.Context, attempt acceptedActivationAttempt, current lifecycle.ReadyResource, scope lifecycle.ResourceTransactionScope, permit lifecycle.LongLivedPermit, activation activationFailure) (lifecycle.PreparedResourceTransaction, error) {
	err := activation.err
	if err == nil {
		return nil, errors.New("job output: activation lacks an outcome")
	}
	if lifecycle.OwnershipRetained(err) {
		return nil, err
	}
	if jobmgr.ContainsOnlyErrorLeaves(err, jobmgr.ErrProcessAttemptRetired, jobmgr.ErrProcessAttemptStopped, context.Canceled) {
		return dcjc.noop(scope, current, permit, noResponseResult())
	}
	if dcjc.configModules.activationWaitsForDependency(attempt.spec.config, err) {
		return dcjc.prepareControllerWait(ctx, attempt, current, scope, permit, err, nil)
	}
	switch activation.kind {
	case activationFailureBusy, activationFailureStaleStore, activationFailureSuperseded:
		return dcjc.prepareControllerWait(ctx, attempt, current, scope, permit, err, dcjc.activationRelease(scope.ID, jobmgr.ProcessAttemptJob))
	case activationFailureProposal:
		settlement := restartObservationTransition{settlement: &activationRestartSettlement{
			work: attempt.spec.work.Token, result: rejectedResult(testFailure(err)),
		}}
		state := dcjc.candidates.view(scope.ID)
		if state != nil && state.Binding != nil {
			if !attempt.spec.work.Tentative && state.Binding.Key == attempt.spec.work.Token.Candidate {
				delta, prepareErr := dcjc.candidates.prepareOutcome(scope.ID, attempt.spec.work.Token, candidateOutcome{Kind: candidateAdmittedInvalid, Listed: state.Binding.Listed}, dcjc.scheduler.retries.now())
				if prepareErr != nil {
					return nil, prepareErr
				}
				postimage, prepareErr := candidateGraphBinding(scope.ID, delta.Decision.Binding)
				if prepareErr != nil {
					return nil, prepareErr
				}
				return dcjc.prepareCandidateDecision(ctx, scope, current, permit, delta, candidateDecisionOptions{
					lifecycle: preparedJobConfigLifecycle{identity: dcjc.postimageJobConfigLifecycleGraphState(postimage).identity, failure: jobConfigFailure(err, "activation")},
					restart:   settlement,
				})
			}
		}
		delta, prepareErr := dcjc.candidates.prepareOutcome(scope.ID, attempt.spec.work.Token, candidateOutcome{Kind: candidateRejected}, dcjc.scheduler.retries.now())
		if prepareErr != nil {
			return nil, prepareErr
		}
		prepared, prepareErr := dcjc.prepareCandidateDecision(ctx, scope, current, permit, delta, candidateDecisionOptions{restart: settlement})
		if prepareErr != nil {
			return nil, prepareErr
		}
		return &candidateCompletionTransaction{PreparedResourceTransaction: prepared, applied: composeAfterApply(attempt.markApplied(), func() {
			jobmgr.ObserveDiagnostic(dcjc.diagnostics, jobmgr.DiagnosticEvent{
				Level: jobmgr.DiagnosticWarning, Name: "discovered collector proposal rejected", Resource: scope.ID,
				State: "rejected", Generation: dcjc.generation, Err: jobmgr.RejectProposal(err),
			})
		})}, nil
	case activationFailureDeadline:
		if attempt.spec.work.Tentative {
			return dcjc.prepareControllerBlocked(ctx, attempt, current, scope, permit, err, candidateBlockAcceptedWait)
		}
		return dcjc.prepareControllerBlocked(ctx, attempt, current, scope, permit, err, candidateBlockFailedRetry)
	case activationFailureTransient:
		return dcjc.prepareControllerProbeFailure(ctx, attempt, current, scope, permit, transientActivationFailure(attempt.spec.config, err))
	case activationFailureQuarantined:
		return dcjc.prepareControllerBlocked(ctx, attempt, current, scope, permit, err, candidateBlockQuarantined)
	default:
		return nil, err
	}
}

func (dcjc *DynCfgJobController) prepareControllerWait(ctx context.Context, attempt acceptedActivationAttempt, current lifecycle.ReadyResource, scope lifecycle.ResourceTransactionScope, permit lifecycle.LongLivedPermit, cause error, release <-chan struct{}) (lifecycle.PreparedResourceTransaction, error) {
	admit := dcjc.configModules.activationWaitsForDependency(attempt.spec.config, cause)
	delta, err := dcjc.candidates.prepareOutcome(scope.ID, attempt.spec.work.Token, candidateOutcome{Kind: candidateWaiting, Listed: true, AdmitWaiting: admit}, dcjc.scheduler.retries.now())
	if err != nil {
		return nil, err
	}
	wait := func() { dcjc.scheduler.accepted.waitForController(scope.ID, attempt.token, release) }
	if !admit && (delta.after.Binding == nil || delta.after.Binding.Key != attempt.spec.work.Token.Candidate || attempt.spec.work.Tentative) {
		commit, err := dcjc.prepareCandidateCommit(delta, false)
		if err != nil {
			return nil, err
		}
		return dcjc.noopWithAfterApply(scope, current, permit, noResponseResult(), composeAfterApply(commit, wait, attempt.markApplied()))
	}
	postimage, err := candidateGraphBinding(scope.ID, delta.after.Binding)
	if err != nil {
		return nil, err
	}
	if postimage == nil {
		return nil, errors.New("job output: admitted wait has no exposed binding")
	}
	reply := internalReply()
	reply.candidate = delta
	return dcjc.prepareMutationAfterApplyAndFallback(ctx, scope, current, nil, permit, resourceRemovalDisposition(current), postimage, reply,
		dcjc.candidateProjectionCleanup(scope.ID, postimage, delta), composeAfterApply(wait, attempt.markApplied()), preparedJobConfigLifecycle{identity: dcjc.postimageJobConfigLifecycleGraphState(postimage).identity, failure: jobConfigFailure(cause, "activation")}, jobConfigIntentEnable, nil, nil, nil)
}

func (dcjc *DynCfgJobController) prepareControllerBlocked(ctx context.Context, attempt acceptedActivationAttempt, current lifecycle.ReadyResource, scope lifecycle.ResourceTransactionScope, permit lifecycle.LongLivedPermit, cause error, block candidateBlockPolicy) (lifecycle.PreparedResourceTransaction, error) {
	retryAfter := 0
	if failure := transientActivationFailure(attempt.spec.config, cause); failure.retry {
		retryAfter = failure.retryAfter
	}
	delta, err := dcjc.candidates.prepareOutcome(scope.ID, attempt.spec.work.Token, candidateOutcome{Kind: candidateFrameworkBlocked, Block: block, RetryAfter: retryAfter, Listed: true}, dcjc.scheduler.retries.now())
	if err != nil {
		return nil, err
	}
	postimage, err := candidateGraphBinding(scope.ID, delta.Decision.Binding)
	if err != nil {
		return nil, err
	}
	option := candidateDecisionOptions{lifecycle: preparedJobConfigLifecycle{
		identity: dcjc.postimageJobConfigLifecycleGraphState(postimage).identity, failure: jobConfigFailure(cause, "activation"),
	}}
	if block != candidateBlockAcceptedWait {
		option.restart.settlement = &activationRestartSettlement{
			work: attempt.spec.work.Token, result: rejectedResult(testFailure(cause)),
		}
	}
	prepared, err := dcjc.prepareCandidateDecision(ctx, scope, current, permit, delta, option)
	if err != nil {
		return nil, err
	}
	after := attempt.markApplied()
	if block == candidateBlockAcceptedWait {
		after = composeAfterApply(after, func() {
			dcjc.scheduler.accepted.waitForController(scope.ID, attempt.token, dcjc.activationRelease(scope.ID, jobmgr.ProcessAttemptJob))
		})
	}
	return &candidateCompletionTransaction{PreparedResourceTransaction: prepared, applied: after}, nil
}

type candidateCompletionTransaction struct {
	lifecycle.PreparedResourceTransaction
	applied func()
}

func (t *candidateCompletionTransaction) Apply(ctx context.Context) (lifecycle.AppliedResourceTransaction, error) {
	applied, err := t.PreparedResourceTransaction.Apply(ctx)
	if err == nil {
		t.applied()
	}
	return applied, err
}

func (dcjc *DynCfgJobController) planAutoDetectionRetry(id string, token candidateWakeToken) (jobmgr.WorkPlan, error) {
	if dcjc == nil || id == "" {
		return jobmgr.WorkPlan{}, errors.New("job output: invalid candidate wake")
	}
	return jobmgr.WorkPlan{Claims: []string{DynCfgJobGraphClaim}, NoResponse: true, YieldClaimOnPrepare: DynCfgJobGraphClaim, Transaction: &jobmgr.ResourceTransactionPlan{ID: id,
		Prepare: func(ctx context.Context, current lifecycle.ReadyResource, scope lifecycle.ResourceTransactionScope, permit lifecycle.LongLivedPermit) (lifecycle.PreparedResourceTransaction, error) {
			delta, err := dcjc.candidates.prepareWake(id, token, dcjc.scheduler.retries.now())
			if err != nil {
				return nil, err
			}
			return dcjc.prepareCandidateDecision(ctx, scope, current, permit, delta)
		},
	}}, nil
}

func (dcjc *DynCfgJobController) candidateFailure(err error) {
	dcjc.scheduler.accepted.fail(fmt.Errorf("job output: candidate continuation: %w", err))
}

// Only accepted public mutations create new exact-config intent. Internal
// transitions supply their already prepared capability explicitly.
func (dcjc *DynCfgJobController) prepareControlCandidate(id string, postimage *dyncfg.GraphConfig, reply jobReply) (*candidateDelta, error) {
	if reply.candidate != nil {
		return reply.candidate, nil
	}
	switch reply.command {
	case dyncfg.CommandDisable:
		return dcjc.candidates.prepareDisable(id)
	case dyncfg.CommandRemove:
		return dcjc.candidates.prepareRemove(id)
	case dyncfg.CommandAdd, dyncfg.CommandUpdate, dyncfg.CommandRestart:
		if postimage == nil {
			return nil, errors.New("job output: exact acceptance has no binding")
		}
		var config confgroup.Config
		if err := yaml.Unmarshal(postimage.Payload, &config); err != nil {
			return nil, err
		}
		status := dyncfg.Status(postimage.Status)
		enabled := status != dyncfg.StatusDisabled && reply.command != dyncfg.CommandAdd
		return dcjc.candidates.prepareAdoptExact(config, enabled, status, true, 0, dcjc.scheduler.retries.now())
	default:
		return nil, nil
	}
}
