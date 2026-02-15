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

	routeCacheHits       uint64
	routeCacheMisses     uint64
	seriesScanned        uint64
	seriesMatched        uint64
	seriesUnmatched      uint64
	seriesAutogenMatched uint64

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
	meter := store.Write().StatefulMeter("chartengine")
	actions := meter.CounterVec("actions_total", []string{"kind"})
	return &runtimeMetrics{
		buildCalls:             meter.Counter("build_calls_total"),
		buildErrors:            meter.Counter("build_errors_total"),
		buildSkippedFailed:     meter.Counter("build_skipped_failed_collect_total"),
		buildDurationMS:        meter.Summary("build_duration_ms", metrix.WithSummaryQuantiles(0.5, 0.9, 0.99)),
		routeCacheHits:         meter.Counter("route_cache_hits_total"),
		routeCacheMisses:       meter.Counter("route_cache_misses_total"),
		seriesScanned:          meter.Counter("series_scanned_total"),
		seriesMatched:          meter.Counter("series_matched_total"),
		seriesUnmatched:        meter.Counter("series_unmatched_total"),
		seriesAutogenMatched:   meter.Counter("series_autogen_matched_total"),
		planChartInstances:     meter.Gauge("plan_chart_instances"),
		planInferredDimensions: meter.Gauge("plan_inferred_dimensions"),
		actionCreateChart:      actions.WithLabelValues("create_chart"),
		actionCreateDimension:  actions.WithLabelValues("create_dimension"),
		actionUpdateChart:      actions.WithLabelValues("update_chart"),
		actionRemoveDimension:  actions.WithLabelValues("remove_dimension"),
		actionRemoveChart:      actions.WithLabelValues("remove_chart"),
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
