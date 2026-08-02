// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
)

const (
	profileMetricTypeCounter = "counter"
	profileMetricTypeSample  = "sample"
	profileMetricTypeState   = "state"

	profileMetricIdentitySource      = "source"
	profileMetricIdentitySourceLabel = "source_label"
	profileMetricIdentityListener    = "listener"

	profileMetricMissingDrop             = "drop"
	profileMetricMissingZero             = "zero"
	profileMetricMissingUnknownDimension = "unknown_dimension"
	profileMetricMissingError            = "error"

	defaultProfileMetricMaxRules              = 500
	defaultProfileMetricMaxSources            = 2000
	defaultProfileMetricMaxResourcesPerSource = 512
	defaultProfileMetricMaxInstancesPerJob    = 50000
	defaultProfileMetricExpireAfterCycles     = 60
	defaultProfileMetricChartMaxInstances     = 2000
)

type ProfileMetricsConfig struct {
	Enabled bool     `yaml:"enabled,omitempty" json:"enabled"`
	Include []string `yaml:"include,omitempty" json:"include"`
}

type profileMetricIdentityPolicy struct {
	Device string
}

type profileMetricLimitsPolicy struct {
	MaxRules              int
	MaxSources            int
	MaxResourcesPerSource int
	MaxInstancesPerJob    int
}

type profileMetricRule = catalog.MetricRule
type profileMetricIdentity = catalog.MetricIdentity
type profileMetricResource = catalog.MetricResource
type profileMetricOutput = catalog.MetricOutput
type profileMetricScale = catalog.MetricScale
type profileMetricState = catalog.MetricState
type profileMetricChart = catalog.MetricChart
type profileMetricPredicates = catalog.MetricPredicates
type profileMetricPredicate = catalog.MetricPredicate

type normalizedProfileMetricsConfig struct {
	enabled  bool
	include  []string
	identity profileMetricIdentityPolicy
	limits   profileMetricLimitsPolicy
}

func normalizeProfileMetricsConfig(cfg ProfileMetricsConfig) (normalizedProfileMetricsConfig, error) {
	seen := make(map[string]bool, len(cfg.Include))
	include := make([]string, 0, len(cfg.Include))
	for i, name := range cfg.Include {
		name = strings.TrimSpace(name)
		if name == "" {
			return normalizedProfileMetricsConfig{}, fmt.Errorf("profile_metrics.include[%d] is empty", i)
		}
		if seen[name] {
			return normalizedProfileMetricsConfig{}, fmt.Errorf("profile_metrics.include[%d]: duplicate rule %q", i, name)
		}
		seen[name] = true
		include = append(include, name)
	}
	if cfg.Enabled && len(include) == 0 {
		return normalizedProfileMetricsConfig{}, errors.New("profile_metrics.include must contain at least one rule when enabled")
	}

	return normalizedProfileMetricsConfig{
		enabled:  cfg.Enabled,
		include:  include,
		identity: defaultProfileMetricIdentityPolicy(),
		limits:   defaultProfileMetricLimitsPolicy(),
	}, nil
}

func defaultProfileMetricIdentityPolicy() profileMetricIdentityPolicy {
	return profileMetricIdentityPolicy{
		Device: profileMetricIdentitySource,
	}
}

func defaultProfileMetricLimitsPolicy() profileMetricLimitsPolicy {
	return profileMetricLimitsPolicy{
		MaxRules:              defaultProfileMetricMaxRules,
		MaxSources:            defaultProfileMetricMaxSources,
		MaxResourcesPerSource: defaultProfileMetricMaxResourcesPerSource,
		MaxInstancesPerJob:    defaultProfileMetricMaxInstancesPerJob,
	}
}

type profileMetricCatalog struct {
	rulesByName map[string]*profileMetricRule
	chartsByID  map[string]*profileMetricChart
}

