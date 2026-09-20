// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

// TemplateEntry is one independently owned native template. ID is local to the job;
// it does not change public chart IDs or contexts. AutogenRules restrict fallback
// while the entry is active and are not replaced by a job's global policy override.
type TemplateEntry struct {
	ID               string
	ContextNamespace string
	Groups           []charttpl.Group
	AutogenRules     []charttpl.EngineAutogenRule
}

// TemplateSetSpec describes the complete desired set. Entry order determines
// precedence for unowned collisions, but does not determine entry identity.
// Policy and FallbackContextNamespace are fixed for a running collector job.
type TemplateSetSpec struct {
	Entries                  []TemplateEntry
	Policy                   EnginePolicy
	FallbackContextNamespace string
}

// TemplateSet is an immutable, prepared snapshot. Reuse it until desired content
// changes. A new snapshot may reuse entry IDs; only changed entries restart their
// chart lifecycle. Returned entry data is detached from the snapshot.
type TemplateSet struct {
	entries    []TemplateEntry
	content    []templateEntryContent
	entryByID  map[string]int
	program    *program.Program
	index      matchIndex
	global     templateGlobalPolicy
	policy     effectiveEnginePolicy
	entryRules []charttpl.ValidatedAutogenRule
	legacy     bool
}

type templateGlobalPolicy struct {
	autogen   AutogenPolicy
	selector  *metrixselector.Expr
	namespace string
}

// NewTemplateSet freezes, normalizes, validates and compiles native entries.
// An empty set is valid and can serve only automatic charts.
func NewTemplateSet(spec TemplateSetSpec) (*TemplateSet, error) {
	cfg, err := applyOptions(WithEnginePolicy(spec.Policy))
	if err != nil {
		return nil, err
	}
	global := templateGlobalPolicy{
		autogen:   cloneAutogenPolicy(cfg.autogen),
		selector:  normalizedSelectorExpr(spec.Policy.Selector),
		namespace: strings.Join(normalizeOptional(spec.FallbackContextNamespace), "."),
	}
	policy := effectiveEnginePolicy{
		autogen:      cfg.autogen,
		autogenRules: cfg.autogenRules,
		selector:     cfg.selector,
	}
	return compileTemplateSet(spec.Entries, global, policy, false, false)
}

// NewTemplateSetYAML adapts one legacy document. It preserves document chart paths
// and diagnostics, and shares the compiler with native sets. Job policy overrides
// are applied by PrepareTemplateSet before the first collection.
func NewTemplateSetYAML(data []byte) (*TemplateSet, error) {
	spec, validation, err := charttpl.DecodeYAMLValidated(data)
	if err != nil {
		return nil, err
	}
	policy, err := resolveEffectivePolicy(engineConfig{}, spec.Engine, validation.AutogenRules())
	if err != nil {
		return nil, err
	}
	global := templateGlobalPolicy{
		autogen:   cloneAutogenPolicy(policy.autogen),
		namespace: spec.ContextNamespace,
	}
	if spec.Engine != nil {
		global.selector = normalizedSelectorExpr(spec.Engine.Selector)
	}
	return compileTemplateSet([]TemplateEntry{{ID: "document", ContextNamespace: spec.ContextNamespace, Groups: spec.Groups}}, global, policy, true, true)
}

