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
	hidden     bool
	static     bool
	order      int
	algorithm  program.Algorithm
	multiplier int
	divisor    int
}

type chartState struct {
	templateID string
	chartID    string
	meta       program.ChartMeta
	lifecycle  program.LifecyclePolicy
	labels     *chartLabelAccumulator
	values     map[string]metrix.SampleValue
	dimensions map[string]dimensionState
	dynamicSet map[string]struct{}
}

type chartLabelAccumulator struct {
	mode        program.PromotionMode
	promoteKeys map[string]struct{}
	excluded    map[string]struct{}
	instance    map[string]string
	selected    map[string]string
	initialized bool
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
	// NOTE: reader used for planning should expose all retained series identity
	// membership (typically ReadRaw() at JobV2 seam). retainSeries() below uses
	// this set to keep/drop route-cache entries in sync with metrix retention.
	aliveSeries := make(map[metrix.SeriesID]struct{})
	seenInfer := make(map[string]struct{})
	chartsByID := make(map[string]*chartState)
	chartOwners := make(map[string]string, len(e.state.materialized.charts))
	for chartID, matChart := range e.state.materialized.charts {
		chartOwners[chartID] = matChart.templateID
	}
	index := e.state.matchIndex
	if index.chartsByID == nil {
		index = buildMatchIndex(prog.Charts())
		e.state.matchIndex = index
	}
	var firstErr error

	flat.ForEachSeriesIdentity(func(identity metrix.SeriesIdentity, meta metrix.SeriesMeta, name string, labels metrix.LabelView, v metrix.SampleValue) {
		aliveSeries[identity.ID] = struct{}{}

		if firstErr != nil {
			return
		}

		if meta.LastSeenSuccessSeq != collectMeta.LastSuccessSeq {
			return
		}

		routes, hit, err := e.resolveSeriesRoutes(
			cache,
			identity,
			name,
			labels,
			meta,
			index,
			prog.Revision(),
		)
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
			ownerTemplateID, ownerExists := chartOwners[route.ChartID]
			if ownerExists && ownerTemplateID != route.ChartTemplateID {
				// Cross-template rendered-id collision.
				// Existing owner keeps chart-id ownership.
				continue
			}
			if !ownerExists {
				chartOwners[route.ChartID] = route.ChartTemplateID
			}

			chart, ok := index.chartsByID[route.ChartTemplateID]
			if !ok {
				firstErr = fmt.Errorf("chartengine: route references unknown chart template %q", route.ChartTemplateID)
				return
			}

			cs, exists := chartsByID[route.ChartID]
			if !exists {
				cs = &chartState{
					templateID: route.ChartTemplateID,
					chartID:    route.ChartID,
					meta:       chart.Meta,
					lifecycle:  chart.Lifecycle,
					labels:     newChartLabelAccumulator(chart),
					values:     make(map[string]metrix.SampleValue),
					dimensions: make(map[string]dimensionState),
					dynamicSet: make(map[string]struct{}),
				}
				chartsByID[route.ChartID] = cs
			}

			prevState, exists := cs.dimensions[route.DimensionName]
			if !exists {
				cs.dimensions[route.DimensionName] = dimensionState{
					hidden:     route.Hidden,
					static:     route.Static,
					order:      route.DimensionIndex,
					algorithm:  route.Algorithm,
					multiplier: 1,
					divisor:    1,
				}
				if !route.Static {
					cs.dynamicSet[route.DimensionName] = struct{}{}
				}
			} else if prevState.hidden != route.Hidden {
				// Keep first resolved hidden option for deterministic shape.
				// Conflict handling metrics/warnings are added in later planner phases.
			}

			cs.values[route.DimensionName] += v
			if err := cs.labels.observe(chart.Identity, labels, route.DimensionKeyLabel); err != nil {
				firstErr = err
				return
			}

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
	// Route-cache lifecycle follows metrix snapshot membership.
	cache.retainSeries(aliveSeries)

	removeByCapDims, removeByCapCharts := enforceLifecycleCaps(collectMeta.LastSuccessSeq, chartsByID, &e.state.materialized)
	for _, action := range removeByCapDims {
		out.Actions = append(out.Actions, action)
	}
	for _, action := range removeByCapCharts {
		out.Actions = append(out.Actions, action)
	}

	chartIDs := make([]string, 0, len(chartsByID))
	for chartID, cs := range chartsByID {
		if len(cs.dimensions) == 0 {
			continue
		}
		chartIDs = append(chartIDs, chartID)
	}
	sort.Strings(chartIDs)

	for _, chartID := range chartIDs {
		cs := chartsByID[chartID]
		matChart, chartCreated := e.state.materialized.ensureChart(cs.chartID, cs.templateID, cs.meta, cs.lifecycle)
		if chartCreated {
			chartLabels, err := cs.labels.materialize()
			if err != nil {
				return Plan{}, err
			}
			out.Actions = append(out.Actions, CreateChartAction{
				ChartTemplateID: cs.templateID,
				ChartID:         cs.chartID,
				Meta:            cs.meta,
				Labels:          chartLabels,
			})
		}
		matChart.lastSeenSuccessSeq = collectMeta.LastSuccessSeq

		observedNames := orderedDimensionNamesFromState(cs.dimensions, cs.dynamicSet)
		for _, name := range observedNames {
			d := cs.dimensions[name]
			matDim, dimCreated := matChart.ensureDimension(name, d)
			if dimCreated {
				out.Actions = append(out.Actions, CreateDimensionAction{
					ChartID:    cs.chartID,
					ChartMeta:  cs.meta,
					Name:       name,
					Hidden:     d.hidden,
					Algorithm:  d.algorithm,
					Multiplier: d.multiplier,
					Divisor:    d.divisor,
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
			ChartID: cs.chartID,
			Values:  values,
		})
	}

	removeDims, removeCharts := collectExpiryRemovals(collectMeta.LastSuccessSeq, &e.state.materialized)
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

func resolveDimensionName(dim program.Dimension, metricName string, labels metrix.LabelView, meta metrix.SeriesMeta) (string, string, bool, error) {
	if dim.InferNameFromSeriesMeta {
		labelKey, ok, err := inferDimensionLabelKey(metricName, meta)
		if err != nil {
			return "", "", false, err
		}
		if !ok {
			return "", "", false, nil
		}
		value, ok := labels.Get(labelKey)
		if !ok || strings.TrimSpace(value) == "" {
			return "", "", false, nil
		}
		return value, labelKey, true, nil
	}

	if dim.NameFromLabel != "" {
		value, ok := labels.Get(dim.NameFromLabel)
		if !ok || strings.TrimSpace(value) == "" {
			return "", "", false, nil
		}
		return value, dim.NameFromLabel, true, nil
	}

	if dim.NameTemplate.Raw != "" {
		// Full placeholder template rendering is added in a later planner step.
		if dim.NameTemplate.IsDynamic() {
			return "", "", false, fmt.Errorf("chartengine: dynamic name template rendering is not implemented yet")
		}
		return dim.NameTemplate.Raw, "", true, nil
	}

	return "", "", false, nil
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
			hidden:     dim.hidden,
			static:     dim.static,
			order:      dim.order,
			algorithm:  dim.algorithm,
			multiplier: dim.multiplier,
			divisor:    dim.divisor,
		}
		if !dim.static {
			dynamicSet[name] = struct{}{}
		}
	}
	return orderedDimensionNamesFromState(dimState, dynamicSet)
}

func enforceLifecycleCaps(
	currentSuccessSeq uint64,
	chartsByID map[string]*chartState,
	state *materializedState,
) ([]RemoveDimensionAction, []RemoveChartAction) {
	if len(chartsByID) == 0 || state == nil {
		return nil, nil
	}

	observedByTemplate := make(map[string][]string)
	for chartID, cs := range chartsByID {
		observedByTemplate[cs.templateID] = append(observedByTemplate[cs.templateID], chartID)
	}
	for templateID := range observedByTemplate {
		sort.Strings(observedByTemplate[templateID])
	}

	existingByTemplate := make(map[string][]string)
	for chartID, matChart := range state.charts {
		existingByTemplate[matChart.templateID] = append(existingByTemplate[matChart.templateID], chartID)
	}
	for templateID := range existingByTemplate {
		sort.Strings(existingByTemplate[templateID])
	}

	removeCharts := make([]RemoveChartAction, 0)
	removeDims := make([]RemoveDimensionAction, 0)

	templateIDs := make([]string, 0, len(observedByTemplate))
	for templateID := range observedByTemplate {
		templateIDs = append(templateIDs, templateID)
	}
	sort.Strings(templateIDs)

	for _, templateID := range templateIDs {
		observedIDs := observedByTemplate[templateID]
		var lifecycle program.LifecyclePolicy
		if len(observedIDs) > 0 {
			lifecycle = chartsByID[observedIDs[0]].lifecycle
		}
		maxInstances := lifecycle.MaxInstances
		if maxInstances <= 0 {
			continue
		}

		existingIDs := existingByTemplate[templateID]
		existingSet := make(map[string]struct{}, len(existingIDs))
		for _, id := range existingIDs {
			existingSet[id] = struct{}{}
		}
		newObserved := make([]string, 0, len(observedIDs))
		for _, id := range observedIDs {
			if _, ok := existingSet[id]; !ok {
				newObserved = append(newObserved, id)
			}
		}

		total := len(existingIDs) + len(newObserved)
		if total <= maxInstances {
			continue
		}
		overflow := total - maxInstances

		type chartCandidate struct {
			chartID  string
			lastSeen uint64
		}
		candidates := make([]chartCandidate, 0, len(existingIDs))
		for _, chartID := range existingIDs {
			matChart := state.charts[chartID]
			if matChart == nil {
				continue
			}
			// Never evict chart instances seen in the current successful cycle.
			if matChart.lastSeenSuccessSeq == currentSuccessSeq {
				continue
			}
			if _, seen := chartsByID[chartID]; seen {
				continue
			}
			candidates = append(candidates, chartCandidate{
				chartID:  chartID,
				lastSeen: matChart.lastSeenSuccessSeq,
			})
		}
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].lastSeen != candidates[j].lastSeen {
				return candidates[i].lastSeen < candidates[j].lastSeen
			}
			return candidates[i].chartID < candidates[j].chartID
		})

		for i := 0; i < len(candidates) && overflow > 0; i++ {
			chartID := candidates[i].chartID
			matChart := state.charts[chartID]
			if matChart == nil {
				continue
			}
			removeCharts = append(removeCharts, RemoveChartAction{
				ChartID: chartID,
				Meta:    matChart.meta,
			})
			delete(state.charts, chartID)
			overflow--
		}

		if overflow > 0 {
			// Drop new chart instances deterministically when no eviction candidates remain.
			for i := len(newObserved) - 1; i >= 0 && overflow > 0; i-- {
				delete(chartsByID, newObserved[i])
				overflow--
			}
		}
	}

	// Per-chart dimension caps: evict least-recently-seen inactive dims first, then drop new dims.
	chartIDs := make([]string, 0, len(chartsByID))
	for chartID := range chartsByID {
		chartIDs = append(chartIDs, chartID)
	}
	sort.Strings(chartIDs)
	for _, chartID := range chartIDs {
		cs := chartsByID[chartID]
		maxDims := cs.lifecycle.Dimensions.MaxDims
		if maxDims <= 0 {
			continue
		}
		matChart := state.charts[chartID]
		existingCount := 0
		if matChart != nil {
			existingCount = len(matChart.dimensions)
		}

		newNames := make([]string, 0, len(cs.dimensions))
		for name := range cs.dimensions {
			if matChart == nil || matChart.dimensions[name] == nil {
				newNames = append(newNames, name)
			}
		}
		sort.Strings(newNames)

		total := existingCount + len(newNames)
		if total <= maxDims {
			continue
		}
		overflow := total - maxDims

		if matChart != nil && overflow > 0 {
			type dimCandidate struct {
				name     string
				lastSeen uint64
			}
			candidates := make([]dimCandidate, 0, len(matChart.dimensions))
			for name, dim := range matChart.dimensions {
				if dim.lastSeenSuccessSeq == currentSuccessSeq {
					continue
				}
				if _, seen := cs.dimensions[name]; seen {
					continue
				}
				candidates = append(candidates, dimCandidate{
					name:     name,
					lastSeen: dim.lastSeenSuccessSeq,
				})
			}
			sort.Slice(candidates, func(i, j int) bool {
				if candidates[i].lastSeen != candidates[j].lastSeen {
					return candidates[i].lastSeen < candidates[j].lastSeen
				}
				return candidates[i].name < candidates[j].name
			})
			for i := 0; i < len(candidates) && overflow > 0; i++ {
				name := candidates[i].name
				dim := matChart.dimensions[name]
				if dim == nil {
					continue
				}
				removeDims = append(removeDims, RemoveDimensionAction{
					ChartID:    chartID,
					ChartMeta:  matChart.meta,
					Name:       name,
					Hidden:     dim.hidden,
					Algorithm:  dim.algorithm,
					Multiplier: dim.multiplier,
					Divisor:    dim.divisor,
				})
				delete(matChart.dimensions, name)
				overflow--
			}
		}

		if overflow > 0 {
			orderedObserved := orderedDimensionNamesFromState(cs.dimensions, cs.dynamicSet)
			// Drop newest/least-priority candidates first (end of deterministic order).
			for i := len(orderedObserved) - 1; i >= 0 && overflow > 0; i-- {
				name := orderedObserved[i]
				if matChart != nil && matChart.dimensions[name] != nil {
					continue
				}
				delete(cs.dimensions, name)
				delete(cs.values, name)
				delete(cs.dynamicSet, name)
				overflow--
			}
		}
	}

	return removeDims, removeCharts
}

