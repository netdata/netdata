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

const DynCfgJobGraphClaim = jobmgr.DynCfgJobGraphClaim

type DiscoveredJobChange struct {
	Config  confgroup.Config // discovered job configuration
	Status  dyncfg.Status    // target graph status (Accepted / Running)
	Remove  bool             // remove the job rather than install it
	Restart bool             // force a running job to re-prepare
	pending pendingJobToken  // latest-pending token (zero = ordinary change)
}

func (dcjc *DynCfgJobController) planAutoDetectionRetry(
	config confgroup.Config,
	token autoDetectionRetryToken,
) (jobmgr.WorkPlan, error) {
	if dcjc == nil || config == nil {
		return jobmgr.WorkPlan{}, errors.New("job output: invalid activation retry")
	}
	return jobmgr.WorkPlan{
		Claims: []string{DynCfgJobGraphClaim}, NoResponse: true,
		Transaction: &jobmgr.ResourceTransactionPlan{
			ID: config.FullName(),
			Prepare: func(_ context.Context, current lifecycle.ReadyResource, scope lifecycle.ResourceTransactionScope, permit lifecycle.LongLivedPermit) (lifecycle.PreparedResourceTransaction, error) {
				settle := dcjc.retrySettlement(scope.ID, token)
				record, exists := dcjc.graph.Lookup(scope.ID)
				if !dcjc.scheduler.retries.isCurrent(scope.ID, token) || current != nil {
					return dcjc.noopWithAfterApply(scope, current, permit, noResponseResult(), settle)
				}
				committed := config
				if exists {
					var err error
					committed, err = graphRecordConfig(record)
					if err != nil {
						return nil, err
					}
					if record.Status != dyncfg.StatusFailed.String() || committed.UID() != token.uid {
						return dcjc.noopWithAfterApply(scope, current, permit, noResponseResult(), settle)
					}
				} else if config.SourceType() != confgroup.TypeStock {
					return dcjc.noopWithAfterApply(scope, current, permit, noResponseResult(), settle)
				}
				spec, err := newAcceptedActivationSpec(committed)
				if err != nil {
					return nil, err
				}
				payload, err := yaml.Marshal(committed)
				if err != nil {
					return nil, err
				}
				postimage := dyncfg.GraphConfig{ID: scope.ID, Module: committed.Module(), Name: committed.Name(), Status: dyncfg.StatusAccepted.String(), Payload: payload}
				cleanup := dcjc.configStatusCleanup(scope.ID, dyncfg.StatusAccepted)
				if !exists {
					cleanup = dcjc.configCreateCleanup(postimage, committed.SourceType(), committed.Source(), dcjc.configType(dcjc.modules[committed.Module()]))
				}
				return dcjc.prepareMutationWithRetryAfterApply(scope, current, nil, permit, lifecycle.ResourceTransactionUnchanged,
					&postimage, internalReply(), cleanup, token,
					func() { dcjc.scheduler.accepted.arm(spec) })
			},
		},
	}, nil
}

func (dcjc *DynCfgJobController) planPendingJob(
	config confgroup.Config,
	token pendingJobToken,
) (jobmgr.WorkPlan, error) {
	return dcjc.PlanDiscovered(DiscoveredJobChange{
		Config:  config,
		Status:  dyncfg.StatusRunning,
		Restart: true,
		pending: token,
	})
}

