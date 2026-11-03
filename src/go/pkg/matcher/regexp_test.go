// SPDX-License-Identifier: GPL-3.0-or-later

package matcher

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestNewRegExpMatcher_multiByte(t *testing.T) {
	tests := map[string]struct {
		expr string
		line string
		want bool
	}{
		"contains, match":                {expr: "é", line: "café", want: true},
		"contains, no match":             {expr: "é", line: "cafe", want: false},
		"prefix, match":                  {expr: "^é", line: "élan", want: true},
		"prefix, no match":               {expr: "^é", line: "café", want: false},
		"suffix, match":                  {expr: "é$", line: "café", want: true},
		"suffix, no match":               {expr: "é$", line: "élan", want: false},
		"exact, match":                   {expr: "^日本語$", line: "日本語", want: true},
		"exact, no match":                {expr: "^日本語$", line: "日本語x", want: false},
		"escaped meta between runes":     {expr: `日\.本`, line: "日.本", want: true},
		"escaped meta is not a wildcard": {expr: `日\.本`, line: "日x本", want: false},
		"real regexp":                    {expr: "é+", line: "caféé", want: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var (
				m   Matcher
				err error
			)
			require.NotPanics(t, func() { m, err = NewRegExpMatcher(tc.expr) })
			require.NoError(t, err)
			assert.Equal(t, tc.want, m.MatchString(tc.line))
		})
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
