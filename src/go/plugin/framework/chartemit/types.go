// SPDX-License-Identifier: GPL-3.0-or-later

package chartemit

import (
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
)

// HostScope controls which Netdata host context one emitted chart batch uses.
//
// Nil means "explicit global context". Define is optional and caller-driven:
// callers decide when a host still needs HOST_DEFINE before selecting it.
type HostScope struct {
	GUID   string
	Define *netdataapi.HostInfo
}

// EmitEnv carries runtime context for translating engine actions to Netdata wire.
type EmitEnv struct {
	TypeID      string
	UpdateEvery int
	Plugin      string
	Module      string
	JobName     string
	JobLabels   map[string]string
	HostScope   *HostScope
	MSSinceLast int
}

// Plan is one emission plan from chartengine planner.
type Plan = chartengine.Plan

// Action aliases keep chartemit independent from planner internals while
// reusing the action model.
type (
	EngineAction          = chartengine.EngineAction
	CreateChartAction     = chartengine.CreateChartAction
	CreateDimensionAction = chartengine.CreateDimensionAction
	UpdateChartAction     = chartengine.UpdateChartAction
	UpdateDimensionValue  = chartengine.UpdateDimensionValue
	RemoveDimensionAction = chartengine.RemoveDimensionAction
	RemoveChartAction     = chartengine.RemoveChartAction
)
