// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"fmt"
	"sort"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
)

type (
	// A Histogram counts individual observations from an event or sample stream in
	// configurable buckets. Similar to a summary, it also provides a sum of
	// observations and an observation count.
	//
	// Note that Histograms, in contrast to Summaries, can be aggregated.
	// However, Histograms require the user to pre-define suitable
	// buckets, and they are in general less accurate. The Observe method of a
	// histogram has a very low performance overhead in comparison with the Observe
	// method of a summary.
	//
	// To create histogram instances, use NewHistogram.
	Histogram interface {
		Observer
	}

	histogram struct {
		buckets      []int64
		upperBounds  []float64
		sum          float64
		count        int64
		rangeBuckets bool
	}
)

var (
	_ stm.Value = histogram{}
)

// DefBuckets are the default histogram buckets. The default buckets are
// tailored to broadly measure the response time (in seconds) of a network
// service. Most likely, however, you will be required to define buckets
// customized to your use case.
var DefBuckets = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}

// LinearBuckets creates 'count' buckets, each 'width' wide, where the lowest
// bucket has an upper bound of 'start'. The final +Inf bucket is not counted
// and not included in the returned slice. The returned slice is meant to be
// used for the Buckets field of HistogramOpts.
//
// The function panics if 'count' is zero or negative.
func LinearBuckets(start, width float64, count int) []float64 {
	if count < 1 {
		panic("LinearBuckets needs a positive count")
	}
	buckets := make([]float64, count)
	for i := range buckets {
		buckets[i] = start
		start += width
	}
	return buckets
}

// ExponentialBuckets creates 'count' buckets, where the lowest bucket has an
// upper bound of 'start' and each following bucket's upper bound is 'factor'
// times the previous bucket's upper bound. The final +Inf bucket is not counted
// and not included in the returned slice. The returned slice is meant to be
// used for the Buckets field of HistogramOpts.
//
// The function panics if 'count' is 0 or negative, if 'start' is 0 or negative,
// or if 'factor' is less than or equal 1.
func ExponentialBuckets(start, factor float64, count int) []float64 {
	if count < 1 {
		panic("ExponentialBuckets needs a positive count")
	}
	if start <= 0 {
		panic("ExponentialBuckets needs a positive start value")
	}
	if factor <= 1 {
		panic("ExponentialBuckets needs a factor greater than 1")
	}
	buckets := make([]float64, count)
	for i := range buckets {
		buckets[i] = start
		start *= factor
	}
	return buckets
}

// NewHistogram creates a new Histogram.
func NewHistogram(buckets []float64) Histogram {
	if len(buckets) == 0 {
		buckets = DefBuckets
	} else {
		sort.Slice(buckets, func(i, j int) bool { return buckets[i] < buckets[j] })
	}

	return &histogram{
		buckets:     make([]int64, len(buckets)),
		upperBounds: buckets,
		count:       0,
		sum:         0,
	}
}

func NewHistogramWithRangeBuckets(buckets []float64) Histogram {
	if len(buckets) == 0 {
		buckets = DefBuckets
	} else {
		sort.Slice(buckets, func(i, j int) bool { return buckets[i] < buckets[j] })
	}

	return &histogram{
		buckets:      make([]int64, len(buckets)),
		upperBounds:  buckets,
		count:        0,
		sum:          0,
		rangeBuckets: true,
	}
}

// WriteTo writes its values into given map.
// It adds those key-value pairs:
//
//	${key}_sum        gauge, for sum of it's observed values
//	${key}_count      counter, for count of it's observed values (equals to +Inf bucket)
//	${key}_bucket_1   counter, for 1st bucket count
//	${key}_bucket_2   counter, for 2nd bucket count
//	...
//	${key}_bucket_N   counter, for Nth bucket count
func (h histogram) WriteTo(rv map[string]int64, key string, mul, div int) {
	rv[key+"_sum"] = int64(h.sum * float64(mul) / float64(div))
	rv[key+"_count"] = h.count
	var conn int64
	for i, bucket := range h.buckets {
		name := fmt.Sprintf("%s_bucket_%d", key, i+1)
		conn += bucket
		if h.rangeBuckets {
			rv[name] = bucket
		} else {
			rv[name] = conn
		}
	}
	if h.rangeBuckets {
		name := fmt.Sprintf("%s_bucket_inf", key)
		rv[name] = h.count - conn
	}
}

// Observe observes a value
func (h *histogram) Observe(v float64) {
	hotIdx := h.searchBucketIndex(v)
	if hotIdx < len(h.buckets) {
		h.buckets[hotIdx]++
	}
	h.sum += v
	h.count++
}

func (h *histogram) searchBucketIndex(v float64) int {
	if len(h.upperBounds) < 30 {
		for i, upper := range h.upperBounds {
			if upper >= v {
				return i
			}
		}
		return len(h.upperBounds)
	}
	return sort.SearchFloat64s(h.upperBounds, v)
}
