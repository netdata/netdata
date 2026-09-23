// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"cmp"
	"fmt"
	"maps"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
)

type labelSliceView struct {
	items []metrix.Label
}

func (v labelSliceView) Get(key string) (string, bool) {
	i := sort.Search(len(v.items), func(i int) bool { return v.items[i].Key >= key })
	if i < len(v.items) && v.items[i].Key == key {
		return v.items[i].Value, true
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

// Plan is the deterministic planner output consumed by chartemit. Actions and
// their referenced maps and slices are immutable snapshots; consumers must not
// mutate them.
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

type dimensionAlgorithm uint8

const (
	dimensionAlgorithmAbsolute dimensionAlgorithm = iota
	dimensionAlgorithmIncremental
)

func compactDimensionAlgorithm(algorithm program.Algorithm) dimensionAlgorithm {
	if algorithm == program.AlgorithmIncremental {
		return dimensionAlgorithmIncremental
	}
	return dimensionAlgorithmAbsolute
}

func (a dimensionAlgorithm) programAlgorithm() program.Algorithm {
	if a == dimensionAlgorithmIncremental {
		return program.AlgorithmIncremental
	}
	return program.AlgorithmAbsolute
}

type dimensionState struct {
	sortKey     dimensionSortKey
	order       int
	multiplier  int
	divisor     int
	algorithm   dimensionAlgorithm
	hidden      bool
	float       bool
	static      bool
	aggregation program.Aggregation
}

type dimensionSortKind uint8

const (
	dimensionSortDefault dimensionSortKind = iota
	dimensionSortHistogramBucket
)

type dimensionSortKey struct {
	kind       dimensionSortKind
	upperBound float64
}

type dimBuildEntry struct {
	seenSeq      uint64
	observations uint64
	value        metrix.SampleValue
	dimensionState
	journalToken uint64
}

func (e *dimBuildEntry) aggregate(value metrix.SampleValue) {
	if e.aggregation == program.AggregationSum {
		e.value += value
		return
	}
	e.aggregateNonSum(value)
}

func (e *dimBuildEntry) aggregateNonSum(value metrix.SampleValue) {
	switch e.aggregation {
	case program.AggregationMin:
		if math.IsNaN(e.value) || (!math.IsNaN(value) && value < e.value) {
			e.value = value
		}
	case program.AggregationMax:
		if math.IsNaN(e.value) || (!math.IsNaN(value) && value > e.value) {
			e.value = value
		}
	case program.AggregationAvg:
		e.observations++
		e.value = runningAverage(e.value, value, e.observations)
	}
}

func runningAverage(current, value metrix.SampleValue, observations uint64) metrix.SampleValue {
	if math.IsNaN(current) || math.IsNaN(value) {
		return math.NaN()
	}
	if math.IsInf(current, 0) {
		if math.IsInf(value, 0) && math.Signbit(current) != math.Signbit(value) {
			return math.NaN()
		}
		return current
	}
	if math.IsInf(value, 0) {
		return value
	}

	n := float64(observations)
	delta := value - current
	if !math.IsInf(delta, 0) {
		return math.FMA(delta, 1/n, current)
	}

	return current*((n-1)/n) + value/n
}

type chartState struct {
	templateID   string
	chartID      string
	meta         program.ChartMeta
	lifecycle    program.LifecyclePolicy
	labelTracker chartLabelTracker
	entries      map[string]*dimBuildEntry
	// entriesOwner is the materialized chart whose scratch map entries is, if any.
	entriesOwner    *materializedChartState
	unownedDeletes  int // cap deletions from entries while it has no owner
	observedCount   int
	currentBuildSeq uint64
}

type planBuildContext struct {
	out               *Plan
	reader            metrix.Reader
	collectMeta       metrix.CollectMeta
	buildCycle        uint64
	prog              *program.Program
	cache             *routeCache
	routeCacheEnabled bool
	routeObserver     func(PlanRouteDiagnostic)
	index             matchIndex
	flat              metrix.Reader

	seenInfer  map[inferredDimensionKey]struct{}
	chartsByID map[string]*chartState
	// chartOwnerOverrides holds ownership decided in this build; other charts are
	// owned by their materialized template.
	chartOwnerOverrides map[string]string
	// chartStates and values back per-build chart state and update values; both
	// are sized from the previous build and fall back to ordinary allocation.
	chartStates      []chartState
	values           []UpdateDimensionValue
	materialized     *materializedState
	materializedByID map[string]*materializedChartState
	retired          map[string]*materializedChartState
	journal          *planJournal
	scan             planSeriesScan
	collisions       chartIDCollisions

	planRouteStats
}

type flattenedReadChecker interface {
	FlattenedRead() bool
}

func isHistogramBucketSeries(meta metrix.SeriesMeta) bool {
	return meta.SourceKind == metrix.MetricKindHistogram &&
		meta.FlattenRole == metrix.FlattenRoleHistogramBucket
}

// buildPlan runs with the owning engine locked, possibly against a private
// candidate view. It stages lifecycle changes in the materialized state in place,
// recording committed values in journal. Until an attempt is reserved for the
// result, PreparePlanWithOptions rolls the journal back on any exit, so a build that
// fails or panics leaves committed state unchanged.
func (e *Engine) buildPlan(
	reader metrix.Reader,
	retired map[string]*materializedChartState,
	journal *planJournal,
) (Plan, materializedState, bool, error) {
	out := Plan{
		Actions:            make([]EngineAction, 0, e.state.hints.actions),
		InferredDimensions: make([]InferredDimension, 0, e.state.hints.seenInfer),
	}
	collectMeta := reader.CollectMeta()

	sample := PlanRuntimeSample{
		startedAt: time.Now(),
	}
	defer func() { e.observeBuildSample(sample) }()
	// Failed attempt must not trigger lifecycle transitions.
	if collectMeta.LastAttemptStatus != metrix.CollectStatusSuccess {
		sample.skippedFailed = true
		e.logDebugf("chartengine build skipped: collect status=%d", collectMeta.LastAttemptStatus)
		return out, materializedState{}, false, nil
	}

	obs := e.observeBuildSuccessSeq(collectMeta.LastSuccessSeq)
	buildCycle := e.nextBuildCycle(collectMeta.LastSuccessSeq)
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
	staged := e.state.materialized
	ctx, err := e.preparePlanBuildContext(reader, &out, collectMeta, buildCycle, &staged)
	sample.phasePrepareSeconds = time.Since(phaseStartedAt).Seconds()
	if err != nil {
		sample.buildErr = true
		e.logWarningf("chartengine build prepare failed: %v", err)
		return Plan{}, materializedState{}, false, err
	}
	ctx.retired = retired
	ctx.journal = journal
	phaseStartedAt = time.Now()
	if err := validateBuildReaderForInferredDimensions(ctx.index, reader); err != nil {
		sample.phaseValidateSeconds = time.Since(phaseStartedAt).Seconds()
		sample.buildErr = true
		e.logWarningf("chartengine build reader validation failed: %v", err)
		return Plan{}, materializedState{}, false, err
	}
	sample.phaseValidateSeconds = time.Since(phaseStartedAt).Seconds()
	phaseStartedAt = time.Now()
	if err := e.scanPlanSeries(ctx); err != nil {
		sample.phaseScanSeconds = time.Since(phaseStartedAt).Seconds()
		sample.buildErr = true
		e.logWarningf("chartengine build scan failed: %v", err)
		return Plan{}, materializedState{}, false, err
	}
	sample.phaseScanSeconds = time.Since(phaseStartedAt).Seconds()

	// Route-cache lifecycle follows metrix snapshot membership. Diagnostic plans
	// bypass it completely so repeated attempts retain complete route facts.
	if ctx.routeCacheEnabled {
		phaseStartedAt = time.Now()
		retainStats := ctx.cache.RetainSeen(ctx.collectMeta.LastSuccessSeq)
		sample.phaseRetainSeconds = time.Since(phaseStartedAt).Seconds()
		sample.routeCacheEntries = retainStats.EntriesAfter
		sample.routeCacheRetained = retainStats.EntriesAfter
		sample.routeCachePruned = retainStats.Pruned
		sample.routeCacheFullDrop = retainStats.FullDrop
	}

	phaseStartedAt = time.Now()
	removeByCapDims, removeByCapCharts := enforceLifecycleCapsWithObserver(
		ctx.collectMeta.LastSuccessSeq,
		ctx.chartsByID,
		ctx.materialized,
		ctx.routeObserver,
		journal,
	)
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
		return Plan{}, materializedState{}, false, err
	}
	sample.phaseMaterializeSeconds = time.Since(phaseStartedAt).Seconds()
	phaseStartedAt = time.Now()
	removeDims, removeCharts := collectExpiryRemovals(ctx.collectMeta.LastSuccessSeq, ctx.materialized, journal)
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
	ctx.reconcileRetirements()
	staged.recordEmittedChartDefinitions(journal, out.Actions)
	sortInferredDimensions(out.InferredDimensions)
	sample.phaseSortSeconds = time.Since(phaseStartedAt).Seconds()

	sample.planRouteStats = ctx.planRouteStats
	sample.planChartInstances = len(ctx.chartsByID)
	sample.planInferredDimensions = len(out.InferredDimensions)

	actionCounts := actionKindCounts(out.Actions)
	sample.actionCreateChart = actionCounts.actionCreateChart
	sample.actionCreateDimension = actionCounts.actionCreateDimension
	sample.actionUpdateChartLabels = actionCounts.actionUpdateChartLabels
	sample.actionUpdateChart = actionCounts.actionUpdateChart
	sample.actionRemoveDimension = actionCounts.actionRemoveDimension
	sample.actionRemoveChart = actionCounts.actionRemoveChart
	sample.buildSuccess = true
	e.warnChartIDCollisions(ctx)
	e.state.hints.chartsByID = len(ctx.chartsByID)
	e.state.hints.seenInfer = len(ctx.seenInfer)
	e.state.hints.actions = len(out.Actions)
	e.state.hints.values = len(ctx.values)

	return out, staged, true, nil
}

func validateBuildReaderForInferredDimensions(index matchIndex, reader metrix.Reader) error {
	infer := index.firstInfer
	if !infer.ok {
		return nil
	}
	aware, ok := reader.(flattenedReadChecker)
	if ok && aware.FlattenedRead() {
		return nil
	}
	return fmt.Errorf(
		"chartengine: inferred dimension requires flattened reader metadata (template_id=%q dim_index=%d); use store.Read(metrix.ReadFlatten())",
		infer.templateID,
		infer.dimIndex,
	)
}

func (e *Engine) preparePlanBuildContext(
	reader metrix.Reader,
	out *Plan,
	collectMeta metrix.CollectMeta,
	buildCycle uint64,
	materialized *materializedState,
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
	if materialized == nil {
		return nil, fmt.Errorf("chartengine: nil materialized state")
	}
	if materialized.charts == nil {
		*materialized = newMaterializedState()
	}
	index := e.state.matchIndex
	if index.chartsByID == nil {
		index = buildMatchIndex(prog.Charts())
		e.state.matchIndex = index
	}
	chartsCap := max(e.state.hints.chartsByID, len(materialized.charts))
	var seenInfer map[inferredDimensionKey]struct{}
	if hint := e.state.hints.seenInfer; hint > 0 {
		seenInfer = make(map[inferredDimensionKey]struct{}, hint)
	}
	return &planBuildContext{
		out:               out,
		reader:            reader,
		collectMeta:       collectMeta,
		buildCycle:        buildCycle,
		prog:              prog,
		cache:             cache,
		routeCacheEnabled: e.state.cfg.routeObserver == nil,
		routeObserver:     e.state.templateSet.diagnosticObserver(e.state.cfg.routeObserver),
		index:             index,
		flat:              reader,
		seenInfer:         seenInfer,
		chartsByID:        make(map[string]*chartState, chartsCap),
		chartStates:       make([]chartState, 0, e.state.hints.chartsByID),
		values:            make([]UpdateDimensionValue, 0, e.state.hints.values),
		materialized:      materialized,
		materializedByID:  materialized.charts,
	}, nil
}

func (e *Engine) scanPlanSeries(ctx *planBuildContext) error {
	return e.forEachPlanSeriesRoute(ctx, false)
}

// planSeriesScan is the state of one pass over the reader's series. It lives in the
// build context, so a pass allocates only its callback.
type planSeriesScan struct {
	engine       *Engine
	replayLabels bool
	buildSeq     uint64
	err          error
	view         labelSliceView
}

func (e *Engine) forEachPlanSeriesRoute(ctx *planBuildContext, replayLabels bool) error {
	ctx.scan = planSeriesScan{
		engine:       e,
		replayLabels: replayLabels,
		buildSeq:     ctx.collectMeta.LastSuccessSeq,
	}
	if rawIter, ok := ctx.flat.(metrix.SeriesIdentityRawIterator); ok {
		rawIter.ForEachSeriesIdentityRaw(ctx.visitRawPlanSeries)
	} else {
		ctx.flat.ForEachSeriesIdentity(ctx.visitPlanSeries)
	}
	err := ctx.scan.err
	ctx.scan = planSeriesScan{}
	return err
}

func (ctx *planBuildContext) visitRawPlanSeries(
	identity metrix.SeriesIdentity,
	meta metrix.SeriesMeta,
	name string,
	labels []metrix.Label,
	v metrix.SampleValue,
) {
	ctx.scan.view.items = labels
	ctx.visitPlanSeries(identity, meta, name, &ctx.scan.view, v)
}

func (ctx *planBuildContext) visitPlanSeries(
	identity metrix.SeriesIdentity,
	meta metrix.SeriesMeta,
	name string,
	labels metrix.LabelView,
	v metrix.SampleValue,
) {
	e := ctx.scan.engine
	if !ctx.scan.replayLabels {
		ctx.seriesScanned++
	}
	if ctx.scan.err != nil {
		return
	}
	if e.state.cfg.seriesSelection == seriesSelectionLastSuccessOnly &&
		meta.LastSeenSuccessSeq != ctx.collectMeta.LastSuccessSeq {
		if !ctx.scan.replayLabels {
			ctx.seriesFilteredBySeq++
			if ctx.routeCacheEnabled {
				ctx.cache.MarkSeenIfPresent(identity, ctx.scan.buildSeq)
			}
			if ctx.routeObserver != nil {
				ctx.observeRouteDiagnostic(PlanRouteDiagnostic{
					Decision:       PlanRouteSeriesFilteredBySequence,
					SeriesIdentity: identity,
					MetricName:     name,
				})
			}
		}
		return
	}
	if selector := e.state.cfg.selector; selector != nil && !selector.Matches(name, labels) {
		if !ctx.scan.replayLabels {
			ctx.seriesFilteredBySel++
			if ctx.routeCacheEnabled {
				ctx.cache.MarkSeenIfPresent(identity, ctx.scan.buildSeq)
			}
			if ctx.routeObserver != nil {
				ctx.observeRouteDiagnostic(PlanRouteDiagnostic{
					Decision:       PlanRouteSeriesFilteredBySelector,
					SeriesIdentity: identity,
					MetricName:     name,
				})
			}
		}
		return
	}

	observer := ctx.routeObserver
	if ctx.scan.replayLabels {
		observer = nil
	}
	routes, hit, err := e.resolveSeriesRoutes(
		ctx.cache,
		ctx.routeCacheEnabled,
		observer,
		identity,
		name,
		labels,
		meta,
		ctx.reader,
		ctx.index,
		ctx.prog.Revision(),
		ctx.scan.buildSeq,
	)
	if err != nil {
		ctx.scan.err = err
		return
	}
	if !ctx.scan.replayLabels && ctx.routeCacheEnabled {
		if hit {
			ctx.routeCacheHits++
		} else {
			ctx.routeCacheMisses++
		}
	}
	if len(routes) > 0 && routes[0].Autogen && !routes[0].autogenGuard.valid(ctx.reader, meta) {
		routes = nil
		// Release obsolete discovery even if autogen rebuilding is rejected.
		if ctx.routeCacheEnabled {
			ctx.cache.Store(identity, ctx.prog.Revision(), ctx.scan.buildSeq, nil)
		}
	}
	if len(routes) == 0 {
		autoRoutes, ok, reason, ruleIndex, err := e.resolveAutogenRouteWithReason(ctx.reader, name, labels, meta)
		if err != nil {
			ctx.scan.err = err
			return
		}
		if ok {
			routes = autoRoutes
			if ctx.routeCacheEnabled {
				ctx.cache.Store(identity, ctx.prog.Revision(), ctx.scan.buildSeq, routes)
			}
			if !ctx.scan.replayLabels {
				ctx.seriesAutogenMatched++
				ctx.seriesMatched++
			}
		} else {
			if !ctx.scan.replayLabels {
				ctx.seriesUnmatched++
				if ctx.routeObserver != nil {
					ruleScope := ""
					if ruleIndex >= 0 && ruleIndex < len(e.state.cfg.autogen.Rules) {
						ruleScope = e.state.cfg.autogen.Rules[ruleIndex].Scope
					}
					ctx.observeRouteDiagnostic(PlanRouteDiagnostic{
						Decision:         PlanRouteUnmatched,
						Reason:           reason,
						SeriesIdentity:   identity,
						MetricName:       name,
						MetricFamilyName: diagnosticMetricFamilyName(name, labels, meta),
						AutogenRuleIndex: ruleIndex,
						AutogenRuleScope: ruleScope,
					})
				}
			}
			return
		}
	} else if !ctx.scan.replayLabels {
		if routes[0].Autogen {
			ctx.seriesAutogenMatched++
		}
		ctx.seriesMatched++
	}

	for _, route := range routes {
		if ctx.scan.replayLabels {
			chart := ctx.chartsByID[route.ChartID]
			if chart == nil || chart.templateID != route.ChartTemplateID || !chart.labelTracker.needsReplay() {
				continue
			}
			if err := chart.labelTracker.observeReplay(labels, route.DimensionKeyLabel); err != nil {
				ctx.scan.err = err
				return
			}
			continue
		}
		route = finalizeRouteAlgorithm(route, meta.Kind)
		if err := ctx.accumulateRoute(ctx.index, route, identity, name, meta, labels, v); err != nil {
			ctx.scan.err = err
			return
		}
	}
}

// newChartState returns zeroed chart state from the build block while it has room.
func (ctx *planBuildContext) newChartState() *chartState {
	if len(ctx.chartStates) < cap(ctx.chartStates) {
		ctx.chartStates = ctx.chartStates[:len(ctx.chartStates)+1]
		return &ctx.chartStates[len(ctx.chartStates)-1]
	}
	return new(chartState)
}

// adoptedScratchEntries returns the scratch map the chart takes from this build: a map
// the build created and a cap then trimmed heavily is rebuilt at its live size.
func (cs *chartState) adoptedScratchEntries() map[string]*dimBuildEntry {
	if cs.entriesOwner == nil && needsCompaction(len(cs.entries), cs.unownedDeletes) {
		cs.entries = rebuildMap(cs.entries)
		cs.unownedDeletes = 0
	}
	return cs.entries
}

func (ctx *planBuildContext) chartOwner(chartID string) (string, bool) {
	if owner, ok := ctx.chartOwnerOverrides[chartID]; ok {
		return owner, true
	}
	if chart := ctx.materializedByID[chartID]; chart != nil {
		return chart.templateID, true
	}
	return "", false
}

func (ctx *planBuildContext) setChartOwner(chartID, templateID string) {
	if ctx.chartOwnerOverrides == nil {
		ctx.chartOwnerOverrides = make(map[string]string)
	}
	ctx.chartOwnerOverrides[chartID] = templateID
}

type inferredDimensionKey struct {
	chartTemplateID string
	dimensionIndex  int
	name            string
}

func (ctx *planBuildContext) accumulateRoute(
	index matchIndex,
	route routeBinding,
	identity metrix.SeriesIdentity,
	metricName string,
	seriesMeta metrix.SeriesMeta,
	labels metrix.LabelView,
	value metrix.SampleValue,
) error {
	cs, exists := ctx.chartsByID[route.ChartID]
	if exists && cs.templateID != route.ChartTemplateID {
		if !route.Autogen && isAutogenTemplateID(cs.templateID) {
			// Template wins over autogen on chart-id collision.
			ctx.observeAutogenDisplacement(route, identity, metricName, cs.templateID)
			ctx.setChartOwner(route.ChartID, route.ChartTemplateID)
			delete(ctx.chartsByID, route.ChartID)
			cs = nil
			exists = false
		} else {
			// Cross-template rendered-id collision.
			// Existing owner keeps chart-id ownership.
			if !route.Autogen {
				ctx.collisions.add(route.ChartID, cs.templateID, route.ChartTemplateID)
			}
			if ctx.routeObserver != nil {
				ctx.observeRouteDiagnostic(PlanRouteDiagnostic{
					Decision:                PlanRouteCollisionRejected,
					SeriesIdentity:          identity,
					MetricName:              metricName,
					ChartTemplateID:         route.ChartTemplateID,
					DimensionIndex:          route.DimensionIndex,
					ChartID:                 route.ChartID,
					DimensionName:           route.DimensionName,
					DimensionKeyLabel:       route.DimensionKeyLabel,
					ExistingChartTemplateID: cs.templateID,
					Autogen:                 route.Autogen,
				})
			}
			return nil
		}
	}
	if !exists {
		ownerTemplateID, ownerExists := ctx.chartOwner(route.ChartID)
		if ownerExists && ownerTemplateID != route.ChartTemplateID {
			if !route.Autogen && isAutogenTemplateID(ownerTemplateID) {
				// Template wins over autogen on chart-id collision.
				ctx.observeAutogenDisplacement(route, identity, metricName, ownerTemplateID)
				ctx.setChartOwner(route.ChartID, route.ChartTemplateID)
				delete(ctx.chartsByID, route.ChartID)
			} else {
				// Cross-template rendered-id collision.
				// Existing owner keeps chart-id ownership.
				if !route.Autogen {
					ctx.collisions.add(route.ChartID, ownerTemplateID, route.ChartTemplateID)
				}
				if ctx.routeObserver != nil {
					ctx.observeRouteDiagnostic(PlanRouteDiagnostic{
						Decision:                PlanRouteCollisionRejected,
						SeriesIdentity:          identity,
						MetricName:              metricName,
						ChartTemplateID:         route.ChartTemplateID,
						DimensionIndex:          route.DimensionIndex,
						ChartID:                 route.ChartID,
						DimensionName:           route.DimensionName,
						DimensionKeyLabel:       route.DimensionKeyLabel,
						ExistingChartTemplateID: ownerTemplateID,
						Autogen:                 route.Autogen,
					})
				}
				return nil
			}
		}
		if !ownerExists {
			ctx.setChartOwner(route.ChartID, route.ChartTemplateID)
		}

		var entries map[string]*dimBuildEntry
		var entriesOwner *materializedChartState
		matChart := ctx.materializedByID[route.ChartID]
		dimCap := 0
		if matChart != nil {
			dimCap = len(matChart.dimensions)
		}
		matchingMaterializedChart := matChart != nil && matChart.templateID == route.ChartTemplateID

		if matchingMaterializedChart {
			entries = matChart.checkoutScratchEntries(ctx.journal, dimCap)
			entriesOwner = matChart
		} else {
			entries = make(map[string]*dimBuildEntry, dimCap)
		}

		var previousPresentation *materializedChartPresentation
		if matchingMaterializedChart && matChart.presentation != nil {
			previousPresentation = matChart.presentation
		}
		cs = ctx.newChartState()
		*cs = chartState{
			templateID:      route.ChartTemplateID,
			chartID:         route.ChartID,
			meta:            route.Meta,
			lifecycle:       route.Lifecycle,
			labelTracker:    newChartLabelTracker(previousPresentation),
			entries:         entries,
			entriesOwner:    entriesOwner,
			currentBuildSeq: ctx.buildCycle,
		}
		ctx.chartsByID[route.ChartID] = cs
	}
	if isHistogramBucketSeries(seriesMeta) {
		cs.meta.Type = program.ChartTypeHeatmap
	}
	sortKey := histogramBucketDimensionSortKey(route, seriesMeta, labels)

	entry, exists := cs.entries[route.DimensionName]
	if !exists {
		entry = &dimBuildEntry{}
		ctx.journal.putEntry(cs.entries, route.DimensionName, entry)
	} else {
		ctx.journal.touchEntry(entry)
	}
	if entry.seenSeq != cs.currentBuildSeq {
		entry.seenSeq = cs.currentBuildSeq
		entry.observations = 1
		entry.value = value
		entry.dimensionState = dimensionState{
			hidden:      route.Hidden,
			float:       route.Float,
			static:      route.Static,
			order:       route.DimensionIndex,
			sortKey:     sortKey,
			aggregation: route.Aggregation,
			algorithm:   compactDimensionAlgorithm(route.Algorithm),
			multiplier:  route.Multiplier,
			divisor:     route.Divisor,
		}
		cs.observedCount++
	} else {
		if entry.hidden != route.Hidden {
			// First-observed hidden flag wins within one build; conflicting routes are ignored.
		}
		if entry.float != route.Float {
			// First-observed float flag wins within one build; conflicting routes are ignored.
		}
		entry.aggregate(value)
	}

	if cs.labelTracker.observeMembership(identity, route.DimensionKeyLabel) {
		policy := labelPolicyForTemplate(index, route.ChartTemplateID, route.Autogen)
		if policy == nil {
			return fmt.Errorf("chartengine: route references unknown chart template %q", route.ChartTemplateID)
		}
		if err := cs.labelTracker.observeDuringScan(policy, labels, route.DimensionKeyLabel); err != nil {
			return err
		}
	}

	if route.Inferred {
		key := inferredDimensionKey{
			chartTemplateID: route.ChartTemplateID,
			dimensionIndex:  route.DimensionIndex,
			name:            route.DimensionName,
		}
		if _, exists := ctx.seenInfer[key]; !exists {
			if ctx.seenInfer == nil {
				ctx.seenInfer = make(map[inferredDimensionKey]struct{})
			}
			ctx.seenInfer[key] = struct{}{}
			ctx.out.InferredDimensions = append(ctx.out.InferredDimensions, InferredDimension{
				ChartTemplateID: route.ChartTemplateID,
				DimensionIndex:  route.DimensionIndex,
				Name:            route.DimensionName,
			})
		}
	}
	if ctx.routeObserver != nil {
		policy := labelPolicyForTemplate(index, route.ChartTemplateID, route.Autogen)
		if policy == nil {
			return fmt.Errorf("chartengine: route references unknown chart template %q", route.ChartTemplateID)
		}
		instanceLabels, ok := diagnosticChartInstanceLabels(policy.instancePlan, labels)
		if !ok {
			return fmt.Errorf("chartengine: accepted route has unresolved instance labels for chart template %q", route.ChartTemplateID)
		}
		promotionMode, promotedLabels := diagnosticLabelPromotion(policy)
		ctx.observeRouteDiagnostic(PlanRouteDiagnostic{
			Decision:           PlanRouteAccepted,
			SeriesIdentity:     identity,
			MetricName:         metricName,
			MetricFamilyName:   diagnosticMetricFamilyName(metricName, labels, seriesMeta),
			ChartTemplateID:    route.ChartTemplateID,
			DimensionIndex:     route.DimensionIndex,
			ChartID:            route.ChartID,
			DimensionName:      route.DimensionName,
			DimensionKeyLabel:  route.DimensionKeyLabel,
			InstanceLabels:     instanceLabels,
			Context:            cs.meta.Context,
			Family:             cs.meta.Family,
			Units:              cs.meta.Units,
			Algorithm:          string(route.Algorithm),
			Aggregation:        diagnosticAggregation(route.Aggregation),
			Presentation:       string(cs.meta.Type),
			SeriesKind:         diagnosticMetricKind(seriesMeta.Kind),
			Multiplier:         route.Multiplier,
			Divisor:            route.Divisor,
			LabelPromotionMode: promotionMode,
			PromotedLabels:     promotedLabels,
			Autogen:            route.Autogen,
		})
	}
	return nil
}

func labelPolicyForTemplate(index matchIndex, templateID string, autogen bool) *chartLabelPolicy {
	if autogen {
		return index.autogenLabels
	}
	return index.labelPolicies[templateID]
}

func (e *Engine) materializePlanCharts(ctx *planBuildContext) error {
	if err := e.reconcileChangedChartLabels(ctx); err != nil {
		return err
	}

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
		previous := ctx.materialized.charts[chartID]
		dimensionExpiry := cs.lifecycle.Dimensions.ExpireAfterCycles
		if previous != nil {
			dimensionExpiry = previous.lifecycle.Dimensions.ExpireAfterCycles
		}
		if previous != nil &&
			(previous.templateID != cs.templateID || needsChartRevival(previous, cs, ctx.collectMeta.LastSuccessSeq)) {
			// Caps may already have pruned staged dimensions and queued their removal.
			// Reconciliation needs the complete committed definition to retain those removals.
			ctx.rememberRetired(chartID, ctx.journal.committedChart(previous))
			ctx.journal.deleteChart(ctx.materialized.charts, chartID)
		}
		matChart, chartCreated := ctx.materialized.ensureChart(
			ctx.journal,
			cs.chartID,
			cs.templateID,
			cs.meta,
			cs.lifecycle,
		)
		previousPresentation := matChart.presentation
		membershipChanged := cs.labelTracker.changed()
		if membershipChanged {
			membership := cs.labelTracker.membership()
			accumulator := cs.labelTracker.accumulator()
			labelsChanged := previousPresentation == nil ||
				!accumulator.materializedEquals(previousPresentation.labelValues)
			if labelsChanged {
				chartLabels := accumulator.materialize()
				matChart.replaceLabels(ctx.journal, maps.Clone(chartLabels), membership)
				if chartCreated {
					ctx.out.Actions = append(ctx.out.Actions, CreateChartAction{
						ChartTemplateID: cs.templateID,
						ChartID:         cs.chartID,
						Meta:            cs.meta,
						Labels:          chartLabels,
					})
				} else {
					ctx.out.Actions = append(ctx.out.Actions, UpdateChartLabelsAction{
						ChartID: cs.chartID,
						Meta:    cs.meta,
						Labels:  chartLabels,
					})
				}
			} else {
				matChart.replaceLabelMembership(ctx.journal, membership)
			}
		} else if chartCreated {
			if previous == nil || previous.presentation == nil {
				return fmt.Errorf("chartengine: new chart %q unexpectedly matched prior label membership", cs.chartID)
			}
			// Revival can recreate a definition without changing its exact series membership.
			matChart.replaceLabels(ctx.journal, previous.presentation.labelValues, previous.presentation.labelMembership)
			ctx.out.Actions = append(ctx.out.Actions, CreateChartAction{
				ChartTemplateID: cs.templateID,
				ChartID:         cs.chartID,
				Meta:            cs.meta,
				Labels:          maps.Clone(previous.presentation.labelValues),
			})
		}
		ctx.journal.setChartSeen(matChart, ctx.collectMeta.LastSuccessSeq)

		observedNames := observedDimensionNames(ctx.journal, cs, matChart)
		for _, name := range observedNames {
			entry := cs.entries[name]
			if entry == nil || entry.seenSeq != cs.currentBuildSeq {
				continue
			}
			if dim := matChart.dimensions[name]; dim != nil &&
				expiredBeforeCycle(dim.lastSeenSuccessSeq, ctx.collectMeta.LastSuccessSeq, dimensionExpiry) &&
				dimensionDefinitionChanged(dim, entry.dimensionState) {
				matChart.removeDimension(ctx.journal, name)
			}
			matDim, dimCreated := matChart.ensureDimension(ctx.journal, name, entry.dimensionState)
			if dimCreated {
				ctx.out.Actions = append(ctx.out.Actions, CreateDimensionAction{
					ChartID:    cs.chartID,
					ChartMeta:  cs.meta,
					Name:       name,
					Hidden:     entry.hidden,
					Float:      entry.float,
					Algorithm:  entry.algorithm.programAlgorithm(),
					Multiplier: entry.multiplier,
					Divisor:    entry.divisor,
				})
			}
			ctx.journal.setDimSeen(matDim, ctx.collectMeta.LastSuccessSeq)
		}

		updateNames := matChart.orderedDimensionNames(ctx.journal)
		start := len(ctx.values)
		values := ctx.values
		for _, name := range updateNames {
			entry, ok := cs.entries[name]
			if ok && entry != nil && entry.seenSeq == cs.currentBuildSeq {
				if math.IsNaN(entry.value) || math.IsInf(entry.value, 0) {
					// A non-finite value (e.g. a summary quantile with no observations this
					// cycle) must render as a gap, not 0: emit SETEMPTY rather than carry NaN.
					values = append(values, UpdateDimensionValue{
						Name:    name,
						IsEmpty: true,
					})
					continue
				}
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
		// Each chart owns a capacity-limited window of the build's value block.
		ctx.values = values
		ctx.out.Actions = append(ctx.out.Actions, UpdateChartAction{
			ChartID: cs.chartID,
			Values:  values[start:len(values):len(values)],
		})
		matChart.storeScratchEntries(ctx.journal, cs.adoptedScratchEntries(), cs.entriesOwner)
		matChart.pruneScratchEntries(ctx.journal, cs.currentBuildSeq)
	}
	return nil
}

func (e *Engine) reconcileChangedChartLabels(ctx *planBuildContext) error {
	needsReplay := false
	for _, chart := range ctx.chartsByID {
		if chart == nil || chart.observedCount == 0 || !chart.labelTracker.finishMembership() {
			continue
		}
		if chart.labelTracker.needsReplay() {
			policy := labelPolicyForTemplate(ctx.index, chart.templateID, isAutogenTemplateID(chart.templateID))
			if policy == nil {
				return fmt.Errorf("chartengine: missing label policy for chart template %q", chart.templateID)
			}
			chart.labelTracker.beginReplay(policy)
			needsReplay = true
		}
	}
	if !needsReplay {
		return nil
	}

	// A mismatch may be discovered after a matching prefix whose labels were not
	// retained. Replay the immutable current snapshot only on changed cycles and
	// use the same route walk so selection and autogen semantics cannot diverge.
	return e.forEachPlanSeriesRoute(ctx, true)
}

func observedDimensionNames(j *planJournal, cs *chartState, matChart *materializedChartState) []string {
	if cs == nil {
		return nil
	}
	if cs.observedCount == 0 {
		return nil
	}
	if matChart == nil || len(matChart.dimensions) == 0 {
		return orderedObservedDimensionNames(cs.entries, cs.currentBuildSeq)
	}
	prev := matChart.orderedDimensionNames(j)
	if len(prev) != cs.observedCount {
		return orderedObservedDimensionNames(cs.entries, cs.currentBuildSeq)
	}
	for _, name := range prev {
		entry, ok := cs.entries[name]
		if !ok || entry == nil || entry.seenSeq != cs.currentBuildSeq {
			return orderedObservedDimensionNames(cs.entries, cs.currentBuildSeq)
		}
		existing := matChart.dimensions[name]
		if existing == nil || existing.static != entry.static || existing.order != entry.order || existing.sortKey != entry.sortKey {
			return orderedObservedDimensionNames(cs.entries, cs.currentBuildSeq)
		}
	}
	return prev
}

func histogramBucketDimensionSortKey(route routeBinding, meta metrix.SeriesMeta, labels metrix.LabelView) dimensionSortKey {
	if !isHistogramBucketSeries(meta) || route.DimensionKeyLabel != metrix.HistogramBucketLabel || labels == nil {
		return dimensionSortKey{}
	}
	upperBound, ok := labels.Get(metrix.HistogramBucketLabel)
	if !ok {
		return dimensionSortKey{}
	}
	return histogramBucketSortKey(upperBound)
}

func histogramBucketSortKey(upperBound string) dimensionSortKey {
	value, ok := parseHistogramBucketUpperBound(upperBound)
	if !ok {
		return dimensionSortKey{}
	}
	return dimensionSortKey{
		kind:       dimensionSortHistogramBucket,
		upperBound: value,
	}
}

func parseHistogramBucketUpperBound(upperBound string) (float64, bool) {
	upperBound = strings.TrimSpace(upperBound)
	if upperBound == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(upperBound, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, -1) {
		return 0, false
	}
	return value, true
}

func sortInferredDimensions(in []InferredDimension) {
	if len(in) < 2 {
		return
	}
	histogramGroups := inferredHistogramBucketGroups(in)
	// Keys are unique per build, so the order is total and sort stability is irrelevant.
	slices.SortFunc(in, func(lhs, rhs InferredDimension) int {
		if c := cmp.Compare(lhs.ChartTemplateID, rhs.ChartTemplateID); c != 0 {
			return c
		}
		if c := cmp.Compare(lhs.DimensionIndex, rhs.DimensionIndex); c != 0 {
			return c
		}
		if histogramGroups[inferredDimensionGroup{
			chartTemplateID: lhs.ChartTemplateID,
			dimensionIndex:  lhs.DimensionIndex,
		}] {
			lhsBound, lhsOK := parseHistogramBucketUpperBound(lhs.Name)
			rhsBound, rhsOK := parseHistogramBucketUpperBound(rhs.Name)
			if lhsOK && rhsOK && lhsBound != rhsBound {
				return cmp.Compare(lhsBound, rhsBound)
			}
		}
		return cmp.Compare(lhs.Name, rhs.Name)
	})
}

type inferredDimensionGroup struct {
	chartTemplateID string
	dimensionIndex  int
}

type inferredHistogramGroupStats struct {
	total     int
	parseable int
	hasInf    bool
}

func inferredHistogramBucketGroups(in []InferredDimension) map[inferredDimensionGroup]bool {
	stats := make(map[inferredDimensionGroup]inferredHistogramGroupStats)
	for _, dim := range in {
		group := inferredDimensionGroup{
			chartTemplateID: dim.ChartTemplateID,
			dimensionIndex:  dim.DimensionIndex,
		}
		item := stats[group]
		item.total++
		if upperBound, ok := parseHistogramBucketUpperBound(dim.Name); ok {
			item.parseable++
			item.hasInf = item.hasInf || math.IsInf(upperBound, 1)
		}
		stats[group] = item
	}

	out := make(map[inferredDimensionGroup]bool, len(stats))
	for group, item := range stats {
		out[group] = item.total > 0 && item.parseable == item.total && item.hasInf
	}
	return out
}

func isAutogenTemplateID(templateID string) bool {
	return strings.HasPrefix(templateID, autogenTemplatePrefix)
}

func (e *Engine) nextBuildCycle(sourceSuccessSeq uint64) uint64 {
	if !e.state.cfg.runtimePlanner {
		return sourceSuccessSeq
	}
	e.state.plannerBuildSeq++
	// Seen-seq zero value is reserved for "never seen".
	if e.state.plannerBuildSeq == 0 {
		e.state.plannerBuildSeq = 1
	}
	return e.state.plannerBuildSeq
}
