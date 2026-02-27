// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
)

const (
	histogramBucketLabel = "le"
	summaryQuantileLabel = "quantile"
)

type labelSliceView struct {
	items []metrix.Label
}

func (v labelSliceView) Get(key string) (string, bool) {
	for _, item := range v.items {
		if item.Key == key {
			return item.Value, true
		}
		if item.Key > key {
			break
		}
	}
	return "", false
}

func (v labelSliceView) Range(fn func(key, value string) bool) {
	for _, item := range v.items {
		if !fn(item.Key, item.Value) {
			return
		}
	}
}

func (v labelSliceView) Len() int {
	return len(v.items)
}

func (v labelSliceView) CloneMap() map[string]string {
	out := make(map[string]string, len(v.items))
	for _, item := range v.items {
		out[item.Key] = item.Value
	}
	return out
}

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
	float      bool
	static     bool
	order      int
	algorithm  program.Algorithm
	multiplier int
	divisor    int
}

type dimBuildEntry struct {
	seenSeq uint64
	value   metrix.SampleValue
	dimensionState
}

type chartState struct {
	templateID      string
	chartID         string
	meta            program.ChartMeta
	lifecycle       program.LifecyclePolicy
	labels          *chartLabelAccumulator
	entries         map[string]*dimBuildEntry
	observedCount   int
	currentBuildSeq uint64
}