// PlanDiscovered builds one typed, response-free graph/job reconciliation
// plan. The caller submits it through jobmgr.PreparedCommandPort with the
// config carried directly on the plan.
func (dcjc *DynCfgJobController) PlanDiscovered(change DiscoveredJobChange) (jobmgr.WorkPlan, error) {
	if dcjc == nil || change.Config == nil {
		return jobmgr.WorkPlan{}, errors.New("job output: invalid discovered job change")
	}
	config, err := change.Config.Clone()
	if err != nil {
		return jobmgr.WorkPlan{}, jobmgr.RejectProposal(fmt.Errorf(
			"job output: clone discovered config: %w",
			err,
		))
	}
	if config.Module() == "" || config.Name() == "" {
		return jobmgr.WorkPlan{}, jobmgr.RejectProposal(
			errors.New("job output: discovered config has no identity"),
		)
	}
	if err := dyncfg.JobNameRuleStrict(config.Name()); err != nil {
		return jobmgr.WorkPlan{}, jobmgr.RejectProposal(fmt.Errorf(
			"job output: discovered config has an unpublishable name %q: %w",
			config.Name(),
			err,
		))
	}
	creator, ok := dcjc.modules.Lookup(config.Module())
	if !ok {
		return jobmgr.WorkPlan{}, jobmgr.RejectProposal(
			errors.New("job output: discovered module is not registered"),
		)
	}
	if err := validateFactoryConfigIdentity(config, creator); err != nil {
		return jobmgr.WorkPlan{}, jobmgr.RejectProposal(err)
	}
	if !netdataapi.ValidSingleQuotedProtocolField(config.Source()) ||
		!netdataapi.ValidBareProtocolField(config.SourceType()) {
		return jobmgr.WorkPlan{}, jobmgr.RejectProposal(
			errors.New("job output: discovered config has invalid protocol metadata"),
		)
	}
	if change.Remove {
		if change.Restart {
			return jobmgr.WorkPlan{}, errors.New("job output: removed discovery config cannot restart")
		}
		change.Status = ""
	} else if change.Status != dyncfg.StatusAccepted && change.Status != dyncfg.StatusRunning {
		return jobmgr.WorkPlan{}, errors.New("job output: invalid discovered config status")
	}
	permit := lifecycle.LongLivedPlan{}
	if change.Restart && change.Status != dyncfg.StatusRunning {
		return jobmgr.WorkPlan{}, errors.New("job output: only a running discovery config can restart")
	}
	if change.Status == dyncfg.StatusRunning {
		permit = lifecycle.NewJobLongLivedPlan()
	}
	change.Config = config
	return jobmgr.WorkPlan{
		Claims:              []string{DynCfgJobGraphClaim},
		NoResponse:          true,
		YieldClaimOnPrepare: DynCfgJobGraphClaim,
		Transaction: &jobmgr.ResourceTransactionPlan{
			ID:                config.FullName(),
			AllocateSuccessor: change.Status == dyncfg.StatusRunning,
			Permit:            permit,
			Prepare: func(
				ctx context.Context,
				current lifecycle.ReadyResource,
				scope lifecycle.ResourceTransactionScope,
				permit lifecycle.LongLivedPermit,
			) (lifecycle.PreparedResourceTransaction, error) {
				return dcjc.prepareDiscovered(ctx, change, current, scope, permit)
			},
		},
	}, nil
}

