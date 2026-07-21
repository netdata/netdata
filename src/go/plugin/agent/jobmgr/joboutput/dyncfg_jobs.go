// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	frameworkfunctions "github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"gopkg.in/yaml.v2"
)

const (
	DynCfgFunctionName = "config"

	dynCfgCollectorPrefixFormat = "%s:collector:"
	dynCfgCollectorPathFormat   = "/collectors/%s/Jobs"
)

var dynCfgJobNameReplacer = strings.NewReplacer(" ", "_", ":", "_")

type DynCfgJobRequest struct {
	Args         []string // dyncfg command arguments
	Payload      []byte   // request payload
	ContentType  string   // payload content type
	CallerSource string   // caller source string
	HasPayload   bool     // whether a payload is present
}

type DynCfgJobControllerConfig struct {
	PluginName    string                // owning plugin name
	Modules       collectorapi.Registry // collector module registry
	Defaults      confgroup.Registry    // per-module config defaults
	Factory       *Factory              // job construction factory
	ConfigModules *ConfigModuleFactory  // short-lived config-probe factory
	Graph         *dyncfg.Graph         // dyncfg graph authority
	Frames        *lifecycle.FrameOwner // protocol frame sink
	Dependencies  JobDependencyIndex    // secret-dependency index (optional)
}

type JobDependencyIndex interface {
	PrepareJobChange(
		string,
		*dyncfg.GraphConfig,
	) (func(), error)
}

// DynCfgJobController prepares collector configuration graph/resource
// transactions. CommandKernel remains the only current-job authority.
type DynCfgJobController struct {
	pluginName    string                // owning plugin
	prefix        string                // "<plugin>:collector:" ID prefix
	path          string                // "/collectors/<plugin>/jobs" config path
	modules       collectorapi.Registry // collector registry
	defaults      confgroup.Registry    // per-module config defaults
	factory       *Factory              // job construction factory
	configModules *ConfigModuleFactory  // short-lived config-probe factory
	graph         *dyncfg.Graph         // dyncfg graph authority
	frames        *lifecycle.FrameOwner // protocol frame sink
	dependencies  JobDependencyIndex    // secret-dependency index (optional)
	scheduler     *Scheduler            // tick + retry scheduler
}

func NewDynCfgJobController(
	config DynCfgJobControllerConfig,
) (*DynCfgJobController, error) {
	if config.PluginName == "" ||
		config.Modules == nil ||
		config.Defaults == nil ||
		config.Factory == nil ||
		config.ConfigModules == nil ||
		config.Graph == nil ||
		config.Frames == nil {
		return nil, errors.New(
			"job output: incomplete DynCfg job controller configuration",
		)
	}
	return &DynCfgJobController{
		pluginName: config.PluginName,
		prefix:     DynCfgJobPrefix(config.PluginName),
		path: fmt.Sprintf(
			dynCfgCollectorPathFormat,
			config.PluginName,
		),
		modules: config.Modules, defaults: config.Defaults,
		factory: config.Factory, configModules: config.ConfigModules,
		graph: config.Graph, frames: config.Frames,
		dependencies: config.Dependencies,
		scheduler:    config.Factory.config.Scheduler,
	}, nil
}

func (dcjc *DynCfgJobController) BindAutoDetectionRetries(
	commands jobmgr.PreparedCommandPort,
	run uint64,
	failure func(error),
) error {
	if dcjc == nil || dcjc.scheduler == nil {
		return errors.New(
			"job output: invalid autodetection retry controller",
		)
	}
	return dcjc.scheduler.bindAutoDetectionRetries(
		commands,
		dcjc.planAutoDetectionRetry,
		run,
		failure,
	)
}

func DynCfgJobPrefix(pluginName string) string {
	if pluginName == "" {
		return ""
	}
	return fmt.Sprintf(dynCfgCollectorPrefixFormat, pluginName)
}

func (dcjc *DynCfgJobController) Prefix() string {
	if dcjc == nil {
		return ""
	}
	return dcjc.prefix
}