type planBuildContext struct {
	out         *Plan
	reader      metrix.Reader
	collectMeta metrix.CollectMeta
	prog        *program.Program
	cache       *routeCache
	index       matchIndex
	flat        metrix.Reader

	seenInfer        map[string]struct{}
	chartsByID       map[string]*chartState
	chartOwners      map[string]string
	dimCapHints      map[string]int
	materializedByID map[string]*materializedChartState

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

	obs := e.observeBuildSuccessSeq(collectMeta.LastSuccessSeq)
	sample.buildSeqViolation = e.state.buildSeq.violating
	sample.buildSeqObserved = true
	switch obs.transition {
	case buildSeqTransitionBroken:
		sample.buildSeqBroken = true
		e.logWarningf(
			"chartengine build sequence is non-monotonic: current=%d previous=%d (suppressing repeats until recovery)",
			collectMeta.LastSuccessSeq,
			obs.previous,
		)
	case buildSeqTransitionRecovered:
		sample.buildSeqRecovered = true
		e.logInfof(
			"chartengine build sequence monotonicity recovered: current=%d previous=%d",
			collectMeta.LastSuccessSeq,
			obs.previous,
		)
	}

	phaseStartedAt := time.Now()
	ctx, err := e.preparePlanBuildContext(reader, &out, collectMeta)
	sample.phasePrepareSeconds = time.Since(phaseStartedAt).Seconds()
	if err != nil {
		sample.buildErr = true
		e.logWarningf("chartengine build prepare failed: %v", err)
		return Plan{}, err
	}
	phaseStartedAt = time.Now()
	if err := validateBuildReaderForInferredDimensions(ctx.index, reader); err != nil {
		sample.phaseValidateSeconds = time.Since(phaseStartedAt).Seconds()
		sample.buildErr = true
		e.logWarningf("chartengine build reader validation failed: %v", err)
		return Plan{}, err
	}
	sample.phaseValidateSeconds = time.Since(phaseStartedAt).Seconds()
	phaseStartedAt = time.Now()
	if err := e.scanPlanSeries(ctx); err != nil {
		sample.phaseScanSeconds = time.Since(phaseStartedAt).Seconds()
		sample.buildErr = true
		e.logWarningf("chartengine build scan failed: %v", err)
		return Plan{}, err
	}
	sample.phaseScanSeconds = time.Since(phaseStartedAt).Seconds()

	// Route-cache lifecycle follows metrix snapshot membership.
	phaseStartedAt = time.Now()
	retainStats := ctx.cache.RetainSeen(ctx.collectMeta.LastSuccessSeq)
	sample.phaseRetainSeconds = time.Since(phaseStartedAt).Seconds()
	sample.routeCacheEntries = retainStats.EntriesAfter
	sample.routeCacheRetained = retainStats.EntriesAfter
	sample.routeCachePruned = retainStats.Pruned
	sample.routeCacheFullDrop = retainStats.FullDrop

	phaseStartedAt = time.Now()
	removeByCapDims, removeByCapCharts := enforceLifecycleCaps(ctx.collectMeta.LastSuccessSeq, ctx.chartsByID, &e.state.materialized)
	sample.phaseLifecycleCapsSec = time.Since(phaseStartedAt).Seconds()
	sample.lifecycleRemovedDimensionByCap = len(removeByCapDims)
	sample.lifecycleRemovedChartByCap = len(removeByCapCharts)
	for _, action := range removeByCapDims {
		out.Actions = append(out.Actions, action)
	}
	for _, action := range removeByCapCharts {
		out.Actions = append(out.Actions, action)
	}
	phaseStartedAt = time.Now()
	if err := e.materializePlanCharts(ctx); err != nil {
		sample.phaseMaterializeSeconds = time.Since(phaseStartedAt).Seconds()
		sample.buildErr = true
		e.logWarningf("chartengine build materialization failed: %v", err)
		return Plan{}, err
	}
	sample.phaseMaterializeSeconds = time.Since(phaseStartedAt).Seconds()
	phaseStartedAt = time.Now()
	removeDims, removeCharts := collectExpiryRemovals(ctx.collectMeta.LastSuccessSeq, &e.state.materialized)
	sample.phaseExpirySeconds = time.Since(phaseStartedAt).Seconds()
	sample.lifecycleRemovedDimensionByExpiry = len(removeDims)
	sample.lifecycleRemovedChartByExpiry = len(removeCharts)
	for _, action := range removeDims {
		out.Actions = append(out.Actions, action)
	}
	for _, action := range removeCharts {
		out.Actions = append(out.Actions, action)
	}
	phaseStartedAt = time.Now()
	sortInferredDimensions(out.InferredDimensions)
	sample.phaseSortSeconds = time.Since(phaseStartedAt).Seconds()

	sample.planRouteStats = ctx.planRouteStats
	sample.planChartInstances = len(ctx.chartsByID)
	sample.planInferredDimensions = len(out.InferredDimensions)

	actionCounts := actionKindCounts(out.Actions)
	sample.actionCreateChart = actionCounts.actionCreateChart
	sample.actionCreateDimension = actionCounts.actionCreateDimension
	sample.actionUpdateChart = actionCounts.actionUpdateChart
	sample.actionRemoveDimension = actionCounts.actionRemoveDimension
	sample.actionRemoveChart = actionCounts.actionRemoveChart
	sample.buildSuccess = true
	e.state.hints.chartsByID = len(ctx.chartsByID)
	e.state.hints.seenInfer = len(ctx.seenInfer)

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
	dimCapHints := make(map[string]int, len(e.state.materialized.charts))
	for chartID, matChart := range e.state.materialized.charts {
		chartOwners[chartID] = matChart.templateID
		if n := len(matChart.dimensions); n > 0 {
			dimCapHints[chartID] = n
		}
	}
	chartsCap := e.state.hints.chartsByID
	if chartsCap < len(e.state.materialized.charts) {
		chartsCap = len(e.state.materialized.charts)
	}
	seenInferCap := e.state.hints.seenInfer
	return &planBuildContext{
		out:              out,
		reader:           reader,
		collectMeta:      collectMeta,
		prog:             prog,
		cache:            cache,
		index:            index,
		flat:             reader,
		seenInfer:        make(map[string]struct{}, seenInferCap),
		chartsByID:       make(map[string]*chartState, chartsCap),
		chartOwners:      chartOwners,
		dimCapHints:      dimCapHints,
		materializedByID: e.state.materialized.charts,
	}, nil
}

