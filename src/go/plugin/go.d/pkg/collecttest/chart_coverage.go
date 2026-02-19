// SPDX-License-Identifier: GPL-3.0-or-later

package collecttest

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/charttpl"
)

type chartCoverage struct {
	ActualByContext   map[string]map[string]struct{}
	ExpectedByContext map[string][]string
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
// coverage and optional explicit required context checks.
func AssertChartCoverage(
	t *testing.T,
	collector interface {
		MetricStore() metrix.CollectorStore
		ChartTemplateYAML() string
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
	templateYAML := collector.ChartTemplateYAML()

	coverage, err := buildChartCoverage(
		templateYAML,
		1,
		store.Read(metrix.ReadRaw(), metrix.ReadFlatten()),
		exp.ExcludeContextPatterns,
	)
	if err != nil {
		t.Fatalf("collecttest: build chart coverage: %v", err)
		return
	}

	for contextName, dims := range coverage.ExpectedByContext {
		requireContextDims(t, coverage.ActualByContext, contextName, dims)
	}
	for contextName, dims := range exp.RequiredContexts {
		requireContextDims(t, coverage.ActualByContext, contextName, dims)
	}
}

func buildChartCoverage(
	templateYAML string,
	revision uint64,
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

	plan, err := buildPlanFromTemplate(templateYAML, revision, reader)
	if err != nil {
		return chartCoverage{}, err
	}

	actualByContext := materializedContextsByPattern(plan, contextMatchers)
	expectedByContext, err := expectedTemplateCoverage(templateYAML, reader, contextMatchers)
	if err != nil {
		return chartCoverage{}, err
	}

	return chartCoverage{
		ActualByContext:   actualByContext,
		ExpectedByContext: expectedByContext,
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

func expectedTemplateCoverage(
	templateYAML string,
	reader metrix.Reader,
	contextMatchers []matcher.Matcher,
) (map[string][]string, error) {
	spec, err := charttpl.DecodeYAML([]byte(templateYAML))
	if err != nil {
		return nil, err
	}

	byContextSet := make(map[string]map[string]struct{})
	selectorMatchCache := make(map[string]bool)
	rootContext := normalizeOptionalContextPart(spec.ContextNamespace)

	for i := range spec.Groups {
		if err := collectTemplateContexts(
			byContextSet,
			spec.Groups[i],
			rootContext,
			reader,
			contextMatchers,
			selectorMatchCache,
		); err != nil {
			return nil, err
		}
	}

	out := make(map[string][]string, len(byContextSet))
	for contextName, dimSet := range byContextSet {
		dims := make([]string, 0, len(dimSet))
		for dimName := range dimSet {
			dims = append(dims, dimName)
		}
		sort.Strings(dims)
		out[contextName] = dims
	}
	return out, nil
}

func collectTemplateContexts(
	out map[string]map[string]struct{},
	group charttpl.Group,
	parentContextParts []string,
	reader metrix.Reader,
	contextMatchers []matcher.Matcher,
	selectorMatchCache map[string]bool,
) error {
	scopeContext := append([]string(nil), parentContextParts...)
	scopeContext = append(scopeContext, normalizeOptionalContextPart(group.ContextNamespace)...)

	for _, chart := range group.Charts {
		parts := append([]string(nil), scopeContext...)
		parts = append(parts, strings.TrimSpace(chart.Context))
		contextName := strings.Join(filterEmptyString(parts), ".")
		if contextName == "" || matchesAny(contextName, contextMatchers) {
			continue
		}

		matchedAnyDimension := false
		dims := make(map[string]struct{})
		for _, dim := range chart.Dimensions {
			matched, err := selectorMatchesReader(reader, strings.TrimSpace(dim.Selector), selectorMatchCache)
			if err != nil {
				return err
			}
			if !matched {
				continue
			}
			matchedAnyDimension = true
			if name := strings.TrimSpace(dim.Name); name != "" {
				dims[name] = struct{}{}
			}
		}
		if !matchedAnyDimension {
			continue
		}

		existingDims, ok := out[contextName]
		if !ok {
			existingDims = make(map[string]struct{})
			out[contextName] = existingDims
		}
		for name := range dims {
			existingDims[name] = struct{}{}
		}
	}

	for i := range group.Groups {
		if err := collectTemplateContexts(
			out,
			group.Groups[i],
			scopeContext,
			reader,
			contextMatchers,
			selectorMatchCache,
		); err != nil {
			return err
		}
	}
	return nil
}

func selectorMatchesReader(
	reader metrix.Reader,
	selectorExpr string,
	cache map[string]bool,
) (bool, error) {
	if matched, ok := cache[selectorExpr]; ok {
		return matched, nil
	}

	sel, err := metrixselector.Parse(selectorExpr)
	if err != nil {
		return false, fmt.Errorf("collecttest: invalid selector %q: %w", selectorExpr, err)
	}

	matched := false
	reader.ForEachSeries(func(name string, labels metrix.LabelView, _ metrix.SampleValue) {
		if matched {
			return
		}
		if sel.Matches(name, labels) {
			matched = true
		}
	})
	cache[selectorExpr] = matched
	return matched, nil
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

func requireContextDims(t *testing.T, byContext map[string]map[string]struct{}, contextName string, dimNames []string) {
	t.Helper()

	dims, ok := byContext[contextName]
	if !ok {
		t.Fatalf("collecttest: missing chart context %q", contextName)
		return
	}
	for _, name := range dimNames {
		if _, exists := dims[name]; !exists {
			t.Fatalf("collecttest: missing dimension %q in context %q", name, contextName)
			return
		}
	}
}
