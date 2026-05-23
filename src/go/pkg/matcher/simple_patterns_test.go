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
