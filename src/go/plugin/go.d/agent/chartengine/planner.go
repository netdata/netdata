// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"sort"
	"strings"
	"time"

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

type planBuildContext struct {
	out         *Plan
	reader      metrix.Reader
	collectMeta metrix.CollectMeta
	prog        *program.Program
	cache       *routeCache
	index       matchIndex
	flat        metrix.Reader

	aliveSeries map[metrix.SeriesID]struct{}
	seenInfer   map[string]struct{}
	chartsByID  map[string]*chartState
	chartOwners map[string]string

	planRouteStats
}

type flattenedReadChecker interface {
	FlattenedRead() bool
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
	sample := planRuntimeSample{startedAt: time.Now()}
	defer func() { e.observeBuildSample(sample) }()

	out := Plan{
		Actions:            make([]EngineAction, 0),
		InferredDimensions: make([]InferredDimension, 0),
	}
	collectMeta := reader.CollectMeta()
	// Failed attempt must not trigger lifecycle transitions.
	if collectMeta.LastAttemptStatus != metrix.CollectStatusSuccess {
		sample.skippedFailed = true
		e.logDebugf("chartengine build skipped: collect status=%d", collectMeta.LastAttemptStatus)
		return out, nil
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	ctx, err := e.preparePlanBuildContext(reader, &out, collectMeta)
	if err != nil {
		sample.buildErr = true
		e.logWarningf("chartengine build prepare failed: %v", err)
		return Plan{}, err
	}
	if err := validateBuildReaderForInferredDimensions(ctx.index, reader); err != nil {
		sample.buildErr = true
		e.logWarningf("chartengine build reader validation failed: %v", err)
		return Plan{}, err
	}
	if err := e.scanPlanSeries(ctx); err != nil {
		sample.buildErr = true
		e.logWarningf("chartengine build scan failed: %v", err)
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
		sample.buildErr = true
		e.logWarningf("chartengine build materialization failed: %v", err)
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

	sample.planRouteStats = ctx.planRouteStats
	sample.planChartInstances = len(ctx.chartsByID)
	sample.planInferredDimensions = len(out.InferredDimensions)

	actionCounts := actionKindCounts(out.Actions)
	sample.actionCreateChart = actionCounts.actionCreateChart
	sample.actionCreateDimension = actionCounts.actionCreateDimension
	sample.actionUpdateChart = actionCounts.actionUpdateChart
	sample.actionRemoveDimension = actionCounts.actionRemoveDimension
	sample.actionRemoveChart = actionCounts.actionRemoveChart

	return out, nil
}

func validateBuildReaderForInferredDimensions(index matchIndex, reader metrix.Reader) error {
	templateID, dimIndex, requiresFlatten := firstInferDimension(index)
	if !requiresFlatten {
		return nil
	}
	aware, ok := reader.(flattenedReadChecker)
	if ok && aware.FlattenedRead() {
		return nil
	}
	return fmt.Errorf(
		"chartengine: inferred dimension requires flattened reader metadata (template_id=%q dim_index=%d); use store.Read(metrix.ReadFlatten())",
		templateID,
		dimIndex,
	)
}

func firstInferDimension(index matchIndex) (string, int, bool) {
	if len(index.chartsByID) == 0 {
		return "", 0, false
	}
	templateIDs := make([]string, 0, len(index.chartsByID))
	for templateID := range index.chartsByID {
		templateIDs = append(templateIDs, templateID)
	}
	sort.Strings(templateIDs)
	for _, templateID := range templateIDs {
		chart := index.chartsByID[templateID]
		for i := range chart.Dimensions {
			if chart.Dimensions[i].InferNameFromSeriesMeta {
				return templateID, i, true
			}
		}
	}
	return "", 0, false
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
		reader:      reader,
		collectMeta: collectMeta,
		prog:        prog,
		cache:       cache,
		index:       index,
		flat:        reader,
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
		ctx.seriesScanned++
		if firstErr != nil {
			return
		}
		if e.state.cfg.seriesSelection == seriesSelectionLastSuccessOnly &&
			meta.LastSeenSuccessSeq != ctx.collectMeta.LastSuccessSeq {
			return
		}
		if selector := e.state.cfg.selector; selector != nil && !selector.Matches(name, labels) {
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
			ctx.routeCacheHits++
		} else {
			e.addRouteCacheMiss()
			ctx.routeCacheMisses++
		}
		if len(routes) == 0 {
			autoRoutes, ok, err := e.resolveAutogenRoute(ctx.reader, name, labels, meta)
			if err != nil {
				firstErr = err
				return
			}
			if ok {
				routes = autoRoutes
				ctx.seriesAutogenMatched++
				ctx.seriesMatched++
			} else {
				ctx.seriesUnmatched++
				return
			}
		} else {
			ctx.seriesMatched++
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
	var chart program.Chart
	identity := program.ChartIdentity{}
	if !route.Autogen {
		var ok bool
		chart, ok = index.chartsByID[route.ChartTemplateID]
		if !ok {
			return fmt.Errorf("chartengine: route references unknown chart template %q", route.ChartTemplateID)
		}
		identity = chart.Identity
	}

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

	cs, exists := ctx.chartsByID[route.ChartID]
	if !exists {
		labelsAcc := newAutogenChartLabelAccumulator()
		if !route.Autogen {
			labelsAcc = newChartLabelAccumulator(chart)
		}
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
			multiplier: route.Multiplier,
			divisor:    route.Divisor,
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

func isAutogenTemplateID(templateID string) bool {
	return strings.HasPrefix(templateID, autogenTemplatePrefix)
}