func collectExpiryRemovals(
	currentSuccessSeq uint64,
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
		if shouldExpire(matChart.lastSeenSuccessSeq, currentSuccessSeq, matChart.lifecycle.ExpireAfterCycles) {
			toRemoveChart[chartID] = struct{}{}
		}
	}

	removeDims := make([]RemoveDimensionAction, 0)
	for _, chartID := range chartIDs {
		if _, removed := toRemoveChart[chartID]; removed {
			continue
		}
		matChart := state.charts[chartID]
		expireAfter := matChart.lifecycle.Dimensions.ExpireAfterCycles
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
				ChartID:    chartID,
				ChartMeta:  matChart.meta,
				Name:       name,
				Hidden:     dim.hidden,
				Algorithm:  dim.algorithm,
				Multiplier: dim.multiplier,
				Divisor:    dim.divisor,
			})
			delete(matChart.dimensions, name)
		}
	}

	removeCharts := make([]RemoveChartAction, 0, len(toRemoveChart))
	for _, chartID := range chartIDs {
		if _, removed := toRemoveChart[chartID]; !removed {
			continue
		}
		matChart := state.charts[chartID]
		if matChart == nil {
			continue
		}
		removeCharts = append(removeCharts, RemoveChartAction{
			ChartID: chartID,
			Meta:    matChart.meta,
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

func newChartLabelAccumulator(chart program.Chart) *chartLabelAccumulator {
	acc := &chartLabelAccumulator{
		mode:        chart.Labels.Mode,
		promoteKeys: make(map[string]struct{}, len(chart.Labels.PromoteKeys)),
		excluded: make(map[string]struct{},
			len(chart.Labels.Exclusions.SelectorConstrainedKeys)+len(chart.Labels.Exclusions.DimensionKeyLabels)),
		instance: make(map[string]string),
		selected: make(map[string]string),
	}
	for _, key := range chart.Labels.PromoteKeys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		acc.promoteKeys[key] = struct{}{}
	}
	for _, key := range chart.Labels.Exclusions.SelectorConstrainedKeys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		acc.excluded[key] = struct{}{}
	}
	for _, key := range chart.Labels.Exclusions.DimensionKeyLabels {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		acc.excluded[key] = struct{}{}
	}
	return acc
}

func (a *chartLabelAccumulator) observe(identity program.ChartIdentity, labels metrix.LabelView, dimensionKeyLabel string) error {
	if a == nil || labels.Len() == 0 {
		return nil
	}

	if key := strings.TrimSpace(dimensionKeyLabel); key != "" {
		a.excluded[key] = struct{}{}
	}

	instanceLabels, ok, err := resolveInstanceLabelValues(identity, labelViewAccessor{view: labels})
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	instanceSet := make(map[string]struct{}, len(instanceLabels))
	for _, label := range instanceLabels {
		instanceSet[label.Key] = struct{}{}
		if _, exists := a.instance[label.Key]; !exists {
			a.instance[label.Key] = label.Value
		}
	}

	eligible := make(map[string]string)
	switch a.mode {
	case program.PromotionModeExplicitIntersection:
		for key := range a.promoteKeys {
			if _, excluded := a.excluded[key]; excluded {
				continue
			}
			if _, identityKey := instanceSet[key]; identityKey {
				continue
			}
			value, ok := labels.Get(key)
			if !ok {
				continue
			}
			eligible[key] = value
		}
	default:
		labels.Range(func(key, value string) bool {
			if _, excluded := a.excluded[key]; excluded {
				return true
			}
			if _, identityKey := instanceSet[key]; identityKey {
				return true
			}
			eligible[key] = value
			return true
		})
	}

	if !a.initialized {
		for key, value := range eligible {
			a.selected[key] = value
		}
		a.initialized = true
		return nil
	}

	for key, value := range a.selected {
		next, ok := eligible[key]
		if !ok || next != value {
			delete(a.selected, key)
		}
	}
	return nil
}

func (a *chartLabelAccumulator) materialize() (map[string]string, error) {
	if a == nil {
		return nil, nil
	}
	out := make(map[string]string, len(a.instance)+len(a.selected))
	for key, value := range a.instance {
		if strings.TrimSpace(key) == "" {
			continue
		}
		out[key] = value
	}
	for key, value := range a.selected {
		if strings.TrimSpace(key) == "" {
			continue
		}
		out[key] = value
	}
	delete(out, "_collect_job")
	return out, nil
}
