// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"gopkg.in/yaml.v2"
)

func (dcjc *DynCfgJobController) prepareAdd(
	ctx context.Context,
	request DynCfgJobRequest,
	target dynCfgTarget,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
) (lifecycle.PreparedResourceTransaction, error) {
	if target.creator.InstancePolicy == collectorapi.InstancePolicySingle {
		return dcjc.noop(
			scope,
			current,
			lifecycle.LongLivedPermit{},
			mustDynCfgMessage(400, fmt.Sprintf("Single-instance collector %s does not support add.", target.module)),
		)
	}
	config, failure := dcjc.parseConfig(request, target.module, target.name)
	if failure.valid {
		return dcjc.noop(scope, current, lifecycle.LongLivedPermit{}, failure.result)
	}
	if err := dcjc.factory.ValidateConfig(ctx, config); err != nil {
		return dcjc.noop(scope, current, lifecycle.LongLivedPermit{}, mustDynCfgMessage(400, err.Error()))
	}
	payload, err := yaml.Marshal(config)
	if err != nil {
		return nil, err
	}
	postimage := dyncfg.GraphConfig{
		ID:      target.resourceID,
		Module:  target.module,
		Name:    target.name,
		Status:  dyncfg.StatusAccepted.String(),
		Payload: payload,
	}
	disposition := lifecycle.ResourceTransactionUnchanged
	if current != nil {
		disposition = lifecycle.ResourceTransactionRemoved
	}
	return dcjc.prepareMutation(
		scope,
		current,
		nil,
		lifecycle.LongLivedPermit{},
		disposition,
		&postimage,
		mustDynCfgMessage(202, ""),
		dcjc.configCreateCleanup(postimage, confgroup.TypeDyncfg, request.CallerSource, dyncfg.ConfigTypeJob),
	)
}

// updateCleanup selects the protocol cleanup for an update postimage: a plain
// status echo for dyncfg-sourced configs, or a full CONFIG CREATE that adopts a
// non-dyncfg (stock/discovered) config into dyncfg ownership.
func (dcjc *DynCfgJobController) updateCleanup(
	target dynCfgTarget,
	request DynCfgJobRequest,
	oldConfig confgroup.Config,
	postimage dyncfg.GraphConfig,
	status dyncfg.Status,
) lifecycle.TaskCleanup {
	if oldConfig.SourceType() != confgroup.TypeDyncfg {
		return dcjc.configCreateCleanup(
			postimage,
			confgroup.TypeDyncfg,
			request.CallerSource,
			dcjc.configType(target.creator),
		)
	}
	return dcjc.configStatusCleanup(target.resourceID, status)
}

func (dcjc *DynCfgJobController) prepareUpdate(
	ctx context.Context,
	request DynCfgJobRequest,
	target dynCfgTarget,
	record dyncfg.GraphRecord,
	exists bool,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
	permit lifecycle.LongLivedPermit,
) (lifecycle.PreparedResourceTransaction, error) {
	if !exists {
		return dcjc.noop(scope, current, permit, mustDynCfgMessage(404, "config not found."))
	}
	config, failure := dcjc.parseConfig(request, target.module, target.name)
	if failure.valid {
		return dcjc.noop(scope, current, permit, failure.result)
	}
	if err := dcjc.factory.ValidateConfig(ctx, config); err != nil {
		return dcjc.noop(
			scope,
			current,
			permit,
			mustDynCfgMessage(400, err.Error()),
			dcjc.configStatusCleanup(target.resourceID, dyncfg.Status(record.Status)),
		)
	}
	if record.Status == dyncfg.StatusAccepted.String() {
		return dcjc.noop(
			scope,
			current,
			permit,
			mustDynCfgMessage(403, "updating is not allowed in 'accepted' state."),
			dcjc.configStatusCleanup(target.resourceID, dyncfg.StatusAccepted),
		)
	}
	oldConfig, err := graphRecordConfig(record)
	if err != nil {
		return nil, err
	}
	if record.Status == dyncfg.StatusRunning.String() &&
		oldConfig.SourceType() == confgroup.TypeDyncfg &&
		oldConfig.Hash() == config.Hash() {
		return dcjc.noop(
			scope,
			current,
			permit,
			mustDynCfgMessage(200, ""),
			dcjc.configStatusCleanup(target.resourceID, dyncfg.StatusRunning),
		)
	}
	payload, err := yaml.Marshal(config)
	if err != nil {
		return nil, err
	}
	postimage := dyncfg.GraphConfig{
		ID:      target.resourceID,
		Module:  target.module,
		Name:    target.name,
		Status:  record.Status,
		Payload: payload,
	}
	if record.Status == dyncfg.StatusDisabled.String() {
		cleanup := dcjc.updateCleanup(target, request, oldConfig, postimage, dyncfg.StatusDisabled)
		return dcjc.prepareMutation(
			scope,
			current,
			nil,
			permit,
			lifecycle.ResourceTransactionUnchanged,
			&postimage,
			mustDynCfgMessage(200, ""),
			cleanup,
		)
	}
	successor, err := dcjc.factory.Prepare(ctx, config, scope.Successor, permit)
	if err != nil {
		if ctx.Err() != nil || lifecycle.OwnershipRetained(err) {
			return nil, err
		}
		return dcjc.noop(
			scope,
			current,
			permit,
			mustDynCfgMessage(200, err.Error()),
			dcjc.configStatusCleanup(target.resourceID, dyncfg.Status(record.Status)),
		)
	}
	postimage.Status = dyncfg.StatusRunning.String()
	disposition := lifecycle.ResourceTransactionInstalled
	if current != nil {
		disposition = lifecycle.ResourceTransactionReplaced
	}
	cleanup := dcjc.updateCleanup(target, request, oldConfig, postimage, dyncfg.StatusRunning)
	failedPostimage := postimage
	failedPostimage.Status = dyncfg.StatusFailed.String()
	failedCleanup := dcjc.updateCleanup(target, request, oldConfig, failedPostimage, dyncfg.StatusFailed)
	return dcjc.prepareMutation(
		scope,
		current,
		successor,
		lifecycle.LongLivedPermit{},
		disposition,
		&postimage,
		mustDynCfgMessage(200, ""),
		cleanup,
		successorFailurePlan{
			postimage:      failedPostimage,
			failedCleanup:  failedCleanup,
			removedCleanup: dcjc.configDeleteCleanup(dcjc.configID(target.module, target.name)),
			result:         autoDetectionFailureResultFunc(200, "config update failed: %v"),
			afterApply:     dcjc.scheduleRetryAfterApply(config),
			removePlainStock: config.SourceType() ==
				confgroup.TypeStock,
		},
	)
}

