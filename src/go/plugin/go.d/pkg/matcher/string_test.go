// SPDX-License-Identifier: GPL-3.0-or-later

package matcher

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var stringMatcherTestCases = []struct {
	line                          string
	expr                          string
	full, prefix, suffix, partial bool
}{
	{"", "", true, true, true, true},
	{"abc", "", false, true, true, true},
	{"power", "pow", false, true, false, true},
	{"netdata", "data", false, false, true, true},
	{"abc", "def", false, false, false, false},
	{"soon", "o", false, false, false, true},
}

func TestStringFullMatcher_MatchString(t *testing.T) {
	for _, c := range stringMatcherTestCases {
		t.Run(c.line, func(t *testing.T) {
			m := stringFullMatcher(c.expr)
			assert.Equal(t, c.full, m.Match([]byte(c.line)))
			assert.Equal(t, c.full, m.MatchString(c.line))
		})
	}
}

func TestStringPrefixMatcher_MatchString(t *testing.T) {
	for _, c := range stringMatcherTestCases {
		t.Run(c.line, func(t *testing.T) {
			m := stringPrefixMatcher(c.expr)
			assert.Equal(t, c.prefix, m.Match([]byte(c.line)))
			assert.Equal(t, c.prefix, m.MatchString(c.line))
		})
	}
}

func TestStringSuffixMatcher_MatchString(t *testing.T) {
	for _, c := range stringMatcherTestCases {
		t.Run(c.line, func(t *testing.T) {
			m := stringSuffixMatcher(c.expr)
			assert.Equal(t, c.suffix, m.Match([]byte(c.line)))
			assert.Equal(t, c.suffix, m.MatchString(c.line))
		})
	}
}

func TestStringPartialMatcher_MatchString(t *testing.T) {
	for _, c := range stringMatcherTestCases {
		t.Run(c.line, func(t *testing.T) {
			m := stringPartialMatcher(c.expr)
			assert.Equal(t, c.partial, m.Match([]byte(c.line)))
			assert.Equal(t, c.partial, m.MatchString(c.line))
		})
	}
}
