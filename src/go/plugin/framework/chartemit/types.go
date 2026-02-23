// SPDX-License-Identifier: GPL-3.0-or-later

package chartemit

import (
	chartengine2 "github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
)

// EmitEnv carries runtime context for translating engine actions to Netdata wire.
type EmitEnv struct {
	TypeID      string
	UpdateEvery int
	Plugin      string
	Module      string
	JobName     string
	JobLabels   map[string]string
	MSSinceLast int
}

// Plan is one emission plan from chartengine planner.
type Plan = chartengine2.Plan

// Action aliases keep chartemit independent from planner internals while
// reusing the action model.
type (
	EngineAction          = chartengine2.EngineAction
	CreateChartAction     = chartengine2.CreateChartAction
	CreateDimensionAction = chartengine2.CreateDimensionAction
	UpdateChartAction     = chartengine2.UpdateChartAction
	UpdateDimensionValue  = chartengine2.UpdateDimensionValue
	RemoveDimensionAction = chartengine2.RemoveDimensionAction
	RemoveChartAction     = chartengine2.RemoveChartAction
)
