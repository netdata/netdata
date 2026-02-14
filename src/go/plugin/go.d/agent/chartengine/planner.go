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

// Plan is the deterministic planner output consumed by chartemit.
//
// Current scope:
//   - create/update/remove actions for chart and dimension lifecycle,
//   - inferred dynamic dimension names resolved from flattened metric metadata.
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

type planBuildContext struct {
	out         *Plan
	collectMeta metrix.CollectMeta
	prog        *program.Program
	cache       *routeCache
	index       matchIndex
	flat        metrix.Reader

	aliveSeries map[metrix.SeriesID]struct{}
	seenInfer   map[string]struct{}
	chartsByID  map[string]*chartState
	chartOwners map[string]string
}

// BuildPlan builds a minimal plan snapshot from the provided reader.
//
// Current scope:
//   - template routes with cache,
//   - optional unmatched-series autogen fallback,
//   - runtime-inferred dimension names from flattened metadata.
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

	ctx, err := e.preparePlanBuildContext(reader, &out, collectMeta)
	if err != nil {
		return Plan{}, err
	}
	if err := e.scanPlanSeries(ctx); err != nil {
		return Plan{}, err
	}

	// Route-cache lifecycle follows metrix snapshot membership.
	ctx.cache.RetainSeries(ctx.aliveSeries)

	removeByCapDims, removeByCapCharts := enforceLifecycleCaps(ctx.collectMeta.LastSuccessSeq, ctx.chartsByID, &e.state.materialized)
	for _, action := range removeByCapDims {
		out.Actions = append(out.Actions, action)
	}
	for _, action := range removeByCapCharts {
		out.Actions = append(out.Actions, action)
	}
	if err := e.materializePlanCharts(ctx); err != nil {
		return Plan{}, err
	}
	removeDims, removeCharts := collectExpiryRemovals(ctx.collectMeta.LastSuccessSeq, &e.state.materialized)
	for _, action := range removeDims {
		out.Actions = append(out.Actions, action)
	}
	for _, action := range removeCharts {
		out.Actions = append(out.Actions, action)
	}
	sortInferredDimensions(out.InferredDimensions)
	return out, nil
}

func (e *Engine) preparePlanBuildContext(
	reader metrix.Reader,
	out *Plan,
	collectMeta metrix.CollectMeta,
) (*planBuildContext, error) {
	prog := e.state.program
	if prog == nil {
		return nil, fmt.Errorf("chartengine: no compiled program loaded")
	}
	cache := e.state.routeCache
	if cache == nil {
		cache = newRouteCache()
		e.state.routeCache = cache
	}
	if e.state.materialized.charts == nil {
		e.state.materialized = newMaterializedState()
	}
	index := e.state.matchIndex
	if index.chartsByID == nil {
		index = buildMatchIndex(prog.Charts())
		e.state.matchIndex = index
	}
	chartOwners := make(map[string]string, len(e.state.materialized.charts))
	for chartID, matChart := range e.state.materialized.charts {
		chartOwners[chartID] = matChart.templateID
	}
	return &planBuildContext{
		out:         out,
		collectMeta: collectMeta,
		prog:        prog,
		cache:       cache,
		index:       index,
		flat:        reader.Flatten(),
		aliveSeries: make(map[metrix.SeriesID]struct{}),
		seenInfer:   make(map[string]struct{}),
		chartsByID:  make(map[string]*chartState),
		chartOwners: chartOwners,
	}, nil
}

