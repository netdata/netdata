// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

type runtimeMetrics struct {
	buildSuccessTotal        metrix.StatefulCounter
	buildErrorTotal          metrix.StatefulCounter
	buildSkippedFailedTotal  metrix.StatefulCounter
	buildDurationSeconds     metrix.StatefulSummary
	buildPhasePrepareSec     metrix.StatefulSummary
	buildPhaseValidateSec    metrix.StatefulSummary
	buildPhaseScanSec        metrix.StatefulSummary
	buildPhaseRetainSec      metrix.StatefulSummary
	buildPhaseCapsSec        metrix.StatefulSummary
	buildPhaseMaterializeSec metrix.StatefulSummary
	buildPhaseExpirySec      metrix.StatefulSummary
	buildPhaseSortSec        metrix.StatefulSummary

	buildSeqBrokenTotal    metrix.StatefulCounter
	buildSeqRecoveredTotal metrix.StatefulCounter
	buildSeqViolation      metrix.StatefulGauge

	routeCacheHitsTotal      metrix.StatefulCounter
	routeCacheMissesTotal    metrix.StatefulCounter
	routeCacheEntries        metrix.StatefulGauge
	routeCacheRetainedTotal  metrix.StatefulCounter
	routeCachePrunedTotal    metrix.StatefulCounter
	routeCacheFullDropsTotal metrix.StatefulCounter

	seriesScannedTotal        metrix.StatefulCounter
	seriesMatchedTotal        metrix.StatefulCounter
	seriesUnmatchedTotal      metrix.StatefulCounter
	seriesAutogenMatchedTotal metrix.StatefulCounter
	seriesFilteredBySeq       metrix.StatefulCounter
	seriesFilteredBySelector  metrix.StatefulCounter

	planChartInstances     metrix.StatefulGauge
	planInferredDimensions metrix.StatefulGauge

	actionCreateChart     metrix.StatefulCounter
	actionCreateDimension metrix.StatefulCounter
	actionUpdateChart     metrix.StatefulCounter
	actionRemoveDimension metrix.StatefulCounter
	actionRemoveChart     metrix.StatefulCounter

	lifecycleRemovedChartByCap        metrix.StatefulCounter
	lifecycleRemovedChartByExpiry     metrix.StatefulCounter
	lifecycleRemovedDimensionByCap    metrix.StatefulCounter
	lifecycleRemovedDimensionByExpiry metrix.StatefulCounter
}

// PlanRuntimeSample is an opaque snapshot of one planner build used by
// chartengine runtime observers and RuntimeAggregator. Its fields are
// intentionally package-private so chartengine remains the owner of runtime
// metric semantics while callers can pass samples between chartengine APIs.
type PlanRuntimeSample struct {
	startedAt time.Time

	buildErr      bool
	skippedFailed bool
	buildSuccess  bool

	planRouteStats

	planChartInstances     int
	planInferredDimensions int

	actionCreateChart     int
	actionCreateDimension int
	actionUpdateChart     int
	actionRemoveDimension int
	actionRemoveChart     int

	lifecycleRemovedChartByCap        int
	lifecycleRemovedChartByExpiry     int
	lifecycleRemovedDimensionByCap    int
	lifecycleRemovedDimensionByExpiry int

	routeCacheEntries  int
	routeCacheRetained int
	routeCachePruned   int
	routeCacheFullDrop bool

	buildSeqBroken    bool
	buildSeqRecovered bool
	buildSeqViolation bool
	buildSeqObserved  bool

	phasePrepareSeconds     float64
	phaseValidateSeconds    float64
	phaseScanSeconds        float64
	phaseRetainSeconds      float64
	phaseLifecycleCapsSec   float64
	phaseMaterializeSeconds float64
	phaseExpirySeconds      float64
	phaseSortSeconds        float64
}

