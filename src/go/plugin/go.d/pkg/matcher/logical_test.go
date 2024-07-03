// SPDX-License-Identifier: GPL-3.0-or-later

package matcher

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTRUE(t *testing.T) {
	assert.True(t, TRUE().Match(nil))
	assert.True(t, TRUE().MatchString(""))
}

func TestFALSE(t *testing.T) {
	assert.False(t, FALSE().Match(nil))
	assert.False(t, FALSE().MatchString(""))
}

func TestAnd(t *testing.T) {
	assert.Equal(t,
		matcherF,
		And(FALSE(), stringFullMatcher("")))
	assert.Equal(t,
		matcherF,
		And(stringFullMatcher(""), FALSE()))

	assert.Equal(t,
		stringFullMatcher(""),
		And(TRUE(), stringFullMatcher("")))
	assert.Equal(t,
		stringFullMatcher(""),
		And(stringFullMatcher(""), TRUE()))

	assert.Equal(t,
		andMatcher{stringPartialMatcher("a"), stringPartialMatcher("b")},
		And(stringPartialMatcher("a"), stringPartialMatcher("b")))

	assert.Equal(t,
		andMatcher{
			andMatcher{stringPartialMatcher("a"), stringPartialMatcher("b")},
			stringPartialMatcher("c"),
		},
		And(stringPartialMatcher("a"), stringPartialMatcher("b"), stringPartialMatcher("c")))
}

func TestOr(t *testing.T) {
	assert.Equal(t,
		stringFullMatcher(""),
		Or(FALSE(), stringFullMatcher("")))
	assert.Equal(t,
		stringFullMatcher(""),
		Or(stringFullMatcher(""), FALSE()))

	assert.Equal(t,
		TRUE(),
		Or(TRUE(), stringFullMatcher("")))
	assert.Equal(t,
		TRUE(),
		Or(stringFullMatcher(""), TRUE()))

	assert.Equal(t,
		orMatcher{stringPartialMatcher("a"), stringPartialMatcher("b")},
		Or(stringPartialMatcher("a"), stringPartialMatcher("b")))

	assert.Equal(t,
		orMatcher{
			orMatcher{stringPartialMatcher("a"), stringPartialMatcher("b")},
			stringPartialMatcher("c"),
		},
		Or(stringPartialMatcher("a"), stringPartialMatcher("b"), stringPartialMatcher("c")))
}

func TestAndMatcher_Match(t *testing.T) {
	and := andMatcher{
		stringPrefixMatcher("a"),
		stringSuffixMatcher("c"),
	}
	assert.True(t, and.Match([]byte("abc")))
	assert.True(t, and.MatchString("abc"))
}

func TestOrMatcher_Match(t *testing.T) {
	or := orMatcher{
		stringPrefixMatcher("a"),
		stringPrefixMatcher("c"),
	}
	assert.True(t, or.Match([]byte("aaa")))
	assert.True(t, or.MatchString("ccc"))
}

func TestNegMatcher_Match(t *testing.T) {
	neg := negMatcher{stringPrefixMatcher("a")}
	assert.False(t, neg.Match([]byte("aaa")))
	assert.True(t, neg.MatchString("ccc"))
}
