// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

type runtimeMetrics struct {
	buildCalls             metrix.StatefulCounter
	buildErrors            metrix.StatefulCounter
	buildSkippedFailed     metrix.StatefulCounter
	buildDurationMS        metrix.StatefulSummary
	routeCacheHits         metrix.StatefulCounter
	routeCacheMisses       metrix.StatefulCounter
	seriesScanned          metrix.StatefulCounter
	seriesMatched          metrix.StatefulCounter
	seriesUnmatched        metrix.StatefulCounter
	seriesAutogenMatched   metrix.StatefulCounter
	planChartInstances     metrix.StatefulGauge
	planInferredDimensions metrix.StatefulGauge
	actionCreateChart      metrix.StatefulCounter
	actionCreateDimension  metrix.StatefulCounter
	actionUpdateChart      metrix.StatefulCounter
	actionRemoveDimension  metrix.StatefulCounter
	actionRemoveChart      metrix.StatefulCounter
}

type planRuntimeSample struct {
	startedAt time.Time

	buildErr      bool
	skippedFailed bool

	planRouteStats

	planChartInstances     int
	planInferredDimensions int

	actionCreateChart     int
	actionCreateDimension int
	actionUpdateChart     int
	actionRemoveDimension int
	actionRemoveChart     int
}

func newRuntimeMetrics(store metrix.RuntimeStore) *runtimeMetrics {
	if store == nil {
		return nil
	}
	meter := store.Write().StatefulMeter("netdata.go.plugin.chartengine")
	actions := meter.Vec("kind").Counter(
		"actions_total",
		metrix.WithDescription("Planner actions by kind"),
		metrix.WithChartFamily("Actions"),
		metrix.WithUnit("actions"),
	)
	return &runtimeMetrics{
		buildCalls: meter.Counter(
			"build_calls_total",
			metrix.WithDescription("Build plan calls"),
			metrix.WithChartFamily("Planner"),
			metrix.WithUnit("calls"),
		),
		buildErrors: meter.Counter(
			"build_errors_total",
			metrix.WithDescription("Build plan errors"),
			metrix.WithChartFamily("Planner"),
			metrix.WithUnit("errors"),
		),
		buildSkippedFailed: meter.Counter(
			"build_skipped_failed_collect_total",
			metrix.WithDescription("Builds skipped because collector cycle failed"),
			metrix.WithChartFamily("Planner"),
			metrix.WithUnit("skips"),
		),
		buildDurationMS: meter.Summary(
			"build_duration_ms",
			metrix.WithSummaryQuantiles(0.5, 0.9, 0.99),
			metrix.WithDescription("Build plan duration"),
			metrix.WithChartFamily("Planner"),
			metrix.WithUnit("ms"),
		),
		routeCacheHits: meter.Counter(
			"route_cache_hits_total",
			metrix.WithDescription("Route cache hits"),
			metrix.WithChartFamily("Route Cache"),
			metrix.WithUnit("hits"),
		),
		routeCacheMisses: meter.Counter(
			"route_cache_misses_total",
			metrix.WithDescription("Route cache misses"),
			metrix.WithChartFamily("Route Cache"),
			metrix.WithUnit("misses"),
		),
		seriesScanned: meter.Counter(
			"series_scanned_total",
			metrix.WithDescription("Metric series scanned"),
			metrix.WithChartFamily("Series"),
			metrix.WithUnit("series"),
		),
		seriesMatched: meter.Counter(
			"series_matched_total",
			metrix.WithDescription("Metric series matched by templates or autogen"),
			metrix.WithChartFamily("Series"),
			metrix.WithUnit("series"),
		),
		seriesUnmatched: meter.Counter(
			"series_unmatched_total",
			metrix.WithDescription("Metric series left unmatched after routing"),
			metrix.WithChartFamily("Series"),
			metrix.WithUnit("series"),
		),
		seriesAutogenMatched: meter.Counter(
			"series_autogen_matched_total",
			metrix.WithDescription("Metric series matched by autogen fallback"),
			metrix.WithChartFamily("Series"),
			metrix.WithUnit("series"),
		),
		planChartInstances: meter.Gauge(
			"plan_chart_instances",
			metrix.WithDescription("Chart instances produced by latest build plan"),
			metrix.WithChartFamily("Plan"),
			metrix.WithUnit("charts"),
		),
		planInferredDimensions: meter.Gauge(
			"plan_inferred_dimensions",
			metrix.WithDescription("Inferred dimensions produced by latest build plan"),
			metrix.WithChartFamily("Plan"),
			metrix.WithUnit("dimensions"),
		),
		actionCreateChart:     actions.WithLabelValues("create_chart"),
		actionCreateDimension: actions.WithLabelValues("create_dimension"),
		actionUpdateChart:     actions.WithLabelValues("update_chart"),
		actionRemoveDimension: actions.WithLabelValues("remove_dimension"),
		actionRemoveChart:     actions.WithLabelValues("remove_chart"),
	}
}

func (m *runtimeMetrics) observeBuild(sample planRuntimeSample) {
	if m == nil {
		return
	}
	if sample.startedAt.IsZero() {
		sample.startedAt = time.Now()
	}

	m.buildCalls.Add(1)
	if sample.skippedFailed {
		m.buildSkippedFailed.Add(1)
	}
	if sample.buildErr {
		m.buildErrors.Add(1)
	}
	m.buildDurationMS.Observe(float64(time.Since(sample.startedAt).Milliseconds()))
	m.routeCacheHits.Add(float64(sample.routeCacheHits))
	m.routeCacheMisses.Add(float64(sample.routeCacheMisses))
	m.seriesScanned.Add(float64(sample.seriesScanned))
	m.seriesMatched.Add(float64(sample.seriesMatched))
	m.seriesUnmatched.Add(float64(sample.seriesUnmatched))
	m.seriesAutogenMatched.Add(float64(sample.seriesAutogenMatched))
	m.planChartInstances.Set(float64(sample.planChartInstances))
	m.planInferredDimensions.Set(float64(sample.planInferredDimensions))

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
