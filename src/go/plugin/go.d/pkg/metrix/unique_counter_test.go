// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHyperLogLogUniqueCounter_Value(t *testing.T) {
	for _, useHLL := range []bool{true, false} {
		t.Run(fmt.Sprintf("HLL=%v", useHLL), func(t *testing.T) {
			c := NewUniqueCounter(useHLL)
			assert.Equal(t, 0, c.Value())

			c.Insert("foo")
			assert.Equal(t, 1, c.Value())

			c.Insert("foo")
			assert.Equal(t, 1, c.Value())

			c.Insert("bar")
			assert.Equal(t, 2, c.Value())

			c.Insert("baz")
			assert.Equal(t, 3, c.Value())

			c.Reset()
			assert.Equal(t, 0, c.Value())

			c.Insert("foo")
			assert.Equal(t, 1, c.Value())
		})
	}
}

func TestHyperLogLogUniqueCounter_WriteTo(t *testing.T) {
	for _, useHLL := range []bool{true, false} {
		t.Run(fmt.Sprintf("HLL=%v", useHLL), func(t *testing.T) {
			c := NewUniqueCounterVec(useHLL)
			c.Get("a").Insert("foo")
			c.Get("a").Insert("bar")
			c.Get("b").Insert("foo")

			m := map[string]int64{}
			c.WriteTo(m, "pi", 100, 1)
			assert.Len(t, m, 2)
			assert.EqualValues(t, 200, m["pi_a"])
			assert.EqualValues(t, 100, m["pi_b"])
		})
	}
}

func TestUniqueCounterVec_Reset(t *testing.T) {
	for _, useHLL := range []bool{true, false} {
		t.Run(fmt.Sprintf("HLL=%v", useHLL), func(t *testing.T) {
			c := NewUniqueCounterVec(useHLL)
			c.Get("a").Insert("foo")
			c.Get("a").Insert("bar")
			c.Get("b").Insert("foo")

			assert.Equal(t, 2, len(c.Items))
			assert.Equal(t, 2, c.Get("a").Value())
			assert.Equal(t, 1, c.Get("b").Value())

			c.Reset()
			assert.Equal(t, 2, len(c.Items))
			assert.Equal(t, 0, c.Get("a").Value())
			assert.Equal(t, 0, c.Get("b").Value())
		})
	}
}

func BenchmarkUniqueCounter_Insert(b *testing.B) {
	benchmarks := []struct {
		name        string
		same        bool
		hyperloglog bool
		nop         bool
	}{

		{"map-same", true, false, false},
		{"hll-same", true, true, false},

		{"nop", false, false, true},
		{"map-diff", false, false, false},
		{"hll-diff", false, true, false},
	}
	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			c := NewUniqueCounter(bm.hyperloglog)
			if bm.same {
				for i := 0; i < b.N; i++ {
					c.Insert("foo")
				}
			} else if bm.nop {
				for i := 0; i < b.N; i++ {
					strconv.Itoa(i)
				}
			} else {
				for i := 0; i < b.N; i++ {
					c.Insert(strconv.Itoa(i))
				}
			}
		})
	}
}

func BenchmarkUniqueCounterVec_Insert(b *testing.B) {
	benchmarks := []struct {
		name        string
		same        bool
		hyperloglog bool
		nop         bool
	}{

		{"map-same", true, false, false},
		{"hll-same", true, true, false},

		{"nop", false, false, true},
		{"map-diff", false, false, false},
		{"hll-diff", false, true, false},
	}
	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			c := NewUniqueCounterVec(bm.hyperloglog)
			if bm.same {
				for i := 0; i < b.N; i++ {
					c.Get("a").Insert("foo")
				}
			} else if bm.nop {
				for i := 0; i < b.N; i++ {
					strconv.Itoa(i)
				}
			} else {
				for i := 0; i < b.N; i++ {
					c.Get("a").Insert(strconv.Itoa(i))
				}
			}
		})
	}
}
