// SPDX-License-Identifier: GPL-3.0-or-later

package matcher

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGlobIntersectionWitness(t *testing.T) {
	tests := map[string]struct {
		left     string
		right    string
		exclude  []string
		want     bool
		wantText string
	}{
		"disjoint class":    {left: "app_a*", right: "app_[b]*"},
		"overlapping class": {left: "app_[ab]*", right: "app_b*", want: true, wantText: "app_b"},
		"negated class":     {left: "app_?", right: "app_[^a]", want: true, wantText: "app__"},
		"stars match empty": {left: "app_*", right: "app_", want: true, wantText: "app_"},
		"ordered exclusions": {
			left: "app_*", right: "app_[ab]*", exclude: []string{"app_a*", "app_b*"},
		},
		"unicode":     {left: "métrique_*", right: "métrique_[é]", want: true, wantText: "métrique_é"},
		"multi range": {left: "[a-f0-4]", right: "[d-z3-9]", want: true, wantText: "3"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			analyzer := newTestAnalyzer(t, context.Background(), AnalysisBudget{})
			witness, intersects, err := analyzer.GlobIntersectionWitness(tc.left, tc.right, tc.exclude, true)
			require.NoError(t, err)
			assert.Equal(t, tc.want, intersects)
			assert.Equal(t, tc.wantText, witness)
			if intersects {
				left := Must(NewGlobMatcher(tc.left))
				right := Must(NewGlobMatcher(tc.right))
				assert.True(t, left.MatchString(witness))
				assert.True(t, right.MatchString(witness))
				for _, excluded := range tc.exclude {
					assert.False(t, Must(NewGlobMatcher(excluded)).MatchString(witness))
				}
			}
		})
	}
}

func TestSimplePatternIntersectionWitnessHonorsOrderedNegatives(t *testing.T) {
	tests := map[string]struct {
		left  string
		right string
		want  bool
	}{
		"class excluded":          {left: `!app_[ab]* app_*`, right: `app_a*`},
		"negative union excluded": {left: `!app_a* !app_b* app_*`, right: `app_[ab]*`},
		"one branch remains":      {left: `!app_a* app_*`, right: `app_[ab]*`, want: true},
		"early positive wins":     {left: `app_a* !app_a* app_*`, right: `app_a*`, want: true},
		"early negative wins":     {left: `!app_a* app_a*`, right: `app_a*`},
		"both operands exclude":   {left: `!app_a* app_*`, right: `!app_b* app_[ab]*`},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			analyzer := newTestAnalyzer(t, context.Background(), AnalysisBudget{})
			witness, intersects, err := analyzer.SimplePatternIntersectionWitness(tc.left, tc.right, true)
			require.NoError(t, err)
			assert.Equal(t, tc.want, intersects)
			if intersects {
				left := Must(NewSimplePatternsMatcher(tc.left))
				right := Must(NewSimplePatternsMatcher(tc.right))
				assert.True(t, left.MatchString(witness))
				assert.True(t, right.MatchString(witness))
			}
		})
	}
}

func TestSimplePatternIntersectionWitnessChoosesShortestAcrossBranches(t *testing.T) {
	analyzer := newTestAnalyzer(t, context.Background(), AnalysisBudget{})
	witness, intersects, err := analyzer.SimplePatternIntersectionWitness(`zz* a`, `zz* a`, true)
	require.NoError(t, err)
	require.True(t, intersects)
	assert.Equal(t, "a", witness)
}

func TestGlobIntersectionWitnessHonorsCancellationAndBudget(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	analyzer := newTestAnalyzer(t, ctx, AnalysisBudget{})
	_, _, err := analyzer.GlobIntersectionWitness("*a*a*a*", "*a*a*a*", nil, false)
	assert.ErrorIs(t, err, context.Canceled)

	analyzer = newTestAnalyzer(t, context.Background(), AnalysisBudget{MaxStates: 1, MaxTransitions: 1})
	_, _, err = analyzer.GlobIntersectionWitness("*a*a*a*", "*a*a*a*", nil, false)
	assert.ErrorIs(t, err, ErrAnalysisBudgetExceeded)
}

