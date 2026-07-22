// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"gopkg.in/yaml.v2"
)

const DynCfgJobGraphClaim = "dyncfg:jobs"

type DiscoveredJobChange struct {
	Config  confgroup.Config        // discovered job configuration
	Status  dyncfg.Status           // target graph status (Accepted / Running)
	Remove  bool                    // remove the job rather than install it
	Restart bool                    // force a running job to re-prepare
	retry   autoDetectionRetryToken // auto-detection retry token (zero = not a retry)
}

func (dcjc *DynCfgJobController) planAutoDetectionRetry(
	config confgroup.Config,
	token autoDetectionRetryToken,
) (jobmgr.WorkPlan, error) {
	return dcjc.PlanDiscovered(DiscoveredJobChange{
		Config: config, Status: dyncfg.StatusRunning,
		Restart: true, retry: token,
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
		return jobmgr.WorkPlan{}, err
	}
	creator, ok := dcjc.modules.Lookup(config.Module())
	if !ok {
		return jobmgr.WorkPlan{}, errors.New("job output: discovered module is not registered")
	}
	if err := validateFactoryConfigIdentity(config, creator); err != nil {
		return jobmgr.WorkPlan{}, err
	}
	if config.FullName() == "" {
		return jobmgr.WorkPlan{}, errors.New("job output: discovered config has no identity")
	}
	if !validDynCfgProtocolField(config.Source()) || !validDynCfgProtocolField(config.SourceType()) {
		return jobmgr.WorkPlan{}, errors.New("job output: discovered config has invalid protocol metadata")
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
	return jobmgr.WorkPlan{
		Claims:     []string{DynCfgJobGraphClaim},
		NoResponse: true,
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
				return dcjc.prepareDiscovered(
					ctx,
					DiscoveredJobChange{
						Config:  config,
						Status:  change.Status,
						Remove:  change.Remove,
						Restart: change.Restart,
						retry:   change.retry,
					},
					current,
					scope,
					permit,
				)
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
	}()
	record, exists := dcjc.graph.Lookup(scope.ID)
	if err := validateGraphResourcePair(record, exists, current, scope); err != nil {
		return nil, err
	}
	result := mustDynCfgMessage(204, "")
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
				dcjc.retrySettlement(scope.ID, change.retry),
			)
		}
	}
	if change.Remove {
		disposition := lifecycle.ResourceTransactionUnchanged
		if current != nil {
			disposition = lifecycle.ResourceTransactionRemoved
		}
		return dcjc.prepareMutationWithRetry(
			scope,
			current,
			nil,
			lifecycle.LongLivedPermit{},
			disposition,
			nil,
			result,
			dcjc.configDeleteCleanup(dcjc.configID(change.Config.Module(), change.Config.Name())),
			change.retry,
		)
	}

	payload, err := yaml.Marshal(change.Config)
	if err != nil {
		return nil, err
	}
	postimage := dyncfg.GraphConfig{
		ID: scope.ID, Module: change.Config.Module(),
		Name: change.Config.Name(), Status: change.Status.String(),
		Payload: payload,
	}
	cleanup := dcjc.configCreateCleanup(
		postimage,
		change.Config.SourceType(),
		change.Config.Source(),
		dcjc.configType(dcjc.modules[change.Config.Module()]),
	)
	if change.Status == dyncfg.StatusAccepted {
		disposition := lifecycle.ResourceTransactionUnchanged
		if current != nil {
			disposition = lifecycle.ResourceTransactionRemoved
		}
		return dcjc.prepareMutationWithRetry(
			scope,
			current,
			nil,
			lifecycle.LongLivedPermit{},
			disposition,
			&postimage,
			result,
			cleanup,
			change.retry,
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
			dcjc.retrySettlement(scope.ID, change.retry),
			cleanup,
		)
	}
	successor, err := dcjc.factory.Prepare(ctx, change.Config, scope.Successor, permit)
	if err != nil {
		return nil, err
	}
	disposition := lifecycle.ResourceTransactionInstalled
	if current != nil {
		disposition = lifecycle.ResourceTransactionReplaced
	}
	failedPostimage := postimage
	failedPostimage.Status = dyncfg.StatusFailed.String()
	return dcjc.prepareMutationWithRetry(
		scope,
		current,
		successor,
		lifecycle.LongLivedPermit{},
		disposition,
		&postimage,
		result,
		cleanup,
		change.retry,
		successorFailurePlan{
			postimage: failedPostimage,
			failedCleanup: dcjc.configCreateCleanup(
				failedPostimage,
				change.Config.SourceType(),
				change.Config.Source(),
				dcjc.configType(dcjc.modules[change.Config.Module()]),
			),
			removedCleanup: dcjc.configDeleteCleanup(dcjc.configID(change.Config.Module(), change.Config.Name())),
			result: func(*autoDetectionFailure) lifecycle.SealedResult {
				return result
			},
			afterApply: dcjc.scheduleRetryAfterApply(change.Config),
			removePlainStock: change.Config.SourceType() ==
				confgroup.TypeStock,
		},
	)
}
