// SPDX-License-Identifier: GPL-3.0-or-later

package matcher

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithCache(t *testing.T) {
	regMatcher, _ := NewRegExpMatcher("[0-9]+")
	cached := WithCache(regMatcher)

	assert.True(t, cached.MatchString("1"))
	assert.True(t, cached.MatchString("1"))
	assert.True(t, cached.Match([]byte("2")))
	assert.True(t, cached.Match([]byte("2")))
}

func TestWithCache_specialCase(t *testing.T) {
	assert.Equal(t, TRUE(), WithCache(TRUE()))
	assert.Equal(t, FALSE(), WithCache(FALSE()))
}
func BenchmarkCachedMatcher_MatchString_cache_hit(b *testing.B) {
	benchmarks := []struct {
		name   string
		expr   string
		target string
	}{
		{"stringFullMatcher", "= abc123", "abc123"},
		{"stringPrefixMatcher", "~ ^abc123", "abc123456"},
		{"stringSuffixMatcher", "~ abc123$", "hello abc123"},
		{"stringSuffixMatcher", "~ abc123", "hello abc123 world"},
		{"globMatcher", "* abc*def", "abc12345678def"},
		{"regexp", "~ [0-9]+", "1234567890"},
	}
	for _, bm := range benchmarks {
		m := Must(Parse(bm.expr))
		b.Run(bm.name+"_raw", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				m.MatchString(bm.target)
			}
		})
		b.Run(bm.name+"_cache", func(b *testing.B) {
			cached := WithCache(m)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				cached.MatchString(bm.target)
			}
		})
	}
}
