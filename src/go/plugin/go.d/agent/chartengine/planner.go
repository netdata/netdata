// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/chartengine/internal/program"
)

const (
	histogramBucketLabel = "le"
	summaryQuantileLabel = "quantile"
)

// Plan is the phase-3 planner output scaffold.
//
// Current scope includes inferred dynamic dimension names resolved from flattened
// metric metadata for dimensions compiled with InferNameFromSeriesMeta=true.
type Plan struct {
	InferredDimensions []InferredDimension
}

// InferredDimension is one resolved dynamic dimension name from planner input.
type InferredDimension struct {
	ChartTemplateID string
	DimensionIndex  int
	Name            string
}

// BuildPlan builds a minimal plan snapshot from the provided reader.
//
// This scaffold currently resolves runtime-inferred dimension names only.
func (e *Engine) BuildPlan(reader metrix.Reader) (Plan, error) {
	if e == nil {
		return Plan{}, fmt.Errorf("chartengine: nil engine")
	}
	if reader == nil {
		return Plan{}, fmt.Errorf("chartengine: nil metrics reader")
	}
	prog := e.Program()
	if prog == nil {
		return Plan{}, fmt.Errorf("chartengine: no compiled program loaded")
	}

	flat := reader.Flatten()
	out := Plan{
		InferredDimensions: make([]InferredDimension, 0),
	}
	seen := make(map[string]struct{})

	for _, chart := range prog.Charts() {
		for i, dim := range chart.Dimensions {
			if !dim.InferNameFromSeriesMeta {
				continue
			}

			var firstErr error
			flat.ForEachSeries(func(name string, labels metrix.LabelView, _ metrix.SampleValue) {
				if firstErr != nil {
					return
				}
				labelsMap := labels.CloneMap()
				if !dim.Selector.Matcher.Matches(name, labelsMap) {
					return
				}
				meta, ok := flat.SeriesMeta(name, labelsMap)
				if !ok {
					return
				}
				dimName, ok, err := resolveDimensionName(dim, name, labels, meta)
				if err != nil {
					firstErr = err
					return
				}
				if !ok {
					return
				}

				key := fmt.Sprintf("%s\xff%d\xff%s", chart.TemplateID, i, dimName)
				if _, exists := seen[key]; exists {
					return
				}
				seen[key] = struct{}{}
				out.InferredDimensions = append(out.InferredDimensions, InferredDimension{
					ChartTemplateID: chart.TemplateID,
					DimensionIndex:  i,
					Name:            dimName,
				})
			})

			if firstErr != nil {
				return Plan{}, firstErr
			}
		}
	}

	sort.Slice(out.InferredDimensions, func(i, j int) bool {
		lhs := out.InferredDimensions[i]
		rhs := out.InferredDimensions[j]
		if lhs.ChartTemplateID != rhs.ChartTemplateID {
			return lhs.ChartTemplateID < rhs.ChartTemplateID
		}
		if lhs.DimensionIndex != rhs.DimensionIndex {
			return lhs.DimensionIndex < rhs.DimensionIndex
		}
		return lhs.Name < rhs.Name
	})

	return out, nil
}

// BuildPlan is a package-level convenience wrapper around Engine.BuildPlan.
func BuildPlan(engine *Engine, reader metrix.Reader) (Plan, error) {
	if engine == nil {
		return Plan{}, fmt.Errorf("chartengine: nil engine")
	}
	return engine.BuildPlan(reader)
}

func resolveDimensionName(dim program.Dimension, metricName string, labels metrix.LabelView, meta metrix.SeriesMeta) (string, bool, error) {
	if dim.InferNameFromSeriesMeta {
		labelKey, ok, err := inferDimensionLabelKey(metricName, meta)
		if err != nil {
			return "", false, err
		}
		if !ok {
			return "", false, nil
		}
		value, ok := labels.Get(labelKey)
		if !ok || strings.TrimSpace(value) == "" {
			return "", false, nil
		}
		return value, true, nil
	}

	if dim.NameFromLabel != "" {
		value, ok := labels.Get(dim.NameFromLabel)
		if !ok || strings.TrimSpace(value) == "" {
			return "", false, nil
		}
		return value, true, nil
	}

	if dim.NameTemplate.Raw != "" {
		// Full placeholder template rendering is added in a later planner step.
		if dim.NameTemplate.IsDynamic() {
			return "", false, fmt.Errorf("chartengine: dynamic name template rendering is not implemented yet")
		}
		return dim.NameTemplate.Raw, true, nil
	}

	return "", false, nil
}

func inferDimensionLabelKey(metricName string, meta metrix.SeriesMeta) (string, bool, error) {
	switch meta.FlattenRole {
	case metrix.FlattenRoleHistogramBucket:
		return histogramBucketLabel, true, nil
	case metrix.FlattenRoleSummaryQuantile:
		return summaryQuantileLabel, true, nil
	case metrix.FlattenRoleStateSetState:
		if strings.TrimSpace(metricName) == "" {
			return "", false, fmt.Errorf("chartengine: stateset inference requires metric family name")
		}
		return metricName, true, nil
	case metrix.FlattenRoleHistogramCount,
		metrix.FlattenRoleHistogramSum,
		metrix.FlattenRoleSummaryCount,
		metrix.FlattenRoleSummarySum:
		return "", false, nil
	case metrix.FlattenRoleNone:
		return "", false, fmt.Errorf("chartengine: cannot infer dimension label key from non-flattened series metadata")
	default:
		return "", false, fmt.Errorf("chartengine: unsupported flatten role %d for runtime dimension inference", meta.FlattenRole)
	}
}
