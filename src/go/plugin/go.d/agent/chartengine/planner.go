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
	Actions            []EngineAction
	InferredDimensions []InferredDimension
}

// InferredDimension is one resolved dynamic dimension name from planner input.
type InferredDimension struct {
	ChartTemplateID string
	DimensionIndex  int
	Name            string
}

type dimensionState struct {
	hidden bool
	static bool
	order  int
}

type chartState struct {
	meta       program.ChartMeta
	values     map[string]metrix.SampleValue
	dimensions map[string]dimensionState
	dynamicSet map[string]struct{}
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
	out := Plan{
		Actions:            make([]EngineAction, 0),
		InferredDimensions: make([]InferredDimension, 0),
	}
	collectMeta := reader.CollectMeta()
	// Failed attempt must not trigger lifecycle transitions.
	if collectMeta.LastAttemptStatus != metrix.CollectStatusSuccess {
		return out, nil
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	prog := e.state.program
	if prog == nil {
		return Plan{}, fmt.Errorf("chartengine: no compiled program loaded")
	}
	cache := e.state.routeCache
	if cache == nil {
		cache = newRouteCache()
		e.state.routeCache = cache
	}
	if e.state.materialized.charts == nil {
		e.state.materialized = newMaterializedState()
	}

	flat := reader.Flatten()
	seenInfer := make(map[string]struct{})
	chartsByTemplate := make(map[string]*chartState)
	index := buildMatchIndex(prog.Charts())
	var firstErr error

	flat.ForEachSeriesIdentity(func(identity metrix.SeriesIdentity, name string, labels metrix.LabelView, v metrix.SampleValue) {
		if firstErr != nil {
			return
		}

		labelsMap := labels.CloneMap()
		meta, ok := flat.SeriesMeta(name, labelsMap)
		if !ok {
			return
		}
		if meta.LastSeenSuccessSeq != collectMeta.LastSuccessSeq {
			return
		}

		routes, hit, err := e.resolveSeriesRoutes(cache, identity, name, labels, labelsMap, meta, index, prog.Revision())
		if err != nil {
			firstErr = err
			return
		}
		if hit {
			e.addRouteCacheHit()
		} else {
			e.addRouteCacheMiss()
		}

		for _, route := range routes {
			chart, ok := index.chartsByID[route.ChartTemplateID]
			if !ok {
				firstErr = fmt.Errorf("chartengine: route references unknown chart template %q", route.ChartTemplateID)
				return
			}

			cs, exists := chartsByTemplate[route.ChartTemplateID]
			if !exists {
				cs = &chartState{
					meta:       chart.Meta,
					values:     make(map[string]metrix.SampleValue),
					dimensions: make(map[string]dimensionState),
					dynamicSet: make(map[string]struct{}),
				}
				chartsByTemplate[route.ChartTemplateID] = cs
			}

			prevState, exists := cs.dimensions[route.DimensionName]
			if !exists {
				cs.dimensions[route.DimensionName] = dimensionState{
					hidden: route.Hidden,
					static: route.Static,
					order:  route.DimensionIndex,
				}
				if !route.Static {
					cs.dynamicSet[route.DimensionName] = struct{}{}
				}
			} else if prevState.hidden != route.Hidden {
				// Keep first resolved hidden option for deterministic shape.
				// Conflict handling metrics/warnings are added in later planner phases.
			}

			cs.values[route.DimensionName] += v

			if route.Inferred {
				key := fmt.Sprintf("%s\xff%d\xff%s", route.ChartTemplateID, route.DimensionIndex, route.DimensionName)
				if _, exists := seenInfer[key]; !exists {
					seenInfer[key] = struct{}{}
					out.InferredDimensions = append(out.InferredDimensions, InferredDimension{
						ChartTemplateID: route.ChartTemplateID,
						DimensionIndex:  route.DimensionIndex,
						Name:            route.DimensionName,
					})
				}
			}
		}
	})
	if firstErr != nil {
		return Plan{}, firstErr
	}

	templateIDs := make([]string, 0, len(chartsByTemplate))
	for templateID, cs := range chartsByTemplate {
		if len(cs.dimensions) == 0 {
			continue
		}
		templateIDs = append(templateIDs, templateID)
	}
	sort.Strings(templateIDs)

	for _, templateID := range templateIDs {
		cs := chartsByTemplate[templateID]
		matChart, chartCreated := e.state.materialized.ensureChart(templateID, cs.meta)
		if chartCreated {
			out.Actions = append(out.Actions, CreateChartAction{
				ChartTemplateID: templateID,
				Meta:            cs.meta,
			})
		}
		matChart.lastSeenSuccessSeq = collectMeta.LastSuccessSeq

		observedNames := orderedDimensionNamesFromState(cs.dimensions, cs.dynamicSet)
		for _, name := range observedNames {
			d := cs.dimensions[name]
			matDim, dimCreated := matChart.ensureDimension(name, d)
			if dimCreated {
				out.Actions = append(out.Actions, CreateDimensionAction{
					ChartTemplateID: templateID,
					Name:            name,
					Hidden:          d.hidden,
				})
			}
			matDim.lastSeenSuccessSeq = collectMeta.LastSuccessSeq
		}

		updateNames := orderedMaterializedDimensionNames(matChart.dimensions)
		values := make([]UpdateDimensionValue, 0, len(updateNames))
		for _, name := range updateNames {
			value, ok := cs.values[name]
			if ok {
				values = append(values, UpdateDimensionValue{
					Name:    name,
					IsFloat: true,
					Float64: value,
				})
				continue
			}
			values = append(values, UpdateDimensionValue{
				Name:    name,
				IsEmpty: true,
			})
		}
		out.Actions = append(out.Actions, UpdateChartAction{
			ChartTemplateID: templateID,
			Values:          values,
		})
	}

	removeDims, removeCharts := collectExpiryRemovals(collectMeta.LastSuccessSeq, index, &e.state.materialized)
	for _, action := range removeDims {
		out.Actions = append(out.Actions, action)
	}
	for _, action := range removeCharts {
		out.Actions = append(out.Actions, action)
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

func orderedMaterializedDimensionNames(dimensions map[string]*materializedDimensionState) []string {
	dimState := make(map[string]dimensionState, len(dimensions))
	dynamicSet := make(map[string]struct{})
	for name, dim := range dimensions {
		dimState[name] = dimensionState{
			hidden: dim.hidden,
			static: dim.static,
			order:  dim.order,
		}
		if !dim.static {
			dynamicSet[name] = struct{}{}
		}
	}
	return orderedDimensionNamesFromState(dimState, dynamicSet)
}

func collectExpiryRemovals(
	currentSuccessSeq uint64,
	index matchIndex,
	state *materializedState,
) ([]RemoveDimensionAction, []RemoveChartAction) {
	if state == nil || len(state.charts) == 0 {
		return nil, nil
	}

	chartIDs := make([]string, 0, len(state.charts))
	for chartID := range state.charts {
		chartIDs = append(chartIDs, chartID)
	}
	sort.Strings(chartIDs)

	toRemoveChart := make(map[string]struct{})
	for _, chartID := range chartIDs {
		matChart := state.charts[chartID]
		compiled, ok := index.chartsByID[chartID]
		if !ok {
			toRemoveChart[chartID] = struct{}{}
			continue
		}
		if shouldExpire(matChart.lastSeenSuccessSeq, currentSuccessSeq, compiled.Lifecycle.ExpireAfterCycles) {
			toRemoveChart[chartID] = struct{}{}
		}
	}

	removeDims := make([]RemoveDimensionAction, 0)
	for _, chartID := range chartIDs {
		if _, removed := toRemoveChart[chartID]; removed {
			continue
		}
		matChart := state.charts[chartID]
		compiled := index.chartsByID[chartID]
		expireAfter := compiled.Lifecycle.Dimensions.ExpireAfterCycles
		if expireAfter <= 0 || len(matChart.dimensions) == 0 {
			continue
		}

		dimNames := make([]string, 0, len(matChart.dimensions))
		for name := range matChart.dimensions {
			dimNames = append(dimNames, name)
		}
		sort.Strings(dimNames)
		for _, name := range dimNames {
			dim := matChart.dimensions[name]
			if !shouldExpire(dim.lastSeenSuccessSeq, currentSuccessSeq, expireAfter) {
				continue
			}
			removeDims = append(removeDims, RemoveDimensionAction{
				ChartTemplateID: chartID,
				Name:            name,
			})
			delete(matChart.dimensions, name)
		}
	}

	removeCharts := make([]RemoveChartAction, 0, len(toRemoveChart))
	for _, chartID := range chartIDs {
		if _, removed := toRemoveChart[chartID]; !removed {
			continue
		}
		removeCharts = append(removeCharts, RemoveChartAction{
			ChartTemplateID: chartID,
		})
		delete(state.charts, chartID)
	}
	return removeDims, removeCharts
}

func orderedDimensionNamesFromState(dimensions map[string]dimensionState, dynamicSet map[string]struct{}) []string {
	type staticEntry struct {
		name  string
		order int
	}
	staticEntries := make([]staticEntry, 0, len(dimensions))
	for name, state := range dimensions {
		if state.static {
			staticEntries = append(staticEntries, staticEntry{
				name:  name,
				order: state.order,
			})
		}
	}
	sort.Slice(staticEntries, func(i, j int) bool {
		if staticEntries[i].order != staticEntries[j].order {
			return staticEntries[i].order < staticEntries[j].order
		}
		return staticEntries[i].name < staticEntries[j].name
	})

	dynamicNames := mapKeysSorted(dynamicSet)
	out := make([]string, 0, len(staticEntries)+len(dynamicNames))
	for _, entry := range staticEntries {
		out = append(out, entry.name)
	}
	out = append(out, dynamicNames...)
	return out
}
