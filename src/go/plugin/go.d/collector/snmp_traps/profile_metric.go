// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

const (
	profileMetricModeNone     = "none"
	profileMetricModeAuto     = "auto"
	profileMetricModeExact    = "exact"
	profileMetricModeCombined = "combined"

	profileMetricTypeCounter = "counter"
	profileMetricTypeSample  = "sample"
	profileMetricTypeState   = "state"

	profileMetricIdentitySource      = "source"
	profileMetricIdentitySourceLabel = "source_label"
	profileMetricIdentityListener    = "listener"

	profileMetricUnresolvedSourceLabel = "source_label"
	profileMetricDropMetricInstance    = "drop_metric_instance"

	profileMetricSourceIDRaw  = "raw"
	profileMetricSourceIDHash = "hash"

	profileMetricOverflowDropAndCount = "drop_and_count"

	profileMetricMissingDrop             = "drop"
	profileMetricMissingZero             = "zero"
	profileMetricMissingUnknownDimension = "unknown_dimension"
	profileMetricMissingError            = "error"

	profileMetricTTLBehaviorClearAndExpire = "clear_and_expire"

	defaultProfileMetricMaxRules              = 500
	defaultProfileMetricMaxSources            = 2000
	defaultProfileMetricMaxResourcesPerSource = 512
	defaultProfileMetricMaxInstancesPerJob    = 50000
	defaultProfileMetricExpireAfterCycles     = 60
	defaultProfileMetricChartMaxInstances     = 2000
)

var (
	profileMetricRuleNameRE      = regexp.MustCompile(`^[A-Za-z0-9_.:-]+::[A-Za-z0-9_.:-]+$|^[A-Za-z0-9_.:-]+$`)
	profileMetricOutputNameRE    = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	profileMetricChartIDRE       = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	profileMetricResourceClassRE = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	profileMetricSiteClassRE     = regexp.MustCompile(`^site_[a-z0-9][a-z0-9_]*$`)
)

var stockProfileMetricResourceClasses = map[string]bool{
	"interface":   true,
	"peer":        true,
	"neighbor":    true,
	"sensor":      true,
	"alarm":       true,
	"pool":        true,
	"l2_topology": true,
	"component":   true,
}

var reservedProfileMetricPrefixes = []string{
	"snmp_trap_events_",
	"snmp_trap_severity_",
	"snmp_trap_errors_",
	"snmp_trap_dedup_",
	"snmp_trap_pipeline_",
	"snmp_trap_source_",
	"snmp_trap_sources_",
	"snmp_trap_metric_",
	"snmp_trap_profile_metrics_",
}

var builtInProfileMetricChartIDs = map[string]bool{
	"events":                     true,
	"severity":                   true,
	"errors":                     true,
	"dedup_suppressed":           true,
	"pipeline":                   true,
	"sources":                    true,
	"source_attribution":         true,
	"source_pipeline":            true,
	"source_errors":              true,
	"source_last_seen":           true,
	"profile_metric_diagnostics": true,
}

var builtInProfileMetricChartContexts = map[string]bool{
	"snmp.trap.events":                     true,
	"snmp.trap.severity":                   true,
	"snmp.trap.errors":                     true,
	"snmp.trap.dedup_suppressed":           true,
	"snmp.trap.pipeline":                   true,
	"snmp.trap.sources":                    true,
	"snmp.trap.source_attribution":         true,
	"snmp.trap.source_pipeline":            true,
	"snmp.trap.source_errors":              true,
	"snmp.trap.source_last_seen":           true,
	"snmp.trap.profile_metric_diagnostics": true,
	"profile_metric_diagnostics":           true,
}

type ProfileMetricsConfig struct {
	Enabled  bool                        `yaml:"enabled,omitempty" json:"enabled"`
	Mode     string                      `yaml:"mode,omitempty" json:"mode"`
	Include  []string                    `yaml:"include,omitempty" json:"include"`
	Identity ProfileMetricIdentityConfig `yaml:"identity,omitempty" json:"identity"`
	Limits   ProfileMetricLimitsConfig   `yaml:"limits,omitempty" json:"limits"`
}

type ProfileMetricIdentityConfig struct {
	Device           string `yaml:"device,omitempty" json:"device"`
	UnresolvedSource string `yaml:"unresolved_source,omitempty" json:"unresolved_source"`
	SourceIDPrivacy  string `yaml:"source_id_privacy,omitempty" json:"source_id_privacy"`
}

type ProfileMetricLimitsConfig struct {
	MaxRules              int    `yaml:"max_rules,omitempty" json:"max_rules"`
	MaxSources            int    `yaml:"max_sources,omitempty" json:"max_sources"`
	MaxResourcesPerSource int    `yaml:"max_resources_per_source,omitempty" json:"max_resources_per_source"`
	MaxInstancesPerJob    int    `yaml:"max_instances_per_job,omitempty" json:"max_instances_per_job"`
	Overflow              string `yaml:"overflow,omitempty" json:"overflow"`
}

type profileMetricRule struct {
	Name        string `yaml:"name"`
	Type        string `yaml:"type"`
	AutoSafe    bool   `yaml:"auto_safe,omitempty"`
	Enabled     *bool  `yaml:"enabled,omitempty"`
	OnTrap      string `yaml:"on_trap,omitempty"`
	ProblemTrap string `yaml:"problem_trap,omitempty"`
	ClearTrap   string `yaml:"clear_trap,omitempty"`

	Where profileMetricPredicates `yaml:"where,omitempty"`

	Identity profileMetricIdentity `yaml:"identity,omitempty"`
	Resource profileMetricResource `yaml:"resource,omitempty"`
	Output   profileMetricOutput   `yaml:"output,omitempty"`
	State    profileMetricState    `yaml:"state,omitempty"`
	Scale    profileMetricScale    `yaml:"scale,omitempty"`

	Missing          string                  `yaml:"missing,omitempty"`
	ValueFromVarbind string                  `yaml:"value_from_varbind,omitempty"`
	ChartMeta        *profileMetricChartMeta `yaml:"chart_meta,omitempty"`

	Metric    string `yaml:"metric,omitempty"`
	Dimension string `yaml:"dimension,omitempty"`
	ChartID   string `yaml:"chart_id,omitempty"`
	Value     string `yaml:"value,omitempty"`

	sourceFile string
}

