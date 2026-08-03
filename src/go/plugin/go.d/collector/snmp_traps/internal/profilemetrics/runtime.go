// SPDX-License-Identifier: GPL-3.0-or-later

package profilemetrics

import (
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/attribution"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
)

type Runtime struct {
	mu sync.Mutex

	cfg        Policy
	rules      []*compiledProfileMetricRule
	rulesByOID map[string][]*compiledProfileMetricRule

	series         map[profileMetricSeriesKey]*profileMetricSeries
	sources        map[string]time.Time
	sourceRoutes   *attribution.RouteTracker
	resources      map[string]map[string]struct{}
	chartInstances map[profileMetricChartInstanceKey]struct{}
	chartCounts    map[string]int
	collectCycle   uint64
	sourceHashSalt string
	chartTemplate  string

	diagnostics profileMetricDiagnostics
}

type profileMetricDiagnostics struct {
	ruleMissed        uint64
	extractionFailed  uint64
	attributionFailed uint64
	overflowDropped   uint64
	sourceTransitions uint64
}

type compiledProfileMetricRule struct {
	rule              *profileMetricRule
	trapOIDs          map[string]*catalog.TrapDef
	problemOIDs       map[string]*catalog.TrapDef
	clearOIDs         map[string]*catalog.TrapDef
	chart             *profileMetricChart
	valueVarbind      *catalog.VarbindDef
	resourceVarbind   *catalog.VarbindDef
	stateTTL          time.Duration
	expireAfterCycles int
}

type profileMetricSeriesKey struct {
	ruleName      string
	scopeKey      string
	sourceID      string
	sourceKind    string
	resourceClass string
	resourceID    string
}

type profileMetricChartInstanceKey struct {
	chartID       string
	scopeKey      string
	sourceID      string
	sourceKind    string
	resourceClass string
	resourceID    string
}

type profileMetricSeries struct {
	key                profileMetricSeriesKey
	rule               *compiledProfileMetricRule
	scope              metrix.HostScope
	labels             []metrix.Label
	value              float64
	lastUpdate         time.Time
	lastCycle          uint64
	removeAfterCollect bool
}

type profileMetricSeriesSnapshot struct {
	rule   *compiledProfileMetricRule
	scope  metrix.HostScope
	labels []metrix.Label
	value  float64
}

func (rt *Runtime) Update(entry *model.TrapEntry) {
	if rt == nil || entry == nil {
		return
	}
	rules := rt.rulesByOID[entry.TrapOID]
	if len(rules) == 0 {
		return
	}
	now := time.Now()
	rt.mu.Lock()
	defer rt.mu.Unlock()
	for _, rule := range rules {
		rt.updateRuleLocked(rule, entry, now)
	}
}

func (rt *Runtime) updateRuleLocked(rule *compiledProfileMetricRule, entry *model.TrapEntry, now time.Time) {
	td := rule.trapDefForOID(entry.TrapOID)
	if td == nil {
		rt.diagnostics.ruleMissed++
		return
	}
	if !profileMetricPredicatesMatch(rule.rule.Where, entry, td) {
		rt.diagnostics.ruleMissed++
		return
	}
	if rule.rule.Type == catalog.MetricTypeState && len(rule.trapOIDs) > 0 {
		stateValue, matched := rule.sameOIDStateValue(entry, td)
		if !matched {
			rt.diagnostics.ruleMissed++
			return
		}
		rt.setSeriesValueLocked(rule, entry, td, stateValue, now)
		return
	}
	switch rule.rule.Type {
	case catalog.MetricTypeCounter:
		rt.addCounterLocked(rule, entry, td, now)
	case catalog.MetricTypeSample:
		val, status := profileMetricNumericVarbindValue(entry, rule.valueVarbind)
		if status != profileMetricValueOK {
			if status == profileMetricValueMissing && rule.rule.Missing == catalog.MetricMissingZero {
				rt.setSeriesValueLocked(rule, entry, td, 0, now)
				return
			}
			if status == profileMetricValueMissing && rule.rule.Missing == catalog.MetricMissingDrop {
				rt.diagnostics.ruleMissed++
				return
			}
			rt.diagnostics.extractionFailed++
			return
		}
		val = rule.rule.Scale.Apply(val)
		rt.setSeriesValueLocked(rule, entry, td, val, now)
	case catalog.MetricTypeState:
		if _, ok := rule.problemOIDs[entry.TrapOID]; ok {
			rt.setSeriesValueLocked(rule, entry, td, rule.rule.StateProblemValue(), now)
			return
		}
		if _, ok := rule.clearOIDs[entry.TrapOID]; ok {
			rt.setSeriesValueLocked(rule, entry, td, rule.rule.StateClearValue(), now)
			return
		}
		rt.diagnostics.ruleMissed++
	}
}