func (dcjc *DynCfgJobController) prepareDiscovered(
	ctx context.Context,
	change DiscoveredJobChange,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
	permit lifecycle.LongLivedPermit,
) (
	transaction lifecycle.PreparedResourceTransaction,
	resultErr error,
) {
	defer func() {
		if resultErr != nil && change.pending.version != 0 {
			dcjc.scheduler.pending.settle(scope.ID, change.pending)
		}
	}()
	pendingSettlement := dcjc.pendingSettlement(scope.ID, change.pending)
	settlement := pendingSettlement
	record, exists := dcjc.graph.Lookup(scope.ID)
	if err := validateGraphResourcePair(record, exists, current, scope); err != nil {
		return nil, err
	}
	result := noResponseResult()
	reply := internalReply()
	if change.pending.version != 0 &&
		(!dcjc.scheduler.pending.isCurrent(scope.ID, change.pending) ||
			change.pending.uid != change.Config.UID()) {
		return dcjc.noopWithAfterApply(scope, current, permit, result, settlement)
	}
	var incumbent confgroup.Config
	if exists {
		var err error
		incumbent, err = graphRecordConfig(record)
		if err != nil {
			return nil, err
		}
	}
	if change.pending.version != 0 &&
		(change.pending.baselineUID == "" && exists ||
			change.pending.baselineUID != "" &&
				(!exists || incumbent.UID() != change.pending.baselineUID)) {
		return dcjc.noopWithAfterApply(
			scope,
			current,
			permit,
			result,
			settlement,
		)
	}
	if change.Remove {
		settlement = composeAfterApply(settlement, func() { dcjc.scheduler.retries.cancelConfig(scope.ID, change.Config.UID()) })
		if !exists || incumbent.UID() != change.Config.UID() {
			return dcjc.noopWithAfterApply(
				scope,
				current,
				permit,
				result,
				composeAfterApply(
					settlement,
					dcjc.pendingDesiredSettlement(change.Config, change.pending),
				),
			)
		}
		return dcjc.prepareMutationWithRetryAfterApply(
			scope,
			current,
			nil,
			lifecycle.LongLivedPermit{},
			resourceRemovalDisposition(current),
			nil,
			reply,
			dcjc.configDeleteCleanup(dcjc.configID(change.Config.Module(), change.Config.Name())),
			autoDetectionRetryToken{},
			dcjc.pendingDesiredSettlement(change.Config, change.pending),
		)
	}
	if exists &&
		confgroup.SourceTypePriority(incumbent.SourceType()) >
			confgroup.SourceTypePriority(change.Config.SourceType()) {
		return dcjc.noopWithAfterApply(
			scope,
			current,
			permit,
			result,
			settlement,
		)
	}

	payload, err := yaml.Marshal(change.Config)
	if err != nil {
		return nil, jobmgr.RejectProposal(fmt.Errorf("job output: marshal discovered config: %w", err))
	}
	postimage := dyncfg.GraphConfig{
		ID:      scope.ID,
		Module:  change.Config.Module(),
		Name:    change.Config.Name(),
		Status:  change.Status.String(),
		Payload: payload,
	}
	cleanup := dcjc.configCreateCleanup(
		postimage,
		change.Config.SourceType(),
		change.Config.Source(),
		dcjc.configType(dcjc.modules[change.Config.Module()]),
	)
	if change.Status == dyncfg.StatusAccepted {
		return dcjc.prepareMutationWithRetryAfterApply(
			scope,
			current,
			nil,
			lifecycle.LongLivedPermit{},
			resourceRemovalDisposition(current),
			&postimage,
			reply,
			cleanup,
			autoDetectionRetryToken{},
			dcjc.pendingDesiredSettlement(change.Config, change.pending),
		)
	}
	if exists &&
		(record.Status == dyncfg.StatusRunning.String() || record.Status == dyncfg.StatusAccepted.String() && current != nil) &&
		record.Payload() == string(payload) &&
		!change.Restart {
		postimage.Status = record.Status
		cleanup = dcjc.configCreateCleanup(postimage, change.Config.SourceType(), change.Config.Source(), dcjc.configType(dcjc.modules[change.Config.Module()]))
		return dcjc.noopWithAfterApply(
			scope,
			current,
			permit,
			result,
			composeAfterApply(
				settlement,
				dcjc.pendingDesiredSettlement(change.Config, change.pending),
			),
			cleanup,
		)
	}
	stage, probeFailure, activation := dcjc.prepareContainedCandidate(ctx, change.Config)
	if err := activation.err; err != nil {
		if ctx.Err() != nil || lifecycle.OwnershipRetained(err) {
			return nil, err
		}
		if activationWaitsForDependency(change.Config, err) {
			return dcjc.prepareDiscoveredActivationWait(change, current, scope, permit, postimage, pendingSettlement, err, 0)
		}
		switch activation.kind {
		case activationFailureQuarantined, activationFailureProposal:
			if activation.kind == activationFailureProposal {
				trustRevoked := incumbent.SourceType() == confgroup.TypeDiscovered &&
					incumbent.TrustDiscoveredTargets() &&
					incumbent.DiscoveryPipelineID() != "" &&
					incumbent.DiscoveryPipelineID() == change.Config.DiscoveryPipelineID() &&
					change.Config.SourceType() == confgroup.TypeDiscovered &&
					!change.Config.TrustDiscoveredTargets()
				if !trustRevoked {
					return nil, jobmgr.RejectProposal(err)
				}
				// Only the owning pipeline can revoke trust despite invalid literal fields.
			}
			failedPostimage := postimage
			failedPostimage.Status = dyncfg.StatusFailed.String()
			return dcjc.prepareMutationWithRetryAfterApply(
				scope,
				current,
				nil,
				permit,
				resourceRemovalDisposition(current),
				&failedPostimage,
				reply,
				dcjc.configCreateCleanup(
					failedPostimage,
					change.Config.SourceType(),
					change.Config.Source(),
					dcjc.configType(dcjc.modules[change.Config.Module()]),
				),
				autoDetectionRetryToken{},
				pendingSettlement,
				jobConfigFailure(err, "activation"),
			)
		case activationFailureBusy, activationFailureStaleStore:
			baselineUID := ""
			if exists {
				baselineUID = incumbent.UID()
			}
			return dcjc.noopWithAfterApply(
				scope,
				current,
				permit,
				result,
				composeAfterApply(
					settlement,
					dcjc.retainPendingAfterApply(
						change.Config,
						jobmgr.ProcessAttemptJob,
						baselineUID,
					),
				),
			)
		case activationFailureSuperseded:
			return dcjc.noopWithAfterApply(scope, current, permit, result, settlement)
		case activationFailureDeadline:
			return dcjc.prepareDiscoveredActivationWait(change, current, scope, permit, postimage, pendingSettlement, err, jobmgr.ProcessAttemptJob)
		case activationFailureTransient:
			failedPostimage := postimage
			failedPostimage.Status = dyncfg.StatusFailed.String()
			failure := transientActivationFailure(change.Config, err)
			return dcjc.prepareMutationWithRetryAfterApply(
				scope,
				current,
				nil,
				permit,
				resourceRemovalDisposition(current),
				&failedPostimage,
				reply,
				dcjc.configCreateCleanup(
					failedPostimage,
					change.Config.SourceType(),
					change.Config.Source(),
					dcjc.configType(dcjc.modules[change.Config.Module()]),
				),
				autoDetectionRetryToken{},
				composeAfterApply(
					pendingSettlement,
					func() {
						dcjc.scheduleAutoDetectionRetry(change.Config, failure)
					},
				),
				jobConfigFailure(err, "activation"),
			)
		default:
			return nil, err
		}
	}
	failedPostimage := postimage
	failedPostimage.Status = dyncfg.StatusFailed.String()
	failurePlan := probeFailurePlan{
		postimage: failedPostimage,
		failedCleanup: dcjc.configCreateCleanup(
			failedPostimage,
			change.Config.SourceType(),
			change.Config.Source(),
			dcjc.configType(dcjc.modules[change.Config.Module()]),
		),
		removedCleanup: dcjc.configDeleteCleanup(
			dcjc.configID(change.Config.Module(), change.Config.Name()),
		),
		reply: internalFailureReply,
		afterApply: func(failure *autoDetectionFailure) {
			dcjc.scheduleAutoDetectionRetry(change.Config, failure)
			if pendingSettlement != nil {
				pendingSettlement()
			}
		},
		removePlainStock: change.Config.SourceType() == confgroup.TypeStock,
	}
	if probeFailure != nil {
		if activationWaitsForDependency(change.Config, probeFailure) {
			return dcjc.prepareDiscoveredActivationWait(change, current, scope, permit, postimage, pendingSettlement, probeFailure, 0)
		}
		return dcjc.prepareProbeFailure(
			scope,
			current,
			permit,
			autoDetectionRetryToken{},
			probeFailure,
			failurePlan,
		)
	}
	postimage.Status = dyncfg.StatusAccepted.String()
	cleanup = dcjc.configCreateCleanup(postimage, change.Config.SourceType(), change.Config.Source(), dcjc.configType(dcjc.modules[change.Config.Module()]))
	return dcjc.prepareCandidateAdoption(
		scope, current, permit, stage, change.Config, &postimage, reply, cleanup,
		dcjc.pendingDesiredSettlement(change.Config, change.pending),
	)
}

func (dcjc *DynCfgJobController) prepareDiscoveredActivationWait(
	change DiscoveredJobChange,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
	permit lifecycle.LongLivedPermit,
	postimage dyncfg.GraphConfig,
	settle func(),
	cause error,
	namespace jobmgr.ProcessAttemptNamespace,
) (lifecycle.PreparedResourceTransaction, error) {
	spec, err := newAcceptedActivationSpec(change.Config)
	if err != nil {
		return nil, err
	}
	activate := func() { dcjc.scheduler.accepted.arm(spec) }
	if namespace != 0 {
		activate = func() { dcjc.scheduler.accepted.armAfterRelease(spec, dcjc.activationRelease(scope.ID, namespace)) }
	}
	postimage.Status = dyncfg.StatusAccepted.String()
	return dcjc.prepareMutationWithRetryAfterApply(scope, current, nil, permit,
		resourceRemovalDisposition(current), &postimage, internalReply(),
		dcjc.configCreateCleanup(postimage, change.Config.SourceType(), change.Config.Source(), dcjc.configType(dcjc.modules[change.Config.Module()])),
		autoDetectionRetryToken{}, composeAfterApply(settle, activate), jobConfigFailure(cause, "activation"))
}
