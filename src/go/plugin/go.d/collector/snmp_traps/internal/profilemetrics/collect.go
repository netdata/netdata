// SPDX-License-Identifier: GPL-3.0-or-later

package profilemetrics

import (
	"slices"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
)

func (rt *Runtime) Collect(store metrix.CollectorStore, jobName string) {
	if rt == nil {
		return
	}
	now := time.Now()
	rt.mu.Lock()
	rt.collectCycle++
	rt.sweepLocked(now)
	series := make([]profileMetricSeriesSnapshot, 0, len(rt.series))
	for _, s := range rt.series {
		series = append(series, profileMetricSeriesSnapshot{
			rule:   s.rule,
			scope:  s.scope,
			labels: slices.Clone(s.labels),
			value:  s.value,
		})
	}
	diag := rt.diagnostics
	removed := false
	for _, s := range rt.series {
		if s.removeAfterCollect {
			delete(rt.series, s.key)
			removed = true
		}
	}
	if removed {
		rt.rebuildCardinalityIndexesLocked()
	}
	rt.mu.Unlock()

	for _, s := range series {
		rt.collectSeries(store, s)
	}
	collectProfileMetricDiagnostics(store, jobName, diag)
}

func (rt *Runtime) sweepLocked(now time.Time) {
	removed := false
	for key, series := range rt.series {
		rule := series.rule
		if rule == nil {
			delete(rt.series, key)
			removed = true
			continue
		}
		if rule.rule.Type == catalog.MetricTypeState && rule.stateTTL > 0 && now.Sub(series.lastUpdate) >= rule.stateTTL {
			series.value = rule.rule.StateClearValue()
			series.removeAfterCollect = true
			continue
		}
		if rule.expireAfterCycles > 0 && rt.collectCycle-series.lastCycle > uint64(rule.expireAfterCycles) {
			delete(rt.series, key)
			removed = true
		}
	}
	if removed {
		rt.rebuildCardinalityIndexesLocked()
	}
}

func (rt *Runtime) rebuildCardinalityIndexesLocked() {
	sources := make(map[string]time.Time, len(rt.sources))
	resources := make(map[string]map[string]struct{}, len(rt.resources))
	chartInstances := make(map[profileMetricChartInstanceKey]struct{}, len(rt.chartInstances))
	chartCounts := make(map[string]int, len(rt.chartCounts))
	for _, series := range rt.series {
		key := series.key
		if key.sourceKind != "" && key.sourceKind != "listener" && key.sourceID != "" && rt.cfg.limits.MaxSources != 0 {
			lastSeen := rt.sources[key.sourceID]
			if lastSeen.IsZero() || lastSeen.Before(series.lastUpdate) {
				lastSeen = series.lastUpdate
			}
			if existing := sources[key.sourceID]; existing.IsZero() || existing.Before(lastSeen) {
				sources[key.sourceID] = lastSeen
			}
		}
		if series.rule != nil && series.rule.chart != nil {
			instanceKey := profileMetricChartInstanceKey{
				chartID:       series.rule.chart.ID,
				scopeKey:      key.scopeKey,
				sourceID:      key.sourceID,
				sourceKind:    key.sourceKind,
				resourceClass: key.resourceClass,
				resourceID:    key.resourceID,
			}
			if _, ok := chartInstances[instanceKey]; !ok {
				chartInstances[instanceKey] = struct{}{}
				chartCounts[series.rule.chart.ID]++
			}
		}
		if key.sourceKind == "" || key.sourceID == "" || key.resourceClass == "" || key.resourceID == "" {
			continue
		}
		resourceKey := key.sourceKind + ":" + key.sourceID + ":" + key.resourceClass
		set := resources[resourceKey]
		if set == nil {
			set = make(map[string]struct{})
			resources[resourceKey] = set
		}
		set[key.resourceID] = struct{}{}
	}
	rt.sources = sources
	rt.resources = resources
	rt.chartInstances = chartInstances
	rt.chartCounts = chartCounts
	rt.pruneSourceRoutesLocked()
}

func (rt *Runtime) pruneSourceRoutesLocked() {
	rt.sourceRoutes.Prune()
}

func (rt *Runtime) collectSeries(store metrix.CollectorStore, series profileMetricSeriesSnapshot) {
	if series.rule == nil || series.rule.rule == nil {
		return
	}
	meter := store.Write().SnapshotMeter("").WithHostScope(series.scope).WithLabels(series.labels...)
	switch series.rule.rule.Type {
	case catalog.MetricTypeCounter:
		meter.Counter(series.rule.rule.Output.Metric).ObserveTotal(series.value)
	case catalog.MetricTypeSample, catalog.MetricTypeState:
		meter.Gauge(series.rule.rule.Output.Metric).Observe(series.value)
	}
}

func collectProfileMetricDiagnostics(store metrix.CollectorStore, jobName string, diag profileMetricDiagnostics) {
	meter := store.Write().SnapshotMeter("").WithLabels(metrix.Label{Key: "job_name", Value: jobName})
	meter.Counter("snmp_trap_profile_metrics_rule_missed").ObserveTotal(metrix.SampleValue(diag.ruleMissed))
	meter.Counter("snmp_trap_profile_metrics_extraction_failed").ObserveTotal(metrix.SampleValue(diag.extractionFailed))
	meter.Counter("snmp_trap_profile_metrics_attribution_failed").ObserveTotal(metrix.SampleValue(diag.attributionFailed))
	meter.Counter("snmp_trap_profile_metrics_overflow_dropped").ObserveTotal(metrix.SampleValue(diag.overflowDropped))
	meter.Counter("snmp_trap_profile_metrics_source_transitions").ObserveTotal(metrix.SampleValue(diag.sourceTransitions))
}
