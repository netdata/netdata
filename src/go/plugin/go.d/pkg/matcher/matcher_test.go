// SPDX-License-Identifier: GPL-3.0-or-later

package matcher

import (
	"log"
	"reflect"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/assert"
)

func TestParse(t *testing.T) {
	tests := []struct {
		valid   bool
		line    string
		matcher Matcher
	}{
		{false, "", nil},
		{false, "abc", nil},
		{false, `~ abc\`, nil},
		{false, `invalid_fmt:abc`, nil},

		{true, "=", stringFullMatcher("")},
		{true, "= ", stringFullMatcher("")},
		{true, "=full", stringFullMatcher("full")},
		{true, "= full", stringFullMatcher("full")},
		{true, "= \t\ffull", stringFullMatcher("full")},

		{true, "string:", stringFullMatcher("")},
		{true, "string:full", stringFullMatcher("full")},

		{true, "!=", Not(stringFullMatcher(""))},
		{true, "!=full", Not(stringFullMatcher("full"))},
		{true, "!= full", Not(stringFullMatcher("full"))},
		{true, "!= \t\ffull", Not(stringFullMatcher("full"))},

		{true, "!string:", Not(stringFullMatcher(""))},
		{true, "!string:full", Not(stringFullMatcher("full"))},

		{true, "~", TRUE()},
		{true, "~ ", TRUE()},
		{true, `~ ^$`, stringFullMatcher("")},
		{true, "~ partial", stringPartialMatcher("partial")},
		{true, `~ part\.ial`, stringPartialMatcher("part.ial")},
		{true, "~ ^prefix", stringPrefixMatcher("prefix")},
		{true, "~ suffix$", stringSuffixMatcher("suffix")},
		{true, "~ ^full$", stringFullMatcher("full")},
		{true, "~ [0-9]+", regexp.MustCompile(`[0-9]+`)},
		{true, `~ part\s1`, regexp.MustCompile(`part\s1`)},

		{true, "!~", FALSE()},
		{true, "!~ ", FALSE()},
		{true, "!~ partial", Not(stringPartialMatcher("partial"))},
		{true, `!~ part\.ial`, Not(stringPartialMatcher("part.ial"))},
		{true, "!~ ^prefix", Not(stringPrefixMatcher("prefix"))},
		{true, "!~ suffix$", Not(stringSuffixMatcher("suffix"))},
		{true, "!~ ^full$", Not(stringFullMatcher("full"))},
		{true, "!~ [0-9]+", Not(regexp.MustCompile(`[0-9]+`))},

		{true, `regexp:partial`, stringPartialMatcher("partial")},
		{true, `!regexp:partial`, Not(stringPartialMatcher("partial"))},

		{true, `*`, stringFullMatcher("")},
		{true, `* foo`, stringFullMatcher("foo")},
		{true, `* foo*`, stringPrefixMatcher("foo")},
		{true, `* *foo`, stringSuffixMatcher("foo")},
		{true, `* *foo*`, stringPartialMatcher("foo")},
		{true, `* foo*bar`, globMatcher("foo*bar")},
		{true, `* *foo*bar`, globMatcher("*foo*bar")},
		{true, `* foo?bar`, globMatcher("foo?bar")},

		{true, `!*`, Not(stringFullMatcher(""))},
		{true, `!* foo`, Not(stringFullMatcher("foo"))},
		{true, `!* foo*`, Not(stringPrefixMatcher("foo"))},
		{true, `!* *foo`, Not(stringSuffixMatcher("foo"))},
		{true, `!* *foo*`, Not(stringPartialMatcher("foo"))},
		{true, `!* foo*bar`, Not(globMatcher("foo*bar"))},
		{true, `!* *foo*bar`, Not(globMatcher("*foo*bar"))},
		{true, `!* foo?bar`, Not(globMatcher("foo?bar"))},

		{true, "glob:foo*bar", globMatcher("foo*bar")},
		{true, "!glob:foo*bar", Not(globMatcher("foo*bar"))},

		{true, `simple_patterns:`, FALSE()},
		{true, `simple_patterns:  `, FALSE()},
		{true, `simple_patterns: foo`, simplePatternsMatcher{
			{stringFullMatcher("foo"), true},
		}},
		{true, `simple_patterns: !foo`, simplePatternsMatcher{
			{stringFullMatcher("foo"), false},
		}},
	}
	for _, test := range tests {
		t.Run(test.line, func(t *testing.T) {
			m, err := Parse(test.line)
			if test.valid {
				require.NoError(t, err)
				if test.matcher != nil {
					log.Printf("%s %#v", reflect.TypeOf(m).Name(), m)
					assert.Equal(t, test.matcher, m)
				}
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestMust(t *testing.T) {
	assert.NotPanics(t, func() {
		m := Must(New(FmtRegExp, `[0-9]+`))
		assert.NotNil(t, m)
	})

	assert.Panics(t, func() {
		Must(New(FmtRegExp, `[0-9]+\`))
	})
}
