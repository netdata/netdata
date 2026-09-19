// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

type sourceDimension struct{ metric, dimension string }

// enforceContextLimits counts complete rollups before deciding which contexts to render.
func (e *Engine) enforceContextLimits(ctx *planBuildContext) {
	maxSeries, maxPerMetric := e.state.cfg.maxTimeSeries, e.state.cfg.maxTimeSeriesPerMetric
	if maxSeries <= 0 && maxPerMetric <= 0 {
		return
	}
	type cardinality struct {
		series  int
		metrics map[string]int
		reject  PlanRouteDiagnostic
	}
	contexts := make(map[string]*cardinality)
	for _, chart := range ctx.chartsByID {
		count := contexts[chart.meta.Context]
		if count == nil {
			count = &cardinality{metrics: make(map[string]int)}
			contexts[chart.meta.Context] = count
		}
		count.series += chart.observedCount
		for source := range chart.sourceDimensions {
			count.metrics[source.metric]++
		}
	}
	for context, count := range contexts {
		if maxSeries > 0 && count.series > maxSeries {
			count.reject = PlanRouteDiagnostic{
				Reason: PlanRouteReasonContextSeriesCap, SeriesCount: count.series, SeriesLimit: maxSeries,
			}
		} else if maxPerMetric > 0 {
			for metric, series := range count.metrics {
				if series > maxPerMetric && (count.reject.MetricFamilyName == "" || metric < count.reject.MetricFamilyName) {
					count.reject = PlanRouteDiagnostic{
						Reason: PlanRouteReasonContextMetricSeriesCap, MetricFamilyName: metric,
						SeriesCount: series, SeriesLimit: maxPerMetric,
					}
				}
			}
		}
		if count.reject.Reason != "" {
			e.logDebugf("skip context %q: %s (metric=%q, series=%d, limit=%d)",
				context, count.reject.Reason, count.reject.MetricFamilyName, count.reject.SeriesCount, count.reject.SeriesLimit)
		}
	}
	for chartID, chart := range ctx.chartsByID {
		fact := contexts[chart.meta.Context].reject
		if fact.Reason == "" {
			continue
		}
		delete(ctx.chartsByID, chartID)
		fact.Decision = PlanRouteLifecycleRejected
		fact.Context = chart.meta.Context
		fact.ChartID = chartID
		fact.ChartTemplateID = chart.templateID
		fact.Autogen = isAutogenTemplateID(chart.templateID)
		ctx.observeRouteDiagnostic(fact)
	}
}
