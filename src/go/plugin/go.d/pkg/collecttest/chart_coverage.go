// SPDX-License-Identifier: GPL-3.0-or-later

package collecttest

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	histogramBucketLabel = "le"
	summaryQuantileLabel = "quantile"
)

type chartCoverage struct {
	ActualByContext   map[string]map[string]struct{}
	ExpectedByContext map[string][]string
	// Chart variables, by context, as dimensions are.
	ActualVariablesByContext   map[string]map[string]struct{}
	ExpectedVariablesByContext map[string][]string
	// ShadowedVariables lists chart variables that a materialized dimension of
	// the same chart hides from health, which reads the dimension first.
	ShadowedVariables []string
	// UnboundVariables lists variable values whose series render a chart
	// instance that was not updated: no dimension series shares their identity.
	UnboundVariables []string
}

type scopedChartCoverage struct {
	ScopeKey string
	Coverage chartCoverage
}

// ChartCoverageExpectation defines per-scenario chart coverage assertions.
type ChartCoverageExpectation struct {
	// ExcludeContextPatterns are glob patterns for contexts ignored from
	// template-derived coverage assertions in this scenario.
	ExcludeContextPatterns []string
	// RequiredContexts defines additional required context/dimension checks.
	// Key: chart context, value: required dimension names.
	RequiredContexts map[string][]string
}

// AssertChartCoverage validates chart materialization against template-derived
// coverage and optional explicit required context checks. Template-derived checks
// exclude routes rejected by the planner for chart-ID collisions; they do not
// independently verify collision winners. Pin intended winners with RequiredContexts
// or explicit plan assertions when testing overlapping templates.
//
// Chart variables are covered like dimensions: a declared variable of an expected
// chart whose selector matches a current series carrying the chart's required
// instance labels must be sent with that chart's values. A materialized dimension
// with the name of a declared variable of its chart fails the check.
func AssertChartCoverage(
	t *testing.T,
	collector interface {
		MetricStore() metrix.CollectorStore
	},
	exp ChartCoverageExpectation,
) {
	t.Helper()

	if collector == nil {
		t.Fatalf("collecttest: nil collector")
		return
	}
	store := collector.MetricStore()
	if store == nil {
		t.Fatalf("collecttest: nil metric store")
		return
	}
	source, err := collectorapi.NewChartTemplateSource(collector)
	if err != nil {
		t.Fatalf("collecttest: chart provider: %v", err)
		return
	}
	set, err := source.Capture()
	if err != nil {
		t.Fatalf("collecttest: chart template: %v", err)
		return
	}
	coverages, err := buildChartCoveragesFromStore(set, store, exp.ExcludeContextPatterns)
	if err != nil {
		t.Fatalf("collecttest: build chart coverage: %v", err)
		return
	}

	for _, scoped := range coverages {
		for contextName, dims := range scoped.Coverage.ExpectedByContext {
			requireContextDims(t, scoped.Coverage.ActualByContext, contextName, dims, scoped.ScopeKey)
		}
		for contextName, names := range scoped.Coverage.ExpectedVariablesByContext {
			requireContextVariables(t, scoped.Coverage.ActualVariablesByContext, contextName, names, scoped.ScopeKey)
		}
		for _, problem := range append(scoped.Coverage.ShadowedVariables, scoped.Coverage.UnboundVariables...) {
			t.Fatalf("collecttest: %s%s", problem, scopeSuffix(scoped.ScopeKey))
		}
		for contextName, dims := range exp.RequiredContexts {
			requireContextDims(t, scoped.Coverage.ActualByContext, contextName, dims, scoped.ScopeKey)
		}
	}
}

func buildChartCoveragesFromStore(
	set *chartengine.TemplateSet,
	store metrix.CollectorStore,
	excludeContextPatterns []string,
) ([]scopedChartCoverage, error) {
	reader := store.Read(metrix.ReadRaw(), metrix.ReadFlatten())
	scopes := reader.HostScopes()
	if len(scopes) == 0 {
		coverage, err := buildChartCoverage(set, reader, excludeContextPatterns)
		if err != nil {
			return nil, err
		}
		return []scopedChartCoverage{{Coverage: coverage}}, nil
	}

	out := make([]scopedChartCoverage, 0, len(scopes))
	for _, scope := range scopes {
		reader := store.Read(metrix.ReadRaw(), metrix.ReadFlatten(), metrix.ReadHostScope(scope.ScopeKey))
		coverage, err := buildChartCoverage(set, reader, excludeContextPatterns)
		if err != nil {
			return nil, err
		}
		out = append(out, scopedChartCoverage{ScopeKey: scope.ScopeKey, Coverage: coverage})
	}
	return out, nil
}

