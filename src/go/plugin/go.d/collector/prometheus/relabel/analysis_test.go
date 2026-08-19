// SPDX-License-Identifier: GPL-3.0-or-later

package relabel

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	benchmarkReplacementTokens []replacementToken
	benchmarkReplacementOK     bool
)

func TestAnalyzerEnumerateFiniteRegexp(t *testing.T) {
	analyzer := newTestAnalyzer(t)
	tests := map[string]struct {
		expr   string
		want   []string
		finite bool
	}{
		"literal alternatives": {expr: `app_(a|b)`, want: []string{"app_a", "app_b"}, finite: true},
		"unicode":              {expr: `métrique_(été|hiver)`, want: []string{"métrique_hiver", "métrique_été"}, finite: true},
		"bounded repeat":       {expr: `a{1,2}`, want: []string{"a", "aa"}, finite: true},
		"deduplicated":         {expr: `(a|a)`, want: []string{"a"}, finite: true},
		"open repeat":          {expr: `a+`},
		"open wildcard":        {expr: `.*`},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, finite, err := analyzer.EnumerateFiniteRegexp(tc.expr)
			require.NoError(t, err)
			assert.Equal(t, tc.finite, finite)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestAnalyzerReplacementOutputs(t *testing.T) {
	tests := map[string]struct {
		expr        string
		replacement string
		want        []string
		finite      bool
	}{
		"finite suffix capture": {
			expr: `app_worker_(.+)_(temperature|requests_total)`, replacement: `app_${2}`,
			want: []string{"app_requests_total", "app_temperature"}, finite: true,
		},
		"nested finite capture in open capture": {
			expr: `app_worker_(.+(temperature|requests_total))`, replacement: `app_${2}`,
		},
		"dynamic capture": {expr: `app_worker_(.+)_old`, replacement: `${1}`},
		"constant": {
			expr: `app_temperature_(.+)`, replacement: `app_temperature`,
			want: []string{"app_temperature"}, finite: true,
		},
		"capture absent on one branch": {
			expr: `app_(foo|(bar))_(.+)`, replacement: `app_${2}`,
			want: []string{"app_", "app_bar"}, finite: true,
		},
		"correlated finite captures stay correlated": {
			expr: `(a)|(b)`, replacement: `${1}${2}`,
			want: []string{"a", "b"}, finite: true,
		},
		"ambiguous named capture": {
			expr: `app_(?P<kind>temperature)|app_(?P<kind>requests_total)`, replacement: `app_${kind}`,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			analyzer := newTestAnalyzer(t)
			got, finite, err := analyzer.ReplacementOutputs(MustNewRegexp(tc.expr), tc.replacement)
			require.NoError(t, err)
			assert.Equal(t, tc.finite, finite)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestAnalyzerFiniteResultsMatchRuntimeOnCompleteSmallDomains(t *testing.T) {
	tests := []struct {
		expr        string
		replacement string
		alphabet    []rune
		maxLength   int
	}{
		{expr: `[ab]{0,2}`, replacement: `prefix_${0}`, alphabet: []rune{'a', 'b'}, maxLength: 2},
		{expr: `(a|b)(c|)`, replacement: `${1}_${2}`, alphabet: []rune{'a', 'b', 'c'}, maxLength: 2},
		{expr: `m(é|a)`, replacement: `metric_${1}`, alphabet: []rune{'m', 'a', 'é'}, maxLength: 2},
	}
	for _, tc := range tests {
		t.Run(tc.expr, func(t *testing.T) {
			analyzer := newTestAnalyzer(t)
			regexp := MustNewRegexp(tc.expr)
			gotInputs, finite, err := analyzer.EnumerateFiniteRegexp(tc.expr)
			require.NoError(t, err)
			require.True(t, finite)
			gotOutputs, finite, err := analyzer.ReplacementOutputs(regexp, tc.replacement)
			require.NoError(t, err)
			require.True(t, finite)

			wantInputs := make(map[string]struct{})
			wantOutputs := make(map[string]struct{})
			for _, candidate := range finiteAnalysisTestStrings(tc.alphabet, tc.maxLength) {
				if regexp.MatchString(candidate) {
					wantInputs[candidate] = struct{}{}
					wantOutputs[regexp.ReplaceAllString(candidate, tc.replacement)] = struct{}{}
				}
			}
			assert.Equal(t, slices.Sorted(maps.Keys(wantInputs)), gotInputs)
			assert.Equal(t, slices.Sorted(maps.Keys(wantOutputs)), gotOutputs)
		})
	}
}

func TestAnalyzerRuleMayWriteMetricName(t *testing.T) {
	tests := map[string]struct {
		action      Action
		regex       string
		target      string
		replacement string
		want        bool
	}{
		"static labelmap destination": {
			action: LabelMap, regex: "metric_name", replacement: labels.MetricName, want: true,
		},
		"finite safe labelmap captures": {
			action: LabelMap, regex: "(instance|family)", replacement: "$1",
		},
		"finite reachable labelmap captures": {
			action: LabelMap, regex: "(name|instance)", replacement: "__${1}__", want: true,
		},
		"identity labelmap excludes metric name input": {
			action: LabelMap, regex: "(.*)", replacement: "$1",
		},
		"finite safe replace target": {
			action: Replace, regex: "(instance|family)", target: "__${1}__",
		},
		"finite reachable replace target": {
			action: Replace, regex: "(name|instance)", target: "__${1}__", want: true,
		},
		"literal safe label": {action: Replace, regex: "(.*)", target: "instance"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			analyzer := newTestAnalyzer(t)
			got, err := analyzer.RuleMayWriteLabel(Config{
				Regex: MustNewRegexp(tc.regex), TargetLabel: tc.target, Replacement: tc.replacement, Action: tc.action,
			}, labels.MetricName)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestAnalyzerAppliesProcessorRuleDefaults(t *testing.T) {
	analyzer := newTestAnalyzer(t)

	preserves, err := analyzer.RulesPreserveLabel([]Config{{Action: LabelDrop}}, "instance", nil)
	require.NoError(t, err)
	assert.False(t, preserves, "the default (.*) labeldrop regexp removes every label")

	writes, err := analyzer.RuleMayWriteLabel(Config{Action: LabelMap}, labels.MetricName)
	require.NoError(t, err)
	assert.False(t, writes, "the default (.*) -> $1 labelmap preserves each input name")
}

func TestAnalyzerReplacementGlobEscapesLiteralMetacharacters(t *testing.T) {
	analyzer := newTestAnalyzer(t)
	pattern, possible, err := analyzer.ReplacementGlob(MustNewRegexp(`(.+)`), `literal[*]_${1}`)
	require.NoError(t, err)
	require.True(t, possible)
	assert.Equal(t, `literal\[\*\]_*`, pattern)
}

func TestAnalyzerHonorsCancellationAndBudget(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	analyzer, err := NewAnalyzer(ctx, AnalysisBudget{})
	require.NoError(t, err)
	_, _, err = analyzer.EnumerateFiniteRegexp(`[`) // cancellation must win before parsing
	assert.ErrorIs(t, err, context.Canceled)

	_, err = analyzer.RulesMayAffectFutureRouting([]Config{{Action: Drop}})
	assert.ErrorIs(t, err, context.Canceled)

	analyzer, err = NewAnalyzer(context.Background(), AnalysisBudget{MaxValues: 256, MaxOperations: 1})
	require.NoError(t, err)
	_, _, err = analyzer.EnumerateFiniteRegexp(strings.Repeat(`(`, 128)) // budget must win before parsing
	assert.ErrorIs(t, err, ErrAnalysisBudgetExceeded)

	analyzer, err = NewAnalyzer(context.Background(), AnalysisBudget{MaxValues: 256, MaxOperations: 128})
	require.NoError(t, err)
	_, _, err = analyzer.ReplacementOutputs(MustNewRegexp(`(.*)`), strings.Repeat(`$-`, 512))
	assert.ErrorIs(t, err, ErrAnalysisBudgetExceeded)
}

func TestAnalyzerMaxValuesIncludesOptionalEmptyValue(t *testing.T) {
	tests := map[string]func(*Analyzer) ([]string, bool, error){
		"optional regexp": func(analyzer *Analyzer) ([]string, bool, error) {
			return analyzer.EnumerateFiniteRegexp(`a?`)
		},
		"optional capture projection": func(analyzer *Analyzer) ([]string, bool, error) {
			return analyzer.ReplacementOutputs(MustNewRegexp(`.+|(b)`), `${1}`)
		},
	}

	for name, run := range tests {
		t.Run(name, func(t *testing.T) {
			analyzer, err := NewAnalyzer(context.Background(), AnalysisBudget{MaxValues: 1})
			require.NoError(t, err)
			values, finite, err := run(analyzer)
			require.NoError(t, err)
			assert.False(t, finite)
			assert.Nil(t, values)
		})
	}
}

func TestAnalyzerDeduplicatesConcatenationBeforeMaxValues(t *testing.T) {
	analyzer, err := NewAnalyzer(context.Background(), AnalysisBudget{MaxValues: 3})
	require.NoError(t, err)

	values, finite, err := analyzer.EnumerateFiniteRegexp(`ab?b?`)
	require.NoError(t, err)
	assert.True(t, finite)
	assert.Equal(t, []string{"a", "ab", "abb"}, values)
}

func TestParseReplacementTemplateAllocationEnvelope(t *testing.T) {
	template := strings.Repeat(`$-`, 512)
	allocations := testing.AllocsPerRun(10, func() {
		tokens, ok := parseReplacementTemplate(template, captureMetadata{})
		if !ok || len(tokens) != 1 {
			panic("unexpected replacement parse")
		}
	})
	assert.Less(t, allocations, float64(20))
}

func FuzzAnalyzerFiniteRegexpRuntimeParity(f *testing.F) {
	for _, seed := range []string{`a|b`, `métrique_(été|hiver)`, `[a-c]{1,2}`, `(foo)?`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, expr string) {
		analyzer, err := NewAnalyzer(context.Background(), AnalysisBudget{MaxValues: 64, MaxOperations: 10_000})
		require.NoError(t, err)
		values, finite, err := analyzer.EnumerateFiniteRegexp(expr)
		if err != nil || !finite {
			return
		}
		re, err := NewRegexp(expr)
		if err != nil {
			t.Fatalf("analysis accepted invalid regexp %q", expr)
		}
		for _, value := range values {
			assert.Truef(t, re.MatchString(value), "regexp %q did not match enumerated value %q", expr, value)
		}
		assert.True(t, slices.IsSorted(values))
	})
}

func BenchmarkAnalyzerReplacementOutputs(b *testing.B) {
	for range b.N {
		analyzer, err := NewAnalyzer(context.Background(), AnalysisBudget{})
		if err != nil {
			b.Fatal(err)
		}
		_, finite, err := analyzer.ReplacementOutputs(
			MustNewRegexp(`app_worker_(.+)_(temperature|requests_total)`), `app_${2}`,
		)
		if err != nil || !finite {
			b.Fatalf("finite=%v err=%v", finite, err)
		}
	}
}

func BenchmarkParseReplacementTemplate(b *testing.B) {
	for _, size := range []int{128, 1024, 8192} {
		template := strings.Repeat(`$-`, size/2)
		b.Run(fmt.Sprintf("bytes_%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				benchmarkReplacementTokens, benchmarkReplacementOK = parseReplacementTemplate(template, captureMetadata{})
			}
		})
	}
}

func newTestAnalyzer(t *testing.T) *Analyzer {
	t.Helper()
	analyzer, err := NewAnalyzer(context.Background(), AnalysisBudget{})
	require.NoError(t, err)
	return analyzer
}

func finiteAnalysisTestStrings(alphabet []rune, maxLength int) []string {
	values := []string{""}
	frontier := []string{""}
	for range maxLength {
		var next []string
		for _, prefix := range frontier {
			for _, value := range alphabet {
				next = append(next, prefix+string(value))
			}
		}
		values = append(values, next...)
		frontier = next
	}
	return values
}

func TestAnalysisBudgetRejectsNegativeValues(t *testing.T) {
	_, err := NewAnalyzer(context.Background(), AnalysisBudget{MaxValues: -1})
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrAnalysisBudgetExceeded))
}