type profileMetricIdentity struct {
	Device   string                 `yaml:"device,omitempty"`
	Resource *profileMetricResource `yaml:"resource,omitempty"`
}

type profileMetricResource struct {
	Class          string `yaml:"class,omitempty"`
	KeyFromVarbind string `yaml:"key_from_varbind,omitempty"`
	MaxPerSource   int    `yaml:"max_per_source,omitempty"`

	Key string `yaml:"key,omitempty"`
	Max int    `yaml:"max,omitempty"`
}

type profileMetricOutput struct {
	Metric    string `yaml:"metric,omitempty"`
	Dimension string `yaml:"dimension,omitempty"`
	Chart     string `yaml:"chart,omitempty"`
}

type profileMetricScale struct {
	Multiplier int `yaml:"multiplier,omitempty"`
	Divisor    int `yaml:"divisor,omitempty"`
}

type profileMetricState struct {
	SetWhen   *profileMetricPredicate `yaml:"set_when,omitempty"`
	ClearWhen *profileMetricPredicate `yaml:"clear_when,omitempty"`

	ProblemValue *float64 `yaml:"problem_value,omitempty"`
	ClearValue   float64  `yaml:"clear_value,omitempty"`
	TTL          string   `yaml:"ttl,omitempty"`
	TTLBehavior  string   `yaml:"ttl_behavior,omitempty"`

	Varbind string `yaml:"varbind,omitempty"`
	Set     any    `yaml:"set,omitempty"`
	Clear   any    `yaml:"clear,omitempty"`
}

type profileMetricChart struct {
	ID          string              `yaml:"id"`
	Title       string              `yaml:"title"`
	Family      string              `yaml:"family,omitempty"`
	Context     string              `yaml:"context"`
	Units       string              `yaml:"units"`
	Algorithm   string              `yaml:"algorithm,omitempty"`
	Type        string              `yaml:"type,omitempty"`
	Description string              `yaml:"description,omitempty"`
	Lifecycle   *charttpl.Lifecycle `yaml:"lifecycle,omitempty"`

	sourceFile string
}

type profileMetricChartMeta struct {
	Title       string              `yaml:"title,omitempty"`
	Family      string              `yaml:"family,omitempty"`
	Context     string              `yaml:"context,omitempty"`
	Units       string              `yaml:"units,omitempty"`
	Algorithm   string              `yaml:"algorithm,omitempty"`
	Type        string              `yaml:"type,omitempty"`
	Description string              `yaml:"description,omitempty"`
	Lifecycle   *charttpl.Lifecycle `yaml:"lifecycle,omitempty"`
}

type profileMetricPredicates []profileMetricPredicate

type profileMetricPredicate struct {
	Varbind     string `yaml:"varbind,omitempty"`
	Field       string `yaml:"field,omitempty"`
	Equals      any    `yaml:"equals,omitempty"`
	In          []any  `yaml:"in,omitempty"`
	Exists      *bool  `yaml:"exists,omitempty"`
	Absent      *bool  `yaml:"absent,omitempty"`
	GreaterThan any    `yaml:"greater_than,omitempty"`
	LessThan    any    `yaml:"less_than,omitempty"`
	Range       []any  `yaml:"range,omitempty"`
	Not         bool   `yaml:"not,omitempty"`
}

func (p *profileMetricPredicates) UnmarshalYAML(unmarshal func(any) error) error {
	var raw any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	preds, err := normalizeProfileMetricPredicates(raw)
	if err != nil {
		return err
	}
	*p = preds
	return nil
}

func (p *profileMetricPredicate) UnmarshalYAML(unmarshal func(any) error) error {
	var raw map[any]any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	pred, err := normalizeProfileMetricPredicateMap(raw, "")
	if err != nil {
		return err
	}
	*p = pred
	return nil
}

func normalizeProfileMetricPredicates(raw any) ([]profileMetricPredicate, error) {
	switch v := raw.(type) {
	case nil:
		return nil, nil
	case []any:
		out := make([]profileMetricPredicate, 0, len(v))
		for i, item := range v {
			m, ok := item.(map[any]any)
			if !ok {
				return nil, fmt.Errorf("where[%d]: predicate must be a map", i)
			}
			pred, err := normalizeProfileMetricPredicateMap(m, "")
			if err != nil {
				return nil, fmt.Errorf("where[%d]: %w", i, err)
			}
			out = append(out, pred)
		}
		return out, nil
	case map[any]any:
		out := make([]profileMetricPredicate, 0, len(v))
		for rawKey, rawVal := range v {
			name, ok := rawKey.(string)
			if !ok || name == "" {
				return nil, fmt.Errorf("where: compact predicate key must be a non-empty string")
			}
			if opMap, ok := rawVal.(map[any]any); ok {
				pred, err := normalizeProfileMetricPredicateMap(opMap, name)
				if err != nil {
					return nil, fmt.Errorf("where.%s: %w", name, err)
				}
				out = append(out, pred)
			} else {
				out = append(out, profileMetricPredicate{Varbind: name, Equals: rawVal})
			}
		}
		slices.SortFunc(out, func(a, b profileMetricPredicate) int {
			return strings.Compare(a.Varbind+a.Field, b.Varbind+b.Field)
		})
		return out, nil
	default:
		return nil, fmt.Errorf("where must be a predicate list or compact map")
	}
}