func buildChartCoverage(
	set *chartengine.TemplateSet,
	reader metrix.Reader,
	excludeContextPatterns []string,
) (chartCoverage, error) {
	if reader == nil {
		return chartCoverage{}, fmt.Errorf("collecttest: nil reader")
	}

	contextMatchers, err := compileContextGlobMatchers(excludeContextPatterns)
	if err != nil {
		return chartCoverage{}, err
	}

	collisions := make(map[coverageRouteKey]struct{})
	var unbound []chartengine.PlanRouteDiagnostic
	omitted := make(map[string]bool)
	engine, err := chartengine.New(
		chartengine.WithRuntimeStore(nil),
		chartengine.WithPlanRouteDiagnosticObserver(func(fact chartengine.PlanRouteDiagnostic) {
			switch {
			case fact.Decision == chartengine.PlanRouteCollisionRejected && !fact.Autogen:
				collisions[coverageRouteKey{
					templateID:     fact.ChartTemplateID,
					dimensionIndex: fact.DimensionIndex,
					series:         fact.SeriesIdentity,
				}] = struct{}{}
			case fact.Decision == chartengine.PlanRouteContextRejected,
				fact.Decision == chartengine.PlanRouteLifecycleRejected &&
					fact.Reason == chartengine.PlanRouteReasonChartInstanceCap:
				omitted[fact.ChartID] = true
			case fact.Decision == chartengine.PlanRouteVariableUnbound &&
				fact.Reason == chartengine.PlanRouteReasonVariableChartNotUpdated &&
				!matchesAny(fact.Context, contextMatchers):
				// A variable owned by another template's chart follows the collision rules instead.
				unbound = append(unbound, fact)
			}
		}),
	)
	if err != nil {
		return chartCoverage{}, err
	}
	attempt, err := engine.PreparePlanWithOptions(reader, chartengine.PlanOptions{TemplateSet: set})
	if err != nil {
		return chartCoverage{}, err
	}
	defer attempt.Abort()
	plan := attempt.Plan()

	actualByContext := materializedContextsByPattern(plan, contextMatchers)
	actualVariables, shadowed := materializedVariablesByContext(plan, contextMatchers)
	var unboundVariables []string
	for _, fact := range unbound {
		// Charts omitted by an output limit or instance cap are not a template mismatch.
		if omitted[fact.ChartID] {
			continue
		}
		unboundVariables = append(unboundVariables, fmt.Sprintf(
			"chart variable %q has series for chart %q, which was not updated: no dimension series has the same instance identity",
			fact.VariableName,
			fact.ChartID,
		))
	}
	expectedByContext, expectedVariables, err := expectedTemplateCoverage(set, reader, contextMatchers, collisions)
	if err != nil {
		return chartCoverage{}, err
	}

	return chartCoverage{
		ActualByContext:            actualByContext,
		ExpectedByContext:          expectedByContext,
		ActualVariablesByContext:   actualVariables,
		ExpectedVariablesByContext: expectedVariables,
		ShadowedVariables:          shadowed,
		UnboundVariables:           unboundVariables,
	}, nil
}

func compileContextGlobMatchers(patterns []string) ([]matcher.Matcher, error) {
	out := make([]matcher.Matcher, 0, len(patterns))
	for i, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		m, err := matcher.NewGlobMatcher(pattern)
		if err != nil {
			return nil, fmt.Errorf("collecttest: invalid context glob pattern[%d]=%q: %w", i, pattern, err)
		}
		out = append(out, m)
	}
	return out, nil
}

func matchesAny(value string, matchers []matcher.Matcher) bool {
	for _, m := range matchers {
		if m.MatchString(value) {
			return true
		}
	}
	return false
}

func materializedContextsByPattern(plan chartengine.Plan, contextMatchers []matcher.Matcher) map[string]map[string]struct{} {
	out := make(map[string]map[string]struct{})
	for _, action := range plan.Actions {
		switch v := action.(type) {
		case chartengine.CreateChartAction:
			if matchesAny(v.Meta.Context, contextMatchers) {
				continue
			}
			if _, ok := out[v.Meta.Context]; !ok {
				out[v.Meta.Context] = make(map[string]struct{})
			}
		case chartengine.CreateDimensionAction:
			if matchesAny(v.ChartMeta.Context, contextMatchers) {
				continue
			}
			dims, ok := out[v.ChartMeta.Context]
			if !ok {
				dims = make(map[string]struct{})
				out[v.ChartMeta.Context] = dims
			}
			dims[v.Name] = struct{}{}
		}
	}
	return out
}