func (dcjc *DynCfgJobController) Handle(
	ctx context.Context,
	request DynCfgJobRequest,
) (lifecycle.SealedResult, error) {
	if dcjc == nil || ctx == nil {
		return lifecycle.SealedResult{},
			errors.New("job output: invalid DynCfg request")
	}
	target, result := dcjc.resolveRequest(request)
	if result.valid {
		return result.result, nil
	}
	switch target.command {
	case dyncfg.CommandSchema:
		if target.creator.JobConfigSchema == "" {
			return dynCfgMessage(
				500,
				fmt.Sprintf(
					"Module %s configuration schema not found.",
					target.module,
				),
			)
		}
		return lifecycle.NewSealedResult(
			200,
			"application/json",
			[]byte(target.creator.JobConfigSchema),
		)
	case dyncfg.CommandUserconfig:
		return dcjc.userConfig(request, target)
	case dyncfg.CommandTest:
		config, failure := dcjc.parseConfig(
			request,
			target.module,
			target.name,
		)
		if failure.valid {
			return failure.result, nil
		}
		if err := dcjc.configModules.Test(ctx, config); err != nil {
			return dynCfgMessage(422, err.Error())
		}
		return dynCfgMessage(200, "")
	case dyncfg.CommandGet:
		record, ok := dcjc.graph.Lookup(target.resourceID)
		if !ok {
			return dynCfgMessage(
				404,
				fmt.Sprintf(
					"The specified module '%s' job '%s' is not registered.",
					target.module,
					target.name,
				),
			)
		}
		config, err := graphRecordConfig(record)
		if err != nil {
			return lifecycle.SealedResult{}, err
		}
		payload, err := dcjc.configModules.Configuration(
			ctx,
			config,
		)
		if err != nil {
			return dynCfgMessage(500, err.Error())
		}
		return lifecycle.NewSealedResult(
			200,
			"application/json",
			payload,
		)
	default:
		return dynCfgMessage(
			501,
			fmt.Sprintf(
				"Function '%s' command '%s' is not implemented.",
				DynCfgFunctionName,
				target.command,
			),
		)
	}
}