func normalizeProfileMetricPredicateMap(m map[any]any, compactVarbind string) (profileMetricPredicate, error) {
	pred := profileMetricPredicate{Varbind: compactVarbind}
	for rawKey, rawVal := range m {
		key, ok := rawKey.(string)
		if !ok {
			return pred, fmt.Errorf("predicate key %v is not a string", rawKey)
		}
		switch key {
		case "varbind":
			s, _ := rawVal.(string)
			pred.Varbind = s
		case "field":
			s, _ := rawVal.(string)
			pred.Field = s
		case "equals":
			pred.Equals = rawVal
		case "in":
			values, ok := rawVal.([]any)
			if !ok {
				return pred, fmt.Errorf("in must be a list")
			}
			pred.In = append([]any(nil), values...)
		case "exists":
			b, ok := rawVal.(bool)
			if !ok {
				return pred, fmt.Errorf("exists must be boolean")
			}
			pred.Exists = &b
		case "absent":
			b, ok := rawVal.(bool)
			if !ok {
				return pred, fmt.Errorf("absent must be boolean")
			}
			pred.Absent = &b
		case "greater_than":
			pred.GreaterThan = rawVal
		case "less_than":
			pred.LessThan = rawVal
		case "range":
			values, ok := rawVal.([]any)
			if !ok || len(values) != 2 {
				return pred, fmt.Errorf("range must be a two-element list")
			}
			pred.Range = append([]any(nil), values...)
		case "not":
			b, ok := rawVal.(bool)
			if !ok {
				return pred, fmt.Errorf("not must be boolean")
			}
			pred.Not = b
		default:
			return pred, fmt.Errorf("unknown predicate key %q", key)
		}
	}
	if pred.Varbind == "" && pred.Field == "" {
		return pred, fmt.Errorf("predicate requires varbind or field")
	}
	return pred, nil
}

type normalizedProfileMetricsConfig struct {
	enabled  bool
	mode     string
	include  []string
	identity ProfileMetricIdentityConfig
	limits   ProfileMetricLimitsConfig
}