// materializedVariablesByContext returns the chart variables sent with chart
// values, by context, and the declared variables a materialized dimension of the
// same chart shadows. The coverage engine is fresh, so every chart it updates is
// created in the same plan.
func materializedVariablesByContext(
	plan chartengine.Plan,
	contextMatchers []matcher.Matcher,
) (map[string]map[string]struct{}, []string) {
	created := make(map[string]chartengine.CreateChartAction)
	dimsByChart := make(map[string][]string)
	for _, action := range plan.Actions {
		switch v := action.(type) {
		case chartengine.CreateChartAction:
			created[v.ChartID] = v
		case chartengine.CreateDimensionAction:
			dimsByChart[v.ChartID] = append(dimsByChart[v.ChartID], v.Name)
		}
	}

	out := make(map[string]map[string]struct{})
	for _, action := range plan.Actions {
		update, ok := action.(chartengine.UpdateChartAction)
		if !ok || len(update.Variables) == 0 {
			continue
		}
		contextName := created[update.ChartID].Meta.Context
		if matchesAny(contextName, contextMatchers) {
			continue
		}
		names, ok := out[contextName]
		if !ok {
			names = make(map[string]struct{})
			out[contextName] = names
		}
		for _, variable := range update.Variables {
			names[variable.Name] = struct{}{}
		}
	}

	var shadowed []string
	for chartID, chart := range created {
		if matchesAny(chart.Meta.Context, contextMatchers) {
			continue
		}
		for _, dim := range dimsByChart[chartID] {
			if slices.Contains(chart.Variables, dim) {
				shadowed = append(shadowed, fmt.Sprintf(
					"dimension %q of chart %q has the name of a chart variable; health reads the dimension",
					dim,
					chartID,
				))
			}
		}
	}
	sort.Strings(shadowed)
	return out, shadowed
}

type coverageRouteKey struct {
	templateID     string
	dimensionIndex int
	series         metrix.SeriesIdentity
}

type templateCoverageBuilder struct {
	set                *chartengine.TemplateSet
	reader             metrix.Reader
	contextMatchers    []matcher.Matcher
	selectorParseCache map[string]metrixselector.Selector
	globalSelector     metrixselector.Selector
	collisions         map[coverageRouteKey]struct{}
	byContext          map[string]map[string]struct{}
	variablesByContext map[string]map[string]struct{}
}

func expectedTemplateCoverage(
	set *chartengine.TemplateSet,
	reader metrix.Reader,
	contextMatchers []matcher.Matcher,
	collisions map[coverageRouteKey]struct{},
) (map[string][]string, map[string][]string, error) {
	policy := set.GlobalPolicy()
	entries := set.Entries()
	b := templateCoverageBuilder{
		set:                set,
		reader:             reader,
		contextMatchers:    contextMatchers,
		selectorParseCache: make(map[string]metrixselector.Selector),
		collisions:         collisions,
		byContext:          make(map[string]map[string]struct{}),
		variablesByContext: make(map[string]map[string]struct{}),
	}
	if policy.Selector != nil {
		var err error
		b.globalSelector, err = policy.Selector.Parse()
		if err != nil {
			return nil, nil, err
		}
	}
	for _, entry := range entries {
		rootContext := normalizeOptionalContextPart(entry.ContextNamespace)
		for i, group := range entry.Groups {
			if err := b.collectTemplateContexts(group, rootContext, entry.ID, []int{i}); err != nil {
				return nil, nil, err
			}
		}
	}
	return sortedNamesByContext(b.byContext), sortedNamesByContext(b.variablesByContext), nil
}

func sortedNamesByContext(byContext map[string]map[string]struct{}) map[string][]string {
	out := make(map[string][]string, len(byContext))
	for contextName, nameSet := range byContext {
		names := make([]string, 0, len(nameSet))
		for name := range nameSet {
			names = append(names, name)
		}
		sort.Strings(names)
		out[contextName] = names
	}
	return out
}

