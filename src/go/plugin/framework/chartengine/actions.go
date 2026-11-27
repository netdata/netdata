// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
)

// ActionKind defines deterministic action ordering groups.
type ActionKind uint8

const (
	ActionCreateChart ActionKind = iota + 1
	ActionCreateDimension
	ActionUpdateChartLabels
	ActionUpdateChart
	ActionRemoveDimension
	ActionRemoveChart
)

// EngineAction is one planner-emitted operation.
type EngineAction interface {
	Kind() ActionKind
}

// CreateChartAction materializes a chart instance before updates.
type CreateChartAction struct {
	ChartTemplateID string
	ChartID         string
	Meta            program.ChartMeta
	Labels          map[string]string
	// Variables are the chart variable names defined as unknown (nan) right after
	// the chart's dimensions: the template's declared names, then any name a
	// previous definition of the same chart declared and this one does not. The
	// Agent keeps chart variables across a re-definition and has no delete, so
	// nan is the only reset.
	Variables []string
}

func (a CreateChartAction) Kind() ActionKind { return ActionCreateChart }

// CreateDimensionAction materializes one dimension for a chart instance.
type CreateDimensionAction struct {
	ChartID    string
	ChartMeta  program.ChartMeta
	Name       string
	Hidden     bool
	Float      bool
	Algorithm  program.Algorithm
	Multiplier int
	Divisor    int
}

func (a CreateDimensionAction) Kind() ActionKind { return ActionCreateDimension }

// UpdateChartLabelsAction replaces the non-identity labels of one materialized chart.
type UpdateChartLabelsAction struct {
	ChartID string
	Meta    program.ChartMeta
	Labels  map[string]string
}

func (a UpdateChartLabelsAction) Kind() ActionKind { return ActionUpdateChartLabels }

// UpdateDimensionValue carries one resolved value for UPDATE emission.
type UpdateDimensionValue struct {
	Name    string
	IsEmpty bool
	IsFloat bool
	Int64   int64
	Float64 float64
}

// UpdateVariableValue carries one chart variable value for UPDATE emission.
// Value may be non-finite (unknown).
type UpdateVariableValue struct {
	Name  string
	Value float64
}

// UpdateChartAction updates one chart instance values in deterministic order.
// Variables lists, in declaration order, the chart variables that received at
// least one series in this build; an absent variable keeps its last value.
type UpdateChartAction struct {
	ChartID   string
	Values    []UpdateDimensionValue
	Variables []UpdateVariableValue
}

func (a UpdateChartAction) Kind() ActionKind { return ActionUpdateChart }

// RemoveDimensionAction marks one dimension obsolete.
type RemoveDimensionAction struct {
	ChartID    string
	ChartMeta  program.ChartMeta
	Name       string
	Hidden     bool
	Float      bool
	Algorithm  program.Algorithm
	Multiplier int
	Divisor    int
}

func (a RemoveDimensionAction) Kind() ActionKind { return ActionRemoveDimension }

// RemoveChartAction marks one chart obsolete.
type RemoveChartAction struct {
	ChartID string
	Meta    program.ChartMeta
}

func (a RemoveChartAction) Kind() ActionKind { return ActionRemoveChart }
