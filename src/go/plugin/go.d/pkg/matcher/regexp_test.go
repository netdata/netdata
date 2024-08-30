// SPDX-License-Identifier: GPL-3.0-or-later

package matcher

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegExpMatch_Match(t *testing.T) {
	m := regexp.MustCompile("[0-9]+")

	cases := []struct {
		expected bool
		line     string
	}{
		{
			expected: true,
			line:     "2019",
		},
		{
			expected: true,
			line:     "It's over 9000!",
		},
		{
			expected: false,
			line:     "This will never fail!",
		},
	}

	for _, c := range cases {
		assert.Equal(t, c.expected, m.MatchString(c.line))
	}
}

func BenchmarkRegExp_MatchString(b *testing.B) {
	benchmarks := []struct {
		expr string
		test string
	}{
		{"", ""},
		{"abc", "abcd"},
		{"^abc", "abcd"},
		{"abc$", "abcd"},
		{"^abc$", "abcd"},
		{"[a-z]+", "abcd"},
	}
	for _, bm := range benchmarks {
		b.Run(bm.expr+"_raw", func(b *testing.B) {
			m := regexp.MustCompile(bm.expr)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				m.MatchString(bm.test)
			}
		})
		b.Run(bm.expr+"_optimized", func(b *testing.B) {
			m, _ := NewRegExpMatcher(bm.expr)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				m.MatchString(bm.test)
			}
		})
	}
}