func (b *templateCoverageBuilder) collectTemplateContexts(
	group charttpl.Group,
	parentContextParts []string,
	entryID string,
	groupPath []int,
) error {
	scopeContext := append([]string(nil), parentContextParts...)
	scopeContext = append(scopeContext, normalizeOptionalContextPart(group.ContextNamespace)...)

	for chartIndex, chart := range group.Charts {
		parts := append([]string(nil), scopeContext...)
		parts = append(parts, strings.TrimSpace(chart.Context))
		contextName := strings.Join(filterEmptyString(parts), ".")
		if contextName == "" || matchesAny(contextName, b.contextMatchers) {
			continue
		}
		instances, err := chartengine.ResolveInstanceLabelPolicy(chart.Instances)
		if err != nil {
			return err
		}
		templateID, ok := b.set.ChartTemplateIDAt(entryID, groupPath, chartIndex)
		if !ok {
			return fmt.Errorf("collecttest: entry %q chart %v/%d has no compiled template", entryID, groupPath, chartIndex)
		}
		matchedAnyDimension := false
		dims := make(map[string]struct{})
		for i, dim := range chart.Dimensions {
			key := coverageRouteKey{templateID: templateID, dimensionIndex: i}
			dimNames, matched, err := b.collectExpectedDimensionNames(dim, instances.RequiredKeys, key)
			if err != nil {
				return err
			}
			if !matched {
				continue
			}
			matchedAnyDimension = true
			for _, name := range dimNames {
				dims[name] = struct{}{}
			}
		}
		if !matchedAnyDimension {
			continue
		}

		existingDims, ok := b.byContext[contextName]
		if !ok {
			existingDims = make(map[string]struct{})
			b.byContext[contextName] = existingDims
		}
		for name := range dims {
			existingDims[name] = struct{}{}
		}
		for _, variable := range chart.Variables {
			matched, err := b.matchesExpectedVariable(variable, instances.RequiredKeys)
			if err != nil {
				return err
			}
			if !matched {
				continue
			}
			names, ok := b.variablesByContext[contextName]
			if !ok {
				names = make(map[string]struct{})
				b.variablesByContext[contextName] = names
			}
			names[strings.TrimSpace(variable.Name)] = struct{}{}
		}
	}

	for i := range group.Groups {
		path := append(append([]int(nil), groupPath...), i)
		if err := b.collectTemplateContexts(group.Groups[i], scopeContext, entryID, path); err != nil {
			return err
		}
	}
	return nil
}

func (b *templateCoverageBuilder) collectExpectedDimensionNames(
	dim charttpl.Dimension,
	requiredKeys []string,
	key coverageRouteKey,
) ([]string, bool, error) {
	selectorExpr := strings.TrimSpace(dim.Selector)
	sel, err := parseSelectorCached(selectorExpr, b.selectorParseCache)
	if err != nil {
		return nil, false, err
	}

	explicitName := strings.TrimSpace(dim.Name)
	nameFromLabel := strings.TrimSpace(dim.NameFromLabel)
	names := make(map[string]struct{})
	matched := false
	lastSuccessSeq := b.reader.CollectMeta().LastSuccessSeq

	b.reader.ForEachSeriesIdentity(func(identity metrix.SeriesIdentity, meta metrix.SeriesMeta, metricName string, labels metrix.LabelView, _ metrix.SampleValue) {
		if err != nil {
			return
		}
		// Keep expected coverage aligned with chartengine's default series selection.
		if meta.LastSeenSuccessSeq != lastSuccessSeq {
			return
		}
		if b.globalSelector != nil && !b.globalSelector.Matches(metricName, labels) {
			return
		}
		if !sel.Matches(metricName, labels) {
			return
		}
		for _, required := range requiredKeys {
			if _, present := labels.Get(required); !present {
				return
			}
		}
		key.series = identity
		if _, rejected := b.collisions[key]; rejected {
			return
		}
		matched = true
		name, ok, nameErr := resolveExpectedDimensionName(explicitName, nameFromLabel, metricName, labels, meta)
		if nameErr != nil {
			err = nameErr
			return
		}
		if !ok {
			return
		}
		names[name] = struct{}{}
	})
	if err != nil {
		return nil, false, err
	}

	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	sort.Strings(out)
	return out, matched, nil
}

// matchesExpectedVariable reports whether a current series can feed the variable:
// it passes the global and variable selectors and carries the chart's required
// instance labels, as the planner requires to bind it to a chart instance.
func (b *templateCoverageBuilder) matchesExpectedVariable(
	variable charttpl.ChartVariable,
	requiredKeys []string,
) (bool, error) {
	sel, err := parseSelectorCached(strings.TrimSpace(variable.Selector), b.selectorParseCache)
	if err != nil {
		return false, err
	}
	matched := false
	lastSuccessSeq := b.reader.CollectMeta().LastSuccessSeq
	b.reader.ForEachSeriesIdentity(
		func(_ metrix.SeriesIdentity, meta metrix.SeriesMeta, metricName string, labels metrix.LabelView, _ metrix.SampleValue) {
			if matched || meta.LastSeenSuccessSeq != lastSuccessSeq {
				return
			}
			if b.globalSelector != nil && !b.globalSelector.Matches(metricName, labels) {
				return
			}
			if !sel.Matches(metricName, labels) {
				return
			}
			for _, required := range requiredKeys {
				if _, present := labels.Get(required); !present {
					return
				}
			}
			matched = true
		},
	)
	return matched, nil
}