func TestAnalyzerSharesBudgetAcrossQueries(t *testing.T) {
	analyzer, err := NewAnalyzer(context.Background(), AnalysisBudget{MaxStates: 1, MaxTransitions: 1})
	require.NoError(t, err)

	_, intersects, err := analyzer.GlobIntersectionWitness("", "", nil, false)
	require.NoError(t, err)
	assert.True(t, intersects)
	_, _, err = analyzer.GlobIntersectionWitness("", "", nil, false)
	assert.ErrorIs(t, err, ErrAnalysisBudgetExceeded)
}

func TestGlobIntersectionWitnessRejectsInvalidInput(t *testing.T) {
	analyzer := newTestAnalyzer(t, context.Background(), AnalysisBudget{})
	_, _, err := analyzer.GlobIntersectionWitness("[", "*", nil, false)
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrAnalysisBudgetExceeded))
}

func TestGlobIntersectionWitnessMatchesRuntimeOnFiniteDomain(t *testing.T) {
	patterns := []string{"", "*", "a", "b", "é", "?", "a*", "*b", "[ab]", "[^a]", "[a-bé]"}
	candidates := finiteTestStrings([]rune{'a', 'b', 'é'}, 3)
	for _, leftPattern := range patterns {
		left := Must(NewGlobMatcher(leftPattern))
		for _, rightPattern := range patterns {
			right := Must(NewGlobMatcher(rightPattern))
			want := false
			for _, candidate := range candidates {
				if left.MatchString(candidate) && right.MatchString(candidate) {
					want = true
					break
				}
			}
			analyzer := newTestAnalyzer(t, context.Background(), AnalysisBudget{})
			witness, got, err := analyzer.GlobIntersectionWitness(leftPattern, rightPattern, nil, false)
			require.NoError(t, err)
			assert.Equalf(t, want, got, "%q intersect %q with witness %q", leftPattern, rightPattern, witness)
			if got {
				assert.True(t, left.MatchString(witness))
				assert.True(t, right.MatchString(witness))
			}
		}
	}
}

func FuzzGlobIntersectionWitnessRuntimeParity(f *testing.F) {
	for _, seed := range [][2]string{{"app_*", "app_[ab]*"}, {"métrique_*", "*_[é]"}, {"[^a]", "?"}} {
		f.Add(seed[0], seed[1])
	}
	f.Fuzz(func(t *testing.T, leftPattern, rightPattern string) {
		analyzer := newTestAnalyzer(t, context.Background(), AnalysisBudget{MaxStates: 2_000, MaxTransitions: 20_000})
		witness, intersects, err := analyzer.GlobIntersectionWitness(leftPattern, rightPattern, nil, false)
		if err != nil || !intersects {
			return
		}
		left, err := NewGlobMatcher(leftPattern)
		require.NoError(t, err)
		right, err := NewGlobMatcher(rightPattern)
		require.NoError(t, err)
		assert.True(t, left.MatchString(witness))
		assert.True(t, right.MatchString(witness))
	})
}

func finiteTestStrings(alphabet []rune, maxLength int) []string {
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

func BenchmarkSimplePatternIntersectionWitness(b *testing.B) {
	for range b.N {
		analyzer := newTestAnalyzer(b, context.Background(), AnalysisBudget{})
		_, _, err := analyzer.SimplePatternIntersectionWitness(
			`!app_debug_[a-z0-9]* app_[a-z0-9]*`,
			`!app_private_* app_[^x-z]*`,
			true,
		)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func newTestAnalyzer(t testing.TB, ctx context.Context, budget AnalysisBudget) *Analyzer {
	t.Helper()
	analyzer, err := NewAnalyzer(ctx, budget)
	require.NoError(t, err)
	return analyzer
}