func (dcjc *DynCfgJobController) Prepare(
	ctx context.Context,
	request DynCfgJobRequest,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
	permit lifecycle.LongLivedPermit,
) (lifecycle.PreparedResourceTransaction, error) {
	if dcjc == nil || ctx == nil || !scope.Valid() {
		return nil, errors.New(
			"job output: invalid DynCfg transaction preparation",
		)
	}
	target, failure := dcjc.resolveRequest(request)
	if failure.valid {
		return dcjc.noop(scope, current, permit, failure.result)
	}
	if target.resourceID != scope.ID {
		return nil, errors.New(
			"job output: DynCfg target differs from transaction scope",
		)
	}
	record, exists := dcjc.graph.Lookup(target.resourceID)
	if err := validateGraphResourcePair(
		record,
		exists,
		current,
		scope,
	); err != nil {
		return nil, err
	}

	switch target.command {
	case dyncfg.CommandAdd:
		return dcjc.prepareAdd(
			ctx,
			request,
			target,
			current,
			scope,
		)
	case dyncfg.CommandUpdate:
		return dcjc.prepareUpdate(
			ctx,
			request,
			target,
			record,
			exists,
			current,
			scope,
			permit,
		)
	case dyncfg.CommandEnable:
		return dcjc.prepareEnable(
			ctx,
			target,
			record,
			exists,
			current,
			scope,
			permit,
		)
	case dyncfg.CommandRestart:
		return dcjc.prepareRestart(
			ctx,
			target,
			record,
			exists,
			current,
			scope,
			permit,
		)
	case dyncfg.CommandDisable:
		return dcjc.prepareDisable(
			target,
			record,
			exists,
			current,
			scope,
		)
	case dyncfg.CommandRemove:
		return dcjc.prepareRemove(
			target,
			record,
			exists,
			current,
			scope,
		)
	default:
		return dcjc.noop(
			scope,
			current,
			permit,
			mustDynCfgMessage(
				501,
				fmt.Sprintf(
					"Function '%s' command '%s' is not implemented.",
					DynCfgFunctionName,
					target.command,
				),
			),
		)
	}
}

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
			mustDynCfgMessage(
				400,
				fmt.Sprintf(
					"Single-instance collector %s does not support add.",
					target.module,
				),
			),
		)
	}
	config, failure := dcjc.parseConfig(
		request,
		target.module,
		target.name,
	)
	if failure.valid {
		return dcjc.noop(
			scope,
			current,
			lifecycle.LongLivedPermit{},
			failure.result,
		)
	}
	if err := dcjc.factory.ValidateConfig(ctx, config); err != nil {
		return dcjc.noop(
			scope,
			current,
			lifecycle.LongLivedPermit{},
			mustDynCfgMessage(400, err.Error()),
		)
	}
	payload, err := yaml.Marshal(config)
	if err != nil {
		return nil, err
	}
	postimage := dyncfg.GraphConfig{
		ID: target.resourceID, Module: target.module, Name: target.name,
		Status: dyncfg.StatusAccepted.String(), Payload: payload,
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
		dcjc.configCreateCleanup(
			postimage,
			confgroup.TypeDyncfg,
			request.CallerSource,
			dyncfg.ConfigTypeJob,
		),
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
		return dcjc.noop(
			scope,
			current,
			permit,
			mustDynCfgMessage(404, "config not found."),
		)
	}
	config, failure := dcjc.parseConfig(
		request,
		target.module,
		target.name,
	)
	if failure.valid {
		return dcjc.noop(
			scope,
			current,
			permit,
			failure.result,
		)
	}
	if err := dcjc.factory.ValidateConfig(ctx, config); err != nil {
		return dcjc.noop(
			scope,
			current,
			permit,
			mustDynCfgMessage(400, err.Error()),
			dcjc.configStatusCleanup(
				target.resourceID,
				dyncfg.Status(record.Status),
			),
		)
	}
	if record.Status == dyncfg.StatusAccepted.String() {
		return dcjc.noop(
			scope,
			current,
			permit,
			mustDynCfgMessage(
				403,
				"updating is not allowed in 'accepted' state.",
			),
			dcjc.configStatusCleanup(
				target.resourceID,
				dyncfg.StatusAccepted,
			),
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
			dcjc.configStatusCleanup(
				target.resourceID,
				dyncfg.StatusRunning,
			),
		)
	}
	payload, err := yaml.Marshal(config)
	if err != nil {
		return nil, err
	}
	postimage := dyncfg.GraphConfig{
		ID: target.resourceID, Module: target.module, Name: target.name,
		Status: record.Status, Payload: payload,
	}
	if record.Status == dyncfg.StatusDisabled.String() {
		cleanup := dcjc.updateCleanup(
			target, request, oldConfig, postimage, dyncfg.StatusDisabled,
		)
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
	successor, err := dcjc.factory.Prepare(
		ctx,
		config,
		scope.Successor,
		permit,
	)
	if err != nil {
		if ctx.Err() != nil ||
			lifecycle.OwnershipRetained(err) {
			return nil, err
		}
		return dcjc.noop(
			scope,
			current,
			permit,
			mustDynCfgMessage(200, err.Error()),
			dcjc.configStatusCleanup(
				target.resourceID,
				dyncfg.Status(record.Status),
			),
		)
	}
	postimage.Status = dyncfg.StatusRunning.String()
	disposition := lifecycle.ResourceTransactionInstalled
	if current != nil {
		disposition = lifecycle.ResourceTransactionReplaced
	}
	cleanup := dcjc.updateCleanup(
		target, request, oldConfig, postimage, dyncfg.StatusRunning,
	)
	failedPostimage := postimage
	failedPostimage.Status = dyncfg.StatusFailed.String()
	failedCleanup := dcjc.updateCleanup(
		target, request, oldConfig, failedPostimage, dyncfg.StatusFailed,
	)
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
			postimage:     failedPostimage,
			failedCleanup: failedCleanup,
			removedCleanup: dcjc.configDeleteCleanup(
				dcjc.configID(target.module, target.name),
			),
			result: autoDetectionFailureResultFunc(
				422,
				"config update failed: %v",
			),
			afterApply: dcjc.scheduleRetryAfterApply(config),
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
		return dcjc.noop(
			scope,
			current,
			permit,
			mustDynCfgMessage(404, "config not found."),
		)
	}
	if record.Status == dyncfg.StatusRunning.String() {
		return dcjc.noop(
			scope,
			current,
			permit,
			mustDynCfgMessage(200, ""),
			dcjc.configStatusCleanup(
				target.resourceID,
				dyncfg.StatusRunning,
			),
		)
	}
	config, err := graphRecordConfig(record)
	if err != nil {
		return nil, err
	}
	successor, err := dcjc.factory.Prepare(
		ctx,
		config,
		scope.Successor,
		permit,
	)
	if err != nil {
		if ctx.Err() != nil ||
			lifecycle.OwnershipRetained(err) {
			return nil, err
		}
		return dcjc.noop(
			scope,
			current,
			permit,
			mustDynCfgMessage(200, err.Error()),
		)
	}
	postimage := graphConfig(record, dyncfg.StatusRunning)
	failedPostimage := graphConfig(
		record,
		dyncfg.StatusFailed,
	)
	return dcjc.prepareMutation(
		scope,
		current,
		successor,
		lifecycle.LongLivedPermit{},
		lifecycle.ResourceTransactionInstalled,
		&postimage,
		mustDynCfgMessage(200, ""),
		dcjc.configStatusCleanup(
			target.resourceID,
			dyncfg.StatusRunning,
		),
		successorFailurePlan{
			postimage: failedPostimage,
			failedCleanup: dcjc.configStatusCleanup(
				target.resourceID,
				dyncfg.StatusFailed,
			),
			removedCleanup: dcjc.configDeleteCleanup(
				dcjc.externalID(target.resourceID),
			),
			result: autoDetectionFailureResultFunc(
				422,
				"job enable failed: %v",
			),
			afterApply: dcjc.scheduleRetryAfterApply(config),
			removePlainStock: config.SourceType() ==
				confgroup.TypeStock,
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
		return dcjc.noop(
			scope,
			current,
			permit,
			mustDynCfgMessage(404, "config not found."),
		)
	}
	status := dyncfg.Status(record.Status)
	if status != dyncfg.StatusRunning && status != dyncfg.StatusFailed {
		return dcjc.noop(
			scope,
			current,
			permit,
			mustDynCfgMessage(
				405,
				fmt.Sprintf(
					"restarting is not allowed in '%s' state.",
					status,
				),
			),
			dcjc.configStatusCleanup(
				target.resourceID,
				status,
			),
		)
	}
	config, err := graphRecordConfig(record)
	if err != nil {
		return nil, err
	}
	successor, err := dcjc.factory.Prepare(
		ctx,
		config,
		scope.Successor,
		permit,
	)
	if err != nil {
		if ctx.Err() != nil ||
			lifecycle.OwnershipRetained(err) {
			return nil, err
		}
		return dcjc.noop(
			scope,
			current,
			permit,
			mustDynCfgMessage(
				422,
				fmt.Sprintf("config restart failed: %v", err),
			),
			dcjc.configStatusCleanup(
				target.resourceID,
				status,
			),
		)
	}
	postimage := graphConfig(record, dyncfg.StatusRunning)
	failedPostimage := graphConfig(
		record,
		dyncfg.StatusFailed,
	)
	disposition := lifecycle.ResourceTransactionInstalled
	if current != nil {
		disposition = lifecycle.ResourceTransactionReplaced
	}
	return dcjc.prepareMutation(
		scope,
		current,
		successor,
		lifecycle.LongLivedPermit{},
		disposition,
		&postimage,
		mustDynCfgMessage(200, ""),
		dcjc.configStatusCleanup(
			target.resourceID,
			dyncfg.StatusRunning,
		),
		successorFailurePlan{
			postimage: failedPostimage,
			failedCleanup: dcjc.configStatusCleanup(
				target.resourceID,
				dyncfg.StatusFailed,
			),
			removedCleanup: dcjc.configDeleteCleanup(
				dcjc.externalID(target.resourceID),
			),
			result: autoDetectionFailureResultFunc(
				422,
				"config restart failed: %v",
			),
			afterApply: dcjc.scheduleRetryAfterApply(config),
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
		return dcjc.noop(
			scope,
			current,
			lifecycle.LongLivedPermit{},
			mustDynCfgMessage(404, "config not found."),
		)
	}
	if record.Status == dyncfg.StatusDisabled.String() {
		return dcjc.noop(
			scope,
			current,
			lifecycle.LongLivedPermit{},
			mustDynCfgMessage(200, ""),
			dcjc.configStatusCleanup(
				target.resourceID,
				dyncfg.StatusDisabled,
			),
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
		dcjc.configStatusCleanup(
			target.resourceID,
			dyncfg.StatusDisabled,
		),
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
		return dcjc.noop(
			scope,
			current,
			lifecycle.LongLivedPermit{},
			mustDynCfgMessage(404, "config not found."),
		)
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
		dcjc.configDeleteCleanup(
			dcjc.configID(record.Module, record.Name),
		),
	)
}

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
	if len(failurePlans) > 1 ||
		len(failurePlans) == 1 && successor == nil {
		return nil, errors.New(
			"job output: invalid successor-failure plan",
		)
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
		dependencyCommit, err = dcjc.dependencies.PrepareJobChange(
			scope.ID,
			postimage,
		)
		if err != nil {
			if successor != nil {
				err = rollbackSuccessorMutation(successor, err)
			}
			return nil, err
		}
		if failurePlan != nil {
			failedDependencyCommit, err =
				dcjc.dependencies.PrepareJobChange(
					scope.ID,
					&failurePlan.postimage,
				)
			if err == nil &&
				failurePlan.removePlainStock {
				removedDependencyCommit, err =
					dcjc.dependencies.PrepareJobChange(
						scope.ID,
						nil,
					)
			}
			if err != nil {
				return nil, rollbackSuccessorMutation(successor, err)
			}
		}
	}
	mutation, err := dcjc.graph.PrepareMutation(
		[]dyncfg.GraphChange{{
			ID: scope.ID, Config: postimage,
		}},
	)
	if errors.Is(err, dyncfg.ErrGraphNoChange) {
		if successor != nil {
			return dcjc.prepareResourceTransaction(
				ResourceTransactionSpec{
					Scope: scope, Disposition: disposition,
					Current: current, Successor: successor,
					Graph:            dcjc.graph,
					AfterGraphCommit: dependencyCommit,
					AfterApply: dcjc.retrySettlement(
						scope.ID,
						retry,
					),
					Result: result, Cleanup: cleanup,
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
			Scope: scope, Disposition: disposition,
			Current: current, Successor: successor,
			UnusedPermit: unusedPermit,
			Graph:        dcjc.graph, Mutation: mutation,
			MutationPrepared: true,
			AfterGraphCommit: dependencyCommit,
			AfterApply:       dcjc.retrySettlement(scope.ID, retry),
			Result:           result, Cleanup: cleanup,
			SuccessorFailure: successorFailureResolver(
				failurePlan,
				failedDependencyCommit,
				removedDependencyCommit,
			),
		},
	)
}

func (dcjc *DynCfgJobController) retrySettlement(
	id string,
	token autoDetectionRetryToken,
) func() {
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
		rollbackErr = errors.Join(
			rollbackErr,
			rejectPreparedSuccessor(context.Background(), spec.Successor),
		)
	}
	err = errors.Join(err, rollbackErr)
	if rollbackErr != nil {
		err = lifecycle.RetainOwnership(err)
	}
	return nil, err
}

func (dcjc *DynCfgJobController) scheduleAutoDetectionRetry(
	config confgroup.Config,
	failure *autoDetectionFailure,
) {
	if dcjc == nil ||
		dcjc.scheduler == nil ||
		failure == nil ||
		!failure.retry {
		return
	}
	dcjc.scheduler.retries.schedule(
		config,
		failure.retryAfter,
	)
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
) func(
	*autoDetectionFailure,
) (SuccessorFailureResolution, error) {
	if plan == nil {
		return nil
	}
	return func(
		failure *autoDetectionFailure,
	) (SuccessorFailureResolution, error) {
		if failure == nil {
			return SuccessorFailureResolution{},
				errors.New(
					"job output: nil autodetection failure",
				)
		}
		removed := plan.removePlainStock &&
			!failure.coded
		postimage := plan.postimage
		resolution := SuccessorFailureResolution{
			Postimage:        &postimage,
			AfterGraphCommit: failedDependencyCommit,
			Cleanup:          plan.failedCleanup,
		}
		if removed {
			resolution.Postimage = nil
			resolution.AfterGraphCommit =
				removedDependencyCommit
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

func autoDetectionFailureResult(
	failure *autoDetectionFailure,
	defaultCode int,
	message string,
) lifecycle.SealedResult {
	code := defaultCode
	if failure.coded {
		code = failure.code
	}
	return mustDynCfgMessage(
		code,
		fmt.Sprintf(message, failure.cause),
	)
}

// autoDetectionFailureResultFunc adapts autoDetectionFailureResult into a
// successorFailurePlan.result closure with a fixed default code and message.
func autoDetectionFailureResultFunc(
	defaultCode int,
	message string,
) func(*autoDetectionFailure) lifecycle.SealedResult {
	return func(failure *autoDetectionFailure) lifecycle.SealedResult {
		return autoDetectionFailureResult(failure, defaultCode, message)
	}
}

// scheduleRetryAfterApply adapts scheduleAutoDetectionRetry into a
// successorFailurePlan.afterApply closure that reschedules the given config.
func (dcjc *DynCfgJobController) scheduleRetryAfterApply(
	config confgroup.Config,
) func(*autoDetectionFailure) {
	return func(failure *autoDetectionFailure) {
		dcjc.scheduleAutoDetectionRetry(config, failure)
	}
}

func rejectPreparedSuccessor(
	ctx context.Context,
	successor lifecycle.PreparedResource,
) error {
	if prepared, ok := successor.(PreparedJob); ok {
		return prepared.reject(ctx)
	}
	return successor.Dispose(ctx)
}

// rollbackSuccessorMutation rejects a prepared successor after a failed mutation
// prep, joins the rejection error, and retains ownership when the rejection
// itself fails so a leaked resource is never treated as released.
func rollbackSuccessorMutation(
	successor lifecycle.PreparedResource,
	err error,
) error {
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
	return dcjc.noopWithAfterApply(
		scope,
		current,
		permit,
		result,
		nil,
		cleanups...,
	)
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
	return PrepareNoopResourceTransaction(
		scope,
		current,
		permit,
		result,
		cleanup,
		afterApply,
	)
}

func (dcjc *DynCfgJobController) resolveRequest(
	request DynCfgJobRequest,
) (dynCfgTarget, dynCfgFailure) {
	if len(request.Args) < 2 {
		return dynCfgTarget{}, newDynCfgFailure(
			400,
			fmt.Sprintf(
				"missing required arguments: need 2, got %d",
				len(request.Args),
			),
		)
	}
	id := request.Args[0]
	if !strings.HasPrefix(id, dcjc.prefix) {
		return dynCfgTarget{}, newDynCfgFailure(
			400,
			"invalid config ID format.",
		)
	}
	command := dyncfg.Command(strings.ToLower(request.Args[1]))
	target := strings.TrimPrefix(id, dcjc.prefix)
	module, name, hasName := strings.Cut(target, ":")
	if module == "" {
		return dynCfgTarget{}, newDynCfgFailure(
			400,
			"invalid config ID format.",
		)
	}
	creator, ok := dcjc.modules.Lookup(module)
	if !ok {
		return dynCfgTarget{}, newDynCfgFailure(
			404,
			fmt.Sprintf(
				"The specified module '%s' is not registered.",
				module,
			),
		)
	}
	if command == dyncfg.CommandAdd {
		if len(request.Args) < 3 {
			return dynCfgTarget{}, newDynCfgFailure(
				400,
				fmt.Sprintf(
					"missing required arguments: need 3, got %d",
					len(request.Args),
				),
			)
		}
		name = dynCfgJobNameReplacer.Replace(request.Args[2])
		hasName = name != ""
	} else if creator.InstancePolicy == collectorapi.InstancePolicySingle {
		if id != dcjc.prefix+module {
			return dynCfgTarget{}, newDynCfgFailure(
				400,
				fmt.Sprintf(
					"Single-instance collector %s must use config ID %s.",
					module,
					dcjc.prefix+module,
				),
			)
		}
		name = module
		hasName = true
	}
	if !hasName {
		if command == dyncfg.CommandSchema ||
			command == dyncfg.CommandUserconfig ||
			command == dyncfg.CommandTest {
			name = "test"
		} else {
			return dynCfgTarget{}, newDynCfgFailure(
				400,
				"invalid config ID format.",
			)
		}
	}
	if command == dyncfg.CommandUserconfig ||
		command == dyncfg.CommandTest {
		if len(request.Args) >= 3 && request.Args[2] != "" {
			name = dynCfgJobNameReplacer.Replace(request.Args[2])
		} else if creator.InstancePolicy == collectorapi.InstancePolicySingle {
			name = module
		}
	}
	if err := dyncfg.JobNameRuleStrict(name); err != nil {
		return dynCfgTarget{}, newDynCfgFailure(
			400,
			fmt.Sprintf(
				"Unacceptable job name '%s': %v.",
				name,
				err,
			),
		)
	}
	if creator.InstancePolicy == collectorapi.InstancePolicySingle &&
		name != module {
		return dynCfgTarget{}, newDynCfgFailure(
			400,
			fmt.Sprintf(
				"Single-instance collector %s must use config name %q.",
				module,
				module,
			),
		)
	}
	resourceID := module + "_" + name
	if module == name {
		resourceID = module
	}
	return dynCfgTarget{
		command: command, module: module, name: name,
		resourceID: resourceID, creator: creator,
	}, dynCfgFailure{}
}

func (dcjc *DynCfgJobController) parseConfig(
	request DynCfgJobRequest,
	module string,
	name string,
) (confgroup.Config, dynCfgFailure) {
	if !request.HasPayload || len(request.Payload) == 0 {
		return nil, newDynCfgFailure(
			400,
			"missing configuration payload",
		)
	}
	var config confgroup.Config
	var err error
	if request.ContentType == "application/json" {
		err = json.Unmarshal(request.Payload, &config)
		if err == nil && config != nil {
			config, err = config.Clone()
		}
	} else {
		err = yaml.Unmarshal(request.Payload, &config)
	}
	if err != nil {
		return nil, newDynCfgFailure(
			400,
			fmt.Sprintf(
				"invalid configuration format: failed to create configuration from payload: %v",
				err,
			),
		)
	}
	if config == nil {
		return nil, newDynCfgFailure(
			400,
			"invalid configuration format: payload contains no configuration object",
		)
	}
	if !validDynCfgProtocolField(request.CallerSource) {
		return nil, newDynCfgFailure(
			400,
			"invalid Function source",
		)
	}
	config.SetProvider(confgroup.TypeDyncfg)
	config.SetSource(request.CallerSource)
	config.SetSourceType(confgroup.TypeDyncfg)
	config.SetModule(module)
	config.SetName(name)
	if defaults, ok := dcjc.defaults.Lookup(module); ok {
		config.ApplyDefaults(defaults)
	}
	return config, dynCfgFailure{}
}

func (dcjc *DynCfgJobController) userConfig(
	request DynCfgJobRequest,
	target dynCfgTarget,
) (lifecycle.SealedResult, error) {
	if target.creator.Config == nil {
		return dynCfgMessage(
			500,
			fmt.Sprintf(
				"Module %s does not provide configuration.",
				target.module,
			),
		)
	}
	config := target.creator.Config()
	if config == nil {
		return dynCfgMessage(
			500,
			fmt.Sprintf(
				"Module %s does not provide configuration.",
				target.module,
			),
		)
	}
	if request.ContentType == "application/json" {
		if err := json.Unmarshal(request.Payload, config); err != nil {
			return dynCfgMessage(400, err.Error())
		}
	} else if err := yaml.Unmarshal(request.Payload, config); err != nil {
		return dynCfgMessage(400, err.Error())
	}
	payload, err := yaml.Marshal(config)
	if err != nil {
		return lifecycle.SealedResult{}, err
	}
	var values yaml.MapSlice
	if err := yaml.Unmarshal(payload, &values); err != nil {
		return lifecycle.SealedResult{}, err
	}
	filtered := values[:0]
	for _, item := range values {
		if item.Key != "name" {
			filtered = append(filtered, item)
		}
	}
	values = append(
		[]yaml.MapItem{{Key: "name", Value: target.name}},
		filtered...,
	)
	payload, err = yaml.Marshal(
		map[string]any{"jobs": []any{values}},
	)
	if err != nil {
		return lifecycle.SealedResult{}, err
	}
	return lifecycle.NewSealedResult(
		200,
		"application/yaml",
		payload,
	)
}

func (dcjc *DynCfgJobController) configCreateCleanup(
	config dyncfg.GraphConfig,
	sourceType string,
	source string,
	configType dyncfg.ConfigType,
) lifecycle.TaskCleanup {
	id := dcjc.configID(config.Module, config.Name)
	commands := dyncfg.JoinCommands(
		dyncfg.CommandSchema,
		dyncfg.CommandGet,
		dyncfg.CommandEnable,
		dyncfg.CommandDisable,
		dyncfg.CommandUpdate,
		dyncfg.CommandRestart,
		dyncfg.CommandTest,
		dyncfg.CommandUserconfig,
	)
	if sourceType == confgroup.TypeDyncfg &&
		configType == dyncfg.ConfigTypeJob {
		commands += " " + string(dyncfg.CommandRemove)
	}
	return dcjc.protocolCleanup(func(api *netdataapi.API) {
		api.CONFIGCREATE(
			netdataapi.ConfigOpts{
				ID: id, Status: config.Status,
				ConfigType: configType.String(), Path: dcjc.path,
				SourceType: sourceType, Source: source,
				SupportedCommands: commands,
			},
		)
	})
}

func (dcjc *DynCfgJobController) configStatusCleanup(
	id string,
	status dyncfg.Status,
) lifecycle.TaskCleanup {
	return dcjc.protocolCleanup(func(api *netdataapi.API) {
		api.CONFIGSTATUS(
			dcjc.externalID(id),
			status.String(),
		)
	})
}

func (dcjc *DynCfgJobController) configDeleteCleanup(
	externalID string,
) lifecycle.TaskCleanup {
	return dcjc.protocolCleanup(func(api *netdataapi.API) {
		api.CONFIGDELETE(externalID)
	})
}

func (dcjc *DynCfgJobController) protocolCleanup(
	build func(*netdataapi.API),
) lifecycle.TaskCleanup {
	if dcjc == nil || build == nil {
		return func() error {
			return errors.New(
				"job output: invalid DynCfg protocol cleanup",
			)
		}
	}
	var payload bytes.Buffer
	build(netdataapi.New(&payload))
	prepared, err := lifecycle.PrepareProtocolFrame(payload.Bytes())
	if err != nil {
		return func() error { return err }
	}
	return func() error {
		return dcjc.frames.CommitPreparedProtocolFrame(prepared)
	}
}

func (dcjc *DynCfgJobController) configID(
	module string,
	name string,
) string {
	if module == name ||
		dcjc.modules[module].InstancePolicy ==
			collectorapi.InstancePolicySingle {
		return dcjc.prefix + module
	}
	return dcjc.prefix + module + ":" + name
}

func (dcjc *DynCfgJobController) externalID(resourceID string) string {
	record, ok := dcjc.graph.Lookup(resourceID)
	if ok {
		return dcjc.configID(record.Module, record.Name)
	}
	return ""
}

func (dcjc *DynCfgJobController) configType(
	creator collectorapi.Creator,
) dyncfg.ConfigType {
	if creator.InstancePolicy == collectorapi.InstancePolicySingle {
		return dyncfg.ConfigTypeSingle
	}
	return dyncfg.ConfigTypeJob
}

func dynCfgMessage(
	code int,
	message string,
) (lifecycle.SealedResult, error) {
	return lifecycle.NewSealedResult(
		code,
		"application/json",
		frameworkfunctions.BuildJSONPayload(code, message),
	)
}

func mustDynCfgMessage(
	code int,
	message string,
) lifecycle.SealedResult {
	result, err := dynCfgMessage(code, message)
	if err != nil {
		panic(err)
	}
	return result
}

type dynCfgTarget struct {
	command    dyncfg.Command       // parsed dyncfg command
	module     string               // collector module name
	name       string               // job name
	resourceID string               // graph resource ID (module or module_name)
	creator    collectorapi.Creator // resolved collector creator
}

type dynCfgFailure struct {
	valid  bool
	result lifecycle.SealedResult
}

func newDynCfgFailure(
	code int,
	message string,
) dynCfgFailure {
	return dynCfgFailure{
		valid:  true,
		result: mustDynCfgMessage(code, message),
	}
}

func graphRecordConfig(
	record dyncfg.GraphRecord,
) (confgroup.Config, error) {
	var config confgroup.Config
	if err := yaml.Unmarshal(
		[]byte(record.Payload()),
		&config,
	); err != nil {
		return nil, fmt.Errorf(
			"job output: invalid stored DynCfg payload: %w",
			err,
		)
	}
	if config == nil ||
		config.Module() != record.Module ||
		config.Name() != record.Name {
		return nil, errors.New(
			"job output: stored DynCfg identity differs from graph",
		)
	}
	return config, nil
}

func graphConfig(
	record dyncfg.GraphRecord,
	status dyncfg.Status,
) dyncfg.GraphConfig {
	return dyncfg.GraphConfig{
		ID: record.ID, Module: record.Module, Name: record.Name,
		Status: status.String(), Payload: []byte(record.Payload()),
	}
}

func validateGraphResourcePair(
	record dyncfg.GraphRecord,
	exists bool,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
) error {
	if (current == nil) != !scope.Current.Valid() {
		return errors.New(
			"job output: DynCfg transaction current differs from scope",
		)
	}
	if !exists {
		if current != nil {
			return errors.New(
				"job output: missing DynCfg record retained a current job",
			)
		}
		return nil
	}
	running := record.Status == dyncfg.StatusRunning.String()
	if running != (current != nil) {
		return errors.New(
			"job output: DynCfg status differs from current-job slot",
		)
	}
	return nil
}

func validDynCfgProtocolField(value string) bool {
	for _, char := range value {
		if char < ' ' || char == 0x7f || char == '\'' {
			return false
		}
	}
	return true
}

func joinDynCfgCleanups(
	cleanups ...lifecycle.TaskCleanup,
) lifecycle.TaskCleanup {
	joined := make(
		[]lifecycle.TaskCleanup,
		0,
		len(cleanups),
	)
	for _, cleanup := range cleanups {
		if cleanup != nil {
			joined = append(joined, cleanup)
		}
	}
	return func() error {
		var err error
		for _, cleanup := range joined {
			err = errors.Join(err, cleanup())
		}
		return err
	}
}
