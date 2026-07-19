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
	Args         []string
	Payload      []byte
	ContentType  string
	CallerSource string
	HasPayload   bool
}

type DynCfgJobControllerConfig struct {
	PluginName    string
	Modules       collectorapi.Registry
	Defaults      confgroup.Registry
	Factory       *Factory
	ConfigModules *ConfigModuleFactory
	Graph         *dyncfg.Graph
	Frames        *lifecycle.FrameOwner
	Dependencies  JobDependencyIndex
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
	pluginName    string
	prefix        string
	path          string
	modules       collectorapi.Registry
	defaults      confgroup.Registry
	factory       *Factory
	configModules *ConfigModuleFactory
	graph         *dyncfg.Graph
	frames        *lifecycle.FrameOwner
	dependencies  JobDependencyIndex
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
	}, nil
}

func DynCfgJobPrefix(pluginName string) string {
	if pluginName == "" {
		return ""
	}
	return fmt.Sprintf(dynCfgCollectorPrefixFormat, pluginName)
}

func (controller *DynCfgJobController) Prefix() string {
	if controller == nil {
		return ""
	}
	return controller.prefix
}

func (controller *DynCfgJobController) Handle(
	ctx context.Context,
	request DynCfgJobRequest,
) (lifecycle.SealedResult, error) {
	if controller == nil || ctx == nil {
		return lifecycle.SealedResult{},
			errors.New("job output: invalid DynCfg request")
	}
	target, result := controller.resolveRequest(request, false)
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
		return controller.userConfig(request, target)
	case dyncfg.CommandTest:
		config, failure := controller.parseConfig(
			request,
			target.module,
			target.name,
		)
		if failure.valid {
			return failure.result, nil
		}
		if err := controller.configModules.Test(ctx, config); err != nil {
			return dynCfgMessage(422, err.Error())
		}
		return dynCfgMessage(200, "")
	case dyncfg.CommandGet:
		record, ok := controller.graph.Lookup(target.resourceID)
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
		payload, err := controller.configModules.Configuration(
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

func (controller *DynCfgJobController) Prepare(
	ctx context.Context,
	request DynCfgJobRequest,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
	permit lifecycle.LongLivedPermit,
) (lifecycle.PreparedResourceTransaction, error) {
	if controller == nil || ctx == nil || !scope.Valid() {
		return nil, errors.New(
			"job output: invalid DynCfg transaction preparation",
		)
	}
	target, failure := controller.resolveRequest(request, true)
	if failure.valid {
		return controller.noop(scope, current, permit, failure.result)
	}
	if target.resourceID != scope.ID {
		return nil, errors.New(
			"job output: DynCfg target differs from transaction scope",
		)
	}
	record, exists := controller.graph.Lookup(target.resourceID)
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
		return controller.prepareAdd(
			ctx,
			request,
			target,
			record,
			exists,
			current,
			scope,
		)
	case dyncfg.CommandUpdate:
		return controller.prepareUpdate(
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
		return controller.prepareEnable(
			ctx,
			target,
			record,
			exists,
			current,
			scope,
			permit,
		)
	case dyncfg.CommandRestart:
		return controller.prepareRestart(
			ctx,
			target,
			record,
			exists,
			current,
			scope,
			permit,
		)
	case dyncfg.CommandDisable:
		return controller.prepareDisable(
			target,
			record,
			exists,
			current,
			scope,
		)
	case dyncfg.CommandRemove:
		return controller.prepareRemove(
			target,
			record,
			exists,
			current,
			scope,
		)
	default:
		return controller.noop(
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

func (controller *DynCfgJobController) prepareAdd(
	ctx context.Context,
	request DynCfgJobRequest,
	target dynCfgTarget,
	record dyncfg.GraphRecord,
	exists bool,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
) (lifecycle.PreparedResourceTransaction, error) {
	if target.creator.InstancePolicy == collectorapi.InstancePolicySingle {
		return controller.noop(
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
	config, failure := controller.parseConfig(
		request,
		target.module,
		target.name,
	)
	if failure.valid {
		return controller.noop(
			scope,
			current,
			lifecycle.LongLivedPermit{},
			failure.result,
		)
	}
	if err := controller.factory.ValidateConfig(ctx, config); err != nil {
		return controller.noop(
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
	_ = record
	_ = exists
	return controller.prepareMutation(
		scope,
		current,
		nil,
		lifecycle.LongLivedPermit{},
		disposition,
		&postimage,
		mustDynCfgMessage(202, ""),
		controller.configCreateCleanup(
			postimage,
			confgroup.TypeDyncfg,
			request.CallerSource,
			dyncfg.ConfigTypeJob,
		),
	)
}

func (controller *DynCfgJobController) prepareUpdate(
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
		return controller.noop(
			scope,
			current,
			permit,
			mustDynCfgMessage(404, "config not found."),
		)
	}
	config, failure := controller.parseConfig(
		request,
		target.module,
		target.name,
	)
	if failure.valid {
		return controller.noop(
			scope,
			current,
			permit,
			failure.result,
		)
	}
	if err := controller.factory.ValidateConfig(ctx, config); err != nil {
		return controller.noop(
			scope,
			current,
			permit,
			mustDynCfgMessage(400, err.Error()),
			controller.configStatusCleanup(
				target.resourceID,
				dyncfg.Status(record.Status),
			),
		)
	}
	if record.Status == dyncfg.StatusAccepted.String() {
		return controller.noop(
			scope,
			current,
			permit,
			mustDynCfgMessage(
				403,
				"updating is not allowed in 'accepted' state.",
			),
			controller.configStatusCleanup(
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
		return controller.noop(
			scope,
			current,
			permit,
			mustDynCfgMessage(200, ""),
			controller.configStatusCleanup(
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
		cleanup := controller.configStatusCleanup(
			target.resourceID,
			dyncfg.StatusDisabled,
		)
		if oldConfig.SourceType() != confgroup.TypeDyncfg {
			cleanup = controller.configCreateCleanup(
				postimage,
				confgroup.TypeDyncfg,
				request.CallerSource,
				controller.configType(target.creator),
			)
		}
		return controller.prepareMutation(
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
	successor, err := controller.factory.Prepare(
		ctx,
		config,
		scope.Successor,
		permit,
	)
	if err != nil {
		if ctx.Err() != nil {
			return nil, err
		}
		return controller.noop(
			scope,
			current,
			permit,
			mustDynCfgMessage(200, err.Error()),
			controller.configStatusCleanup(
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
	cleanup := controller.configStatusCleanup(
		target.resourceID,
		dyncfg.StatusRunning,
	)
	if oldConfig.SourceType() != confgroup.TypeDyncfg {
		cleanup = controller.configCreateCleanup(
			postimage,
			confgroup.TypeDyncfg,
			request.CallerSource,
			controller.configType(target.creator),
		)
	}
	return controller.prepareMutation(
		scope,
		current,
		successor,
		lifecycle.LongLivedPermit{},
		disposition,
		&postimage,
		mustDynCfgMessage(200, ""),
		cleanup,
	)
}

func (controller *DynCfgJobController) prepareEnable(
	ctx context.Context,
	target dynCfgTarget,
	record dyncfg.GraphRecord,
	exists bool,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
	permit lifecycle.LongLivedPermit,
) (lifecycle.PreparedResourceTransaction, error) {
	if !exists {
		return controller.noop(
			scope,
			current,
			permit,
			mustDynCfgMessage(404, "config not found."),
		)
	}
	if record.Status == dyncfg.StatusRunning.String() {
		return controller.noop(
			scope,
			current,
			permit,
			mustDynCfgMessage(200, ""),
			controller.configStatusCleanup(
				target.resourceID,
				dyncfg.StatusRunning,
			),
		)
	}
	config, err := graphRecordConfig(record)
	if err != nil {
		return nil, err
	}
	successor, err := controller.factory.Prepare(
		ctx,
		config,
		scope.Successor,
		permit,
	)
	if err != nil {
		if ctx.Err() != nil {
			return nil, err
		}
		return controller.noop(
			scope,
			current,
			permit,
			mustDynCfgMessage(200, err.Error()),
		)
	}
	postimage := graphConfig(record, dyncfg.StatusRunning)
	return controller.prepareMutation(
		scope,
		current,
		successor,
		lifecycle.LongLivedPermit{},
		lifecycle.ResourceTransactionInstalled,
		&postimage,
		mustDynCfgMessage(200, ""),
		controller.configStatusCleanup(
			target.resourceID,
			dyncfg.StatusRunning,
		),
	)
}

func (controller *DynCfgJobController) prepareRestart(
	ctx context.Context,
	target dynCfgTarget,
	record dyncfg.GraphRecord,
	exists bool,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
	permit lifecycle.LongLivedPermit,
) (lifecycle.PreparedResourceTransaction, error) {
	if !exists {
		return controller.noop(
			scope,
			current,
			permit,
			mustDynCfgMessage(404, "config not found."),
		)
	}
	status := dyncfg.Status(record.Status)
	if status != dyncfg.StatusRunning && status != dyncfg.StatusFailed {
		return controller.noop(
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
			controller.configStatusCleanup(
				target.resourceID,
				status,
			),
		)
	}
	config, err := graphRecordConfig(record)
	if err != nil {
		return nil, err
	}
	successor, err := controller.factory.Prepare(
		ctx,
		config,
		scope.Successor,
		permit,
	)
	if err != nil {
		if ctx.Err() != nil {
			return nil, err
		}
		return controller.noop(
			scope,
			current,
			permit,
			mustDynCfgMessage(
				422,
				fmt.Sprintf("config restart failed: %v", err),
			),
			controller.configStatusCleanup(
				target.resourceID,
				status,
			),
		)
	}
	postimage := graphConfig(record, dyncfg.StatusRunning)
	disposition := lifecycle.ResourceTransactionInstalled
	if current != nil {
		disposition = lifecycle.ResourceTransactionReplaced
	}
	return controller.prepareMutation(
		scope,
		current,
		successor,
		lifecycle.LongLivedPermit{},
		disposition,
		&postimage,
		mustDynCfgMessage(200, ""),
		controller.configStatusCleanup(
			target.resourceID,
			dyncfg.StatusRunning,
		),
	)
}

func (controller *DynCfgJobController) prepareDisable(
	target dynCfgTarget,
	record dyncfg.GraphRecord,
	exists bool,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
) (lifecycle.PreparedResourceTransaction, error) {
	if !exists {
		return controller.noop(
			scope,
			current,
			lifecycle.LongLivedPermit{},
			mustDynCfgMessage(404, "config not found."),
		)
	}
	if record.Status == dyncfg.StatusDisabled.String() {
		return controller.noop(
			scope,
			current,
			lifecycle.LongLivedPermit{},
			mustDynCfgMessage(200, ""),
			controller.configStatusCleanup(
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
	return controller.prepareMutation(
		scope,
		current,
		nil,
		lifecycle.LongLivedPermit{},
		disposition,
		&postimage,
		mustDynCfgMessage(200, ""),
		controller.configStatusCleanup(
			target.resourceID,
			dyncfg.StatusDisabled,
		),
	)
}

func (controller *DynCfgJobController) prepareRemove(
	target dynCfgTarget,
	record dyncfg.GraphRecord,
	exists bool,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
) (lifecycle.PreparedResourceTransaction, error) {
	if !exists {
		return controller.noop(
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
		return controller.noop(
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
		return controller.noop(
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
	return controller.prepareMutation(
		scope,
		current,
		nil,
		lifecycle.LongLivedPermit{},
		disposition,
		nil,
		mustDynCfgMessage(200, ""),
		controller.configDeleteCleanup(
			controller.configID(record.Module, record.Name),
		),
	)
}

func (controller *DynCfgJobController) prepareMutation(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	successor lifecycle.PreparedResource,
	unusedPermit lifecycle.LongLivedPermit,
	disposition lifecycle.ResourceTransactionDisposition,
	postimage *dyncfg.GraphConfig,
	result lifecycle.SealedResult,
	cleanup lifecycle.TaskCleanup,
) (lifecycle.PreparedResourceTransaction, error) {
	var dependencyCommit func()
	if controller.dependencies != nil {
		var err error
		dependencyCommit, err = controller.dependencies.PrepareJobChange(
			scopeResourceID(scope),
			postimage,
		)
		if err != nil {
			if successor != nil {
				err = errors.Join(
					err,
					successor.Dispose(context.Background()),
				)
			} else if unusedPermit.Valid() {
				err = errors.Join(err, unusedPermit.AbortUnused())
			}
			return nil, err
		}
	}
	mutation, err := controller.graph.PrepareMutation(
		[]dyncfg.GraphChange{{
			ID: scopeResourceID(scope), Config: postimage,
		}},
	)
	if errors.Is(err, dyncfg.ErrGraphNoChange) {
		if successor != nil {
			return PrepareResourceTransaction(
				ResourceTransactionSpec{
					Scope: scope, Disposition: disposition,
					Current: current, Successor: successor,
					Result: result, Cleanup: cleanup,
				},
			)
		}
		return controller.noop(
			scope,
			current,
			unusedPermit,
			result,
			cleanup,
		)
	}
	if err != nil {
		if successor != nil {
			err = errors.Join(
				err,
				successor.Dispose(context.Background()),
			)
		} else if unusedPermit.Valid() {
			err = errors.Join(err, unusedPermit.AbortUnused())
		}
		return nil, err
	}
	return PrepareResourceTransaction(
		ResourceTransactionSpec{
			Scope: scope, Disposition: disposition,
			Current: current, Successor: successor,
			UnusedPermit: unusedPermit,
			Graph:        controller.graph, Mutation: mutation,
			AfterGraphCommit: dependencyCommit,
			Result:           result, Cleanup: cleanup,
		},
	)
}

func (controller *DynCfgJobController) noop(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	permit lifecycle.LongLivedPermit,
	result lifecycle.SealedResult,
	cleanups ...lifecycle.TaskCleanup,
) (lifecycle.PreparedResourceTransaction, error) {
	cleanup := joinDynCfgCleanups(cleanups...)
	return PrepareNoopResourceTransaction(
		scope,
		current,
		permit,
		result,
		cleanup,
	)
}

func (controller *DynCfgJobController) resolveRequest(
	request DynCfgJobRequest,
	mutation bool,
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
	if !strings.HasPrefix(id, controller.prefix) {
		return dynCfgTarget{}, newDynCfgFailure(
			400,
			"invalid config ID format.",
		)
	}
	command := dyncfg.Command(strings.ToLower(request.Args[1]))
	target := strings.TrimPrefix(id, controller.prefix)
	module, name, hasName := strings.Cut(target, ":")
	if module == "" {
		return dynCfgTarget{}, newDynCfgFailure(
			400,
			"invalid config ID format.",
		)
	}
	creator, ok := controller.modules.Lookup(module)
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
		if id != controller.prefix+module {
			return dynCfgTarget{}, newDynCfgFailure(
				400,
				fmt.Sprintf(
					"Single-instance collector %s must use config ID %s.",
					module,
					controller.prefix+module,
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
	if mutation &&
		command != dyncfg.CommandAdd &&
		command != dyncfg.CommandUpdate &&
		command != dyncfg.CommandEnable &&
		command != dyncfg.CommandRestart &&
		command != dyncfg.CommandDisable &&
		command != dyncfg.CommandRemove {
		return dynCfgTarget{
			command: command, module: module, name: name,
			resourceID: resourceID, creator: creator,
		}, dynCfgFailure{}
	}
	return dynCfgTarget{
		command: command, module: module, name: name,
		resourceID: resourceID, creator: creator,
	}, dynCfgFailure{}
}

func (controller *DynCfgJobController) parseConfig(
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
	if defaults, ok := controller.defaults.Lookup(module); ok {
		config.ApplyDefaults(defaults)
	}
	return config, dynCfgFailure{}
}

func (controller *DynCfgJobController) userConfig(
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

func (controller *DynCfgJobController) configCreateCleanup(
	config dyncfg.GraphConfig,
	sourceType string,
	source string,
	configType dyncfg.ConfigType,
) lifecycle.TaskCleanup {
	id := controller.configID(config.Module, config.Name)
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
	return controller.protocolCleanup(func(api *netdataapi.API) {
		api.CONFIGCREATE(
			netdataapi.ConfigOpts{
				ID: id, Status: config.Status,
				ConfigType: configType.String(), Path: controller.path,
				SourceType: sourceType, Source: source,
				SupportedCommands: commands,
			},
		)
	})
}

func (controller *DynCfgJobController) configStatusCleanup(
	id string,
	status dyncfg.Status,
) lifecycle.TaskCleanup {
	return controller.protocolCleanup(func(api *netdataapi.API) {
		api.CONFIGSTATUS(
			controller.externalID(id),
			status.String(),
		)
	})
}

func (controller *DynCfgJobController) configDeleteCleanup(
	externalID string,
) lifecycle.TaskCleanup {
	return controller.protocolCleanup(func(api *netdataapi.API) {
		api.CONFIGDELETE(externalID)
	})
}

func (controller *DynCfgJobController) protocolCleanup(
	build func(*netdataapi.API),
) lifecycle.TaskCleanup {
	if controller == nil || build == nil {
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
		return controller.frames.CommitPreparedProtocolFrame(prepared)
	}
}

func (controller *DynCfgJobController) configID(
	module string,
	name string,
) string {
	if module == name ||
		controller.modules[module].InstancePolicy ==
			collectorapi.InstancePolicySingle {
		return controller.prefix + module
	}
	return controller.prefix + module + ":" + name
}

func (controller *DynCfgJobController) externalID(resourceID string) string {
	record, ok := controller.graph.Lookup(resourceID)
	if ok {
		return controller.configID(record.Module, record.Name)
	}
	return ""
}

func (controller *DynCfgJobController) configType(
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
	command    dyncfg.Command
	module     string
	name       string
	resourceID string
	creator    collectorapi.Creator
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

func transactionScopeID(
	scope lifecycle.ResourceTransactionScope,
) string {
	return scope.ID
}

func scopeResourceID(
	scope lifecycle.ResourceTransactionScope,
) string {
	return transactionScopeID(scope)
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