func (r *compiledProfileMetricRule) trapDefForOID(oid string) *catalog.TrapDef {
	if td := r.trapOIDs[oid]; td != nil {
		return td
	}
	if td := r.problemOIDs[oid]; td != nil {
		return td
	}
	return r.clearOIDs[oid]
}

func (r *compiledProfileMetricRule) sameOIDStateValue(entry *model.TrapEntry, td *catalog.TrapDef) (float64, bool) {
	if r.rule.State.SetWhen != nil && profileMetricPredicateMatches(*r.rule.State.SetWhen, entry, td) {
		return r.rule.StateProblemValue(), true
	}
	if r.rule.State.ClearWhen != nil && profileMetricPredicateMatches(*r.rule.State.ClearWhen, entry, td) {
		return r.rule.StateClearValue(), true
	}
	return 0, false
}

func (rt *Runtime) addCounterLocked(rule *compiledProfileMetricRule, entry *model.TrapEntry, td *catalog.TrapDef, now time.Time) {
	series := rt.getOrCreateSeriesLocked(rule, entry, td, now)
	if series == nil {
		return
	}
	series.value++
	series.lastUpdate = now
	series.lastCycle = rt.collectCycle
}

func (rt *Runtime) setSeriesValueLocked(rule *compiledProfileMetricRule, entry *model.TrapEntry, td *catalog.TrapDef, value float64, now time.Time) {
	series := rt.getOrCreateSeriesLocked(rule, entry, td, now)
	if series == nil {
		return
	}
	series.value = value
	series.lastUpdate = now
	series.lastCycle = rt.collectCycle
	series.removeAfterCollect = false
}

func (rt *Runtime) getOrCreateSeriesLocked(rule *compiledProfileMetricRule, entry *model.TrapEntry, td *catalog.TrapDef, now time.Time) *profileMetricSeries {
	key, scope, labels, ok := rt.seriesIdentityLocked(rule, entry, td, now)
	if !ok {
		return nil
	}
	if series := rt.series[key]; series != nil {
		series.scope = scope
		series.labels = labels
		return series
	}
	if rt.cfg.limits.MaxInstancesPerJob > 0 && len(rt.series) >= rt.cfg.limits.MaxInstancesPerJob {
		rt.diagnostics.overflowDropped++
		return nil
	}
	if !rt.ensureChartInstanceTrackedLocked(rule, key) {
		return nil
	}
	series := &profileMetricSeries{
		key:        key,
		rule:       rule,
		scope:      scope,
		labels:     labels,
		lastUpdate: now,
		lastCycle:  rt.collectCycle,
	}
	rt.series[key] = series
	return series
}

func (rt *Runtime) ensureChartInstanceTrackedLocked(rule *compiledProfileMetricRule, key profileMetricSeriesKey) bool {
	if rule == nil || rule.chart == nil {
		return true
	}
	instanceKey := profileMetricChartInstanceKey{
		chartID:       rule.chart.ID,
		scopeKey:      key.scopeKey,
		sourceID:      key.sourceID,
		sourceKind:    key.sourceKind,
		resourceClass: key.resourceClass,
		resourceID:    key.resourceID,
	}
	if _, ok := rt.chartInstances[instanceKey]; ok {
		return true
	}
	max := catalog.DefaultMetricChartMaxInstances
	if rule.chart.Lifecycle != nil && rule.chart.Lifecycle.MaxInstances > 0 {
		max = rule.chart.Lifecycle.MaxInstances
	}
	if max > 0 && rt.chartCounts[rule.chart.ID] >= max {
		rt.diagnostics.overflowDropped++
		return false
	}
	rt.chartInstances[instanceKey] = struct{}{}
	rt.chartCounts[rule.chart.ID]++
	return true
}