func newRuntimeMetrics(store metrix.RuntimeStore) *runtimeMetrics {
	if store == nil {
		return nil
	}
	meter := store.Write().StatefulMeter("netdata.go.plugin.framework.chartengine")
	phaseDuration := meter.Vec("phase").Summary(
		"build_phase_duration_seconds",
		metrix.WithSummaryQuantiles(0.5, 0.9, 0.99),
		metrix.WithDescription("Build phase duration in seconds"),
		metrix.WithChartFamily("ChartEngine/Build"),
		metrix.WithUnit("seconds"),
	)
	actions := meter.Vec("kind").Counter(
		"planner_actions_total",
		metrix.WithDescription("Planner actions by kind"),
		metrix.WithChartFamily("ChartEngine/Actions"),
		metrix.WithUnit("actions"),
	)
	lifecycleRemoved := meter.Vec("scope", "reason").Counter(
		"lifecycle_removed_total",
		metrix.WithDescription("Lifecycle removals by scope and reason"),
		metrix.WithChartFamily("ChartEngine/Lifecycle"),
		metrix.WithUnit("removals"),
	)
	seriesFiltered := meter.Vec("reason").Counter(
		"series_filtered_total",
		metrix.WithDescription("Series filtered before routing by reason"),
		metrix.WithChartFamily("ChartEngine/Series"),
		metrix.WithUnit("series"),
	)
	buildSeqTransitions := meter.Vec("transition").Counter(
		"build_seq_transitions_total",
		metrix.WithDescription("Build sequence monotonicity transitions"),
		metrix.WithChartFamily("ChartEngine/Build"),
		metrix.WithUnit("transitions"),
	)
	return &runtimeMetrics{
		buildSuccessTotal: metrix.SeededCounter(meter,
			"build_success_total",
			metrix.WithDescription("Successful BuildPlan calls"),
			metrix.WithChartFamily("ChartEngine/Build"),
			metrix.WithUnit("builds"),
		),
		buildErrorTotal: metrix.SeededCounter(meter,
			"build_error_total",
			metrix.WithDescription("Failed BuildPlan calls"),
			metrix.WithChartFamily("ChartEngine/Build"),
			metrix.WithUnit("builds"),
		),
		buildSkippedFailedTotal: metrix.SeededCounter(meter,
			"build_skipped_failed_collect_total",
			metrix.WithDescription("BuildPlan calls skipped due failed collect cycle"),
			metrix.WithChartFamily("ChartEngine/Build"),
			metrix.WithUnit("builds"),
		),
		buildDurationSeconds: meter.Summary(
			"build_duration_seconds",
			metrix.WithSummaryQuantiles(0.5, 0.9, 0.99),
			metrix.WithDescription("BuildPlan duration in seconds"),
			metrix.WithChartFamily("ChartEngine/Build"),
			metrix.WithUnit("seconds"),
		),
		buildPhasePrepareSec:     phaseDuration.WithLabelValues("prepare"),
		buildPhaseValidateSec:    phaseDuration.WithLabelValues("validate_reader"),
		buildPhaseScanSec:        phaseDuration.WithLabelValues("scan"),
		buildPhaseRetainSec:      phaseDuration.WithLabelValues("cache_retain"),
		buildPhaseCapsSec:        phaseDuration.WithLabelValues("lifecycle_caps"),
		buildPhaseMaterializeSec: phaseDuration.WithLabelValues("materialize"),
		buildPhaseExpirySec:      phaseDuration.WithLabelValues("expiry"),
		buildPhaseSortSec:        phaseDuration.WithLabelValues("sort_inferred"),

		buildSeqBrokenTotal:    buildSeqTransitions.WithLabelValues("broken"),
		buildSeqRecoveredTotal: buildSeqTransitions.WithLabelValues("recovered"),
		buildSeqViolation: metrix.SeededGauge(meter,
			"build_seq_violation_active",
			metrix.WithDescription("1 when build sequence monotonicity is currently violated"),
			metrix.WithChartFamily("ChartEngine/Build"),
			metrix.WithUnit("state"),
		),

		routeCacheHitsTotal: metrix.SeededCounter(meter,
			"route_cache_hits_total",
			metrix.WithDescription("Route cache lookup hits"),
			metrix.WithChartFamily("ChartEngine/Route Cache"),
			metrix.WithUnit("hits"),
		),
		routeCacheMissesTotal: metrix.SeededCounter(meter,
			"route_cache_misses_total",
			metrix.WithDescription("Route cache lookup misses"),
			metrix.WithChartFamily("ChartEngine/Route Cache"),
			metrix.WithUnit("misses"),
		),
		routeCacheEntries: metrix.SeededGauge(meter,
			"route_cache_entries",
			metrix.WithDescription("Route cache entries in the latest successful build rollup"),
			metrix.WithChartFamily("ChartEngine/Route Cache"),
			metrix.WithUnit("entries"),
		),
		routeCacheRetainedTotal: metrix.SeededCounter(meter,
			"route_cache_retained_total",
			metrix.WithDescription("Route cache entries retained after prune"),
			metrix.WithChartFamily("ChartEngine/Route Cache"),
			metrix.WithUnit("entries"),
		),
		routeCachePrunedTotal: metrix.SeededCounter(meter,
			"route_cache_pruned_total",
			metrix.WithDescription("Route cache entries pruned"),
			metrix.WithChartFamily("ChartEngine/Route Cache"),
			metrix.WithUnit("entries"),
		),
		routeCacheFullDropsTotal: metrix.SeededCounter(meter,
			"route_cache_full_drops_total",
			metrix.WithDescription("Route cache full-drop prune events"),
			metrix.WithChartFamily("ChartEngine/Route Cache"),
			metrix.WithUnit("events"),
		),

		seriesScannedTotal: metrix.SeededCounter(meter,
			"series_scanned_total",
			metrix.WithDescription("Series scanned by planner"),
			metrix.WithChartFamily("ChartEngine/Series"),
			metrix.WithUnit("series"),
		),
		seriesMatchedTotal: metrix.SeededCounter(meter,
			"series_matched_total",
			metrix.WithDescription("Series matched by template or autogen"),
			metrix.WithChartFamily("ChartEngine/Series"),
			metrix.WithUnit("series"),
		),
		seriesUnmatchedTotal: metrix.SeededCounter(meter,
			"series_unmatched_total",
			metrix.WithDescription("Series left unmatched after routing"),
			metrix.WithChartFamily("ChartEngine/Series"),
			metrix.WithUnit("series"),
		),
		seriesAutogenMatchedTotal: metrix.SeededCounter(meter,
			"series_autogen_matched_total",
			metrix.WithDescription("Series matched by autogen fallback"),
			metrix.WithChartFamily("ChartEngine/Series"),
			metrix.WithUnit("series"),
		),
		seriesFilteredBySeq:      seriesFiltered.WithLabelValues("by_seq"),
		seriesFilteredBySelector: seriesFiltered.WithLabelValues("by_selector"),

		planChartInstances: metrix.SeededGauge(meter,
			"plan_chart_instances",
			metrix.WithDescription("Chart instances in the latest successful build rollup"),
			metrix.WithChartFamily("ChartEngine/Plan"),
			metrix.WithUnit("charts"),
		),
		planInferredDimensions: metrix.SeededGauge(meter,
			"plan_inferred_dimensions",
			metrix.WithDescription("Inferred dimensions in the latest successful build rollup"),
			metrix.WithChartFamily("ChartEngine/Plan"),
			metrix.WithUnit("dimensions"),
		),

		actionCreateChart:     actions.WithLabelValues("create_chart"),
		actionCreateDimension: actions.WithLabelValues("create_dimension"),
		actionUpdateChart:     actions.WithLabelValues("update_chart"),
		actionRemoveDimension: actions.WithLabelValues("remove_dimension"),
		actionRemoveChart:     actions.WithLabelValues("remove_chart"),

		lifecycleRemovedChartByCap:        lifecycleRemoved.WithLabelValues("chart", "cap"),
		lifecycleRemovedChartByExpiry:     lifecycleRemoved.WithLabelValues("chart", "expiry"),
		lifecycleRemovedDimensionByCap:    lifecycleRemoved.WithLabelValues("dimension", "cap"),
		lifecycleRemovedDimensionByExpiry: lifecycleRemoved.WithLabelValues("dimension", "expiry"),
	}
}

