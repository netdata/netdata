// SPDX-License-Identifier: GPL-3.0-or-later

package chartemit

import (
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
)

// HostScope controls which Netdata host context one emitted chart batch uses.
//
// Nil means "explicit global context". Host definitions belong to hostoutput.
type HostScope struct {
	GUID string
}

// EmitEnv carries runtime context for translating engine actions to Netdata wire.
//
// CollectedAt is when the plan's samples were collected. Every chart update ends
// with it (`END <seconds> <microseconds> 1`), so the Agent places each sample at its
// collection time, whatever happened to the chart's earlier samples. A plan with
// chart updates requires it.
type EmitEnv struct {
	TypeID      string
	UpdateEvery int
	Plugin      string
	Module      string
	JobName     string
	JobLabels   map[string]string
	HostScope   *HostScope
	StoreFirst  bool
	CollectedAt netdataapi.SampleTime
}

// Plan is one emission plan from chartengine planner.
type Plan = chartengine.Plan

// Action aliases keep chartemit independent from planner internals while
// reusing the action model.
type (
	EngineAction            = chartengine.EngineAction
	CreateChartAction       = chartengine.CreateChartAction
	CreateDimensionAction   = chartengine.CreateDimensionAction
	UpdateChartLabelsAction = chartengine.UpdateChartLabelsAction
	UpdateChartAction       = chartengine.UpdateChartAction
	UpdateDimensionValue    = chartengine.UpdateDimensionValue
	UpdateVariableValue     = chartengine.UpdateVariableValue
	RemoveDimensionAction   = chartengine.RemoveDimensionAction
	RemoveChartAction       = chartengine.RemoveChartAction
)