func (rt *Runtime) seriesIdentityLocked(rule *compiledProfileMetricRule, entry *model.TrapEntry, td *catalog.TrapDef, now time.Time) (profileMetricSeriesKey, metrix.HostScope, []metrix.Label, bool) {
	identity := rt.cfg.identity
	if rule.rule.Identity.Device != "" && rule.rule.Identity.Device != catalog.MetricIdentitySource {
		identity.Device = attribution.DeviceMode(rule.rule.Identity.Device)
	}
	key := profileMetricSeriesKey{ruleName: rule.rule.Name}

	source, ok := attribution.Resolve(entry, entry.JobName, identity.Device, rt.sourceHashSalt)
	if !ok {
		rt.diagnostics.attributionFailed++
		return profileMetricSeriesKey{}, metrix.HostScope{}, nil, false
	}

	key.scopeKey = source.Key.ScopeKey
	key.sourceID = source.Key.SourceID
	key.sourceKind = source.Key.SourceKind
	labels := source.Labels
	if key.sourceKind != "listener" && !rt.ensureSourceTrackedLocked(key.sourceID, now) {
		return profileMetricSeriesKey{}, metrix.HostScope{}, nil, false
	}
	if rt.sourceRoutes.Observe(source.RawRouteKey, source.RouteKey, now) {
		rt.diagnostics.sourceTransitions++
	}

	if rule.rule.Identity.Resource != nil {
		resourceID, ok := rt.resourceIdentity(rule, entry, td)
		if !ok {
			return profileMetricSeriesKey{}, metrix.HostScope{}, nil, false
		}
		class := rule.rule.Identity.Resource.Class
		key.resourceClass = class
		key.resourceID = resourceID
		sourceKey := key.sourceKind + ":" + key.sourceID
		if !rt.ensureResourceTrackedLocked(rule, sourceKey, class, resourceID) {
			return profileMetricSeriesKey{}, metrix.HostScope{}, nil, false
		}
		labels = append(labels,
			metrix.Label{Key: "resource_class", Value: class},
			metrix.Label{Key: "resource_id", Value: resourceID},
		)
	}

	return key, source.Scope, labels, true
}

func (rt *Runtime) ensureSourceTrackedLocked(sourceID string, now time.Time) bool {
	if sourceID == "" || rt.cfg.limits.MaxSources == 0 {
		return true
	}
	if _, ok := rt.sources[sourceID]; ok {
		rt.sources[sourceID] = now
		return true
	}
	if len(rt.sources) >= rt.cfg.limits.MaxSources {
		rt.diagnostics.overflowDropped++
		return false
	}
	rt.sources[sourceID] = now
	return true
}

func (rt *Runtime) resourceIdentity(rule *compiledProfileMetricRule, entry *model.TrapEntry, _ *catalog.TrapDef) (string, bool) {
	if rule.resourceVarbind == nil {
		rt.diagnostics.extractionFailed++
		return "", false
	}
	v, ok := model.FindVarbindForProfileOID(entry.Varbinds, rule.resourceVarbind.OID)
	if !ok {
		if rule.rule.Missing == catalog.MetricMissingUnknownDimension {
			return "unknown", true
		}
		if rule.rule.Missing == catalog.MetricMissingDrop {
			rt.diagnostics.ruleMissed++
			return "", false
		}
		rt.diagnostics.extractionFailed++
		return "", false
	}
	resourceID := strings.TrimSpace(model.VarbindRawValue(v))
	if resourceID == "" {
		if rule.rule.Missing == catalog.MetricMissingUnknownDimension {
			return "unknown", true
		}
		if rule.rule.Missing == catalog.MetricMissingDrop {
			rt.diagnostics.ruleMissed++
			return "", false
		}
		rt.diagnostics.extractionFailed++
		return "", false
	}
	return resourceID, true
}

func (rt *Runtime) ensureResourceTrackedLocked(rule *compiledProfileMetricRule, sourceKey, class, resourceID string) bool {
	max := rule.rule.Identity.Resource.MaxPerSource
	if max == 0 {
		max = rt.cfg.limits.MaxResourcesPerSource
	}
	if max == 0 {
		return true
	}
	key := sourceKey + ":" + class
	res := rt.resources[key]
	if res == nil {
		res = make(map[string]struct{})
		rt.resources[key] = res
	}
	if _, ok := res[resourceID]; ok {
		return true
	}
	if len(res) >= max {
		rt.diagnostics.overflowDropped++
		return false
	}
	res[resourceID] = struct{}{}
	return true
}