func normalizeProfileMetricsConfig(cfg ProfileMetricsConfig) (normalizedProfileMetricsConfig, error) {
	mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	if mode == "" {
		mode = profileMetricModeNone
	}
	switch mode {
	case profileMetricModeNone, profileMetricModeAuto, profileMetricModeExact, profileMetricModeCombined:
	default:
		return normalizedProfileMetricsConfig{}, fmt.Errorf("profile_metrics.mode must be one of none, auto, exact, combined, got %q", cfg.Mode)
	}
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
	if cfg.Enabled && len(include) > 0 && mode != profileMetricModeExact && mode != profileMetricModeCombined {
		return normalizedProfileMetricsConfig{}, fmt.Errorf("profile_metrics.include requires mode exact or combined")
	}

	identity := cfg.Identity
	identity.Device = defaultString(strings.ToLower(strings.TrimSpace(identity.Device)), profileMetricIdentitySource)
	identity.UnresolvedSource = defaultString(strings.ToLower(strings.TrimSpace(identity.UnresolvedSource)), profileMetricUnresolvedSourceLabel)
	identity.SourceIDPrivacy = defaultString(strings.ToLower(strings.TrimSpace(identity.SourceIDPrivacy)), profileMetricSourceIDHash)
	switch identity.Device {
	case profileMetricIdentitySource, profileMetricIdentitySourceLabel, profileMetricIdentityListener:
	default:
		return normalizedProfileMetricsConfig{}, fmt.Errorf("profile_metrics.identity.device must be source, source_label, or listener, got %q", cfg.Identity.Device)
	}
	switch identity.UnresolvedSource {
	case profileMetricUnresolvedSourceLabel, profileMetricDropMetricInstance:
	default:
		return normalizedProfileMetricsConfig{}, fmt.Errorf("profile_metrics.identity.unresolved_source must be source_label or drop_metric_instance, got %q", cfg.Identity.UnresolvedSource)
	}
	switch identity.SourceIDPrivacy {
	case profileMetricSourceIDRaw, profileMetricSourceIDHash:
	default:
		return normalizedProfileMetricsConfig{}, fmt.Errorf("profile_metrics.identity.source_id_privacy must be raw or hash, got %q", cfg.Identity.SourceIDPrivacy)
	}

	limits := cfg.Limits
	if limits.MaxRules == 0 {
		limits.MaxRules = defaultProfileMetricMaxRules
	}
	if limits.MaxSources == 0 {
		limits.MaxSources = defaultProfileMetricMaxSources
	}
	if limits.MaxResourcesPerSource == 0 {
		limits.MaxResourcesPerSource = defaultProfileMetricMaxResourcesPerSource
	}
	if limits.MaxInstancesPerJob == 0 {
		limits.MaxInstancesPerJob = defaultProfileMetricMaxInstancesPerJob
	}
	limits.Overflow = defaultString(strings.ToLower(strings.TrimSpace(limits.Overflow)), profileMetricOverflowDropAndCount)
	if limits.MaxRules < 0 || limits.MaxSources < 0 || limits.MaxResourcesPerSource < 0 || limits.MaxInstancesPerJob < 0 {
		return normalizedProfileMetricsConfig{}, fmt.Errorf("profile_metrics.limits values must be non-negative")
	}
	switch limits.Overflow {
	case profileMetricOverflowDropAndCount:
	default:
		return normalizedProfileMetricsConfig{}, fmt.Errorf("profile_metrics.limits.overflow must be drop_and_count, got %q", cfg.Limits.Overflow)
	}

	if !cfg.Enabled || mode == profileMetricModeNone {
		return normalizedProfileMetricsConfig{
			enabled:  false,
			mode:     mode,
			include:  include,
			identity: identity,
			limits:   limits,
		}, nil
	}

	return normalizedProfileMetricsConfig{
		enabled:  true,
		mode:     mode,
		include:  include,
		identity: identity,
		limits:   limits,
	}, nil
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

type profileMetricCatalog struct {
	rulesByName map[string]*profileMetricRule
	chartsByID  map[string]*profileMetricChart
}

func (idx *ProfileIndex) profileMetricCatalog() profileMetricCatalog {
	if idx == nil {
		return profileMetricCatalog{}
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	cat := profileMetricCatalog{
		rulesByName: make(map[string]*profileMetricRule, len(idx.metricRulesByName)),
		chartsByID:  make(map[string]*profileMetricChart, len(idx.metricChartsByID)),
	}
	maps.Copy(cat.rulesByName, idx.metricRulesByName)
	maps.Copy(cat.chartsByID, idx.metricChartsByID)
	return cat
}

func selectProfileMetricRules(cfg normalizedProfileMetricsConfig, cat profileMetricCatalog) ([]*profileMetricRule, error) {
	if !cfg.enabled {
		return nil, nil
	}
	if len(cat.rulesByName) == 0 {
		return nil, nil
	}
	includeSet := make(map[string]bool, len(cfg.include))
	for _, name := range cfg.include {
		includeSet[name] = true
	}
	var selected []*profileMetricRule
	for name, rule := range cat.rulesByName {
		if rule == nil || rule.disabled() {
			continue
		}
		include := includeSet[name]
		switch cfg.mode {
		case profileMetricModeAuto:
			if rule.AutoSafe {
				selected = append(selected, rule)
			}
		case profileMetricModeExact:
			if include {
				selected = append(selected, rule)
			}
		case profileMetricModeCombined:
			if include || rule.AutoSafe {
				selected = append(selected, rule)
			}
		}
	}
	for _, name := range cfg.include {
		rule := cat.rulesByName[name]
		if rule == nil {
			return nil, fmt.Errorf("profile_metrics.include rule %q not found", name)
		}
		if rule.disabled() {
			return nil, fmt.Errorf("profile_metrics.include rule %q is disabled by profile", name)
		}
	}
	if len(selected) > cfg.limits.MaxRules {
		return nil, fmt.Errorf("profile_metrics selected %d rules, above max_rules %d", len(selected), cfg.limits.MaxRules)
	}
	slices.SortFunc(selected, func(a, b *profileMetricRule) int {
		return strings.Compare(a.Name, b.Name)
	})
	return selected, nil
}

func (r *profileMetricRule) disabled() bool {
	return r != nil && r.Enabled != nil && !*r.Enabled
}

type profileMetricRuntime struct {
	mu sync.Mutex

	cfg        normalizedProfileMetricsConfig
	rules      []*compiledProfileMetricRule
	rulesByOID map[string][]*compiledProfileMetricRule
	charts     map[string]*profileMetricChart

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

func newProfileMetricRuntime(cfg normalizedProfileMetricsConfig, idx *ProfileIndex) (*profileMetricRuntime, string, error) {
	if !cfg.enabled {
		return nil, "", nil
	}
	if idx == nil {
		return nil, "", errors.New("profile index not available")
	}
	if err := idx.loadStockProfileMetrics(); err != nil {
		return nil, "", err
	}
	cat := idx.profileMetricCatalog()
	selected, err := selectProfileMetricRules(cfg, cat)
	if err != nil {
		return nil, "", err
	}
	if len(selected) == 0 {
		return nil, "", nil
	}

	rt := &profileMetricRuntime{
		cfg:             cfg,
		charts:          cat.chartsByID,
		series:          make(map[profileMetricSeriesKey]*profileMetricSeries),
		sources:         make(map[string]time.Time),
		sourceRoutes:    make(map[string]string),
		sourceRouteSeen: make(map[string]time.Time),
		resources:       make(map[string]map[string]struct{}),
		chartInstances:  make(map[profileMetricChartInstanceKey]struct{}),
		chartCounts:     make(map[string]int),
		rulesByOID:      make(map[string][]*compiledProfileMetricRule),
		sourceHashSalt:  profileMetricSourceHashSalt(),
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
		return nil, fmt.Errorf("%s: profile metric rule %q references unknown chart %q", rule.sourceFile, rule.Name, rule.Output.Chart)
	}
	compiled.chart = chart
	compiled.expireAfterCycles = defaultProfileMetricExpireAfterCycles
	if chart.Lifecycle != nil && chart.Lifecycle.ExpireAfterCycles > 0 {
		compiled.expireAfterCycles = chart.Lifecycle.ExpireAfterCycles
	}

	addTrap := func(dst map[string]*TrapDef, ref, field string) error {
		td, err := resolveProfileMetricTrap(idx, ref)
		if err != nil {
			return fmt.Errorf("%s: profile metric rule %q %s: %w", rule.sourceFile, rule.Name, field, err)
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
				return nil, fmt.Errorf("%s: profile metric rule %q state.ttl: %w", rule.sourceFile, rule.Name, err)
			}
			compiled.stateTTL = ttl
		}
	}
	if rule.ValueFromVarbind != "" {
		td := firstTrapDef(compiled.trapOIDs)
		vb := trapMetricVarbindByName(td, rule.ValueFromVarbind)
		if vb == nil {
			return nil, fmt.Errorf("%s: profile metric rule %q value_from_varbind %q not found", rule.sourceFile, rule.Name, rule.ValueFromVarbind)
		}
		compiled.valueVarbind = vb
	}
	if rule.Identity.Resource != nil && rule.Identity.Resource.KeyFromVarbind != "" {
		td := firstAnyTrapDef(compiled.trapOIDs, compiled.problemOIDs, compiled.clearOIDs)
		vb := trapMetricVarbindByName(td, rule.Identity.Resource.KeyFromVarbind)
		if vb == nil {
			return nil, fmt.Errorf("%s: profile metric rule %q resource key_from_varbind %q not found", rule.sourceFile, rule.Name, rule.Identity.Resource.KeyFromVarbind)
		}
		compiled.resourceVarbind = vb
	}
	return compiled, nil
}

func resolveProfileMetricTrap(idx *ProfileIndex, ref string) (*TrapDef, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, errors.New("trap reference is empty")
	}
	if isNumericOID(ref) {
		td, err := idx.LookupWithError(ref)
		if err != nil {
			return nil, err
		}
		if td == nil {
			return nil, fmt.Errorf("trap oid %q not found", ref)
		}
		return td, nil
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	if td := idx.namesByTrapName[ref]; td != nil {
		return td, nil
	}
	return nil, fmt.Errorf("trap name %q not found", ref)
}

func metricOIDAliasesFromTrap(td *TrapDef) []string {
	if td == nil || td.OID == "" {
		return nil
	}
	aliases := []string{td.OID}
	if alt := alternateTrapOID(td.OID); alt != td.OID {
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
	if td == nil || name == "" {
		return nil
	}
	if vb := td.varbindByName(name); vb != nil {
		return vb
	}
	return td.inlineVarbindByName(name)
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
		val = rule.rule.Scale.apply(val)
		rt.setSeriesValueLocked(rule, entry, td, val, now)
	case profileMetricTypeState:
		if _, ok := rule.problemOIDs[entry.TrapOID]; ok {
			rt.setSeriesValueLocked(rule, entry, td, rule.rule.stateProblemValue(), now)
			return
		}
		if _, ok := rule.clearOIDs[entry.TrapOID]; ok {
			rt.setSeriesValueLocked(rule, entry, td, rule.rule.stateClearValue(), now)
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
		return r.rule.stateProblemValue(), true
	}
	if r.rule.State.ClearWhen != nil && profileMetricPredicateMatches(*r.rule.State.ClearWhen, entry, td) {
		return r.rule.stateClearValue(), true
	}
	return 0, false
}

func (r *profileMetricRule) stateProblemValue() float64 {
	if r.State.ProblemValue != nil {
		return *r.State.ProblemValue
	}
	return 1
}

func (r *profileMetricRule) stateClearValue() float64 {
	return r.State.ClearValue
}

func (s profileMetricScale) apply(v float64) float64 {
	mul := s.Multiplier
	if mul == 0 {
		mul = 1
	}
	div := s.Divisor
	if div == 0 {
		div = 1
	}
	return v * float64(mul) / float64(div)
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
	return fallbackTrapSourceIdentity(entry, entry.JobName, rt.cfg.identity.SourceIDPrivacy, rt.sourceHashSalt)
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
	v, ok := findVarbindForProfileOID(entry, rule.resourceVarbind.OID)
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
	resourceID := strings.TrimSpace(varbindRawValue(v))
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
			if rule.rule.State.TTLBehavior == profileMetricTTLBehaviorClearAndExpire {
				series.value = rule.rule.stateClearValue()
				series.removeAfterCollect = true
				continue
			}
			delete(rt.series, key)
			removed = true
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
	v, ok := findVarbindForProfileOID(entry, vb.OID)
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
	actual := varbindRawValue(value)
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
	v, ok := findVarbindForProfileOID(entry, vb.OID)
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

func normalizeProfileMetricRule(rule *profileMetricRule) error {
	if rule == nil {
		return errors.New("nil metric rule")
	}
	if rule.Name == "" {
		return errors.New("name is required")
	}
	if !profileMetricRuleNameRE.MatchString(rule.Name) {
		return fmt.Errorf("name %q contains invalid characters", rule.Name)
	}
	rule.Type = strings.ToLower(strings.TrimSpace(rule.Type))
	switch rule.Type {
	case profileMetricTypeCounter, profileMetricTypeSample, profileMetricTypeState:
	default:
		return fmt.Errorf("rule %q type must be counter, sample, or state", rule.Name)
	}
	if rule.Value != "" {
		if rule.ValueFromVarbind != "" {
			return fmt.Errorf("rule %q uses both compact value and value_from_varbind", rule.Name)
		}
		rule.ValueFromVarbind = rule.Value
		rule.Value = ""
	}
	if rule.Metric != "" {
		if rule.Output.Metric != "" {
			return fmt.Errorf("rule %q uses both compact metric and output.metric", rule.Name)
		}
		rule.Output.Metric = rule.Metric
		rule.Metric = ""
	}
	if rule.Dimension != "" {
		if rule.Output.Dimension != "" {
			return fmt.Errorf("rule %q uses both compact dimension and output.dimension", rule.Name)
		}
		rule.Output.Dimension = rule.Dimension
		rule.Dimension = ""
	}
	if rule.ChartID != "" {
		if rule.Output.Chart != "" {
			return fmt.Errorf("rule %q uses both compact chart_id and output.chart", rule.Name)
		}
		rule.Output.Chart = rule.ChartID
		rule.ChartID = ""
	}
	if rule.Identity.Device == "" {
		rule.Identity.Device = profileMetricIdentitySource
	}
	if rule.Resource.Key != "" || rule.Resource.Max != 0 || rule.Resource.Class != "" {
		if rule.Identity.Resource != nil {
			return fmt.Errorf("rule %q uses both compact resource and identity.resource", rule.Name)
		}
		res := rule.Resource
		if res.Key != "" {
			res.KeyFromVarbind = res.Key
		}
		if res.Max != 0 {
			res.MaxPerSource = res.Max
		}
		rule.Identity.Resource = &res
		rule.Resource = profileMetricResource{}
	}
	if rule.Output.Metric == "" {
		rule.Output.Metric = "snmp_trap_" + slugForMetric(rule.Name)
		if !strings.HasSuffix(rule.Output.Metric, "_events") && rule.Type == profileMetricTypeCounter {
			rule.Output.Metric += "_events"
		}
	}
	if rule.Output.Dimension == "" {
		switch rule.Type {
		case profileMetricTypeCounter:
			rule.Output.Dimension = "events"
		case profileMetricTypeSample:
			rule.Output.Dimension = "value"
		case profileMetricTypeState:
			rule.Output.Dimension = "state"
		}
	}
	if rule.Output.Chart == "" {
		rule.Output.Chart = slugForMetric(rule.Name)
	}
	if rule.Missing == "" {
		rule.Missing = profileMetricMissingDrop
	}
	rule.Missing = strings.ToLower(strings.TrimSpace(rule.Missing))
	if rule.Scale.Multiplier == 0 {
		rule.Scale.Multiplier = 1
	}
	if rule.Scale.Divisor == 0 {
		rule.Scale.Divisor = 1
	}
	if rule.Type == profileMetricTypeState {
		if rule.State.TTLBehavior == "" {
			rule.State.TTLBehavior = profileMetricTTLBehaviorClearAndExpire
		}
		if rule.State.Varbind != "" {
			if rule.State.SetWhen != nil || rule.State.ClearWhen != nil {
				return fmt.Errorf("rule %q uses compact state.varbind with canonical set_when/clear_when", rule.Name)
			}
			if rule.State.Set == nil || rule.State.Clear == nil {
				return fmt.Errorf("rule %q compact state.varbind requires state.set and state.clear", rule.Name)
			}
			rule.State.SetWhen = &profileMetricPredicate{Varbind: rule.State.Varbind, Equals: rule.State.Set}
			rule.State.ClearWhen = &profileMetricPredicate{Varbind: rule.State.Varbind, Equals: rule.State.Clear}
			rule.State.Varbind = ""
		}
	}
	return nil
}

func validateProfileMetricRule(rule *profileMetricRule, idx *ProfileIndex, charts map[string]*profileMetricChart) error {
	if err := normalizeProfileMetricRule(rule); err != nil {
		return fmt.Errorf("%s: metric rule: %w", rule.sourceFile, err)
	}
	if !profileMetricOutputNameRE.MatchString(rule.Output.Metric) {
		return fmt.Errorf("%s: metric rule %q output.metric %q must match ^[a-z][a-z0-9_]*$", rule.sourceFile, rule.Name, rule.Output.Metric)
	}
	for _, prefix := range reservedProfileMetricPrefixes {
		if strings.HasPrefix(rule.Output.Metric, prefix) {
			return fmt.Errorf("%s: metric rule %q output.metric %q uses reserved prefix %q", rule.sourceFile, rule.Name, rule.Output.Metric, prefix)
		}
	}
	if !profileMetricOutputNameRE.MatchString(rule.Output.Dimension) {
		return fmt.Errorf("%s: metric rule %q output.dimension %q must match ^[a-z][a-z0-9_]*$", rule.sourceFile, rule.Name, rule.Output.Dimension)
	}
	if !profileMetricChartIDRE.MatchString(rule.Output.Chart) {
		return fmt.Errorf("%s: metric rule %q output.chart %q must match ^[a-z][a-z0-9_]*$", rule.sourceFile, rule.Name, rule.Output.Chart)
	}
	if charts[rule.Output.Chart] == nil {
		return fmt.Errorf("%s: metric rule %q references unknown chart %q", rule.sourceFile, rule.Name, rule.Output.Chart)
	}
	switch rule.Missing {
	case profileMetricMissingDrop, profileMetricMissingZero, profileMetricMissingUnknownDimension, profileMetricMissingError:
	default:
		return fmt.Errorf("%s: metric rule %q missing must be drop, zero, unknown_dimension, or error", rule.sourceFile, rule.Name)
	}
	switch rule.Type {
	case profileMetricTypeCounter:
		if rule.OnTrap == "" || rule.ProblemTrap != "" || rule.ClearTrap != "" || rule.ValueFromVarbind != "" {
			return fmt.Errorf("%s: counter rule %q requires only on_trap", rule.sourceFile, rule.Name)
		}
		if _, err := resolveProfileMetricTrap(idx, rule.OnTrap); err != nil {
			return fmt.Errorf("%s: counter rule %q on_trap: %w", rule.sourceFile, rule.Name, err)
		}
	case profileMetricTypeSample:
		if rule.OnTrap == "" || rule.ValueFromVarbind == "" {
			return fmt.Errorf("%s: sample rule %q requires on_trap and value_from_varbind", rule.sourceFile, rule.Name)
		}
		td, err := resolveProfileMetricTrap(idx, rule.OnTrap)
		if err != nil {
			return fmt.Errorf("%s: sample rule %q on_trap: %w", rule.sourceFile, rule.Name, err)
		}
		vb := trapMetricVarbindByName(td, rule.ValueFromVarbind)
		if vb == nil {
			return fmt.Errorf("%s: sample rule %q value_from_varbind %q not found", rule.sourceFile, rule.Name, rule.ValueFromVarbind)
		}
		if !isProfileMetricNumericVarbind(vb) {
			return fmt.Errorf("%s: sample rule %q value_from_varbind %q is non-numeric type %q", rule.sourceFile, rule.Name, rule.ValueFromVarbind, vb.Type)
		}
		if rule.Missing == profileMetricMissingUnknownDimension {
			return fmt.Errorf("%s: sample rule %q missing unknown_dimension requires identity.resource", rule.sourceFile, rule.Name)
		}
		if charts[rule.Output.Chart].Algorithm != "absolute" {
			return fmt.Errorf("%s: sample rule %q chart %q must use absolute algorithm", rule.sourceFile, rule.Name, rule.Output.Chart)
		}
	case profileMetricTypeState:
		if rule.OnTrap != "" {
			if rule.State.SetWhen == nil || rule.State.ClearWhen == nil || rule.ProblemTrap != "" || rule.ClearTrap != "" {
				return fmt.Errorf("%s: same-OID state rule %q requires on_trap with state set_when and clear_when only", rule.sourceFile, rule.Name)
			}
			if _, err := resolveProfileMetricTrap(idx, rule.OnTrap); err != nil {
				return fmt.Errorf("%s: state rule %q on_trap: %w", rule.sourceFile, rule.Name, err)
			}
		} else {
			if rule.ProblemTrap == "" || rule.ClearTrap == "" {
				return fmt.Errorf("%s: separate-OID state rule %q requires problem_trap and clear_trap", rule.sourceFile, rule.Name)
			}
			if _, err := resolveProfileMetricTrap(idx, rule.ProblemTrap); err != nil {
				return fmt.Errorf("%s: state rule %q problem_trap: %w", rule.sourceFile, rule.Name, err)
			}
			if _, err := resolveProfileMetricTrap(idx, rule.ClearTrap); err != nil {
				return fmt.Errorf("%s: state rule %q clear_trap: %w", rule.sourceFile, rule.Name, err)
			}
		}
		if charts[rule.Output.Chart].Algorithm != "absolute" {
			return fmt.Errorf("%s: state rule %q chart %q must use absolute algorithm", rule.sourceFile, rule.Name, rule.Output.Chart)
		}
	}
	if rule.Type != profileMetricTypeSample && rule.Missing == profileMetricMissingZero {
		return fmt.Errorf("%s: metric rule %q missing zero is supported only for sample rules", rule.sourceFile, rule.Name)
	}
	if rule.Missing == profileMetricMissingUnknownDimension && rule.Identity.Resource == nil {
		return fmt.Errorf("%s: metric rule %q missing unknown_dimension requires identity.resource", rule.sourceFile, rule.Name)
	}
	if rule.Scale.Divisor <= 0 {
		return fmt.Errorf("%s: metric rule %q scale.divisor must be greater than zero", rule.sourceFile, rule.Name)
	}
	if rule.Scale.Multiplier < 0 {
		return fmt.Errorf("%s: metric rule %q scale.multiplier must be zero or greater", rule.sourceFile, rule.Name)
	}
	if rule.State.TTLBehavior != "" && rule.State.TTLBehavior != profileMetricTTLBehaviorClearAndExpire {
		return fmt.Errorf("%s: metric rule %q state.ttl_behavior must be %s", rule.sourceFile, rule.Name, profileMetricTTLBehaviorClearAndExpire)
	}
	if rule.State.TTL != "" {
		if _, err := parseProfileMetricStateTTL(rule.State.TTL); err != nil {
			return fmt.Errorf("%s: metric rule %q state.ttl %q is invalid: %w", rule.sourceFile, rule.Name, rule.State.TTL, err)
		}
	}
	for _, pred := range rule.Where {
		if err := validateProfileMetricPredicate(pred); err != nil {
			return fmt.Errorf("%s: metric rule %q where: %w", rule.sourceFile, rule.Name, err)
		}
	}
	if rule.State.SetWhen != nil {
		if err := validateProfileMetricPredicate(*rule.State.SetWhen); err != nil {
			return fmt.Errorf("%s: metric rule %q state.set_when: %w", rule.sourceFile, rule.Name, err)
		}
	}
	if rule.State.ClearWhen != nil {
		if err := validateProfileMetricPredicate(*rule.State.ClearWhen); err != nil {
			return fmt.Errorf("%s: metric rule %q state.clear_when: %w", rule.sourceFile, rule.Name, err)
		}
	}
	if err := validateProfileMetricPredicateReferences(rule, idx); err != nil {
		return fmt.Errorf("%s: metric rule %q: %w", rule.sourceFile, rule.Name, err)
	}
	if err := validateProfileMetricIdentity(rule); err != nil {
		return fmt.Errorf("%s: metric rule %q: %w", rule.sourceFile, rule.Name, err)
	}
	if err := validateProfileMetricResourceVarbind(rule, idx); err != nil {
		return fmt.Errorf("%s: metric rule %q: %w", rule.sourceFile, rule.Name, err)
	}
	return nil
}

func validateProfileMetricPredicateReferences(rule *profileMetricRule, idx *ProfileIndex) error {
	traps, err := profileMetricRuleTrapDefs(rule, idx)
	if err != nil {
		return err
	}
	for i, pred := range rule.Where {
		if err := validateProfileMetricPredicateReference(pred, traps); err != nil {
			return fmt.Errorf("where[%d]: %w", i, err)
		}
	}
	if rule.State.SetWhen != nil {
		if err := validateProfileMetricPredicateReference(*rule.State.SetWhen, traps); err != nil {
			return fmt.Errorf("state.set_when: %w", err)
		}
	}
	if rule.State.ClearWhen != nil {
		if err := validateProfileMetricPredicateReference(*rule.State.ClearWhen, traps); err != nil {
			return fmt.Errorf("state.clear_when: %w", err)
		}
	}
	return nil
}

func validateProfileMetricPredicateReference(pred profileMetricPredicate, traps []*TrapDef) error {
	if pred.Field != "" {
		if isProfileMetricSyntheticField(pred.Field) {
			return nil
		}
		return fmt.Errorf("field %q is not supported", pred.Field)
	}
	if pred.Varbind == "" {
		return fmt.Errorf("varbind or field is required")
	}
	for _, td := range traps {
		if trapMetricVarbindByName(td, pred.Varbind) == nil {
			return fmt.Errorf("varbind %q not found on trap %q", pred.Varbind, td.Name)
		}
	}
	return nil
}

func isProfileMetricSyntheticField(field string) bool {
	switch field {
	case "category", "severity", "trap_name", "trap_oid":
		return true
	default:
		return false
	}
}

func validateProfileMetricPredicate(pred profileMetricPredicate) error {
	if pred.Not && (pred.Exists != nil || pred.Absent != nil) {
		return fmt.Errorf("not cannot be combined with exists or absent")
	}
	if pred.Range != nil && len(pred.Range) != 2 {
		return fmt.Errorf("range requires exactly two values")
	}
	if pred.GreaterThan != nil {
		if err := validateProfileMetricPredicateNumber("greater_than", pred.GreaterThan); err != nil {
			return err
		}
	}
	if pred.LessThan != nil {
		if err := validateProfileMetricPredicateNumber("less_than", pred.LessThan); err != nil {
			return err
		}
	}
	if len(pred.Range) == 2 {
		var bounds [2]float64
		for i, value := range pred.Range {
			if err := validateProfileMetricPredicateNumber(fmt.Sprintf("range[%d]", i), value); err != nil {
				return err
			}
			bounds[i], _ = profileMetricFloatValue(value)
		}
		if bounds[0] > bounds[1] {
			return fmt.Errorf("range[0] must be less than or equal to range[1]")
		}
	} else {
		for i, value := range pred.Range {
			if err := validateProfileMetricPredicateNumber(fmt.Sprintf("range[%d]", i), value); err != nil {
				return err
			}
		}
	}
	if pred.Equals == nil &&
		len(pred.In) == 0 &&
		pred.Exists == nil &&
		pred.Absent == nil &&
		pred.GreaterThan == nil &&
		pred.LessThan == nil &&
		len(pred.Range) == 0 {
		return fmt.Errorf("predicate requires at least one condition")
	}
	return nil
}

func validateProfileMetricPredicateNumber(field string, value any) error {
	if _, ok := profileMetricFloatValue(value); !ok {
		return fmt.Errorf("%s must be a finite number", field)
	}
	return nil
}

func validateProfileMetricResourceVarbind(rule *profileMetricRule, idx *ProfileIndex) error {
	if rule.Identity.Resource == nil {
		return nil
	}
	traps, err := profileMetricRuleTrapDefs(rule, idx)
	if err != nil {
		return err
	}
	key := rule.Identity.Resource.KeyFromVarbind
	var resourceOID string
	for _, td := range traps {
		vb := trapMetricVarbindByName(td, key)
		if vb == nil {
			return fmt.Errorf("identity.resource.key_from_varbind %q not found on trap %q", key, td.Name)
		}
		if !isProfileMetricResourceKeyVarbind(vb) {
			return fmt.Errorf("identity.resource.key_from_varbind %q has unsupported type %q on trap %q; use an integer-like bounded resource key", key, vb.Type, td.Name)
		}
		if resourceOID == "" {
			resourceOID = vb.OID
			continue
		}
		if vb.OID != resourceOID {
			return fmt.Errorf("identity.resource.key_from_varbind %q resolves to different OIDs across state traps", key)
		}
	}
	return nil
}

func profileMetricRuleTrapDefs(rule *profileMetricRule, idx *ProfileIndex) ([]*TrapDef, error) {
	var refs []string
	switch rule.Type {
	case profileMetricTypeCounter, profileMetricTypeSample:
		refs = append(refs, rule.OnTrap)
	case profileMetricTypeState:
		if rule.OnTrap != "" {
			refs = append(refs, rule.OnTrap)
		} else {
			refs = append(refs, rule.ProblemTrap, rule.ClearTrap)
		}
	}
	traps := make([]*TrapDef, 0, len(refs))
	for _, ref := range refs {
		td, err := resolveProfileMetricTrap(idx, ref)
		if err != nil {
			return nil, err
		}
		traps = append(traps, td)
	}
	return traps, nil
}

func isProfileMetricResourceKeyVarbind(vb *VarbindDef) bool {
	if vb == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(vb.Type)) {
	case "integer", "integer32", "unsigned32", "gauge32":
		return true
	default:
		return false
	}
}

func validateProfileMetricIdentity(rule *profileMetricRule) error {
	switch rule.Identity.Device {
	case "", profileMetricIdentitySource, profileMetricIdentitySourceLabel, profileMetricIdentityListener:
	default:
		return fmt.Errorf("identity.device must be source, source_label, or listener")
	}
	if rule.Identity.Resource == nil {
		return nil
	}
	res := rule.Identity.Resource
	if res.Class == "" || !profileMetricResourceClassRE.MatchString(res.Class) {
		return fmt.Errorf("identity.resource.class %q must match ^[a-z][a-z0-9_]*$", res.Class)
	}
	if !stockProfileMetricResourceClasses[res.Class] && !profileMetricSiteClassRE.MatchString(res.Class) {
		return fmt.Errorf("identity.resource.class %q must be a stock class or match ^site_[a-z0-9][a-z0-9_]*$", res.Class)
	}
	if res.KeyFromVarbind == "" {
		return fmt.Errorf("identity.resource.key_from_varbind is required")
	}
	if res.MaxPerSource < 0 {
		return fmt.Errorf("identity.resource.max_per_source must be zero or greater")
	}
	return nil
}

func isProfileMetricNumericVarbind(vb *VarbindDef) bool {
	if vb == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(vb.Type)) {
	case "integer", "integer32", "unsigned32", "gauge32", "counter32", "counter64", "timeticks":
		return true
	default:
		return false
	}
}

func normalizeProfileMetricChart(chart *profileMetricChart) error {
	if chart == nil {
		return errors.New("nil chart")
	}
	if chart.ID == "" || !profileMetricChartIDRE.MatchString(chart.ID) {
		return fmt.Errorf("chart id %q must match ^[a-z][a-z0-9_]*$", chart.ID)
	}
	if builtInProfileMetricChartIDs[chart.ID] {
		return fmt.Errorf("chart id %q collides with built-in SNMP trap chart", chart.ID)
	}
	if chart.Title == "" {
		return fmt.Errorf("chart %q title is required", chart.ID)
	}
	if chart.Context == "" {
		chart.Context = "snmp.trap." + chart.ID
	}
	if !strings.HasPrefix(chart.Context, "snmp.trap.") {
		return fmt.Errorf("chart %q context %q must start with snmp.trap.", chart.ID, chart.Context)
	}
	if builtInProfileMetricChartContexts[chart.Context] {
		return fmt.Errorf("chart %q context %q collides with built-in SNMP trap chart", chart.ID, chart.Context)
	}
	if chart.Units == "" {
		return fmt.Errorf("chart %q units is required", chart.ID)
	}
	if chart.Algorithm == "" {
		chart.Algorithm = "incremental"
	}
	switch chart.Algorithm {
	case "incremental", "absolute":
	default:
		return fmt.Errorf("chart %q algorithm %q is unsupported", chart.ID, chart.Algorithm)
	}
	if chart.Type == "" {
		chart.Type = "line"
	}
	switch chart.Type {
	case "line", "area", "stacked", "heatmap":
	default:
		return fmt.Errorf("chart %q type %q is unsupported", chart.ID, chart.Type)
	}
	if chart.Lifecycle == nil {
		chart.Lifecycle = &charttpl.Lifecycle{
			MaxInstances:      defaultProfileMetricChartMaxInstances,
			ExpireAfterCycles: defaultProfileMetricExpireAfterCycles,
		}
	}
	if chart.Lifecycle.MaxInstances <= 0 {
		chart.Lifecycle.MaxInstances = defaultProfileMetricChartMaxInstances
	}
	if chart.Lifecycle.ExpireAfterCycles <= 0 {
		chart.Lifecycle.ExpireAfterCycles = defaultProfileMetricExpireAfterCycles
	}
	return nil
}

func chartFromMetricRule(rule *profileMetricRule) *profileMetricChart {
	if rule == nil || rule.ChartMeta == nil {
		return nil
	}
	id := rule.Output.Chart
	if id == "" {
		id = slugForMetric(rule.Name)
	}
	units := rule.ChartMeta.Units
	if units == "" {
		switch rule.Type {
		case profileMetricTypeCounter:
			units = "events/s"
		case profileMetricTypeState:
			units = "state"
		default:
			units = "value"
		}
	}
	algorithm := rule.ChartMeta.Algorithm
	if algorithm == "" {
		if rule.Type == profileMetricTypeCounter {
			algorithm = "incremental"
		} else {
			algorithm = "absolute"
		}
	}
	title := rule.ChartMeta.Title
	if title == "" {
		title = strings.ReplaceAll(rule.Name, "::", " ")
	}
	context := rule.ChartMeta.Context
	if context == "" {
		context = "snmp.trap." + strings.ReplaceAll(id, "_", ".")
	}
	return &profileMetricChart{
		ID:          id,
		Title:       title,
		Family:      rule.ChartMeta.Family,
		Context:     context,
		Units:       units,
		Algorithm:   algorithm,
		Type:        rule.ChartMeta.Type,
		Description: rule.ChartMeta.Description,
		Lifecycle:   rule.ChartMeta.Lifecycle,
		sourceFile:  rule.sourceFile,
	}
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
					rule.rule.sourceFile, rule.rule.Name, chartID, dimension, existing.rule.Name, existing.rule.sourceFile)
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
