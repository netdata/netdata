// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
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

type planRuntimeSample struct {
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
			metrix.WithDescription("Current number of route cache entries"),
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
			metrix.WithDescription("Chart instances in last successful build plan"),
			metrix.WithChartFamily("ChartEngine/Plan"),
			metrix.WithUnit("charts"),
		),
		planInferredDimensions: metrix.SeededGauge(meter,
			"plan_inferred_dimensions",
			metrix.WithDescription("Inferred dimensions in last successful build plan"),
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

func (m *runtimeMetrics) observeBuild(sample planRuntimeSample) {
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

func (e *Engine) observeBuildSample(sample planRuntimeSample) {
	if e == nil {
		return
	}
	if e.state.runtimeStats == nil {
		return
	}
	e.state.runtimeStats.observeBuild(sample)
}

func actionKindCounts(actions []EngineAction) planRuntimeSample {
	out := planRuntimeSample{}
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
