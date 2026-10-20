// SPDX-License-Identifier: GPL-3.0-or-later

// Package percentile estimates the p50 and p95 of weighted observations with a
// bounded logarithmic mapping, and withholds both when a 1% relative error
// cannot be certified.
package percentile

import (
	"math"
	"math/bits"
	"slices"

	"github.com/DataDog/sketches-go/ddsketch/mapping"
	"github.com/DataDog/sketches-go/ddsketch/store"
)

// Numeric-domain bounds keep rank certification valid; these are not input quotas.
const (
	// Bins is the production span limit of each sign store.
	Bins            = 1024
	maxObservations = 1 << 26
	minRate         = 0x1p-128
	maxExactUnits   = 1 << 53
)

// Percentile withholding reasons are a fixed diagnostic vocabulary.
const (
	withheldNumericDomain    = "numeric_domain"
	withheldObservationBound = "observation_bound"
	withheldMappingDomain    = "mapping_domain"
	withheldMappingError     = "mapping_error"
	withheldSpan             = "span"
	withheldAmbiguousRank    = "ambiguous_rank"
)

// WithheldReasons lists every reason Withheld can report.
var WithheldReasons = [...]string{
	withheldNumericDomain, withheldObservationBound, withheldMappingDomain,
	withheldMappingError, withheldSpan, withheldAmbiguousRank,
}

// Bin is one query representative and its weight; callers own the scratch slice.
type Bin struct{ value, count float64 }

// Estimator accumulates one observation window.
type Estimator struct {
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

// New returns an empty estimator whose sign stores span at most limit bins.
func New(limit int) *Estimator {
	m, err := mapping.NewLogarithmicMapping(.009)
	if err != nil {
		panic(err)
	}
	return &Estimator{
		mapping:  m,
		positive: store.NewCollapsingLowestDenseStore(limit),
		negative: store.NewCollapsingLowestDenseStore(limit),
		limit:    limit,
		exact:    true,
	}
}

// down and up step a finite value to the adjacent float64, as math.Nextafter
// toward -Inf and +Inf does. Zero, of either sign, steps to the smallest subnormal.
func down(v float64) float64 {
	switch {
	case v == 0:
		return -0x1p-1074
	case v > 0:
		return math.Float64frombits(math.Float64bits(v) - 1)
	default:
		return math.Float64frombits(math.Float64bits(v) + 1)
	}
}

func up(v float64) float64 {
	switch {
	case v == 0:
		return 0x1p-1074
	case v > 0:
		return math.Float64frombits(math.Float64bits(v) + 1)
	default:
		return math.Float64frombits(math.Float64bits(v) - 1)
	}
}

// Withheld reports why both percentiles of this window are withheld, or "" when
// they are available.
func (a *Estimator) Withheld() string { return a.reason }

func (a *Estimator) fail(reason string) {
	if a.reason == "" {
		a.reason = reason
	}
}

// Add runs only after basic statistics pass their atomic finite checks.
// Reliability failure withholds both percentiles, without rejecting that observation.
func (a *Estimator) Add(value, rate float64) {
	if a.reason != "" {
		return
	}
	if !(rate >= minRate && rate <= 1) || math.IsNaN(value) || math.IsInf(value, 0) {
		a.fail(withheldNumericDomain)
		return
	}
	if a.n == maxObservations {
		a.fail(withheldObservationBound)
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
		a.fail(withheldMappingDomain)
		return
	}
	index := a.mapping.Index(magnitude)
	rep := a.mapping.Value(index)
	// Outward error and inward budget prove the 1% inequality for THIS mapping,
	// without assuming the implementation's transcendental errors are zero.
	if !finite(rep) || rep <= 0 || (rep != magnitude && up(math.Abs(rep-magnitude)) > down(magnitude/100)) {
		a.fail(withheldMappingError)
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
			a.fail(withheldSpan)
			return
		}
	}
	target.AddWithCount(index, z)
}

func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

// Reset empties the estimator for the next window.
func (a *Estimator) Reset() {
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

// Query returns p50 and p95, or NaN for both when they are withheld. It reuses
// scratch and returns it for the next query.
func (a *Estimator) Query(scratch []Bin) ([2]float64, []Bin) {
	result := [2]float64{math.NaN(), math.NaN()}
	scratch = scratch[:0]
	if a.reason != "" || a.n == 0 {
		return result, scratch
	}
	a.negative.ForEach(func(i int, c float64) bool {
		scratch = append(scratch, Bin{-a.mapping.Value(i), c})
		return false
	})
	if a.zero > 0 {
		scratch = append(scratch, Bin{0, a.zero})
	}
	a.positive.ForEach(func(i int, c float64) bool {
		scratch = append(scratch, Bin{a.mapping.Value(i), c})
		return false
	})
	slices.SortFunc(scratch, func(a, b Bin) int {
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
	growth, shrink := up(1+delta), down(1-delta)
	var totalLo, totalHi float64
	for _, b := range scratch {
		totalLo = down(totalLo + down(b.count/growth))
		totalHi = up(totalHi + up(b.count/shrink))
	}
	var thresholdLo, thresholdHi [2]float64
	for q := range result {
		thresholdLo[q] = down(down(totalLo*float64(numer[q])) / float64(denom[q]))
		thresholdHi[q] = up(up(totalHi*float64(numer[q])) / float64(denom[q]))
	}
	var prefixLo, prefixHi float64
	for _, b := range scratch {
		beforeHi := prefixHi
		prefixLo = down(prefixLo + down(b.count/growth))
		prefixHi = up(prefixHi + up(b.count/shrink))
		for q := range result {
			if math.IsNaN(result[q]) && beforeHi < thresholdLo[q] && prefixLo >= thresholdHi[q] {
				result[q] = b.value
			}
		}
		if !math.IsNaN(result[0]) && !math.IsNaN(result[1]) {
			break // Later bins cannot change a certified rank.
		}
	}
	if math.IsNaN(result[0]) || math.IsNaN(result[1]) {
		a.fail(withheldAmbiguousRank)
		return [2]float64{math.NaN(), math.NaN()}, scratch
	}
	return result, scratch
}