func (m *runtimeMetrics) observeBuild(sample PlanRuntimeSample) {
	if m == nil {
		return
	}
	if sample.startedAt.IsZero() {
		sample.startedAt = time.Now()
	}

	switch {
	case sample.buildSuccess:
		m.buildSuccessTotal.Add(1)
	case sample.skippedFailed:
		m.buildSkippedFailedTotal.Add(1)
	case sample.buildErr:
		m.buildErrorTotal.Add(1)
	}

	m.buildDurationSeconds.Observe(time.Since(sample.startedAt).Seconds())
	observeDurationSeconds(m.buildPhasePrepareSec, sample.phasePrepareSeconds)
	observeDurationSeconds(m.buildPhaseValidateSec, sample.phaseValidateSeconds)
	observeDurationSeconds(m.buildPhaseScanSec, sample.phaseScanSeconds)
	observeDurationSeconds(m.buildPhaseRetainSec, sample.phaseRetainSeconds)
	observeDurationSeconds(m.buildPhaseCapsSec, sample.phaseLifecycleCapsSec)
	observeDurationSeconds(m.buildPhaseMaterializeSec, sample.phaseMaterializeSeconds)
	observeDurationSeconds(m.buildPhaseExpirySec, sample.phaseExpirySeconds)
	observeDurationSeconds(m.buildPhaseSortSec, sample.phaseSortSeconds)

	if sample.buildSeqObserved {
		if sample.buildSeqBroken {
			m.buildSeqBrokenTotal.Add(1)
		}
		if sample.buildSeqRecovered {
			m.buildSeqRecoveredTotal.Add(1)
		}
		if sample.buildSeqViolation {
			m.buildSeqViolation.Set(1)
		} else {
			m.buildSeqViolation.Set(0)
		}
	}

	m.routeCacheHitsTotal.Add(float64(sample.routeCacheHits))
	m.routeCacheMissesTotal.Add(float64(sample.routeCacheMisses))
	if sample.routeCacheRetained > 0 {
		m.routeCacheRetainedTotal.Add(float64(sample.routeCacheRetained))
	}
	if sample.routeCachePruned > 0 {
		m.routeCachePrunedTotal.Add(float64(sample.routeCachePruned))
	}
	if sample.routeCacheFullDrop {
		m.routeCacheFullDropsTotal.Add(1)
	}
	if sample.buildSuccess {
		m.routeCacheEntries.Set(float64(sample.routeCacheEntries))
	}

	m.seriesScannedTotal.Add(float64(sample.seriesScanned))
	m.seriesMatchedTotal.Add(float64(sample.seriesMatched))
	m.seriesUnmatchedTotal.Add(float64(sample.seriesUnmatched))
	m.seriesAutogenMatchedTotal.Add(float64(sample.seriesAutogenMatched))
	if sample.seriesFilteredBySeq > 0 {
		m.seriesFilteredBySeq.Add(float64(sample.seriesFilteredBySeq))
	}
	if sample.seriesFilteredBySel > 0 {
		m.seriesFilteredBySelector.Add(float64(sample.seriesFilteredBySel))
	}

	if sample.actionCreateChart > 0 {
		m.actionCreateChart.Add(float64(sample.actionCreateChart))
	}
	if sample.actionCreateDimension > 0 {
		m.actionCreateDimension.Add(float64(sample.actionCreateDimension))
	}
	if sample.actionUpdateChart > 0 {
		m.actionUpdateChart.Add(float64(sample.actionUpdateChart))
	}
	if sample.actionRemoveDimension > 0 {
		m.actionRemoveDimension.Add(float64(sample.actionRemoveDimension))
	}
	if sample.actionRemoveChart > 0 {
		m.actionRemoveChart.Add(float64(sample.actionRemoveChart))
	}

	if sample.lifecycleRemovedChartByCap > 0 {
		m.lifecycleRemovedChartByCap.Add(float64(sample.lifecycleRemovedChartByCap))
	}
	if sample.lifecycleRemovedChartByExpiry > 0 {
		m.lifecycleRemovedChartByExpiry.Add(float64(sample.lifecycleRemovedChartByExpiry))
	}
	if sample.lifecycleRemovedDimensionByCap > 0 {
		m.lifecycleRemovedDimensionByCap.Add(float64(sample.lifecycleRemovedDimensionByCap))
	}
	if sample.lifecycleRemovedDimensionByExpiry > 0 {
		m.lifecycleRemovedDimensionByExpiry.Add(float64(sample.lifecycleRemovedDimensionByExpiry))
	}

	if sample.buildSuccess {
		m.planChartInstances.Set(float64(sample.planChartInstances))
		m.planInferredDimensions.Set(float64(sample.planInferredDimensions))
	}
}

