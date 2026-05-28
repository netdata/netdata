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

// UpdateDimensionValue carries one resolved value for UPDATE emission.
type UpdateDimensionValue struct {
	Name    string
	IsEmpty bool
	IsFloat bool
	Int64   int64
	Float64 float64
}

// UpdateChartAction updates one chart instance values in deterministic order.
type UpdateChartAction struct {
	ChartID string
	Values  []UpdateDimensionValue
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
