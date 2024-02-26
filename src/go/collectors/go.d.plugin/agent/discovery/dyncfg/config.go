// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"github.com/netdata/netdata/go/go.d.plugin/agent/confgroup"
	"github.com/netdata/netdata/go/go.d.plugin/agent/functions"
	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
)

type Config struct {
	Plugin               string
	API                  NetdataDyncfgAPI
	Functions            FunctionRegistry
	Modules              module.Registry
	ModuleConfigDefaults confgroup.Registry
}

type NetdataDyncfgAPI interface {
	DynCfgEnable(string) error
	DynCfgReset() error
	DyncCfgRegisterModule(string) error
	DynCfgRegisterJob(_, _, _ string) error
	DynCfgReportJobStatus(_, _, _, _ string) error
	FunctionResultSuccess(_, _, _ string) error
	FunctionResultReject(_, _, _ string) error
}

type FunctionRegistry interface {
	Register(name string, reg func(functions.Function))
}

func validateConfig(cfg Config) error {
	return nil
}
