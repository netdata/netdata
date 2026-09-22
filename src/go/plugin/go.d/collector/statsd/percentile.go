// SPDX-License-Identifier: GPL-3.0-or-later

package statsd

import (
	"math"
	"math/bits"
	"slices"

	"github.com/DataDog/sketches-go/ddsketch/mapping"
	"github.com/DataDog/sketches-go/ddsketch/store"
)

// Numeric-domain bounds keep rank certification valid; these are not input quotas.
const percentileBins = 1024
const maxObservations = 1 << 26
const minRate = 0x1p-128
const maxExactUnits = 1 << 53

type percentileBin struct{ value, count float64 }
type percentiles struct {
	mapping            *mapping.LogarithmicMapping
	positive, negative *store.CollapsingLowestDenseStore
	limit              int
	n                  uint64
	firstRate, zero    float64
	exact              bool
	unitExp            int
	units              uint64
	reason             string
}

func newPercentiles(limit int) *percentiles {
	m, err := mapping.NewLogarithmicMapping(.009)
	if err != nil {
		panic(err)
	}
	return &percentiles{
		mapping:  m,
		positive: store.NewCollapsingLowestDenseStore(limit),
		negative: store.NewCollapsingLowestDenseStore(limit),
		limit:    limit,
		exact:    true,
	}
}

func down(v float64) float64 { return math.Nextafter(v, math.Inf(-1)) }
func up(v float64) float64   { return math.Nextafter(v, math.Inf(1)) }
func (a *percentiles) fail(reason string) {
	if a.reason == "" {
		a.reason = reason
	}
}

// add runs only after basic statistics pass their atomic finite checks.
// Reliability failure withholds both percentiles, without rejecting that observation.
func (a *percentiles) add(value, rate float64) {
	if a.reason != "" {
		return
	}
	if !(rate >= minRate && rate <= 1) || math.IsNaN(value) || math.IsInf(value, 0) {
		a.fail("numeric_domain")
		return
	}
	if a.n == maxObservations {
		a.fail("observation_bound")
		return
	}
	if a.n == 0 {
		a.firstRate = rate
	}
	z := a.firstRate / rate
	a.n++
	if a.exact {
		if math.FMA(z, rate, -a.firstRate) != 0 {
			a.exact = false
		} else {
			// Determine the exact dyadic unit; all sums of <=2^53 such units are exact.
			b := math.Float64bits(z)
			e := int((b>>52)&2047) - 1023 - 52 + bits.TrailingZeros64((b&((1<<52)-1))|(1<<52))
			if a.n == 1 || e < a.unitExp {
				if a.n > 1 && (a.unitExp-e > 53 || a.units > (maxExactUnits>>uint(a.unitExp-e))) {
					a.exact = false
				} else {
					if a.n > 1 {
						a.units <<= uint(a.unitExp - e)
					}
					a.unitExp = e
				}
			}
			if a.exact {
				u := math.Ldexp(z, -a.unitExp)
				if u > maxExactUnits || a.units > maxExactUnits-uint64(u) {
					a.exact = false
				} else {
					a.units += uint64(u)
				}
			}
		}
	}
	if value == 0 {
		a.zero += z
		return
	}
	magnitude := math.Abs(value)
	if magnitude < a.mapping.MinIndexableValue() || magnitude > a.mapping.MaxIndexableValue() {
		a.fail("mapping_domain")
		return
	}
	index := a.mapping.Index(magnitude)
	rep := a.mapping.Value(index)
	// Outward error and inward budget prove the 1% inequality for THIS mapping,
	// without assuming the implementation's transcendental errors are zero.
	if !finite(rep) || rep <= 0 || (rep != magnitude && up(math.Abs(rep-magnitude)) > down(magnitude/100)) {
		a.fail("mapping_error")
		return
	}
	target := a.positive
	if value < 0 {
		target = a.negative
	}
	if !target.IsEmpty() {
		lo, _ := target.MinIndex()
		hi, _ := target.MaxIndex()
		lo = min(lo, index)
		hi = max(hi, index)
		if hi-lo+1 > a.limit {
			a.fail("span")
			return
		}
	}
	target.AddWithCount(index, z)
}
func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }
func (a *percentiles) reset() {
	a.positive.Clear()
	a.negative.Clear()
	a.n = 0
	a.zero = 0
	a.firstRate = 0
	a.exact = true
	a.units = 0
	a.unitExp = 0
	a.reason = ""
}
func (a *percentiles) query(scratch []percentileBin) ([2]float64, []percentileBin) {
	result := [2]float64{math.NaN(), math.NaN()}
	scratch = scratch[:0]
	if a.reason != "" || a.n == 0 {
		return result, scratch
	}
	a.negative.ForEach(func(i int, c float64) bool {
		scratch = append(scratch, percentileBin{-a.mapping.Value(i), c})
		return false
	})
	if a.zero > 0 {
		scratch = append(scratch, percentileBin{0, a.zero})
	}
	a.positive.ForEach(func(i int, c float64) bool {
		scratch = append(scratch, percentileBin{a.mapping.Value(i), c})
		return false
	})
	slices.SortFunc(scratch, func(a, b percentileBin) int {
		if a.value < b.value {
			return -1
		}
		if a.value > b.value {
			return 1
		}
		return 0
	})
	numer := [2]uint64{1, 19}
	denom := [2]uint64{2, 20}
	if a.exact {
		var cumulative uint64
		for _, b := range scratch {
			cumulative += uint64(math.Ldexp(b.count, -a.unitExp))
			for q := range result {
				if math.IsNaN(result[q]) && denom[q]*cumulative >= numer[q]*a.units {
					result[q] = b.value
				}
			}
		}
		return result, scratch
	}
	// Each rounded normalized weight and its bin summation contributes at most
	// gamma_n relative error. 4*n*u bounds both directions for n<=2^26.
	delta := 4 * float64(a.n) * 0x1p-53
	var totalLo, totalHi float64
	for _, b := range scratch {
		lo := down(b.count / up(1+delta))
		hi := up(b.count / down(1-delta))
		totalLo = down(totalLo + lo)
		totalHi = up(totalHi + hi)
	}
	var prefixLo, prefixHi float64
	for _, b := range scratch {
		beforeHi := prefixHi
		lo := down(b.count / up(1+delta))
		hi := up(b.count / down(1-delta))
		prefixLo = down(prefixLo + lo)
		prefixHi = up(prefixHi + hi)
		for q := range result {
			thresholdLo := down(down(totalLo*float64(numer[q])) / float64(denom[q]))
			thresholdHi := up(up(totalHi*float64(numer[q])) / float64(denom[q]))
			if math.IsNaN(result[q]) && beforeHi < thresholdLo && prefixLo >= thresholdHi {
				result[q] = b.value
			}
		}
	}
	if math.IsNaN(result[0]) || math.IsNaN(result[1]) {
		a.fail("ambiguous_rank")
		return [2]float64{math.NaN(), math.NaN()}, scratch
	}
	return result, scratch
}