func (e *Engine) scanPlanSeries(ctx *planBuildContext) error {
	var firstErr error
	buildSeq := ctx.collectMeta.LastSuccessSeq
	process := func(identity metrix.SeriesIdentity, meta metrix.SeriesMeta, name string, labels metrix.LabelView, v metrix.SampleValue) {
		ctx.seriesScanned++
		if firstErr != nil {
			return
		}
		if e.state.cfg.seriesSelection == seriesSelectionLastSuccessOnly &&
			meta.LastSeenSuccessSeq != ctx.collectMeta.LastSuccessSeq {
			ctx.seriesFilteredBySeq++
			ctx.cache.MarkSeenIfPresent(identity, buildSeq)
			return
		}
		if selector := e.state.cfg.selector; selector != nil && !selector.Matches(name, labels) {
			ctx.seriesFilteredBySel++
			ctx.cache.MarkSeenIfPresent(identity, buildSeq)
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
			buildSeq,
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
	}

	if rawIter, ok := ctx.flat.(metrix.SeriesIdentityRawIterator); ok {
		view := &labelSliceView{}
		rawIter.ForEachSeriesIdentityRaw(func(identity metrix.SeriesIdentity, meta metrix.SeriesMeta, name string, labels []metrix.Label, v metrix.SampleValue) {
			view.items = labels
			process(identity, meta, name, view, v)
		})
		return firstErr
	}

	ctx.flat.ForEachSeriesIdentity(func(identity metrix.SeriesIdentity, meta metrix.SeriesMeta, name string, labels metrix.LabelView, v metrix.SampleValue) {
		process(identity, meta, name, labels, v)
	})
	return firstErr
}

func (ctx *planBuildContext) accumulateRoute(
	index matchIndex,
	route routeBinding,
	labels metrix.LabelView,
	value metrix.SampleValue,
) error {
	cs, exists := ctx.chartsByID[route.ChartID]
	if exists && cs.templateID != route.ChartTemplateID {
		if !route.Autogen && isAutogenTemplateID(cs.templateID) {
			// Template wins over autogen on chart-id collision.
			ctx.chartOwners[route.ChartID] = route.ChartTemplateID
			delete(ctx.chartsByID, route.ChartID)
			cs = nil
			exists = false
		} else {
			// Cross-template rendered-id collision.
			// Existing owner keeps chart-id ownership.
			return nil
		}
	}
	if !exists {
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

		dimCap := ctx.dimCapHints[route.ChartID]
		var entries map[string]*dimBuildEntry
		var labelsAcc *chartLabelAccumulator
		if matChart := ctx.materializedByID[route.ChartID]; matChart != nil {
			entries = matChart.checkoutScratchEntries(dimCap)
			// Labels are only emitted on chart creation, so skip observe work for existing charts.
			labelsAcc = nil
		} else {
			entries = make(map[string]*dimBuildEntry, dimCap)
			labelsAcc = newAutogenChartLabelAccumulator()
			if !route.Autogen {
				chart, ok := index.chartsByID[route.ChartTemplateID]
				if !ok {
					return fmt.Errorf("chartengine: route references unknown chart template %q", route.ChartTemplateID)
				}
				labelsAcc = newChartLabelAccumulator(chart)
			}
		}
		cs = &chartState{
			templateID:      route.ChartTemplateID,
			chartID:         route.ChartID,
			meta:            route.Meta,
			lifecycle:       route.Lifecycle,
			labels:          labelsAcc,
			entries:         entries,
			currentBuildSeq: ctx.collectMeta.LastSuccessSeq,
		}
		ctx.chartsByID[route.ChartID] = cs
	}

	entry, exists := cs.entries[route.DimensionName]
	if !exists {
		entry = &dimBuildEntry{}
		cs.entries[route.DimensionName] = entry
	}
	if entry.seenSeq != cs.currentBuildSeq {
		entry.seenSeq = cs.currentBuildSeq
		entry.value = value
		entry.dimensionState = dimensionState{
			hidden:     route.Hidden,
			float:      route.Float,
			static:     route.Static,
			order:      route.DimensionIndex,
			algorithm:  route.Algorithm,
			multiplier: route.Multiplier,
			divisor:    route.Divisor,
		}
		cs.observedCount++
	} else {
		if entry.hidden != route.Hidden {
			// First-observed hidden flag wins within one build; conflicting routes are ignored.
		}
		if entry.float != route.Float {
			// First-observed float flag wins within one build; conflicting routes are ignored.
		}
		entry.value += value
	}

	if cs.labels != nil {
		if err := cs.labels.observe(labels, route.DimensionKeyLabel); err != nil {
			return err
		}
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
		if cs.observedCount == 0 {
			continue
		}
		chartIDs = append(chartIDs, chartID)
	}
	sort.Strings(chartIDs)

	for _, chartID := range chartIDs {
		cs := ctx.chartsByID[chartID]
		matChart, chartCreated := e.state.materialized.ensureChart(cs.chartID, cs.templateID, cs.meta, cs.lifecycle)
		if chartCreated {
			chartLabels := map[string]string(nil)
			if cs.labels != nil {
				labels, err := cs.labels.materialize()
				if err != nil {
					return err
				}
				chartLabels = labels
			}
			ctx.out.Actions = append(ctx.out.Actions, CreateChartAction{
				ChartTemplateID: cs.templateID,
				ChartID:         cs.chartID,
				Meta:            cs.meta,
				Labels:          chartLabels,
			})
		}
		matChart.lastSeenSuccessSeq = ctx.collectMeta.LastSuccessSeq

		observedNames := observedDimensionNames(cs, matChart)
		for _, name := range observedNames {
			entry := cs.entries[name]
			if entry == nil || entry.seenSeq != cs.currentBuildSeq {
				continue
			}
			matDim, dimCreated := matChart.ensureDimension(name, entry.dimensionState)
			if dimCreated {
				ctx.out.Actions = append(ctx.out.Actions, CreateDimensionAction{
					ChartID:    cs.chartID,
					ChartMeta:  cs.meta,
					Name:       name,
					Hidden:     entry.hidden,
					Float:      entry.float,
					Algorithm:  entry.algorithm,
					Multiplier: entry.multiplier,
					Divisor:    entry.divisor,
				})
			}
			matDim.lastSeenSuccessSeq = ctx.collectMeta.LastSuccessSeq
		}

		updateNames := matChart.orderedDimensionNames()
		values := make([]UpdateDimensionValue, 0, len(updateNames))
		for _, name := range updateNames {
			entry, ok := cs.entries[name]
			if ok && entry != nil && entry.seenSeq == cs.currentBuildSeq {
				values = append(values, UpdateDimensionValue{
					Name:    name,
					IsFloat: entry.float,
					Int64:   int64(entry.value),
					Float64: entry.value,
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
		matChart.storeScratchEntries(cs.entries)
		matChart.pruneScratchEntries(cs.currentBuildSeq)
	}
	return nil
}

func observedDimensionNames(cs *chartState, matChart *materializedChartState) []string {
	if cs == nil {
		return nil
	}
	if cs.observedCount == 0 {
		return nil
	}
	if matChart == nil || len(matChart.dimensions) == 0 {
		return orderedObservedDimensionNames(cs.entries, cs.currentBuildSeq)
	}
	prev := matChart.orderedDimensionNames()
	if len(prev) != cs.observedCount {
		return orderedObservedDimensionNames(cs.entries, cs.currentBuildSeq)
	}
	for _, name := range prev {
		entry, ok := cs.entries[name]
		if !ok || entry == nil || entry.seenSeq != cs.currentBuildSeq {
			return orderedObservedDimensionNames(cs.entries, cs.currentBuildSeq)
		}
		existing := matChart.dimensions[name]
		if existing == nil || existing.static != entry.static || existing.order != entry.order {
			return orderedObservedDimensionNames(cs.entries, cs.currentBuildSeq)
		}
	}
	return prev
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

// buildPlan is a package-level convenience wrapper around Engine.BuildPlan.
func buildPlan(engine *Engine, reader metrix.Reader) (Plan, error) {
	if engine == nil {
		return Plan{}, fmt.Errorf("chartengine: nil engine")
	}
	return engine.BuildPlan(reader)
}

func isAutogenTemplateID(templateID string) bool {
	return strings.HasPrefix(templateID, autogenTemplatePrefix)
}
