// SPDX-License-Identifier: GPL-3.0-or-later

package profilemetrics

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/attribution"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
)

type Catalog interface {
	Definitions([]string) (catalog.MetricDefinitions, error)
	ResolveTrap(string) (*catalog.TrapDef, error)
}

type Options struct {
	BaseChartTemplateYAML string
	SourceHashSalt        string
}

// New compiles the selected rules and their chart template for one collector job.
func New(cfg Policy, idx Catalog, opts Options) (*Runtime, error) {
	if !cfg.enabled {
		return nil, nil
	}
	if idx == nil {
		return nil, errors.New("profile index not available")
	}
	defs, err := idx.Definitions(cfg.include)
	if err != nil {
		return nil, err
	}
	cat := profileMetricCatalog{rulesByName: defs.RulesByName, chartsByID: defs.ChartsByID}
	selected, err := selectProfileMetricRules(cfg, cat)
	if err != nil {
		return nil, err
	}
	if len(selected) == 0 {
		return nil, nil
	}

	rt := &Runtime{
		cfg:            cfg,
		series:         make(map[profileMetricSeriesKey]*profileMetricSeries),
		sources:        make(map[string]time.Time),
		sourceRoutes:   attribution.NewRouteTracker(cfg.limits.MaxSources),
		resources:      make(map[string]map[string]struct{}),
		chartInstances: make(map[profileMetricChartInstanceKey]struct{}),
		chartCounts:    make(map[string]int),
		rulesByOID:     make(map[string][]*compiledProfileMetricRule),
		sourceHashSalt: opts.SourceHashSalt,
	}
	for _, rule := range selected {
		compiled, err := compileProfileMetricRule(rule, cat, idx)
		if err != nil {
			return nil, err
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
	yml, err := buildProfileMetricChartTemplateYAML(opts.BaseChartTemplateYAML, rt.rules, cat.chartsByID)
	if err != nil {
		return nil, err
	}
	rt.chartTemplate = yml
	return rt, nil
}

func (rt *Runtime) ChartTemplateYAML() string {
	if rt == nil {
		return ""
	}
	return rt.chartTemplate
}

func compileProfileMetricRule(rule *profileMetricRule, cat profileMetricCatalog, idx Catalog) (*compiledProfileMetricRule, error) {
	if rule == nil {
		return nil, errors.New("nil profile metric rule")
	}
	compiled := &compiledProfileMetricRule{
		rule:        rule,
		trapOIDs:    make(map[string]*catalog.TrapDef),
		problemOIDs: make(map[string]*catalog.TrapDef),
		clearOIDs:   make(map[string]*catalog.TrapDef),
	}
	chart := cat.chartsByID[rule.Output.Chart]
	if chart == nil {
		return nil, fmt.Errorf("%s: profile metric rule %q references unknown chart %q", rule.Source(), rule.Name, rule.Output.Chart)
	}
	compiled.chart = chart
	compiled.expireAfterCycles = catalog.DefaultMetricExpireAfterCycles
	if chart.Lifecycle != nil && chart.Lifecycle.ExpireAfterCycles > 0 {
		compiled.expireAfterCycles = chart.Lifecycle.ExpireAfterCycles
	}

	addTrap := func(dst map[string]*catalog.TrapDef, ref, field string) error {
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
	case catalog.MetricTypeCounter, catalog.MetricTypeSample:
		if err := addTrap(compiled.trapOIDs, rule.OnTrap, "on_trap"); err != nil {
			return nil, err
		}
	case catalog.MetricTypeState:
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
			ttl, err := catalog.ParseMetricStateTTL(rule.State.TTL)
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

func resolveProfileMetricTrap(idx Catalog, ref string) (*catalog.TrapDef, error) {
	return idx.ResolveTrap(ref)
}

func metricOIDAliasesFromTrap(td *catalog.TrapDef) []string {
	if td == nil || td.OID == "" {
		return nil
	}
	aliases := []string{td.OID}
	if alt := model.AlternateTrapOID(td.OID); alt != td.OID {
		aliases = append(aliases, alt)
	}
	return aliases
}

func firstTrapDef(m map[string]*catalog.TrapDef) *catalog.TrapDef {
	for _, td := range m {
		return td
	}
	return nil
}

func firstAnyTrapDef(mapsIn ...map[string]*catalog.TrapDef) *catalog.TrapDef {
	for _, m := range mapsIn {
		if td := firstTrapDef(m); td != nil {
			return td
		}
	}
	return nil
}

func trapMetricVarbindByName(td *catalog.TrapDef, name string) *catalog.VarbindDef {
	return td.VarbindByName(name)
}
