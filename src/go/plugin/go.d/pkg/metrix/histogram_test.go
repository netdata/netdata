// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLinearBuckets(t *testing.T) {
	buckets := LinearBuckets(0, 1, 10)
	assert.Len(t, buckets, 10)
	assert.EqualValues(t, 0, buckets[0])
	assert.EqualValues(t, 5.0, buckets[5])
	assert.EqualValues(t, 9.0, buckets[9])

	assert.Panics(t, func() {
		LinearBuckets(0, 1, 0)
	})
}

func TestExponentialBuckets(t *testing.T) {
	buckets := ExponentialBuckets(1, 2, 10)
	assert.Len(t, buckets, 10)
	assert.EqualValues(t, 1, buckets[0])
	assert.EqualValues(t, 32.0, buckets[5])
	assert.EqualValues(t, 512.0, buckets[9])

	assert.Panics(t, func() {
		ExponentialBuckets(1, 2, 0)
	})
	assert.Panics(t, func() {
		ExponentialBuckets(0, 2, 2)
	})

	assert.Panics(t, func() {
		ExponentialBuckets(1, 1, 2)
	})
}

func TestNewHistogram(t *testing.T) {
	h := NewHistogram(nil).(*histogram)
	assert.EqualValues(t, 0, h.count)
	assert.EqualValues(t, 0.0, h.sum)
	assert.Equal(t, DefBuckets, h.upperBounds)

	h = NewHistogram([]float64{1, 10, 5}).(*histogram)
	assert.Equal(t, []float64{1, 5, 10}, h.upperBounds)
	assert.Len(t, h.buckets, 3)
}

func TestHistogram_WriteTo(t *testing.T) {
	h := NewHistogram([]float64{1, 2, 3})
	m := map[string]int64{}
	h.WriteTo(m, "pi", 100, 1)
	assert.Len(t, m, 5)
	assert.EqualValues(t, 0, m["pi_count"])
	assert.EqualValues(t, 0, m["pi_sum"])
	assert.EqualValues(t, 0, m["pi_bucket_1"])
	assert.EqualValues(t, 0, m["pi_bucket_2"])
	assert.EqualValues(t, 0, m["pi_bucket_3"])

	h.Observe(0)
	h.Observe(1.5)
	h.Observe(3.5)
	h.WriteTo(m, "pi", 100, 1)
	assert.Len(t, m, 5)
	assert.EqualValues(t, 3, m["pi_count"])
	assert.EqualValues(t, 500, m["pi_sum"])
	assert.EqualValues(t, 1, m["pi_bucket_1"])
	assert.EqualValues(t, 2, m["pi_bucket_2"])
	assert.EqualValues(t, 2, m["pi_bucket_3"])
}

func TestHistogram_searchBucketIndex(t *testing.T) {
	h := NewHistogram(LinearBuckets(1, 1, 5)).(*histogram) // [1, 2, ..., 5]
	assert.Equal(t, 0, h.searchBucketIndex(0.1))
	assert.Equal(t, 1, h.searchBucketIndex(1.1))
	assert.Equal(t, 5, h.searchBucketIndex(8.1))

	h = NewHistogram(LinearBuckets(1, 1, 40)).(*histogram) // [1, 2, ..., 5]
	assert.Equal(t, 0, h.searchBucketIndex(0.1))
	assert.Equal(t, 1, h.searchBucketIndex(1.1))
	assert.Equal(t, 5, h.searchBucketIndex(5.1))
	assert.Equal(t, 7, h.searchBucketIndex(8))
	assert.Equal(t, 39, h.searchBucketIndex(39.5))
	assert.Equal(t, 40, h.searchBucketIndex(40.5))
}

func BenchmarkHistogram_Observe(b *testing.B) {
	benchmarks := []struct {
		name    string
		buckets []float64
	}{
		{"default", nil},
		{"len_10", LinearBuckets(0, 0.1, 10)},
		{"len_20", LinearBuckets(0, 0.1, 20)},
		{"len_30", LinearBuckets(0, 0.1, 30)},
		{"len_40", LinearBuckets(0, 0.1, 40)},
	}
	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			h := NewHistogram(bm.buckets)
			for i := 0; i < b.N; i++ {
				h.Observe(2.5)
			}
		})
	}
}

func BenchmarkHistogram_WriteTo(b *testing.B) {
	benchmarks := []struct {
		name    string
		buckets []float64
	}{
		{"default", nil},
		{"len_10", LinearBuckets(0, 0.1, 10)},
		{"len_20", LinearBuckets(0, 0.1, 20)},
		{"len_30", LinearBuckets(0, 0.1, 30)},
		{"len_40", LinearBuckets(0, 0.1, 40)},
	}
	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			h := NewHistogram(bm.buckets)
			h.Observe(0.1)
			h.Observe(0.01)
			h.Observe(0.5)
			h.Observe(10)
			m := map[string]int64{}
			for i := 0; i < b.N; i++ {
				h.WriteTo(m, "pi", 100, 1)
			}
		})
	}
}
