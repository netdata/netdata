// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"gopkg.in/yaml.v2"
)

func (dcjc *DynCfgJobController) resolveRequest(request DynCfgJobRequest) (dynCfgTarget, dynCfgFailure) {
	if len(request.Args) < 2 {
		return dynCfgTarget{}, newDynCfgFailure(
			failureBadRequest,
			fmt.Sprintf("missing required arguments: need 2, got %d", len(request.Args)),
		)
	}
	id := request.Args[0]
	if !strings.HasPrefix(id, dcjc.prefix) {
		return dynCfgTarget{}, newDynCfgFailure(failureBadRequest, "invalid config ID format.")
	}
	command := dyncfg.CommandFromArgs(request.Args)
	target := strings.TrimPrefix(id, dcjc.prefix)
	module, name, hasName := strings.Cut(target, ":")
	if module == "" {
		return dynCfgTarget{}, newDynCfgFailure(failureBadRequest, "invalid config ID format.")
	}
	creator, ok := dcjc.modules.Lookup(module)
	if !ok {
		return dynCfgTarget{}, newDynCfgFailure(
			failureNotFound,
			fmt.Sprintf("The specified module '%s' is not registered.", module),
		)
	}
	if command == dyncfg.CommandAdd {
		if len(request.Args) < 3 {
			return dynCfgTarget{}, newDynCfgFailure(
				failureBadRequest,
				fmt.Sprintf("missing required arguments: need 3, got %d", len(request.Args)),
			)
		}
		// The daemon saves an added job under the name it sent, so the name is
		// validated as sent: rewriting it would adopt a different job.
		name = request.Args[2]
		if name == "" {
			return dynCfgTarget{}, newDynCfgFailure(failureBadRequest, "invalid or missing job name.")
		}
		hasName = true
	} else if creator.InstancePolicy == collectorapi.InstancePolicySingle {
		if id != dcjc.prefix+module {
			return dynCfgTarget{}, newDynCfgFailure(
				failureBadRequest,
				fmt.Sprintf("Single-instance collector %s must use config ID %s.", module, dcjc.prefix+module),
			)
		}
		name = module
		hasName = true
	}
	if !hasName {
		if command == dyncfg.CommandSchema || command == dyncfg.CommandUserconfig || command == dyncfg.CommandTest {
			name = "test"
		} else {
			return dynCfgTarget{}, newDynCfgFailure(failureBadRequest, "invalid config ID format.")
		}
	}
	if command == dyncfg.CommandUserconfig || command == dyncfg.CommandTest {
		if len(request.Args) >= 3 && request.Args[2] != "" {
			name = request.Args[2]
		} else if creator.InstancePolicy == collectorapi.InstancePolicySingle {
			name = module
		}
	}
	if err := dyncfg.JobNameRuleStrict(name); err != nil {
		return dynCfgTarget{}, newDynCfgFailure(failureBadRequest, fmt.Sprintf("Unacceptable job name '%s': %v.", name, err))
	}
	if creator.InstancePolicy == collectorapi.InstancePolicySingle && name != module {
		return dynCfgTarget{}, newDynCfgFailure(
			failureBadRequest,
			fmt.Sprintf("Single-instance collector %s must use config name %q.", module, module),
		)
	}
	resourceID := module + "_" + name
	if module == name {
		resourceID = module
	}
	return dynCfgTarget{
		command:    command,
		module:     module,
		name:       name,
		resourceID: resourceID,
		creator:    creator,
	}, dynCfgFailure{}
}

func (dcjc *DynCfgJobController) parseConfig(
	request DynCfgJobRequest,
	module string,
	name string,
) (confgroup.Config, dynCfgFailure) {
	if !request.HasPayload || len(request.Payload) == 0 {
		return nil, newDynCfgFailure(failureInvalid, "missing configuration payload")
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
			failureInvalid,
			fmt.Sprintf("invalid configuration format: failed to create configuration from payload: %v", err),
		)
	}
	if config == nil {
		return nil, newDynCfgFailure(failureInvalid, "invalid configuration format: payload contains no configuration object")
	}
	if !netdataapi.ValidSingleQuotedProtocolField(request.CallerSource) {
		return nil, newDynCfgFailure(failureBadRequest, "invalid Function source")
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
	missing := jobFailure{
		class:   failureInternal,
		message: fmt.Sprintf("Module %s does not provide configuration.", target.module),
	}
	if target.creator.Config == nil {
		return rejectedResult(missing), nil
	}
	config := target.creator.Config()
	if config == nil {
		return rejectedResult(missing), nil
	}
	if request.ContentType == "application/json" {
		if err := json.Unmarshal(request.Payload, config); err != nil {
			return rejectedResult(jobFailure{class: failureInvalid, message: err.Error()}), nil
		}
	} else if err := yaml.Unmarshal(request.Payload, config); err != nil {
		return rejectedResult(jobFailure{class: failureInvalid, message: err.Error()}), nil
	}
	payload, err := yaml.Marshal(config)
	if err != nil {
		return lifecycle.SealedResult{}, err
	}
	var values yaml.MapSlice
	if err := yaml.Unmarshal(payload, &values); err != nil {
		return lifecycle.SealedResult{}, err
	}
	values = slices.DeleteFunc(values, func(item yaml.MapItem) bool {
		return item.Key == "name"
	})
	values = append([]yaml.MapItem{{Key: "name", Value: target.name}}, values...)
	payload, err = yaml.Marshal(map[string]any{"jobs": []any{values}})
	if err != nil {
		return lifecycle.SealedResult{}, err
	}
	return payloadResult("application/yaml", payload)
}