func observeDurationSeconds(metric metrix.StatefulSummary, value float64) {
	if value <= 0 {
		return
	}
	metric.Observe(value)
}

func (e *Engine) RuntimeStore() metrix.RuntimeStore {
	if e == nil {
		return nil
	}
	return e.state.runtimeStore
}

func (e *Engine) observeBuildSample(sample PlanRuntimeSample) {
	if e == nil {
		return
	}
	if e.state.cfg.runtimeObserver != nil {
		e.state.cfg.runtimeObserver(sample)
	}
	if e.state.runtimeStats == nil {
		return
	}
	e.state.runtimeStats.observeBuild(sample)
}

// RuntimeAggregator records runtime samples from multiple engines and emits one
// rolled-up chartengine runtime stream.
type RuntimeAggregator struct {
	mu      sync.Mutex
	metrics *runtimeMetrics
	samples []PlanRuntimeSample
}

// NewRuntimeAggregator creates a runtime sample aggregator backed by store.
func NewRuntimeAggregator(store metrix.RuntimeStore) *RuntimeAggregator {
	return &RuntimeAggregator{metrics: newRuntimeMetrics(store)}
}

// Observe records one engine build sample.
func (a *RuntimeAggregator) Observe(sample PlanRuntimeSample) {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.samples = append(a.samples, sample)
	a.mu.Unlock()
}

