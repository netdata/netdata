// SPDX-License-Identifier: GPL-3.0-or-later

package program

import "fmt"

// Algorithm defines how Netdata interprets values on wire.
type Algorithm string

const (
	// AlgorithmAbsolute sends direct values.
	AlgorithmAbsolute Algorithm = "absolute"
	// AlgorithmIncremental sends monotonic totals (Netdata computes rates/deltas).
	AlgorithmIncremental Algorithm = "incremental"
)

// ChartType controls chart visualization.
type ChartType string

const (
	ChartTypeLine    ChartType = "line"
	ChartTypeArea    ChartType = "area"
	ChartTypeStacked ChartType = "stacked"
	ChartTypeHeatmap ChartType = "heatmap"
)

// ReduceOp defines aggregation for series collisions mapped to one dimension.
type ReduceOp string

const (
	// ReduceSum is the phase-1 collision reduction rule.
	ReduceSum ReduceOp = "sum"
)

// Chart is one compiled chart template in immutable program IR.
type Chart struct {
	// TemplateID is compiler-assigned stable ID inside one Program revision.
	TemplateID string

	Meta      ChartMeta
	Identity  ChartIdentity
	Labels    LabelPolicy
	Lifecycle LifecyclePolicy

	// Dimensions are declaration-ordered templates.
	Dimensions []Dimension

	// CollisionReduce is currently fixed to ReduceSum in phase-1.
	CollisionReduce ReduceOp
}

// ChartMeta carries user-facing chart metadata after normalization/defaulting.
type ChartMeta struct {
	Title     string
	Family    string
	Context   string
	Units     string
	Algorithm Algorithm
	Type      ChartType
	Priority  int
}

// ChartIdentity describes how chart instances are derived.
//
// Phase-1 uses literal chart IDs and optional instance suffix derivation from
// configured labels.
type ChartIdentity struct {
	// IDTemplate is a normalized literal chart ID.
	IDTemplate Template

	// InstanceByLabels contains resolved explicit identity selectors (if used).
	InstanceByLabels []InstanceLabelSelector

	// ContextNamespace holds normalized namespace fragments that participate in
	// derived context/id building in namespace-based authoring mode.
	ContextNamespace []string

	// Static is true when identity renders one aggregated chart instance.
	Static bool
}

func validateChart(chart Chart) error {
	if chart.TemplateID == "" {
		return fmt.Errorf("template_id is required")
	}
	if chart.Meta.Context == "" {
		return fmt.Errorf("context is required")
	}
	if chart.Meta.Units == "" {
		return fmt.Errorf("units is required")
	}
	if chart.Meta.Algorithm != AlgorithmAbsolute && chart.Meta.Algorithm != AlgorithmIncremental {
		return fmt.Errorf("invalid algorithm %q", chart.Meta.Algorithm)
	}
	if chart.CollisionReduce == "" {
		return fmt.Errorf("collision reduce op is required")
	}
	if len(chart.Dimensions) == 0 {
		return fmt.Errorf("at least one dimension is required")
	}
	for i, dim := range chart.Dimensions {
		if err := validateDimension(dim); err != nil {
			return fmt.Errorf("dimension[%d]: %w", i, err)
		}
	}
	return nil
}

func (c Chart) clone() Chart {
	out := c
	out.Meta = c.Meta
	out.Identity = c.Identity.clone()
	out.Labels = c.Labels.clone()
	out.Lifecycle = c.Lifecycle.clone()

	out.Dimensions = make([]Dimension, 0, len(c.Dimensions))
	for _, dim := range c.Dimensions {
		out.Dimensions = append(out.Dimensions, dim.clone())
	}
	return out
}

func (i ChartIdentity) clone() ChartIdentity {
	out := i
	out.IDTemplate = i.IDTemplate.clone()

	out.InstanceByLabels = make([]InstanceLabelSelector, 0, len(i.InstanceByLabels))
	for _, selector := range i.InstanceByLabels {
		out.InstanceByLabels = append(out.InstanceByLabels, selector.clone())
	}
	out.ContextNamespace = append([]string(nil), i.ContextNamespace...)
	return out
}