func parseSelectorCached(selectorExpr string, cache map[string]metrixselector.Selector) (metrixselector.Selector, error) {
	if sel, ok := cache[selectorExpr]; ok {
		return sel, nil
	}
	sel, err := metrixselector.Parse(selectorExpr)
	if err != nil {
		return nil, fmt.Errorf("collecttest: invalid selector %q: %w", selectorExpr, err)
	}
	cache[selectorExpr] = sel
	return sel, nil
}

func resolveExpectedDimensionName(
	explicitName string,
	nameFromLabel string,
	metricName string,
	labels metrix.LabelView,
	meta metrix.SeriesMeta,
) (string, bool, error) {
	if explicitName != "" {
		return explicitName, true, nil
	}
	if nameFromLabel != "" {
		value, ok := labels.Get(nameFromLabel)
		if !ok || strings.TrimSpace(value) == "" {
			return "", false, nil
		}
		return value, true, nil
	}

	labelKey, ok, err := inferExpectedDimensionLabelKey(metricName, meta)
	if err != nil {
		return "", false, err
	}
	if !ok {
		return "", false, nil
	}
	value, ok := labels.Get(labelKey)
	if !ok || strings.TrimSpace(value) == "" {
		return "", false, nil
	}
	return value, true, nil
}

func inferExpectedDimensionLabelKey(metricName string, meta metrix.SeriesMeta) (string, bool, error) {
	switch meta.FlattenRole {
	case metrix.FlattenRoleHistogramBucket:
		return histogramBucketLabel, true, nil
	case metrix.FlattenRoleSummaryQuantile:
		return summaryQuantileLabel, true, nil
	case metrix.FlattenRoleStateSetState:
		if strings.TrimSpace(metricName) == "" {
			return "", false, fmt.Errorf("collecttest: stateset inference requires metric family name")
		}
		return metricName, true, nil
	case metrix.FlattenRoleHistogramCount,
		metrix.FlattenRoleHistogramSum,
		metrix.FlattenRoleSummaryCount,
		metrix.FlattenRoleSummarySum:
		return "", false, nil
	case metrix.FlattenRoleNone:
		return "", false, fmt.Errorf("collecttest: inferred dimension requires flattened reader metadata; use store.Read(metrix.ReadFlatten())")
	default:
		return "", false, fmt.Errorf("collecttest: unsupported flatten role %d for expected dimension inference", meta.FlattenRole)
	}
}

func normalizeOptionalContextPart(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return []string{value}
}

func filterEmptyString(values []string) []string {
	out := values[:0]
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}

func requireContextDims(t *testing.T, byContext map[string]map[string]struct{}, contextName string, dimNames []string, scopeKey string) {
	t.Helper()

	dims, ok := byContext[contextName]
	if !ok {
		if scopeKey != "" {
			t.Fatalf("collecttest: missing chart context %q in host scope %q", contextName, scopeKey)
			return
		}
		t.Fatalf("collecttest: missing chart context %q", contextName)
		return
	}
	for _, name := range dimNames {
		if _, exists := dims[name]; !exists {
			if scopeKey != "" {
				t.Fatalf("collecttest: missing dimension %q in context %q in host scope %q", name, contextName, scopeKey)
				return
			}
			t.Fatalf("collecttest: missing dimension %q in context %q", name, contextName)
			return
		}
	}
}

func requireContextVariables(
	t *testing.T,
	byContext map[string]map[string]struct{},
	contextName string,
	names []string,
	scopeKey string,
) {
	t.Helper()
	sent := byContext[contextName]
	for _, name := range names {
		if _, ok := sent[name]; !ok {
			t.Fatalf("collecttest: missing chart variable %q in context %q%s", name, contextName, scopeSuffix(scopeKey))
			return
		}
	}
}

func scopeSuffix(scopeKey string) string {
	if scopeKey == "" {
		return ""
	}
	return fmt.Sprintf(" in host scope %q", scopeKey)
}
