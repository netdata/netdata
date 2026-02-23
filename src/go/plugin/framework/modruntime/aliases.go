// SPDX-License-Identifier: GPL-3.0-or-later

package modruntime

import "github.com/netdata/netdata/go/plugins/plugin/framework/module"

type (
	Base                 = module.Base
	Chart                = module.Chart
	Charts               = module.Charts
	Dim                  = module.Dim
	Dims                 = module.Dims
	MetricCollector      = module.MetricCollector
	Module               = module.Module
	ModuleV2             = module.ModuleV2
	ModuleV2EnginePolicy = module.ModuleV2EnginePolicy
	MockConfiguration    = module.MockConfiguration
	MockModule           = module.MockModule
	Opts                 = module.Opts
	Var                  = module.Var
	Vars                 = module.Vars
)

const (
	Absolute         = module.Absolute
	Incremental      = module.Incremental
	LabelSourceAuto  = module.LabelSourceAuto
	LabelSourceConf  = module.LabelSourceConf
	MockConfigSchema = module.MockConfigSchema
)
