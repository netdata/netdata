// SPDX-License-Identifier: GPL-3.0-or-later

package matcher

import (
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		{`a\*`, stringFullMatcher(`a*`)},
		{`a\\*`, stringPrefixMatcher(`a\`)},
		{`a\\\*`, stringFullMatcher(`a\*`)},
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

func TestNewGlobMatcherUnicode(t *testing.T) {
	tests := map[string]struct {
		pattern string
		value   string
		want    bool
	}{
		"literal": {
			pattern: "μέτρο",
			value:   "μέτρο",
			want:    true,
		},
		"prefix": {
			pattern: "μέτρο*",
			value:   "μέτρο_total",
			want:    true,
		},
		"suffix": {
			pattern: "*μέτρο",
			value:   "http_μέτρο",
			want:    true,
		},
		"general glob": {
			pattern: "μ?τρο_*",
			value:   "μέτρο_total",
			want:    true,
		},
		"miss": {
			pattern: "μέτρο*",
			value:   "metric_total",
			want:    false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			m, err := NewGlobMatcher(test.pattern)
			require.NoError(t, err)
			assert.Equal(t, test.want, m.MatchString(test.value))
		})
	}
}

func FuzzNewGlobMatcherValidUTF8DoesNotPanic(f *testing.F) {
	for _, seed := range []string{"μέτρο", "指标*", "*latency_秒", "a[β-ω]", `\!metric`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, pattern string) {
		if !utf8.ValidString(pattern) {
			t.Skip()
		}
		_, _ = NewGlobMatcher(pattern)
	})
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
		{true, `queue\*`, "queue*"},
		{true, "a[b-z]d", "abd"},
		{false, "/a/*/d", "a/b/c/d"},
		{false, "/a/*/d", "This will fail!"},
		{false, `queue\*`, "queue*errors"},
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