func compileTemplateSet(entries []TemplateEntry, global templateGlobalPolicy, policy effectiveEnginePolicy, legacy, normalized bool) (*TemplateSet, error) {
	set := &TemplateSet{
		entryByID: make(map[string]int, len(entries)),
		global:    normalizedGlobalPolicy(global),
		policy:    policy,
		legacy:    legacy,
	}
	c := compiler{
		metricsSet: make(map[string]struct{}),
	}
	rootOffset := 0
	for _, input := range entries {
		if strings.TrimSpace(input.ID) == "" {
			return nil, fmt.Errorf("chartengine: template entry ID is required")
		}
		if _, exists := set.entryByID[input.ID]; exists {
			return nil, fmt.Errorf("chartengine: duplicate template entry ID %q", input.ID)
		}
		entry := input
		entry.ContextNamespace = strings.Join(normalizeOptional(input.ContextNamespace), ".")
		entry.AutogenRules = normalizedAutogenRules(input.AutogenRules)
		var err error
		if normalized {
			entry.Groups = cloneTemplateGroups(input.Groups)
		} else {
			entry.Groups, err = charttpl.NormalizeGroups(input.Groups)
			if err != nil {
				return nil, fmt.Errorf("chartengine: entry %q: %w", input.ID, err)
			}
		}
		// Defaults are already applied. Exclude their redundant spelling from equality.
		clearTemplateDefaults(entry.Groups)
		rules, err := charttpl.CompileAutogenRules(entry.AutogenRules)
		if err != nil {
			return nil, fmt.Errorf("chartengine: entry %q autogen rules: %w", input.ID, err)
		}
		set.entryRules = append(set.entryRules, rules...)
		start := len(c.charts)
		if err := c.compileSpec(&charttpl.Spec{
			Version:          charttpl.VersionV1,
			ContextNamespace: entry.ContextNamespace,
			Groups:           entry.Groups,
		}); err != nil {
			return nil, fmt.Errorf("chartengine: entry %q: %w", input.ID, err)
		}
		set.content = append(set.content, normalizedEntryContent(entry, c.charts[start:]))
		for i := start; i < len(c.charts); i++ {
			chart := &c.charts[i]
			if !legacy {
				chart.EntryID = entry.ID
				chart.LocalTemplateID = chart.TemplateID
				chart.RoutingOrder = templateRoutingOrder(chart.TemplateID, rootOffset)
				chart.TemplateID = strconv.Itoa(len(entry.ID)) + ":" + entry.ID + "/" + chart.LocalTemplateID
			}
		}
		rootOffset += len(entry.Groups)
		set.entryByID[entry.ID] = len(set.entries)
		set.entries = append(set.entries, entry)
	}
	compiled, err := program.New(charttpl.VersionV1, 0, c.metricNames(), c.charts)
	if err != nil {
		return nil, err
	}
	set.program = compiled
	set.index = buildMatchIndex(c.charts)
	set.policy.autogenRules = append(slices.Clone(policy.autogenRules), set.entryRules...)
	return set, nil
}

func templateRoutingOrder(localID string, rootOffset int) string {
	end := strings.IndexByte(localID, '.')
	root, _ := strconv.Atoi(localID[1:end]) // compiler-generated path
	return "g" + strconv.Itoa(root+rootOffset) + localID[end:]
}

func cloneTemplateGroups(groups []charttpl.Group) []charttpl.Group {
	out := make([]charttpl.Group, len(groups))
	for i, group := range groups {
		out[i] = group.Clone()
	}
	return out
}

func clearTemplateDefaults(groups []charttpl.Group) {
	for i := range groups {
		groups[i].ChartDefaults = nil
		clearTemplateDefaults(groups[i].Groups)
	}
}

func normalizedSelectorExpr(expr *metrixselector.Expr) *metrixselector.Expr {
	if expr == nil || expr.Empty() {
		return nil
	}
	out := &metrixselector.Expr{
		Allow: slices.Clone(expr.Allow),
		Deny:  slices.Clone(expr.Deny),
	}
	for _, values := range [][]string{out.Allow, out.Deny} {
		for i := range values {
			values[i] = strings.TrimSpace(values[i])
		}
		slices.Sort(values)
	}
	out.Allow = append([]string(nil), slices.Compact(out.Allow)...)
	out.Deny = append([]string(nil), slices.Compact(out.Deny)...)
	return out
}

// PrepareTemplateSet binds fixed job options without recompiling the immutable
// program or entry restrictions. Global overrides never replace entry restrictions.
func PrepareTemplateSet(set *TemplateSet, opts ...Option) (*TemplateSet, error) {
	if set == nil || set.program == nil {
		return nil, fmt.Errorf("chartengine: invalid template set; use NewTemplateSet or NewTemplateSetYAML")
	}
	cfg, err := applyOptions(opts...)
	if err != nil {
		return nil, err
	}
	if !cfg.autogenOverride.set && !cfg.selectorOverride.set {
		return set, nil
	}
	out := *set
	if cfg.autogenOverride.set {
		out.global.autogen = cloneAutogenPolicy(cfg.autogenOverride.value)
		out.global.autogen.Rules = normalizedAutogenRules(out.global.autogen.Rules)
		out.policy.autogen = out.global.autogen
		out.policy.autogenRules = append(slices.Clone(cfg.autogenRulesOverride.value), set.entryRules...)
	}
	if cfg.selectorOverride.set {
		out.global.selector = normalizedSelectorExpr(cfg.selectorExprOverride)
		out.policy.selector = cfg.selectorOverride.value
	}
	return &out, nil
}

