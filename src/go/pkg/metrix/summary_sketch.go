// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"hash/fnv"
	"math"
	"sort"
)

const initialSummaryReservoirCapacity = 64

// summaryQuantileSketch keeps bounded-memory approximate quantiles.
// It uses reservoir sampling, which is deterministic per series key seed.
type summaryQuantileSketch struct {
	capacity int
	count    uint64
	rng      uint64
	values   []SampleValue
	scratch  []SampleValue
}

func newSummaryQuantileSketch(capacity int, seed uint64) *summaryQuantileSketch {
	if capacity <= 0 {
		capacity = defaultSummaryReservoirSize
	}
	if seed == 0 {
		seed = 1
	}
	initCap := min(capacity, initialSummaryReservoirCapacity)
	return &summaryQuantileSketch{
		capacity: capacity,
		rng:      seed,
		values:   make([]SampleValue, 0, initCap),
	}
}

func (s *summaryQuantileSketch) clone() *summaryQuantileSketch {
	if s == nil {
		return nil
	}
	cp := *s
	cp.values = append([]SampleValue(nil), s.values...)
	cp.scratch = nil
	return &cp
}

func (s *summaryQuantileSketch) observe(v SampleValue) {
	// Not safe for concurrent use. Callers must hold the owning store mutex.
	s.count++
	if len(s.values) < s.capacity {
		s.values = append(s.values, v)
		return
	}

	j := s.next() % s.count
	if j < uint64(s.capacity) {
		s.values[j] = v
	}
}

func (s *summaryQuantileSketch) quantiles(targets []float64) []SampleValue {
	if len(targets) == 0 {
		return nil
	}

	out := make([]SampleValue, len(targets))
	if len(s.values) == 0 {
		for i := range out {
			out[i] = math.NaN()
		}
		return out
	}

	s.scratch = growCopy(s.scratch, s.values)
	sort.Float64s(s.scratch)
	for i, q := range targets {
		out[i] = sampleQuantileLinear(s.scratch, q)
	}
	return out
}

func (s *summaryQuantileSketch) next() uint64 {
	x := s.rng
	x ^= x << 13
	x ^= x >> 7
	x ^= x << 17
	s.rng = x
	return x
}

func growCopy(dst, src []SampleValue) []SampleValue {
	if cap(dst) < len(src) {
		dst = make([]SampleValue, len(src))
	} else {
		dst = dst[:len(src)]
	}
	copy(dst, src)
	return dst
}

func sampleQuantileLinear(sorted []SampleValue, q float64) SampleValue {
	last := len(sorted) - 1
	if q <= 0 {
		return sorted[0]
	}
	if q >= 1 {
		return sorted[last]
	}
	pos := q * float64(last)
	low := int(math.Floor(pos))
	high := int(math.Ceil(pos))
	if low == high {
		return sorted[low]
	}
	w := pos - float64(low)
	return sorted[low]*(1-w) + sorted[high]*w
}

func summarySketchSeed(key string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	seed := h.Sum64()
	if seed == 0 {
		return 1
	}
	return seed
}
