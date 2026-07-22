// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"bytes"
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

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