func selectProfileMetricRules(cfg normalizedProfileMetricsConfig, cat profileMetricCatalog) ([]*profileMetricRule, error) {
	if !cfg.enabled {
		return nil, nil
	}
	selected := make([]*profileMetricRule, 0, len(cfg.include))
	for _, name := range cfg.include {
		rule := cat.rulesByName[name]
		if rule == nil {
			return nil, fmt.Errorf("profile_metrics.include rule %q not found", name)
		}
		if rule.Disabled() {
			return nil, fmt.Errorf("profile_metrics.include rule %q is disabled by profile", name)
		}
		selected = append(selected, rule)
	}
	if len(selected) > cfg.limits.MaxRules {
		return nil, fmt.Errorf("profile_metrics selected %d rules, above fixed maximum %d", len(selected), cfg.limits.MaxRules)
	}
	slices.SortFunc(selected, func(a, b *profileMetricRule) int {
		return strings.Compare(a.Name, b.Name)
	})
	return selected, nil
}

type profileMetricRuntime struct {
	mu sync.Mutex

	cfg        normalizedProfileMetricsConfig
	rules      []*compiledProfileMetricRule
	rulesByOID map[string][]*compiledProfileMetricRule

	series          map[profileMetricSeriesKey]*profileMetricSeries
	sources         map[string]time.Time
	sourceRoutes    map[string]string
	sourceRouteSeen map[string]time.Time
	resources       map[string]map[string]struct{}
	chartInstances  map[profileMetricChartInstanceKey]struct{}
	chartCounts     map[string]int
	collectCycle    uint64
	sourceHashSalt  string

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
	trapOIDs          map[string]*TrapDef
	problemOIDs       map[string]*TrapDef
	clearOIDs         map[string]*TrapDef
	chart             *profileMetricChart
	valueVarbind      *VarbindDef
	resourceVarbind   *VarbindDef
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

func newProfileMetricRuntime(cfg normalizedProfileMetricsConfig, idx *ProfileIndex, sourceHashSalt string) (*profileMetricRuntime, string, error) {
	if !cfg.enabled {
		return nil, "", nil
	}
	if idx == nil {
		return nil, "", errors.New("profile index not available")
	}
	defs, err := idx.Definitions(cfg.include)
	if err != nil {
		return nil, "", err
	}
	cat := profileMetricCatalog{rulesByName: defs.RulesByName, chartsByID: defs.ChartsByID}
	selected, err := selectProfileMetricRules(cfg, cat)
	if err != nil {
		return nil, "", err
	}
	if len(selected) == 0 {
		return nil, "", nil
	}

	rt := &profileMetricRuntime{
		cfg:             cfg,
		series:          make(map[profileMetricSeriesKey]*profileMetricSeries),
		sources:         make(map[string]time.Time),
		sourceRoutes:    make(map[string]string),
		sourceRouteSeen: make(map[string]time.Time),
		resources:       make(map[string]map[string]struct{}),
		chartInstances:  make(map[profileMetricChartInstanceKey]struct{}),
		chartCounts:     make(map[string]int),
		rulesByOID:      make(map[string][]*compiledProfileMetricRule),
		sourceHashSalt:  sourceHashSalt,
	}
	for _, rule := range selected {
		compiled, err := compileProfileMetricRule(rule, cat, idx)
		if err != nil {
			return nil, "", err
		}
		rt.rules = append(rt.rules, compiled)
		for oid := range compiled.trapOIDs {
			rt.rulesByOID[oid] = append(rt.rulesByOID[oid], compiled)
		}
		for oid := range compiled.problemOIDs {
			rt.rulesByOID[oid] = append(rt.rulesByOID[oid], compiled)
		}
		for oid := range compiled.clearOIDs {
			rt.rulesByOID[oid] = append(rt.rulesByOID[oid], compiled)
		}
	}
	for oid, rules := range rt.rulesByOID {
		slices.SortFunc(rules, func(a, b *compiledProfileMetricRule) int {
			if c := strings.Compare(a.chart.ID, b.chart.ID); c != 0 {
				return c
			}
			return strings.Compare(a.rule.Name, b.rule.Name)
		})
		rt.rulesByOID[oid] = rules
	}
	yml, err := buildProfileMetricChartTemplateYAML(rt.rules, cat.chartsByID)
	if err != nil {
		return nil, "", err
	}
	return rt, yml, nil
}

func compileProfileMetricRule(rule *profileMetricRule, cat profileMetricCatalog, idx *ProfileIndex) (*compiledProfileMetricRule, error) {
	if rule == nil {
		return nil, errors.New("nil profile metric rule")
	}
	compiled := &compiledProfileMetricRule{
		rule:        rule,
		trapOIDs:    make(map[string]*TrapDef),
		problemOIDs: make(map[string]*TrapDef),
		clearOIDs:   make(map[string]*TrapDef),
	}
	chart := cat.chartsByID[rule.Output.Chart]
	if chart == nil {
		return nil, fmt.Errorf("%s: profile metric rule %q references unknown chart %q", rule.Source(), rule.Name, rule.Output.Chart)
	}
	compiled.chart = chart
	compiled.expireAfterCycles = defaultProfileMetricExpireAfterCycles
	if chart.Lifecycle != nil && chart.Lifecycle.ExpireAfterCycles > 0 {
		compiled.expireAfterCycles = chart.Lifecycle.ExpireAfterCycles
	}

	addTrap := func(dst map[string]*TrapDef, ref, field string) error {
		td, err := resolveProfileMetricTrap(idx, ref)
		if err != nil {
			return fmt.Errorf("%s: profile metric rule %q %s: %w", rule.Source(), rule.Name, field, err)
		}
		for _, oid := range metricOIDAliasesFromTrap(td) {
			dst[oid] = td
		}
		return nil
	}
	switch rule.Type {
	case profileMetricTypeCounter, profileMetricTypeSample:
		if err := addTrap(compiled.trapOIDs, rule.OnTrap, "on_trap"); err != nil {
			return nil, err
		}
	case profileMetricTypeState:
		if rule.OnTrap != "" {
			if err := addTrap(compiled.trapOIDs, rule.OnTrap, "on_trap"); err != nil {
				return nil, err
			}
		} else {
			if err := addTrap(compiled.problemOIDs, rule.ProblemTrap, "problem_trap"); err != nil {
				return nil, err
			}
			if err := addTrap(compiled.clearOIDs, rule.ClearTrap, "clear_trap"); err != nil {
				return nil, err
			}
		}
		if rule.State.TTL != "" {
			ttl, err := parseProfileMetricStateTTL(rule.State.TTL)
			if err != nil {
				return nil, fmt.Errorf("%s: profile metric rule %q state.ttl: %w", rule.Source(), rule.Name, err)
			}
			compiled.stateTTL = ttl
		}
	}
	if rule.ValueFromVarbind != "" {
		td := firstTrapDef(compiled.trapOIDs)
		vb := trapMetricVarbindByName(td, rule.ValueFromVarbind)
		if vb == nil {
			return nil, fmt.Errorf("%s: profile metric rule %q value_from_varbind %q not found", rule.Source(), rule.Name, rule.ValueFromVarbind)
		}
		compiled.valueVarbind = vb
	}
	if rule.Identity.Resource != nil && rule.Identity.Resource.KeyFromVarbind != "" {
		td := firstAnyTrapDef(compiled.trapOIDs, compiled.problemOIDs, compiled.clearOIDs)
		vb := trapMetricVarbindByName(td, rule.Identity.Resource.KeyFromVarbind)
		if vb == nil {
			return nil, fmt.Errorf("%s: profile metric rule %q resource key_from_varbind %q not found", rule.Source(), rule.Name, rule.Identity.Resource.KeyFromVarbind)
		}
		compiled.resourceVarbind = vb
	}
	return compiled, nil
}

func resolveProfileMetricTrap(idx *ProfileIndex, ref string) (*TrapDef, error) {
	return idx.ResolveTrap(ref)
}

func metricOIDAliasesFromTrap(td *TrapDef) []string {
	if td == nil || td.OID == "" {
		return nil
	}
	aliases := []string{td.OID}
	if alt := model.AlternateTrapOID(td.OID); alt != td.OID {
		aliases = append(aliases, alt)
	}
	return aliases
}

func firstTrapDef(m map[string]*TrapDef) *TrapDef {
	for _, td := range m {
		return td
	}
	return nil
}

func firstAnyTrapDef(mapsIn ...map[string]*TrapDef) *TrapDef {
	for _, m := range mapsIn {
		if td := firstTrapDef(m); td != nil {
			return td
		}
	}
	return nil
}

func trapMetricVarbindByName(td *TrapDef, name string) *VarbindDef {
	return td.VarbindByName(name)
}

func (rt *profileMetricRuntime) update(entry *TrapEntry) {
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

func (rt *profileMetricRuntime) updateRuleLocked(rule *compiledProfileMetricRule, entry *TrapEntry, now time.Time) {
	td := rule.trapDefForOID(entry.TrapOID)
	if td == nil {
		rt.diagnostics.ruleMissed++
		return
	}
	if !profileMetricPredicatesMatch(rule.rule.Where, entry, td) {
		rt.diagnostics.ruleMissed++
		return
	}
	if rule.rule.Type == profileMetricTypeState && len(rule.trapOIDs) > 0 {
		stateValue, matched := rule.sameOIDStateValue(entry, td)
		if !matched {
			rt.diagnostics.ruleMissed++
			return
		}
		rt.setSeriesValueLocked(rule, entry, td, stateValue, now)
		return
	}
	switch rule.rule.Type {
	case profileMetricTypeCounter:
		rt.addCounterLocked(rule, entry, td, now)
	case profileMetricTypeSample:
		val, status := profileMetricNumericVarbindValue(entry, rule.valueVarbind)
		if status != profileMetricValueOK {
			if status == profileMetricValueMissing && rule.rule.Missing == profileMetricMissingZero {
				rt.setSeriesValueLocked(rule, entry, td, 0, now)
				return
			}
			if status == profileMetricValueMissing && rule.rule.Missing == profileMetricMissingDrop {
				rt.diagnostics.ruleMissed++
				return
			}
			rt.diagnostics.extractionFailed++
			return
		}
		val = rule.rule.Scale.Apply(val)
		rt.setSeriesValueLocked(rule, entry, td, val, now)
	case profileMetricTypeState:
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

func (r *compiledProfileMetricRule) trapDefForOID(oid string) *TrapDef {
	if td := r.trapOIDs[oid]; td != nil {
		return td
	}
	if td := r.problemOIDs[oid]; td != nil {
		return td
	}
	return r.clearOIDs[oid]
}

func (r *compiledProfileMetricRule) sameOIDStateValue(entry *TrapEntry, td *TrapDef) (float64, bool) {
	if r.rule.State.SetWhen != nil && profileMetricPredicateMatches(*r.rule.State.SetWhen, entry, td) {
		return r.rule.StateProblemValue(), true
	}
	if r.rule.State.ClearWhen != nil && profileMetricPredicateMatches(*r.rule.State.ClearWhen, entry, td) {
		return r.rule.StateClearValue(), true
	}
	return 0, false
}

func (rt *profileMetricRuntime) addCounterLocked(rule *compiledProfileMetricRule, entry *TrapEntry, td *TrapDef, now time.Time) {
	series := rt.getOrCreateSeriesLocked(rule, entry, td, now)
	if series == nil {
		return
	}
	series.value++
	series.lastUpdate = now
	series.lastCycle = rt.collectCycle
}

func (rt *profileMetricRuntime) setSeriesValueLocked(rule *compiledProfileMetricRule, entry *TrapEntry, td *TrapDef, value float64, now time.Time) {
	series := rt.getOrCreateSeriesLocked(rule, entry, td, now)
	if series == nil {
		return
	}
	series.value = value
	series.lastUpdate = now
	series.lastCycle = rt.collectCycle
	series.removeAfterCollect = false
}

func (rt *profileMetricRuntime) getOrCreateSeriesLocked(rule *compiledProfileMetricRule, entry *TrapEntry, td *TrapDef, now time.Time) *profileMetricSeries {
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

func (rt *profileMetricRuntime) ensureChartInstanceTrackedLocked(rule *compiledProfileMetricRule, key profileMetricSeriesKey) bool {
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
	max := defaultProfileMetricChartMaxInstances
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

func (rt *profileMetricRuntime) seriesIdentityLocked(rule *compiledProfileMetricRule, entry *TrapEntry, td *TrapDef, now time.Time) (profileMetricSeriesKey, metrix.HostScope, []metrix.Label, bool) {
	identity := rt.cfg.identity
	if rule.rule.Identity.Device != "" && rule.rule.Identity.Device != profileMetricIdentitySource {
		identity.Device = rule.rule.Identity.Device
	}
	key := profileMetricSeriesKey{ruleName: rule.rule.Name}

	source, ok := resolveTrapMetricSourceIdentity(entry, entry.JobName, identity, rt.sourceHashSalt)
	if !ok {
		rt.diagnostics.attributionFailed++
		return profileMetricSeriesKey{}, metrix.HostScope{}, nil, false
	}

	key.scopeKey = source.key.scopeKey
	key.sourceID = source.key.sourceID
	key.sourceKind = source.key.sourceKind
	labels := source.labels
	if key.sourceKind != "listener" && !rt.ensureSourceTrackedLocked(key.sourceID, now) {
		return profileMetricSeriesKey{}, metrix.HostScope{}, nil, false
	}
	rt.noteSourceRouteTransitionLocked(source.rawRouteKey, source.routeKey, now)

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

	return key, source.scope, labels, true
}

func (rt *profileMetricRuntime) fallbackSourceIdentity(entry *TrapEntry) (string, string) {
	return fallbackTrapSourceIdentity(entry, entry.JobName, rt.sourceHashSalt)
}

func (rt *profileMetricRuntime) rawFallbackSourceIdentity(entry *TrapEntry) (string, string) {
	return rawFallbackTrapSourceIdentity(entry)
}

func (rt *profileMetricRuntime) noteSourceRouteTransitionLocked(rawRouteKey, routeKey string, now time.Time) {
	if rawRouteKey == "" || routeKey == "" {
		return
	}
	if previous := rt.sourceRoutes[rawRouteKey]; previous != "" && previous != routeKey {
		rt.diagnostics.sourceTransitions++
	}
	rt.sourceRoutes[rawRouteKey] = routeKey
	rt.sourceRouteSeen[rawRouteKey] = now
	rt.pruneSourceRoutesLocked()
}

func (rt *profileMetricRuntime) ensureSourceTrackedLocked(sourceID string, now time.Time) bool {
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

func (rt *profileMetricRuntime) resourceIdentity(rule *compiledProfileMetricRule, entry *TrapEntry, _ *TrapDef) (string, bool) {
	if rule.resourceVarbind == nil {
		rt.diagnostics.extractionFailed++
		return "", false
	}
	v, ok := model.FindVarbindForProfileOID(entry.Varbinds, rule.resourceVarbind.OID)
	if !ok {
		if rule.rule.Missing == profileMetricMissingUnknownDimension {
			return "unknown", true
		}
		if rule.rule.Missing == profileMetricMissingDrop {
			rt.diagnostics.ruleMissed++
			return "", false
		}
		rt.diagnostics.extractionFailed++
		return "", false
	}
	resourceID := strings.TrimSpace(model.VarbindRawValue(v))
	if resourceID == "" {
		if rule.rule.Missing == profileMetricMissingUnknownDimension {
			return "unknown", true
		}
		if rule.rule.Missing == profileMetricMissingDrop {
			rt.diagnostics.ruleMissed++
			return "", false
		}
		rt.diagnostics.extractionFailed++
		return "", false
	}
	return resourceID, true
}

func (rt *profileMetricRuntime) ensureResourceTrackedLocked(rule *compiledProfileMetricRule, sourceKey, class, resourceID string) bool {
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

func (rt *profileMetricRuntime) collect(store metrix.CollectorStore, jobName string) {
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

func (rt *profileMetricRuntime) sweepLocked(now time.Time) {
	removed := false
	for key, series := range rt.series {
		rule := series.rule
		if rule == nil {
			delete(rt.series, key)
			removed = true
			continue
		}
		if rule.rule.Type == profileMetricTypeState && rule.stateTTL > 0 && now.Sub(series.lastUpdate) >= rule.stateTTL {
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

func (rt *profileMetricRuntime) rebuildCardinalityIndexesLocked() {
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

func (rt *profileMetricRuntime) pruneSourceRoutesLocked() {
	if len(rt.sourceRoutes) == 0 {
		return
	}
	limit := rt.cfg.limits.MaxSources
	if limit <= 0 {
		limit = defaultProfileMetricMaxSources
	}
	if limit <= 0 || len(rt.sourceRoutes) <= limit {
		return
	}
	type routeAge struct {
		key  string
		seen time.Time
	}
	ages := make([]routeAge, 0, len(rt.sourceRoutes))
	for rawRouteKey := range rt.sourceRoutes {
		ages = append(ages, routeAge{key: rawRouteKey, seen: rt.sourceRouteSeen[rawRouteKey]})
	}
	slices.SortFunc(ages, func(a, b routeAge) int {
		if a.seen.Equal(b.seen) {
			return strings.Compare(a.key, b.key)
		}
		if a.seen.Before(b.seen) {
			return -1
		}
		return 1
	})
	for _, age := range ages {
		if len(rt.sourceRoutes) <= limit {
			break
		}
		delete(rt.sourceRoutes, age.key)
		delete(rt.sourceRouteSeen, age.key)
	}
}

func (rt *profileMetricRuntime) collectSeries(store metrix.CollectorStore, series profileMetricSeriesSnapshot) {
	if series.rule == nil || series.rule.rule == nil {
		return
	}
	meter := store.Write().SnapshotMeter("").WithHostScope(series.scope).WithLabels(series.labels...)
	switch series.rule.rule.Type {
	case profileMetricTypeCounter:
		meter.Counter(series.rule.rule.Output.Metric).ObserveTotal(metrix.SampleValue(series.value))
	case profileMetricTypeSample, profileMetricTypeState:
		meter.Gauge(series.rule.rule.Output.Metric).Observe(metrix.SampleValue(series.value))
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

func profileMetricPredicatesMatch(preds []profileMetricPredicate, entry *TrapEntry, td *TrapDef) bool {
	for _, pred := range preds {
		if !profileMetricPredicateMatches(pred, entry, td) {
			return false
		}
	}
	return true
}

func profileMetricPredicateMatches(pred profileMetricPredicate, entry *TrapEntry, td *TrapDef) bool {
	present, value, vb := profileMetricPredicateValue(pred, entry, td)
	result := profileMetricPredicateResult(pred, present, value, vb)
	if pred.Not && present {
		return !result
	}
	return result
}

func profileMetricPredicateValue(pred profileMetricPredicate, entry *TrapEntry, td *TrapDef) (bool, VarbindValue, *VarbindDef) {
	if pred.Field != "" {
		return profileMetricSyntheticFieldValue(pred.Field, entry)
	}
	vb := trapMetricVarbindByName(td, pred.Varbind)
	if vb == nil {
		return false, VarbindValue{}, nil
	}
	v, ok := model.FindVarbindForProfileOID(entry.Varbinds, vb.OID)
	return ok, v, vb
}

func profileMetricSyntheticFieldValue(field string, entry *TrapEntry) (bool, VarbindValue, *VarbindDef) {
	var value string
	switch field {
	case "category":
		value = string(entry.Category)
	case "severity":
		value = string(entry.Severity)
	case "trap_name":
		value = entry.TrapName
	case "trap_oid":
		value = entry.TrapOID
	default:
		return false, VarbindValue{}, nil
	}
	if value == "" {
		return false, VarbindValue{}, nil
	}
	return true, VarbindValue{Value: value}, nil
}

func profileMetricPredicateResult(pred profileMetricPredicate, present bool, value VarbindValue, vb *VarbindDef) bool {
	if pred.Absent != nil {
		return *pred.Absent == !present
	}
	if pred.Exists != nil {
		return *pred.Exists == present
	}
	if !present {
		return false
	}
	if pred.Equals != nil {
		return profileMetricValueEquals(value, vb, pred.Equals)
	}
	if len(pred.In) > 0 {
		return slices.ContainsFunc(pred.In, func(want any) bool {
			return profileMetricValueEquals(value, vb, want)
		})
	}
	if pred.GreaterThan != nil || pred.LessThan != nil || len(pred.Range) > 0 {
		actual, ok := profileMetricFloatValue(value.Value)
		if !ok {
			return false
		}
		if pred.GreaterThan != nil {
			want, ok := profileMetricFloatValue(pred.GreaterThan)
			if !ok || actual <= want {
				return false
			}
		}
		if pred.LessThan != nil {
			want, ok := profileMetricFloatValue(pred.LessThan)
			if !ok || actual >= want {
				return false
			}
		}
		if len(pred.Range) > 0 {
			low, okLow := profileMetricFloatValue(pred.Range[0])
			high, okHigh := profileMetricFloatValue(pred.Range[1])
			if !okLow || !okHigh || actual < low || actual > high {
				return false
			}
		}
		return true
	}
	return false
}

func profileMetricValueEquals(value VarbindValue, vb *VarbindDef, want any) bool {
	actual := model.VarbindRawValue(value)
	if vb != nil && len(vb.Enum) > 0 {
		if label := resolveEnum(vb, value.Value); label != "" && label == fmt.Sprintf("%v", want) {
			return true
		}
	}
	return actual == fmt.Sprintf("%v", want)
}

func profileMetricFloatValue(value any) (float64, bool) {
	f, ok := profileMetricRawFloatValue(value)
	if !ok || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

func profileMetricRawFloatValue(value any) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case float32:
		return float64(v), true
	case float64:
		return v, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func parseProfileMetricStateTTL(value string) (time.Duration, error) {
	ttl, err := time.ParseDuration(value)
	if err != nil {
		return 0, err
	}
	if ttl <= 0 {
		return 0, fmt.Errorf("must be greater than zero")
	}
	return ttl, nil
}

type profileMetricValueStatus int

const (
	profileMetricValueOK profileMetricValueStatus = iota
	profileMetricValueMissing
	profileMetricValueInvalid
)

func profileMetricNumericVarbindValue(entry *TrapEntry, vb *VarbindDef) (float64, profileMetricValueStatus) {
	if vb == nil {
		return 0, profileMetricValueInvalid
	}
	v, ok := model.FindVarbindForProfileOID(entry.Varbinds, vb.OID)
	if !ok {
		return 0, profileMetricValueMissing
	}
	value, ok := profileMetricFloatValue(v.Value)
	if !ok {
		return 0, profileMetricValueInvalid
	}
	if strings.EqualFold(strings.TrimSpace(vb.Type), "timeticks") {
		value /= 100
	}
	return value, profileMetricValueOK
}

func slugForMetric(name string) string {
	name = strings.ToLower(name)
	var b strings.Builder
	lastUnderscore := false
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

func buildProfileMetricChartTemplateYAML(rules []*compiledProfileMetricRule, charts map[string]*profileMetricChart) (string, error) {
	spec, err := charttpl.DecodeYAML([]byte(chartTemplateYAML))
	if err != nil {
		return "", fmt.Errorf("failed to decode base chart template: %w", err)
	}
	if len(spec.Groups) == 0 {
		return "", fmt.Errorf("base chart template has no groups")
	}
	group := &spec.Groups[0]
	group.Metrics = append(group.Metrics,
		"snmp_trap_profile_metrics_rule_missed",
		"snmp_trap_profile_metrics_extraction_failed",
		"snmp_trap_profile_metrics_attribution_failed",
		"snmp_trap_profile_metrics_overflow_dropped",
		"snmp_trap_profile_metrics_source_transitions",
	)
	group.Charts = append(group.Charts, profileMetricDiagnosticChart())

	ruleByChart := make(map[string][]*compiledProfileMetricRule)
	for _, rule := range rules {
		group.Metrics = append(group.Metrics, rule.rule.Output.Metric)
		ruleByChart[rule.rule.Output.Chart] = append(ruleByChart[rule.rule.Output.Chart], rule)
	}
	if err := validateSelectedProfileMetricChartDimensions(ruleByChart); err != nil {
		return "", err
	}
	chartIDs := make([]string, 0, len(ruleByChart))
	for id := range ruleByChart {
		chartIDs = append(chartIDs, id)
	}
	slices.Sort(chartIDs)
	for _, id := range chartIDs {
		chart := charts[id]
		if chart == nil {
			return "", fmt.Errorf("profile metric chart %q not found", id)
		}
		group.Charts = append(group.Charts, profileMetricChartToTemplate(chart, ruleByChart[id]))
	}
	raw, err := spec.MarshalTemplate()
	if err != nil {
		return "", fmt.Errorf("invalid chart template: %w", err)
	}
	return raw, nil
}

func validateSelectedProfileMetricChartDimensions(ruleByChart map[string][]*compiledProfileMetricRule) error {
	chartIDs := make([]string, 0, len(ruleByChart))
	for chartID := range ruleByChart {
		chartIDs = append(chartIDs, chartID)
	}
	slices.Sort(chartIDs)
	for _, chartID := range chartIDs {
		rules := append([]*compiledProfileMetricRule(nil), ruleByChart[chartID]...)
		slices.SortFunc(rules, func(a, b *compiledProfileMetricRule) int {
			if c := strings.Compare(a.rule.Output.Dimension, b.rule.Output.Dimension); c != 0 {
				return c
			}
			return strings.Compare(a.rule.Name, b.rule.Name)
		})
		seen := make(map[string]*compiledProfileMetricRule, len(rules))
		for _, rule := range rules {
			if rule == nil || rule.rule == nil {
				continue
			}
			dimension := rule.rule.Output.Dimension
			if existing := seen[dimension]; existing != nil {
				return fmt.Errorf("%s: metric rule %q chart %q reuses output.dimension %q selected by rule %q in %s",
					rule.rule.Source(), rule.rule.Name, chartID, dimension, existing.rule.Name, existing.rule.Source())
			}
			seen[dimension] = rule
		}
	}
	return nil
}

func profileMetricDiagnosticChart() charttpl.Chart {
	return charttpl.Chart{
		ID:    "profile_metric_diagnostics",
		Title: "SNMP trap profile metric diagnostics",
		// Template-local context; the base chart template compiles it under snmp.trap.
		Context:   "profile_metric_diagnostics",
		Units:     "events/s",
		Algorithm: "incremental",
		Type:      "stacked",
		Instances: &charttpl.Instances{ByLabels: []string{"job_name"}},
		Dimensions: []charttpl.Dimension{
			{Selector: "snmp_trap_profile_metrics_rule_missed", Name: "rule_missed"},
			{Selector: "snmp_trap_profile_metrics_extraction_failed", Name: "extraction_failed"},
			{Selector: "snmp_trap_profile_metrics_attribution_failed", Name: "attribution_failed"},
			{Selector: "snmp_trap_profile_metrics_overflow_dropped", Name: "overflow_dropped"},
			{Selector: "snmp_trap_profile_metrics_source_transitions", Name: "source_transitions"},
		},
	}
}

func profileMetricChartToTemplate(chart *profileMetricChart, rules []*compiledProfileMetricRule) charttpl.Chart {
	dims := make([]charttpl.Dimension, 0, len(rules))
	for _, rule := range rules {
		dim := charttpl.Dimension{
			Selector: rule.rule.Output.Metric,
			Name:     rule.rule.Output.Dimension,
		}
		dims = append(dims, dim)
	}
	slices.SortFunc(dims, func(a, b charttpl.Dimension) int {
		return strings.Compare(a.Name, b.Name)
	})
	byLabels := []string{"job_name", "source_id", "source_kind"}
	usesResource := false
	for _, rule := range rules {
		if rule.rule.Identity.Resource != nil {
			usesResource = true
		}
	}
	if usesResource {
		byLabels = append(byLabels, "resource_class", "resource_id")
	}
	context := strings.TrimPrefix(chart.Context, "snmp.trap.")
	return charttpl.Chart{
		ID:         chart.ID,
		Title:      chart.Title,
		Family:     chart.Family,
		Context:    context,
		Units:      chart.Units,
		Algorithm:  chart.Algorithm,
		Type:       chart.Type,
		Instances:  &charttpl.Instances{ByLabels: byLabels},
		Lifecycle:  chart.Lifecycle,
		Dimensions: dims,
	}
}
