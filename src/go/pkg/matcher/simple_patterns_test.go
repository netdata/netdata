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