func (e *Engine) scanPlanSeries(ctx *planBuildContext) error {
	var firstErr error
	ctx.flat.ForEachSeriesIdentity(func(identity metrix.SeriesIdentity, meta metrix.SeriesMeta, name string, labels metrix.LabelView, v metrix.SampleValue) {
		ctx.aliveSeries[identity.ID] = struct{}{}
		if firstErr != nil {
			return
		}
		if meta.LastSeenSuccessSeq != ctx.collectMeta.LastSuccessSeq {
			return
		}

		routes, hit, err := e.resolveSeriesRoutes(
			ctx.cache,
			identity,
			name,
			labels,
			meta,
			ctx.index,
			ctx.prog.Revision(),
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
		if len(routes) == 0 {
			autoRoutes, ok, err := e.resolveAutogenRoute(name, labels, meta)
			if err != nil {
				firstErr = err
				return
			}
			if ok {
				routes = autoRoutes
			}
		}

		for _, route := range routes {
			if err := ctx.accumulateRoute(ctx.index, route, labels, v); err != nil {
				firstErr = err
				return
			}
		}
	})
	return firstErr
}

func (ctx *planBuildContext) accumulateRoute(
	index matchIndex,
	route routeBinding,
	labels metrix.LabelView,
	value metrix.SampleValue,
) error {
	ownerTemplateID, ownerExists := ctx.chartOwners[route.ChartID]
	if ownerExists && ownerTemplateID != route.ChartTemplateID {
		if !route.Autogen && isAutogenTemplateID(ownerTemplateID) {
			// Template wins over autogen on chart-id collision.
			ctx.chartOwners[route.ChartID] = route.ChartTemplateID
			delete(ctx.chartsByID, route.ChartID)
		} else {
			// Cross-template rendered-id collision.
			// Existing owner keeps chart-id ownership.
			return nil
		}
	}
	if !ownerExists {
		ctx.chartOwners[route.ChartID] = route.ChartTemplateID
	}

	identity := program.ChartIdentity{}
	labelsAcc := newAutogenChartLabelAccumulator()
	if !route.Autogen {
		chart, ok := index.chartsByID[route.ChartTemplateID]
		if !ok {
			return fmt.Errorf("chartengine: route references unknown chart template %q", route.ChartTemplateID)
		}
		identity = chart.Identity
		labelsAcc = newChartLabelAccumulator(chart)
	}

	cs, exists := ctx.chartsByID[route.ChartID]
	if !exists {
		cs = &chartState{
			templateID: route.ChartTemplateID,
			chartID:    route.ChartID,
			meta:       route.Meta,
			lifecycle:  route.Lifecycle,
			labels:     labelsAcc,
			values:     make(map[string]metrix.SampleValue),
			dimensions: make(map[string]dimensionState),
			dynamicSet: make(map[string]struct{}),
		}
		ctx.chartsByID[route.ChartID] = cs
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
		// First-observed hidden flag wins; conflicting routes are ignored.
	}

	cs.values[route.DimensionName] += value
	if err := cs.labels.observe(identity, labels, route.DimensionKeyLabel); err != nil {
		return err
	}

	if route.Inferred {
		key := fmt.Sprintf("%s\xff%d\xff%s", route.ChartTemplateID, route.DimensionIndex, route.DimensionName)
		if _, exists := ctx.seenInfer[key]; !exists {
			ctx.seenInfer[key] = struct{}{}
			ctx.out.InferredDimensions = append(ctx.out.InferredDimensions, InferredDimension{
				ChartTemplateID: route.ChartTemplateID,
				DimensionIndex:  route.DimensionIndex,
				Name:            route.DimensionName,
			})
		}
	}
	return nil
}

func (e *Engine) materializePlanCharts(ctx *planBuildContext) error {
	chartIDs := make([]string, 0, len(ctx.chartsByID))
	for chartID, cs := range ctx.chartsByID {
		if len(cs.dimensions) == 0 {
			continue
		}
		chartIDs = append(chartIDs, chartID)
	}
	sort.Strings(chartIDs)

	for _, chartID := range chartIDs {
		cs := ctx.chartsByID[chartID]
		matChart, chartCreated := e.state.materialized.ensureChart(cs.chartID, cs.templateID, cs.meta, cs.lifecycle)
		if chartCreated {
			chartLabels, err := cs.labels.materialize()
			if err != nil {
				return err
			}
			ctx.out.Actions = append(ctx.out.Actions, CreateChartAction{
				ChartTemplateID: cs.templateID,
				ChartID:         cs.chartID,
				Meta:            cs.meta,
				Labels:          chartLabels,
			})
		}
		matChart.lastSeenSuccessSeq = ctx.collectMeta.LastSuccessSeq

		observedNames := orderedDimensionNamesFromState(cs.dimensions, cs.dynamicSet)
		for _, name := range observedNames {
			d := cs.dimensions[name]
			matDim, dimCreated := matChart.ensureDimension(name, d)
			if dimCreated {
				ctx.out.Actions = append(ctx.out.Actions, CreateDimensionAction{
					ChartID:    cs.chartID,
					ChartMeta:  cs.meta,
					Name:       name,
					Hidden:     d.hidden,
					Algorithm:  d.algorithm,
					Multiplier: d.multiplier,
					Divisor:    d.divisor,
				})
			}
			matDim.lastSeenSuccessSeq = ctx.collectMeta.LastSuccessSeq
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
		ctx.out.Actions = append(ctx.out.Actions, UpdateChartAction{
			ChartID: cs.chartID,
			Values:  values,
		})
	}
	return nil
}

func sortInferredDimensions(in []InferredDimension) {
	sort.Slice(in, func(i, j int) bool {
		lhs := in[i]
		rhs := in[j]
		if lhs.ChartTemplateID != rhs.ChartTemplateID {
			return lhs.ChartTemplateID < rhs.ChartTemplateID
		}
		if lhs.DimensionIndex != rhs.DimensionIndex {
			return lhs.DimensionIndex < rhs.DimensionIndex
		}
		return lhs.Name < rhs.Name
	})
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
		// Dynamic name templates are not supported in phase-1 syntax.
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

func isAutogenTemplateID(templateID string) bool {
	return strings.HasPrefix(templateID, autogenTemplatePrefix)
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

func newAutogenChartLabelAccumulator() *chartLabelAccumulator {
	return &chartLabelAccumulator{
		mode:        program.PromotionModeAutoIntersection,
		promoteKeys: make(map[string]struct{}),
		excluded:    make(map[string]struct{}),
		instance:    make(map[string]string),
		selected:    make(map[string]string),
	}
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
