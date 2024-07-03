// SPDX-License-Identifier: GPL-3.0-or-later

package matcher

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewGlobMatcher(t *testing.T) {
	cases := []struct {
		expr    string
		matcher Matcher
	}{
		{"", stringFullMatcher("")},
		{"a", stringFullMatcher("a")},
		{"a*b", globMatcher("a*b")},
		{`a*\b`, globMatcher(`a*\b`)},
		{`a\[`, stringFullMatcher(`a[`)},
		{`ab\`, nil},
		{`ab[`, nil},
		{`ab]`, stringFullMatcher("ab]")},
	}
	for _, c := range cases {
		t.Run(c.expr, func(t *testing.T) {
			m, err := NewGlobMatcher(c.expr)
			if c.matcher != nil {
				assert.NoError(t, err)
				assert.Equal(t, c.matcher, m)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestGlobMatcher_MatchString(t *testing.T) {

	cases := []struct {
		expected bool
		expr     string
		line     string
	}{
		{true, "/a/*/d", "/a/b/c/d"},
		{true, "foo*", "foo123"},
		{true, "*foo*", "123foo123"},
		{true, "*foo", "123foo"},
		{true, "foo*bar", "foobar"},
		{true, "foo*bar", "foo baz bar"},
		{true, "a[bc]d", "abd"},
		{true, "a[^bc]d", "add"},
		{true, "a??d", "abcd"},
		{true, `a\??d`, "a?cd"},
		{true, "a[b-z]d", "abd"},
		{false, "/a/*/d", "a/b/c/d"},
		{false, "/a/*/d", "This will fail!"},
	}

	for _, c := range cases {
		t.Run(c.line, func(t *testing.T) {
			m := globMatcher(c.expr)
			assert.Equal(t, c.expected, m.Match([]byte(c.line)))
			assert.Equal(t, c.expected, m.MatchString(c.line))
		})
	}
}

func BenchmarkGlob_MatchString(b *testing.B) {
	benchmarks := []struct {
		expr string
		test string
	}{
		{"", ""},
		{"abc", "abcd"},
		{"*abc", "abcd"},
		{"abc*", "abcd"},
		{"*abc*", "abcd"},
		{"[a-z]", "abcd"},
	}
	for _, bm := range benchmarks {
		b.Run(bm.expr+"_raw", func(b *testing.B) {
			m := globMatcher(bm.expr)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				m.MatchString(bm.test)
			}
		})
		b.Run(bm.expr+"_optimized", func(b *testing.B) {
			m, _ := NewGlobMatcher(bm.expr)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				m.MatchString(bm.test)
			}
		})
	}
}