func (dcjc *DynCfgJobController) prepareEnable(
	ctx context.Context,
	target dynCfgTarget,
	record dyncfg.GraphRecord,
	exists bool,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
	permit lifecycle.LongLivedPermit,
) (lifecycle.PreparedResourceTransaction, error) {
	if !exists {
		return dcjc.noop(scope, current, permit, mustDynCfgMessage(404, "config not found."))
	}
	if record.Status == dyncfg.StatusRunning.String() {
		return dcjc.noop(
			scope,
			current,
			permit,
			mustDynCfgMessage(200, ""),
			dcjc.configStatusCleanup(target.resourceID, dyncfg.StatusRunning),
		)
	}
	return dcjc.prepareRunningTransition(
		ctx,
		target,
		record,
		current,
		scope,
		permit,
		lifecycle.ResourceTransactionInstalled,
		200,
		"job enable failed: %v",
		func(err error) (lifecycle.PreparedResourceTransaction, error) {
			return dcjc.noop(scope, current, permit, mustDynCfgMessage(200, err.Error()))
		},
	)
}

func (dcjc *DynCfgJobController) prepareRestart(
	ctx context.Context,
	target dynCfgTarget,
	record dyncfg.GraphRecord,
	exists bool,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
	permit lifecycle.LongLivedPermit,
) (lifecycle.PreparedResourceTransaction, error) {
	if !exists {
		return dcjc.noop(scope, current, permit, mustDynCfgMessage(404, "config not found."))
	}
	status := dyncfg.Status(record.Status)
	if status != dyncfg.StatusRunning && status != dyncfg.StatusFailed {
		return dcjc.noop(
			scope,
			current,
			permit,
			mustDynCfgMessage(405, fmt.Sprintf("restarting is not allowed in '%s' state.", status)),
			dcjc.configStatusCleanup(target.resourceID, status),
		)
	}
	disposition := lifecycle.ResourceTransactionInstalled
	if current != nil {
		disposition = lifecycle.ResourceTransactionReplaced
	}
	return dcjc.prepareRunningTransition(
		ctx,
		target,
		record,
		current,
		scope,
		permit,
		disposition,
		422,
		"config restart failed: %v",
		func(err error) (lifecycle.PreparedResourceTransaction, error) {
			return dcjc.noop(
				scope,
				current,
				permit,
				mustDynCfgMessage(422, fmt.Sprintf("config restart failed: %v", err)),
				dcjc.configStatusCleanup(target.resourceID, status),
			)
		},
	)
}