// SameGlobalPolicy reports whether a running job may accept the next snapshot.
// Entry restrictions are intentionally excluded: their lifetime follows entries.
func (s *TemplateSet) SameGlobalPolicy(other *TemplateSet) bool {
	return s != nil && other != nil && reflect.DeepEqual(s.global, other.global)
}

// Entries returns owned native definitions for coverage and authoring tools.
func (s *TemplateSet) Entries() []TemplateEntry {
	if s == nil {
		return nil
	}
	out := make([]TemplateEntry, len(s.entries))
	for i, entry := range s.entries {
		entry.Groups = cloneTemplateGroups(entry.Groups)
		entry.AutogenRules = cloneAutogenRules(entry.AutogenRules)
		out[i] = entry
	}
	return out
}

func (s *TemplateSet) preservesEntry(previous *TemplateSet, entryID string) bool {
	if s == nil || previous == nil || s.legacy != previous.legacy {
		return false
	}
	nextIndex, nextOK := s.entryByID[entryID]
	oldIndex, oldOK := previous.entryByID[entryID]
	return nextOK && oldOK && reflect.DeepEqual(s.content[nextIndex], previous.content[oldIndex])
}

// Compare compiled values so identity follows the same defaults as runtime.
// Retain local structure and declarations, but exclude compiled matcher objects
// and set-wide routing positions. Selector equivalence beyond trimming is not inferred.
type templateEntryContent struct {
	namespace string
	groups    []templateGroupShape
	charts    []program.Chart
	rules     []charttpl.EngineAutogenRule
}

type templateGroupShape struct {
	groups  int
	charts  int
	metrics []string
}

func normalizedEntryContent(entry TemplateEntry, charts []program.Chart) templateEntryContent {
	out := templateEntryContent{
		namespace: entry.ContextNamespace,
		rules:     entry.AutogenRules,
	}
	var visit func([]charttpl.Group)
	visit = func(groups []charttpl.Group) {
		for _, group := range groups {
			metrics := normalizeUnique(group.Metrics)
			slices.Sort(metrics)
			out.groups = append(out.groups, templateGroupShape{
				groups:  len(group.Groups),
				charts:  len(group.Charts),
				metrics: metrics,
			})
			visit(group.Groups)
		}
	}
	visit(entry.Groups)
	out.charts = slices.Clone(charts)
	for i := range out.charts {
		chart := &out.charts[i]
		chart.Dimensions = slices.Clone(chart.Dimensions)
		for j := range chart.Dimensions {
			selector := &chart.Dimensions[j].Selector
			selector.Matcher = nil
			selector.Expression = strings.TrimSpace(selector.Expression)
		}
	}
	return out
}

func normalizedAutogenRules(rules []charttpl.EngineAutogenRule) []charttpl.EngineAutogenRule {
	out := cloneAutogenRules(rules)
	for i := range out {
		out[i].Scope = strings.TrimSpace(out[i].Scope)
		for _, values := range [][]string{out[i].Selector.Allow, out[i].Selector.Deny} {
			for j := range values {
				values[j] = strings.TrimSpace(values[j])
			}
		}
		out[i].Selector.Allow = append([]string(nil), out[i].Selector.Allow...)
		out[i].Selector.Deny = append([]string(nil), out[i].Selector.Deny...)
	}
	return out
}

func normalizedGlobalPolicy(global templateGlobalPolicy) templateGlobalPolicy {
	global.autogen.Rules = normalizedAutogenRules(global.autogen.Rules)
	return global
}

// GlobalPolicy returns detached effective global policy for inspection. Entry
// restrictions are available through Entries and are excluded from this policy.
func (s *TemplateSet) GlobalPolicy() EnginePolicy {
	if s == nil {
		return EnginePolicy{}
	}
	return cloneEnginePolicy(EnginePolicy{
		Autogen:  &s.global.autogen,
		Selector: s.global.selector,
	})
}
