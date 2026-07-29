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
	Config  confgroup.Config        // discovered job configuration
	Status  dyncfg.Status           // target graph status (Accepted / Running)
	Remove  bool                    // remove the job rather than install it
	Restart bool                    // force a running job to re-prepare
	retry   autoDetectionRetryToken // auto-detection retry token (zero = not a retry)
	pending pendingJobToken         // latest-pending token (zero = ordinary change)
}

func (dcjc *DynCfgJobController) planAutoDetectionRetry(
	config confgroup.Config,
	token autoDetectionRetryToken,
) (jobmgr.WorkPlan, error) {
	return dcjc.PlanDiscovered(DiscoveredJobChange{
		Config:  config,
		Status:  dyncfg.StatusRunning,
		Restart: true,
		retry:   token,
	})
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
		if resultErr != nil && change.retry.generation != 0 {
			dcjc.scheduler.retries.cancelToken(scope.ID, change.retry)
		}
		if resultErr != nil && change.pending.version != 0 {
			dcjc.scheduler.pending.settle(scope.ID, change.pending)
		}
	}()
	pendingSettlement := dcjc.pendingSettlement(scope.ID, change.pending)
	settlement := composeAfterApply(
		dcjc.retrySettlement(scope.ID, change.retry),
		pendingSettlement,
	)
	record, exists := dcjc.graph.Lookup(scope.ID)
	if err := validateGraphResourcePair(record, exists, current, scope); err != nil {
		return nil, err
	}
	result := mustDynCfgMessage(204, "")
	if change.pending.version != 0 &&
		(!dcjc.scheduler.pending.isCurrent(scope.ID, change.pending) ||
			change.pending.uid != change.Config.UID()) {
		return dcjc.noopWithAfterApply(scope, current, permit, result, settlement)
	}
	// A child deadline can race with successful terminal settlement. Pending
	// work retained after removal must not restart a job the child restored.
	if change.pending.requireAbsent &&
		(current != nil || !exists || record.Status != dyncfg.StatusFailed.String()) {
		return dcjc.noopWithAfterApply(scope, current, permit, result, settlement)
	}
	if change.retry.generation != 0 {
		currentToken := dcjc.scheduler.retries.isCurrent(scope.ID, change.retry)
		validRecord := true
		if exists {
			config, err := graphRecordConfig(record)
			validRecord = err == nil &&
				record.Status == dyncfg.StatusFailed.String() &&
				config.UID() == change.retry.uid
		} else {
			validRecord = change.Config.SourceType() == confgroup.TypeStock
		}
		if !currentToken || !validRecord {
			return dcjc.noopWithAfterApply(
				scope,
				current,
				permit,
				result,
				settlement,
			)
		}
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
			result,
			dcjc.configDeleteCleanup(dcjc.configID(change.Config.Module(), change.Config.Name())),
			change.retry,
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
			result,
			cleanup,
			change.retry,
			dcjc.pendingDesiredSettlement(change.Config, change.pending),
		)
	}
	if exists &&
		record.Status == dyncfg.StatusRunning.String() &&
		record.Payload() == string(payload) &&
		!change.Restart {
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
	successor, probeFailure, err := dcjc.prepareContainedJob(
		ctx,
		change.Config,
		scope.Successor,
		permit,
	)
	if err != nil {
		if ctx.Err() != nil || lifecycle.OwnershipRetained(err) {
			return nil, err
		}
		if errors.Is(err, jobmgr.ErrProcessAttemptQuarantined) {
			failedPostimage := postimage
			failedPostimage.Status = dyncfg.StatusFailed.String()
			return dcjc.prepareMutationWithRetryAfterApply(
				scope,
				current,
				nil,
				permit,
				resourceRemovalDisposition(current),
				&failedPostimage,
				result,
				dcjc.configCreateCleanup(
					failedPostimage,
					change.Config.SourceType(),
					change.Config.Source(),
					dcjc.configType(dcjc.modules[change.Config.Module()]),
				),
				change.retry,
				pendingSettlement,
			)
		}
		if candidatePreparationBusy(err) {
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
		}
		if errors.Is(err, jobmgr.ErrProcessAttemptSuperseded) {
			return dcjc.noopWithAfterApply(scope, current, permit, result, settlement)
		}
		if errors.Is(err, jobmgr.ErrProcessAttemptDeadline) {
			failedPostimage := postimage
			failedPostimage.Status = dyncfg.StatusFailed.String()
			return dcjc.prepareMutationWithRetryAfterApply(
				scope,
				current,
				nil,
				permit,
				resourceRemovalDisposition(current),
				&failedPostimage,
				result,
				dcjc.configCreateCleanup(
					failedPostimage,
					change.Config.SourceType(),
					change.Config.Source(),
					dcjc.configType(dcjc.modules[change.Config.Module()]),
				),
				change.retry,
				composeAfterApply(
					pendingSettlement,
					dcjc.retainAbsentPendingAfterApply(
						change.Config,
						jobmgr.ProcessAttemptJob,
						change.Config.UID(),
					),
				),
			)
		}
		switch classifyConstructionError(err) {
		case constructionErrorTransient:
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
				result,
				dcjc.configCreateCleanup(
					failedPostimage,
					change.Config.SourceType(),
					change.Config.Source(),
					dcjc.configType(dcjc.modules[change.Config.Module()]),
				),
				change.retry,
				composeAfterApply(
					pendingSettlement,
					func() {
						dcjc.scheduleAutoDetectionRetry(change.Config, failure)
					},
				),
			)
		case constructionErrorProposal:
			return nil, jobmgr.RejectProposal(err)
		default:
			return nil, err
		}
	}
	failedPostimage := postimage
	failedPostimage.Status = dyncfg.StatusFailed.String()
	if probeFailure != nil {
		return dcjc.prepareProbeFailure(
			scope,
			current,
			permit,
			change.retry,
			probeFailure,
			probeFailurePlan{
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
				result: func(*autoDetectionFailure) lifecycle.SealedResult {
					return result
				},
				afterApply: func(failure *autoDetectionFailure) {
					dcjc.scheduleAutoDetectionRetry(change.Config, failure)
					if pendingSettlement != nil {
						pendingSettlement()
					}
				},
				removePlainStock: change.Config.SourceType() == confgroup.TypeStock,
			},
		)
	}
	failedCleanup := dcjc.configCreateCleanup(
		failedPostimage,
		change.Config.SourceType(),
		change.Config.Source(),
		dcjc.configType(dcjc.modules[change.Config.Module()]),
	)
	return dcjc.prepareMutationWithActivationFallbacks(
		scope,
		current,
		successor,
		resourceInstallationDisposition(current),
		&postimage,
		result,
		cleanup,
		change.retry,
		dcjc.pendingDesiredSettlement(change.Config, change.pending),
		activationFallbackPlan{
			postimage: &failedPostimage,
			result:    result,
			cleanup:   failedCleanup,
			afterApply: composeAfterApply(
				settlement,
				dcjc.retainAbsentPendingAfterApply(
					change.Config,
					jobmgr.ProcessAttemptJobRuntime,
					change.Config.UID(),
				),
			),
		},
		activationFallbackPlan{
			postimage:  &failedPostimage,
			result:     result,
			cleanup:    failedCleanup,
			afterApply: settlement,
		},
	)
}