// Reset drops accumulated samples without writing them.
func (a *RuntimeAggregator) Reset() {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.samples = nil
	a.mu.Unlock()
}

// Flush emits accumulated samples into the aggregate runtime store.
func (a *RuntimeAggregator) Flush() {
	if a == nil {
		return
	}
	a.mu.Lock()
	samples := a.samples
	a.samples = nil
	a.mu.Unlock()
	if len(samples) == 0 || a.metrics == nil {
		return
	}
	a.metrics.observeBuildRollup(samples)
}

func (m *runtimeMetrics) observeBuildRollup(samples []PlanRuntimeSample) {
	if m == nil || len(samples) == 0 {
		return
	}

	var buildSuccess, skippedFailed, buildErr int
	var routeCacheHits, routeCacheMisses uint64
	var routeCacheRetained, routeCachePruned, routeCacheFullDrops int
	var seriesScanned, seriesMatched, seriesUnmatched, seriesAutogenMatched uint64
	var seriesFilteredBySeq, seriesFilteredBySel uint64
	var actionCreateChart, actionCreateDimension, actionUpdateChart, actionRemoveDimension, actionRemoveChart int
	var lifecycleRemovedChartByCap, lifecycleRemovedChartByExpiry int
	var lifecycleRemovedDimensionByCap, lifecycleRemovedDimensionByExpiry int
	var routeCacheEntries, planChartInstances, planInferredDimensions int
	var buildSeqBroken, buildSeqRecovered int
	var buildSeqViolation, buildSeqObserved bool

	for _, sample := range samples {
		if sample.startedAt.IsZero() {
			sample.startedAt = time.Now()
		}
		switch {
		case sample.buildSuccess:
			buildSuccess++
		case sample.skippedFailed:
			skippedFailed++
		case sample.buildErr:
			buildErr++
		}

		m.buildDurationSeconds.Observe(time.Since(sample.startedAt).Seconds())
		observeDurationSeconds(m.buildPhasePrepareSec, sample.phasePrepareSeconds)
		observeDurationSeconds(m.buildPhaseValidateSec, sample.phaseValidateSeconds)
		observeDurationSeconds(m.buildPhaseScanSec, sample.phaseScanSeconds)
		observeDurationSeconds(m.buildPhaseRetainSec, sample.phaseRetainSeconds)
		observeDurationSeconds(m.buildPhaseCapsSec, sample.phaseLifecycleCapsSec)
		observeDurationSeconds(m.buildPhaseMaterializeSec, sample.phaseMaterializeSeconds)
		observeDurationSeconds(m.buildPhaseExpirySec, sample.phaseExpirySeconds)
		observeDurationSeconds(m.buildPhaseSortSec, sample.phaseSortSeconds)

		if sample.buildSeqObserved {
			buildSeqObserved = true
			if sample.buildSeqBroken {
				buildSeqBroken++
			}
			if sample.buildSeqRecovered {
				buildSeqRecovered++
			}
			if sample.buildSeqViolation {
				buildSeqViolation = true
			}
		}

		routeCacheHits += sample.routeCacheHits
		routeCacheMisses += sample.routeCacheMisses
		routeCacheRetained += sample.routeCacheRetained
		routeCachePruned += sample.routeCachePruned
		if sample.routeCacheFullDrop {
			routeCacheFullDrops++
		}
		if sample.buildSuccess {
			routeCacheEntries += sample.routeCacheEntries
			planChartInstances += sample.planChartInstances
			planInferredDimensions += sample.planInferredDimensions
		}

		seriesScanned += sample.seriesScanned
		seriesMatched += sample.seriesMatched
		seriesUnmatched += sample.seriesUnmatched
		seriesAutogenMatched += sample.seriesAutogenMatched
		seriesFilteredBySeq += sample.seriesFilteredBySeq
		seriesFilteredBySel += sample.seriesFilteredBySel

		actionCreateChart += sample.actionCreateChart
		actionCreateDimension += sample.actionCreateDimension
		actionUpdateChart += sample.actionUpdateChart
		actionRemoveDimension += sample.actionRemoveDimension
		actionRemoveChart += sample.actionRemoveChart

		lifecycleRemovedChartByCap += sample.lifecycleRemovedChartByCap
		lifecycleRemovedChartByExpiry += sample.lifecycleRemovedChartByExpiry
		lifecycleRemovedDimensionByCap += sample.lifecycleRemovedDimensionByCap
		lifecycleRemovedDimensionByExpiry += sample.lifecycleRemovedDimensionByExpiry
	}

	addIfPositive(m.buildSuccessTotal, float64(buildSuccess))
	addIfPositive(m.buildSkippedFailedTotal, float64(skippedFailed))
	addIfPositive(m.buildErrorTotal, float64(buildErr))
	addIfPositive(m.buildSeqBrokenTotal, float64(buildSeqBroken))
	addIfPositive(m.buildSeqRecoveredTotal, float64(buildSeqRecovered))
	if buildSeqObserved {
		if buildSeqViolation {
			m.buildSeqViolation.Set(1)
		} else {
			m.buildSeqViolation.Set(0)
		}
	} else {
		m.buildSeqViolation.Set(0)
	}

	addIfPositive(m.routeCacheHitsTotal, float64(routeCacheHits))
	addIfPositive(m.routeCacheMissesTotal, float64(routeCacheMisses))
	addIfPositive(m.routeCacheRetainedTotal, float64(routeCacheRetained))
	addIfPositive(m.routeCachePrunedTotal, float64(routeCachePruned))
	addIfPositive(m.routeCacheFullDropsTotal, float64(routeCacheFullDrops))
	if buildSuccess > 0 {
		m.routeCacheEntries.Set(float64(routeCacheEntries))
	}

	addIfPositive(m.seriesScannedTotal, float64(seriesScanned))
	addIfPositive(m.seriesMatchedTotal, float64(seriesMatched))
	addIfPositive(m.seriesUnmatchedTotal, float64(seriesUnmatched))
	addIfPositive(m.seriesAutogenMatchedTotal, float64(seriesAutogenMatched))
	addIfPositive(m.seriesFilteredBySeq, float64(seriesFilteredBySeq))
	addIfPositive(m.seriesFilteredBySelector, float64(seriesFilteredBySel))

	addIfPositive(m.actionCreateChart, float64(actionCreateChart))
	addIfPositive(m.actionCreateDimension, float64(actionCreateDimension))
	addIfPositive(m.actionUpdateChart, float64(actionUpdateChart))
	addIfPositive(m.actionRemoveDimension, float64(actionRemoveDimension))
	addIfPositive(m.actionRemoveChart, float64(actionRemoveChart))

	addIfPositive(m.lifecycleRemovedChartByCap, float64(lifecycleRemovedChartByCap))
	addIfPositive(m.lifecycleRemovedChartByExpiry, float64(lifecycleRemovedChartByExpiry))
	addIfPositive(m.lifecycleRemovedDimensionByCap, float64(lifecycleRemovedDimensionByCap))
	addIfPositive(m.lifecycleRemovedDimensionByExpiry, float64(lifecycleRemovedDimensionByExpiry))

	if buildSuccess > 0 {
		m.planChartInstances.Set(float64(planChartInstances))
		m.planInferredDimensions.Set(float64(planInferredDimensions))
	}
}

func addIfPositive(metric metrix.StatefulCounter, value float64) {
	if value > 0 {
		metric.Add(value)
	}
}

func actionKindCounts(actions []EngineAction) PlanRuntimeSample {
	out := PlanRuntimeSample{}
	for _, action := range actions {
		switch action.Kind() {
		case ActionCreateChart:
			out.actionCreateChart++
		case ActionCreateDimension:
			out.actionCreateDimension++
		case ActionUpdateChart:
			out.actionUpdateChart++
		case ActionRemoveDimension:
			out.actionRemoveDimension++
		case ActionRemoveChart:
			out.actionRemoveChart++
		}
	}
	return out
}
