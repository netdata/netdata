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

// Command preparation. Every branch ends in reject (nothing changes),
// satisfy (the requested state already holds) or an adopted graph change;
// dyncfg_reply.go turns that outcome into the reply.

const validationBusyMessage = "config validation is busy; retry the command."

func (dcjc *DynCfgJobController) prepareAdd(
	ctx context.Context,
	request DynCfgJobRequest,
	target dynCfgTarget,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
) (lifecycle.PreparedResourceTransaction, error) {
	permit := lifecycle.LongLivedPermit{}
	if target.creator.InstancePolicy == collectorapi.InstancePolicySingle {
		return dcjc.reject(scope, current, permit, jobFailure{
			class:   failureBadRequest,
			message: fmt.Sprintf("Single-instance collector %s does not support add.", target.module),
		})
	}
	config, failure := dcjc.parseConfig(request, target.module, target.name)
	if failure.valid {
		return dcjc.reject(scope, current, permit, failure.failure)
	}
	if _, err := dcjc.runConfigOperation(ctx, config, configOperationValidate, true); err != nil {
		if ctx.Err() != nil || lifecycle.OwnershipRetained(err) {
			return nil, err
		}
		// The resource lane stays active while validation yields the graph
		// claim, so same-resource DynCfg work cannot supersede this attempt.
		rejection, ok := preparationFailure(
			classifyActivationError(err),
			validationBusyMessage,
			validationBusyMessage,
			validationBusyMessage,
		)
		if !ok {
			return nil, err
		}
		return dcjc.reject(scope, current, permit, rejection)
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
	return dcjc.prepareMutation(
		scope,
		current,
		nil,
		permit,
		resourceRemovalDisposition(current),
		&postimage,
		adoptedReply(dyncfg.CommandAdd, dyncfg.StatusAccepted, jobFailure{}),
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
		return dcjc.reject(scope, current, permit, jobFailure{
			class:   failureNotFound,
			message: "config not found.",
		})
	}
	config, failure := dcjc.parseConfig(request, target.module, target.name)
	if failure.valid {
		return dcjc.reject(scope, current, permit, failure.failure)
	}
	status := dyncfg.Status(record.Status)
	if status == dyncfg.StatusAccepted && current == nil && !dcjc.scheduler.accepted.enabled(scope.ID) {
		return dcjc.rejectKeeping(scope, current, permit, jobFailure{
			class:   failureForbidden,
			message: "updating is not allowed in 'accepted' state.",
		}, status)
	}
	oldConfig, err := graphRecordConfig(record)
	if err != nil {
		return nil, err
	}
	if status == dyncfg.StatusRunning &&
		oldConfig.SourceType() == confgroup.TypeDyncfg &&
		oldConfig.Hash() == config.Hash() {
		return dcjc.satisfy(scope, current, permit, status, nil)
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
	if status == dyncfg.StatusDisabled {
		if _, err := dcjc.runConfigOperation(ctx, config, configOperationValidate, true); err != nil {
			if ctx.Err() != nil || lifecycle.OwnershipRetained(err) {
				return nil, err
			}
			// The resource lane stays active while validation yields the graph
			// claim, so same-resource DynCfg work cannot supersede this attempt.
			rejection, ok := preparationFailure(
				classifyActivationError(err),
				validationBusyMessage,
				validationBusyMessage,
				validationBusyMessage,
			)
			if !ok {
				return nil, err
			}
			return dcjc.rejectKeeping(scope, current, permit, rejection, status)
		}
		return dcjc.prepareMutation(
			scope,
			current,
			nil,
			permit,
			lifecycle.ResourceTransactionUnchanged,
			&postimage,
			adoptedReply(dyncfg.CommandUpdate, dyncfg.StatusDisabled, jobFailure{}),
			dcjc.updateCleanup(target, request, oldConfig, postimage, dyncfg.StatusDisabled),
		)
	}
	return dcjc.prepareCandidate(ctx, candidateCommand{
		command:        dyncfg.CommandUpdate,
		record:         record,
		config:         config,
		postimage:      postimage,
		adoptsFailures: false,
		cleanup: func(postimage dyncfg.GraphConfig, status dyncfg.Status) lifecycle.TaskCleanup {
			return dcjc.updateCleanup(target, request, oldConfig, postimage, status)
		},
		removedCleanup: dcjc.configDeleteCleanup(dcjc.configID(target.module, target.name)),
		messages: candidateMessages{
			busy:        "config update is busy; retry the command.",
			deadline:    "config update timed out; retry the command.",
			quarantined: "config update is unavailable until the plugin restarts.",
			failed:      "config update failed: %v",
		},
	}, current, scope, permit)
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
		return dcjc.reject(scope, current, permit, jobFailure{
			class:   failureNotFound,
			message: "config not found.",
		})
	}
	status := dyncfg.Status(record.Status)
	if status == dyncfg.StatusRunning {
		return dcjc.satisfy(scope, current, permit, status, nil)
	}
	config, err := graphRecordConfig(record)
	if err != nil {
		return nil, err
	}
	spec, err := dcjc.configModules.newAcceptedActivationSpec(config)
	if err != nil {
		return nil, err
	}
	activate := func() { dcjc.scheduler.accepted.arm(spec) }
	if status == dyncfg.StatusAccepted {
		if current != nil {
			activate = nil // The installed generation already owns startup.
		}
		return dcjc.satisfy(scope, current, permit, status, activate)
	}
	postimage := graphConfig(record, dyncfg.StatusAccepted)
	return dcjc.prepareMutationWithRetryAfterApply(
		scope, current, nil, permit, resourceRemovalDisposition(current), &postimage,
		adoptedReply(dyncfg.CommandEnable, dyncfg.StatusAccepted, jobFailure{}),
		dcjc.configStatusCleanup(scope.ID, dyncfg.StatusAccepted), autoDetectionRetryToken{}, activate,
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
		return dcjc.reject(scope, current, permit, jobFailure{
			class:   failureNotFound,
			message: "config not found.",
		})
	}
	status := dyncfg.Status(record.Status)
	if status != dyncfg.StatusRunning && status != dyncfg.StatusFailed {
		return dcjc.rejectKeeping(scope, current, permit, jobFailure{
			class:   failureNotAllowed,
			message: fmt.Sprintf("restarting is not allowed in '%s' state.", status),
		}, status)
	}
	return dcjc.prepareStoredRestart(ctx, target, record, current, scope, permit)
}

// prepareStoredRestart probes the stored configuration before replacing its runtime.
func (dcjc *DynCfgJobController) prepareStoredRestart(
	ctx context.Context,
	target dynCfgTarget,
	record dyncfg.GraphRecord,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
	permit lifecycle.LongLivedPermit,
) (lifecycle.PreparedResourceTransaction, error) {
	config, err := graphRecordConfig(record)
	if err != nil {
		return nil, err
	}
	messages := candidateMessages{
		busy:        "job activation is busy; retry the command.",
		deadline:    "job activation timed out; retry the command.",
		quarantined: "job activation is unavailable until the plugin restarts.",
		failed:      "config restart failed: %v",
	}
	return dcjc.prepareCandidate(ctx, candidateCommand{
		command:   dyncfg.CommandRestart,
		record:    record,
		config:    config,
		postimage: graphConfig(record, dyncfg.StatusRunning),
		// restart is not saved by the daemon and adopts nothing: it keeps a
		// running job whenever the fresh start fails before stopping it.
		adoptsFailures: current == nil,
		cleanup: func(_ dyncfg.GraphConfig, status dyncfg.Status) lifecycle.TaskCleanup {
			return dcjc.configStatusCleanup(target.resourceID, status)
		},
		removedCleanup: dcjc.configDeleteCleanup(dcjc.externalID(target.resourceID)),
		messages:       messages,
	}, current, scope, permit)
}

// candidateCommand is a command that prepares a candidate job from config and
// replaces the record's state with it.
type candidateCommand struct {
	command        dyncfg.Command
	record         dyncfg.GraphRecord                                            // preimage
	config         confgroup.Config                                              // configuration to start
	postimage      dyncfg.GraphConfig                                            // record for config; the status is set per outcome
	adoptsFailures bool                                                          // whether a failure the plugin retries is adopted
	cleanup        func(dyncfg.GraphConfig, dyncfg.Status) lifecycle.TaskCleanup // protocol frame for an adopted postimage
	removedCleanup lifecycle.TaskCleanup                                         // protocol frame when a plain stock job is removed
	messages       candidateMessages
}

type candidateMessages struct {
	busy        string // the job identity is busy
	deadline    string // preparation exceeded its containment deadline
	quarantined string // the job identity is unusable until the plugin restarts
	failed      string // format for a failure cause
}

// prepareCandidate prepares and probes the candidate while the incumbent keeps
// running. UPDATE rejects every preflight failure. RESTART of an already failed
// job may retain its operational retry policy; rejected edits preserve the graph
// and incumbent. A successful candidate transfers to accepted activation;
// installation then waits for predecessor release outside the mutation lane.
func (dcjc *DynCfgJobController) prepareCandidate(
	ctx context.Context,
	cmd candidateCommand,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
	permit lifecycle.LongLivedPermit,
) (lifecycle.PreparedResourceTransaction, error) {
	kept := dyncfg.Status(cmd.record.Status)
	failedPostimage := cmd.postimage
	failedPostimage.Status = dyncfg.StatusFailed.String()
	failedCleanup := cmd.cleanup(failedPostimage, dyncfg.StatusFailed)

	stage, probeFailure, activation := dcjc.prepareContainedCandidate(ctx, cmd.config)
	if err := activation.err; err != nil {
		if ctx.Err() != nil || lifecycle.OwnershipRetained(err) {
			return nil, err
		}
		rejection, ok := preparationFailure(
			activation,
			cmd.messages.busy,
			cmd.messages.deadline,
			cmd.messages.quarantined,
		)
		if !ok {
			return nil, err
		}
		if rejection.class != failureTemporary {
			return dcjc.rejectKeeping(scope, current, permit, rejection, kept)
		}
		transient := transientActivationFailure(cmd.config, err)
		failure := jobFailure{
			class:   failureTemporary,
			message: fmt.Sprintf(cmd.messages.failed, err),
		}
		if !cmd.adoptsFailures || !adoptsFailure(transient) {
			return dcjc.rejectKeeping(scope, current, permit, failure, kept)
		}
		return dcjc.prepareTransientConstructionFailure(
			scope,
			current,
			permit,
			failedPostimage,
			adoptedReply(cmd.command, dyncfg.StatusFailed, failure),
			failedCleanup,
			autoDetectionRetryToken{},
			cmd.config,
			err,
		)
	}
	reply := func(failure *autoDetectionFailure) jobReply {
		return adoptedReply(cmd.command, dyncfg.StatusFailed, collectorFailure(failure, cmd.messages.failed))
	}
	failurePlan := probeFailurePlan{
		postimage:        failedPostimage,
		failedCleanup:    failedCleanup,
		removedCleanup:   cmd.removedCleanup,
		reply:            reply,
		afterApply:       dcjc.scheduleRetryAfterApply(cmd.config),
		removePlainStock: cmd.config.SourceType() == confgroup.TypeStock,
	}
	if probeFailure != nil {
		if !cmd.adoptsFailures || !adoptsFailure(probeFailure) {
			return dcjc.rejectKeeping(
				scope,
				current,
				permit,
				collectorFailure(probeFailure, cmd.messages.failed),
				kept,
			)
		}
		return dcjc.prepareProbeFailure(scope, current, permit, autoDetectionRetryToken{}, probeFailure, failurePlan)
	}
	acceptedPostimage := cmd.postimage
	acceptedPostimage.Status = dyncfg.StatusAccepted.String()
	return dcjc.prepareCandidateAdoption(
		scope, current, permit, stage, cmd.config, &acceptedPostimage,
		adoptedReply(cmd.command, dyncfg.StatusAccepted, jobFailure{}),
		cmd.cleanup(acceptedPostimage, dyncfg.StatusAccepted), nil,
	)
}

func (dcjc *DynCfgJobController) prepareDisable(
	target dynCfgTarget,
	record dyncfg.GraphRecord,
	exists bool,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
) (lifecycle.PreparedResourceTransaction, error) {
	permit := lifecycle.LongLivedPermit{}
	if !exists {
		return dcjc.reject(scope, current, permit, jobFailure{
			class:   failureNotFound,
			message: "config not found.",
		})
	}
	if record.Status == dyncfg.StatusDisabled.String() {
		return dcjc.satisfy(scope, current, permit, dyncfg.StatusDisabled, nil)
	}
	postimage := graphConfig(record, dyncfg.StatusDisabled)
	return dcjc.prepareMutation(
		scope,
		current,
		nil,
		permit,
		resourceRemovalDisposition(current),
		&postimage,
		adoptedReply(dyncfg.CommandDisable, dyncfg.StatusDisabled, jobFailure{}),
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
	permit := lifecycle.LongLivedPermit{}
	if !exists {
		return dcjc.reject(scope, current, permit, jobFailure{
			class:   failureNotFound,
			message: "config not found.",
		})
	}
	config, err := graphRecordConfig(record)
	if err != nil {
		return nil, err
	}
	if config.SourceType() != confgroup.TypeDyncfg {
		return dcjc.reject(scope, current, permit, jobFailure{
			class: failureNotAllowed,
			message: fmt.Sprintf(
				"removing configurations of source type '%s' is not supported, only 'dyncfg' configurations can be removed.",
				config.SourceType(),
			),
		})
	}
	if target.creator.InstancePolicy == collectorapi.InstancePolicySingle {
		return dcjc.reject(scope, current, permit, jobFailure{
			class:   failureNotAllowed,
			message: "removing configurations of type 'single' is not supported, only 'job' configurations can be removed.",
		})
	}
	return dcjc.prepareMutation(
		scope,
		current,
		nil,
		permit,
		resourceRemovalDisposition(current),
		nil,
		adoptedReply(dyncfg.CommandRemove, "", jobFailure{}),
		dcjc.configDeleteCleanup(dcjc.configID(record.Module, record.Name)),
	)
}