// prepareRunningTransition drives the shared enable/restart body: build the
// successor from the record and, on success, prepare the install/replace
// mutation to StatusRunning with the standard failure plan. A prepare failure
// that is not context-cancelled or ownership-retained is handed to
// onPrepareFailure; an apply-time autodetection failure uses the caller's
// command-specific default code unless the collector supplies its own.
func (dcjc *DynCfgJobController) prepareRunningTransition(
	ctx context.Context,
	target dynCfgTarget,
	record dyncfg.GraphRecord,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
	permit lifecycle.LongLivedPermit,
	disposition lifecycle.ResourceTransactionDisposition,
	autoDetectionFailureCode int,
	failureMessage string,
	onPrepareFailure func(error) (lifecycle.PreparedResourceTransaction, error),
) (lifecycle.PreparedResourceTransaction, error) {
	config, err := graphRecordConfig(record)
	if err != nil {
		return nil, err
	}
	successor, err := dcjc.factory.Prepare(ctx, config, scope.Successor, permit)
	if err != nil {
		if ctx.Err() != nil || lifecycle.OwnershipRetained(err) {
			return nil, err
		}
		return onPrepareFailure(err)
	}
	postimage := graphConfig(record, dyncfg.StatusRunning)
	failedPostimage := graphConfig(record, dyncfg.StatusFailed)
	return dcjc.prepareMutation(
		scope,
		current,
		successor,
		lifecycle.LongLivedPermit{},
		disposition,
		&postimage,
		mustDynCfgMessage(200, ""),
		dcjc.configStatusCleanup(target.resourceID, dyncfg.StatusRunning),
		successorFailurePlan{
			postimage:      failedPostimage,
			failedCleanup:  dcjc.configStatusCleanup(target.resourceID, dyncfg.StatusFailed),
			removedCleanup: dcjc.configDeleteCleanup(dcjc.externalID(target.resourceID)),
			result:         autoDetectionFailureResultFunc(autoDetectionFailureCode, failureMessage),
			afterApply:     dcjc.scheduleRetryAfterApply(config),
			removePlainStock: config.SourceType() ==
				confgroup.TypeStock,
		},
	)
}

func (dcjc *DynCfgJobController) prepareDisable(
	target dynCfgTarget,
	record dyncfg.GraphRecord,
	exists bool,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
) (lifecycle.PreparedResourceTransaction, error) {
	if !exists {
		return dcjc.noop(scope, current, lifecycle.LongLivedPermit{}, mustDynCfgMessage(404, "config not found."))
	}
	if record.Status == dyncfg.StatusDisabled.String() {
		return dcjc.noop(
			scope,
			current,
			lifecycle.LongLivedPermit{},
			mustDynCfgMessage(200, ""),
			dcjc.configStatusCleanup(target.resourceID, dyncfg.StatusDisabled),
		)
	}
	postimage := graphConfig(record, dyncfg.StatusDisabled)
	disposition := lifecycle.ResourceTransactionUnchanged
	if current != nil {
		disposition = lifecycle.ResourceTransactionRemoved
	}
	return dcjc.prepareMutation(
		scope,
		current,
		nil,
		lifecycle.LongLivedPermit{},
		disposition,
		&postimage,
		mustDynCfgMessage(200, ""),
		dcjc.configStatusCleanup(target.resourceID, dyncfg.StatusDisabled),
	)
}

func (dcjc *DynCfgJobController) prepareRemove(
	target dynCfgTarget,
	record dyncfg.GraphRecord,
	exists bool,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
) (lifecycle.PreparedResourceTransaction, error) {
	if !exists {
		return dcjc.noop(scope, current, lifecycle.LongLivedPermit{}, mustDynCfgMessage(404, "config not found."))
	}
	config, err := graphRecordConfig(record)
	if err != nil {
		return nil, err
	}
	if config.SourceType() != confgroup.TypeDyncfg {
		return dcjc.noop(
			scope,
			current,
			lifecycle.LongLivedPermit{},
			mustDynCfgMessage(
				405,
				fmt.Sprintf(
					"removing configurations of source type '%s' is not supported, only 'dyncfg' configurations can be removed.",
					config.SourceType(),
				),
			),
		)
	}
	if target.creator.InstancePolicy == collectorapi.InstancePolicySingle {
		return dcjc.noop(
			scope,
			current,
			lifecycle.LongLivedPermit{},
			mustDynCfgMessage(
				405,
				"removing configurations of type 'single' is not supported, only 'job' configurations can be removed.",
			),
		)
	}
	disposition := lifecycle.ResourceTransactionUnchanged
	if current != nil {
		disposition = lifecycle.ResourceTransactionRemoved
	}
	return dcjc.prepareMutation(
		scope,
		current,
		nil,
		lifecycle.LongLivedPermit{},
		disposition,
		nil,
		mustDynCfgMessage(200, ""),
		dcjc.configDeleteCleanup(dcjc.configID(record.Module, record.Name)),
	)
}
