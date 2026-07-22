// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	PrepareJobChange(string, *dyncfg.GraphConfig) (func(), error)
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

func NewDynCfgJobController(config DynCfgJobControllerConfig) (*DynCfgJobController, error) {
	if config.PluginName == "" ||
		config.Modules == nil ||
		config.Defaults == nil ||
		config.Factory == nil ||
		config.ConfigModules == nil ||
		config.Graph == nil ||
		config.Frames == nil {
		return nil, errors.New("job output: incomplete DynCfg job controller configuration")
	}
	return &DynCfgJobController{
		pluginName:    config.PluginName,
		prefix:        DynCfgJobPrefix(config.PluginName),
		path:          fmt.Sprintf(dynCfgCollectorPathFormat, config.PluginName),
		modules:       config.Modules,
		defaults:      config.Defaults,
		factory:       config.Factory,
		configModules: config.ConfigModules,
		graph:         config.Graph,
		frames:        config.Frames,
		dependencies:  config.Dependencies,
		scheduler:     config.Factory.config.Scheduler,
	}, nil
}

func (dcjc *DynCfgJobController) BindAutoDetectionRetries(
	commands jobmgr.PreparedCommandPort,
	run uint64,
	failure func(error),
) error {
	if dcjc == nil || dcjc.scheduler == nil {
		return errors.New("job output: invalid autodetection retry controller")
	}
	return dcjc.scheduler.bindAutoDetectionRetries(commands, dcjc.planAutoDetectionRetry, run, failure)
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
		return lifecycle.SealedResult{}, errors.New("job output: invalid DynCfg request")
	}
	target, result := dcjc.resolveRequest(request)
	if result.valid {
		return result.result, nil
	}
	switch target.command {
	case dyncfg.CommandSchema:
		if target.creator.JobConfigSchema == "" {
			return dynCfgMessage(500, fmt.Sprintf("Module %s configuration schema not found.", target.module))
		}
		return lifecycle.NewSealedResult(200, "application/json", []byte(target.creator.JobConfigSchema))
	case dyncfg.CommandUserconfig:
		return dcjc.userConfig(request, target)
	case dyncfg.CommandTest:
		config, failure := dcjc.parseConfig(request, target.module, target.name)
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
				fmt.Sprintf("The specified module '%s' job '%s' is not registered.", target.module, target.name),
			)
		}
		config, err := graphRecordConfig(record)
		if err != nil {
			return lifecycle.SealedResult{}, err
		}
		payload, err := dcjc.configModules.Configuration(ctx, config)
		if err != nil {
			return dynCfgMessage(500, err.Error())
		}
		return lifecycle.NewSealedResult(200, "application/json", payload)
	default:
		return dynCfgMessage(
			501,
			fmt.Sprintf("Function '%s' command '%s' is not implemented.", DynCfgFunctionName, target.command),
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
		return nil, errors.New("job output: invalid DynCfg transaction preparation")
	}
	target, failure := dcjc.resolveRequest(request)
	if failure.valid {
		return dcjc.noop(scope, current, permit, failure.result)
	}
	if target.resourceID != scope.ID {
		return nil, errors.New("job output: DynCfg target differs from transaction scope")
	}
	record, exists := dcjc.graph.Lookup(target.resourceID)
	if err := validateGraphResourcePair(record, exists, current, scope); err != nil {
		return nil, err
	}

	switch target.command {
	case dyncfg.CommandAdd:
		return dcjc.prepareAdd(ctx, request, target, current, scope)
	case dyncfg.CommandUpdate:
		return dcjc.prepareUpdate(ctx, request, target, record, exists, current, scope, permit)
	case dyncfg.CommandEnable:
		return dcjc.prepareEnable(ctx, target, record, exists, current, scope, permit)
	case dyncfg.CommandRestart:
		return dcjc.prepareRestart(ctx, target, record, exists, current, scope, permit)
	case dyncfg.CommandDisable:
		return dcjc.prepareDisable(target, record, exists, current, scope)
	case dyncfg.CommandRemove:
		return dcjc.prepareRemove(target, record, exists, current, scope)
	default:
		return dcjc.noop(
			scope,
			current,
			permit,
			mustDynCfgMessage(
				501,
				fmt.Sprintf("Function '%s' command '%s' is not implemented.", DynCfgFunctionName, target.command),
			),
		)
	}
}

func (dcjc *DynCfgJobController) configID(module string, name string) string {
	if module == name || dcjc.modules[module].InstancePolicy == collectorapi.InstancePolicySingle {
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

func (dcjc *DynCfgJobController) configType(creator collectorapi.Creator) dyncfg.ConfigType {
	if creator.InstancePolicy == collectorapi.InstancePolicySingle {
		return dyncfg.ConfigTypeSingle
	}
	return dyncfg.ConfigTypeJob
}

func dynCfgMessage(code int, message string) (lifecycle.SealedResult, error) {
	return lifecycle.NewSealedResult(code, "application/json", frameworkfunctions.BuildJSONPayload(code, message))
}

func mustDynCfgMessage(code int, message string) lifecycle.SealedResult {
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

func newDynCfgFailure(code int, message string) dynCfgFailure {
	return dynCfgFailure{
		valid:  true,
		result: mustDynCfgMessage(code, message),
	}
}

func graphRecordConfig(record dyncfg.GraphRecord) (confgroup.Config, error) {
	var config confgroup.Config
	if err := yaml.Unmarshal([]byte(record.Payload()), &config); err != nil {
		return nil, fmt.Errorf("job output: invalid stored DynCfg payload: %w", err)
	}
	if config == nil || config.Module() != record.Module || config.Name() != record.Name {
		return nil, errors.New("job output: stored DynCfg identity differs from graph")
	}
	return config, nil
}

func graphConfig(record dyncfg.GraphRecord, status dyncfg.Status) dyncfg.GraphConfig {
	return dyncfg.GraphConfig{
		ID:      record.ID,
		Module:  record.Module,
		Name:    record.Name,
		Status:  status.String(),
		Payload: []byte(record.Payload()),
	}
}

func validateGraphResourcePair(
	record dyncfg.GraphRecord,
	exists bool,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
) error {
	if (current == nil) != !scope.Current.Valid() {
		return errors.New("job output: DynCfg transaction current differs from scope")
	}
	if !exists {
		if current != nil {
			return errors.New("job output: missing DynCfg record retained a current job")
		}
		return nil
	}
	running := record.Status == dyncfg.StatusRunning.String()
	if running != (current != nil) {
		return errors.New("job output: DynCfg status differs from current-job slot")
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

func joinDynCfgCleanups(cleanups ...lifecycle.TaskCleanup) lifecycle.TaskCleanup {
	joined := make([]lifecycle.TaskCleanup, 0, len(cleanups))
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
