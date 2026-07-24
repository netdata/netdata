// SPDX-License-Identifier: GPL-3.0-or-later

package matcher

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSimplePatternsMatcher(t *testing.T) {
	tests := []struct {
		expr     string
		expected Matcher
	}{
		{"", FALSE()},
		{" ", FALSE()},
		{"foo", simplePatternsMatcher{
			{stringFullMatcher("foo"), true},
		}},
		{"!foo", simplePatternsMatcher{
			{stringFullMatcher("foo"), false},
		}},
		{"foo bar", simplePatternsMatcher{
			{stringFullMatcher("foo"), true},
			{stringFullMatcher("bar"), true},
		}},
		{"*foobar* !foo* !*bar *", simplePatternsMatcher{
			{stringPartialMatcher("foobar"), true},
			{stringPrefixMatcher("foo"), false},
			{stringSuffixMatcher("bar"), false},
			{TRUE(), true},
		}},
		{`ab\`, nil},
	}
	for _, test := range tests {
		t.Run(test.expr, func(t *testing.T) {
			matcher, err := NewSimplePatternsMatcher(test.expr)
			if test.expected == nil {
				assert.Error(t, err)
			} else {
				assert.Equal(t, test.expected, matcher)
			}
		})
	}
}

func TestNewSimplePatternListMatcher(t *testing.T) {
	tests := map[string]struct {
		patterns []string
		want     Matcher
		wantErr  string
	}{
		"empty list returns false": {
			want: FALSE(),
		},
		"blank entries return false": {
			patterns: []string{"", " ", "\t"},
			want:     FALSE(),
		},
		"single glob": {
			patterns: []string{"foo*"},
			want: simplePatternsMatcher{
				{stringPrefixMatcher("foo"), true},
			},
		},
		"preserves whitespace inside pattern": {
			patterns: []string{"Business Unit"},
			want: simplePatternsMatcher{
				{stringFullMatcher("Business Unit"), true},
			},
		},
		"bare negative marker is invalid": {
			patterns: []string{"!"},
			wantErr:  "invalid empty negative pattern",
		},
		"blank negative pattern is invalid": {
			patterns: []string{"!   "},
			wantErr:  "invalid empty negative pattern",
		},
		"invalid glob is wrapped": {
			patterns: []string{"["},
			wantErr:  "invalid pattern",
		},
		"all negative patterns are invalid": {
			patterns: []string{"!Business Secret"},
			wantErr:  "must include at least one positive pattern",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			matcher, err := NewSimplePatternListMatcher(test.patterns)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, matcher)
		})
	}
}

func TestCompilePositivePatternList(t *testing.T) {
	tests := map[string]struct {
		patterns []string
		want     []string
		wantErr  string
	}{
		"empty list": {},
		"canonicalizes outer whitespace duplicates and order": {
			patterns: []string{" z*", "Business Unit", "z*", " μέτρο* "},
			want:     []string{"Business Unit", "z*", "μέτρο*"},
		},
		"wildcard collapses only after every term validates": {
			patterns: []string{"z*", "*", "a*"},
			want:     []string{"*"},
		},
		"wildcard does not hide an invalid term": {
			patterns: []string{"*", "["},
			wantErr:  "pattern[1]: invalid pattern",
		},
		"empty term is invalid": {
			patterns: []string{"metric*", "  "},
			wantErr:  "pattern[1]: must not be empty",
		},
		"negative term is invalid": {
			patterns: []string{"!metric*"},
			wantErr:  "negative patterns are not allowed",
		},
		"escaped leading exclamation is positive": {
			patterns: []string{`\!metric`},
			want:     []string{`\!metric`},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			compiled, err := CompilePositivePatternList(test.patterns)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, compiled.Patterns())
		})
	}
}

func TestPositivePatternListMatch(t *testing.T) {
	compiled, err := CompilePositivePatternList([]string{
		"http_*_seconds",
		"Business Unit",
		"μέτρο*",
		`\!metric`,
	})
	require.NoError(t, err)

	for _, value := range []string{"http_request_seconds", "Business Unit", "μέτρο_total", "!metric"} {
		assert.True(t, compiled.MatchString(value), value)
		assert.True(t, compiled.Match([]byte(value)), value)
	}
	for _, value := range []string{"http_request_total", "Business", "metric"} {
		assert.False(t, compiled.MatchString(value), value)
	}

	patterns := compiled.Patterns()
	patterns[0] = "mutated"
	assert.Equal(t, []string{"Business Unit", `\!metric`, "http_*_seconds", "μέτρο*"}, compiled.Patterns())
}

func TestSimplePatternList_Match(t *testing.T) {
	tests := map[string]struct {
		patterns []string
		value    string
		want     bool
	}{
		"positive before negative wins": {
			patterns: []string{"Business*", "!Business Secret"},
			value:    "Business Secret",
			want:     true,
		},
		"negative before positive wins": {
			patterns: []string{"!Business Secret", "Business*"},
			value:    "Business Secret",
			want:     false,
		},
		"later positive matches": {
			patterns: []string{"!Business Secret", "Cost Center", "Business*"},
			value:    "Cost Center",
			want:     true,
		},
		"no matching pattern": {
			patterns: []string{"!Business Secret", "Cost Center", "Business*"},
			value:    "Cost",
			want:     false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			m, err := NewSimplePatternListMatcher(test.patterns)
			require.NoError(t, err)

			assert.Equal(t, test.want, m.MatchString(test.value))
			assert.Equal(t, test.want, m.Match([]byte(test.value)))
		})
	}
}

func TestSimplePatterns_Match(t *testing.T) {
	m, err := NewSimplePatternsMatcher("*foobar* !foo* !*bar *")

	require.NoError(t, err)

	cases := []struct {
		expected bool
		line     string
	}{
		{
			expected: true,
			line:     "hello world",
		},
		{
			expected: false,
			line:     "hello world bar",
		},
		{
			expected: true,
			line:     "hello world foobar",
		},
	}

	for _, c := range cases {
		t.Run(c.line, func(t *testing.T) {
			assert.Equal(t, c.expected, m.MatchString(c.line))
			assert.Equal(t, c.expected, m.Match([]byte(c.line)))
		})
	}
}

func TestSimplePatterns_Match2(t *testing.T) {
	m, err := NewSimplePatternsMatcher("*foobar")

	require.NoError(t, err)

	assert.True(t, m.MatchString("foobar"))
	assert.True(t, m.MatchString("foo foobar"))
	assert.False(t, m.MatchString("foobar baz"))
}
