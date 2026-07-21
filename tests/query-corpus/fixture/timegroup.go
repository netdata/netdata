// SPDX-License-Identifier: GPL-3.0-or-later

package fixture

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

// Per-family Go ports of the engine's time-grouping arithmetic
// (src/web/api/queries/<family>/<family>.h), used as layer-3 oracles.
// Inputs are the numeric (non-gap) sample values of each view bucket, in
// timestamp order, already SNRoundTrip'd (that is what tier0 feeds the
// grouping: query_add_point_to_group adds only finite values).

// TGResult is one bucket's oracle output. Empty mirrors RRDR_VALUE_EMPTY
// (a null value in json2).
type TGResult struct {
	Value float64
	Empty bool
}

// consideredEqual mirrors considered_equal_ndd (epsilonndd = 1e-7).
func consideredEqual(a, b float64) bool {
	return math.Abs(a-b) < 0.0000001
}

// percentileOnSorted mirrors percentile_on_sorted_series
// (libnetdata/statistical/statistical.c): linear interpolation on the
// fractional index of an ascending-sorted series.
func percentileOnSorted(series []float64, percentile float64) float64 {
	entries := len(series)
	if entries == 0 {
		return math.NaN()
	}
	if entries == 1 {
		return series[0]
	}
	percentile = math.Max(0.0, math.Min(1.0, percentile))
	index := percentile * float64(entries-1)
	low := int(math.Floor(index))
	high := int(math.Ceil(index))
	if high >= entries || low == high || consideredEqual(index, float64(low)) {
		return series[low]
	}
	weight := index - float64(low)
	return series[low] + weight*(series[high]-series[low])
}

// tgSimple computes the per-bucket value for the stateless families.
// name is the registry name; options the time_group_options string.
func tgSimple(name, options string, values []float64) TGResult {
	n := len(values)
	switch name {
	case "average", "avg", "mean":
		if n == 0 {
			return TGResult{Empty: true}
		}
		sum := 0.0
		for _, v := range values {
			sum += v
		}
		return TGResult{Value: sum / float64(n)}

	case "sum":
		if n == 0 {
			return TGResult{Empty: true}
		}
		sum := 0.0
		for _, v := range values {
			sum += v
		}
		return TGResult{Value: sum}

	case "min":
		// min.h:34 — netdata's min is by ABSOLUTE value: the value
		// closest to zero wins (equals arithmetic min only for
		// non-negative data). Pinned current contract; ruling pending.
		if n == 0 {
			return TGResult{Empty: true}
		}
		m := values[0]
		for _, v := range values[1:] {
			if math.Abs(v) < math.Abs(m) {
				m = v
			}
		}
		return TGResult{Value: m}

	case "max":
		// max.h:34 — by ABSOLUTE value: the value furthest from zero
		// wins (what `extremes` also does, deliberately).
		if n == 0 {
			return TGResult{Empty: true}
		}
		m := values[0]
		for _, v := range values[1:] {
			if math.Abs(v) > math.Abs(m) {
				m = v
			}
		}
		return TGResult{Value: m}

	case "extremes":
		// extremes.h: champion by sign; both signs → larger |abs|
		var minNeg, maxPos float64
		var posCount, negCount, zeroCount int
		for _, v := range values {
			switch {
			case v > 0:
				if posCount == 0 || v > maxPos {
					maxPos = v
				}
				posCount++
			case v < 0:
				if negCount == 0 || v < minNeg {
					minNeg = v
				}
				negCount++
			default:
				zeroCount++
			}
		}
		switch {
		case posCount == 0 && negCount == 0 && zeroCount == 0:
			return TGResult{Empty: true}
		case posCount > 0 && negCount > 0:
			if math.Abs(maxPos) > math.Abs(minNeg) {
				return TGResult{Value: maxPos}
			}
			return TGResult{Value: minNeg}
		case posCount > 0:
			return TGResult{Value: maxPos}
		case negCount > 0:
			return TGResult{Value: minNeg}
		default:
			return TGResult{Value: 0.0}
		}

	case "stddev", "cv", "rsd", "coefficient-of-variation":
		// stddev.h: Welford running mean/variance, SAMPLE variance (n-1)
		var count int
		var oldM, newM, oldS, newS float64
		for _, v := range values {
			count++
			if count == 1 {
				oldM, newM = v, v
				oldS = 0.0
			} else {
				newM = oldM + (v-oldM)/float64(count)
				newS = oldS + (v-oldM)*(v-newM)
				oldM, oldS = newM, newS
			}
		}
		switch {
		case count == 0:
			return TGResult{Empty: true}
		case count == 1:
			return TGResult{Value: 0.0}
		}
		sd := math.Sqrt(newS / float64(count-1))
		if name == "stddev" {
			if math.IsNaN(sd) || math.IsInf(sd, 0) {
				return TGResult{Empty: true}
			}
			return TGResult{Value: sd}
		}
		// coefficient of variation: 100 * stddev / |mean|
		v := 100.0 * sd / math.Abs(newM)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return TGResult{Empty: true}
		}
		return TGResult{Value: v}

	case "countif":
		if n == 0 {
			return TGResult{Empty: true}
		}
		cmp, target := parseCountifOptions(options)
		matched := 0
		for _, v := range values {
			ok := false
			switch cmp {
			case ">":
				ok = v > target
			case ">=":
				ok = v >= target
			case "<":
				ok = v < target
			case "<=":
				ok = v <= target
			case "==":
				ok = v == target
			case "!=":
				ok = v != target
			}
			if ok {
				matched++
			}
		}
		return TGResult{Value: float64(matched) * 100 / float64(n)}
	}

	if pct, ok := medianPercent(name); ok {
		return tgMedian(values, pct, options)
	}
	if pct, ok := meanWindowPercent(name); ok {
		// trimmed_mean.h create: N% trimmed per SIDE, options override
		// (clamped 0..50) — the kept window is (100 - 2N)% of the slots
		if options != "" {
			p, err := strconv.ParseFloat(options, 64)
			if err != nil || math.IsNaN(p) || math.IsInf(p, 0) {
				p = 0.0
			}
			pct = math.Max(0.0, math.Min(50.0, p))
		}
		return tgSlotWindowMean(values, 100.0-2.0*pct, false)
	}
	if pct, ok := percentilePercent(name); ok {
		// percentile.h create: options override clamped 0..100, used as
		// the kept-slots percent directly
		if options != "" {
			p, err := strconv.ParseFloat(options, 64)
			if err != nil || math.IsNaN(p) || math.IsInf(p, 0) {
				p = 0.0
			}
			pct = math.Max(0.0, math.Min(100.0, p))
		}
		return tgSlotWindowMean(values, pct, true)
	}

	panic("unknown time grouping oracle: " + name)
}

// medianPercent maps median-family names to their default trim percent.
func medianPercent(name string) (float64, bool) {
	if name == "median" {
		return 0.0, true
	}
	if s, ok := strings.CutPrefix(name, "trimmed-median"); ok {
		if s == "" {
			return 5.0, true
		}
		p, err := strconv.ParseFloat(s, 64)
		return p, err == nil
	}
	return 0, false
}

// meanWindowPercent maps trimmed-mean names to the kept-slots percent.
func meanWindowPercent(name string) (float64, bool) {
	if s, ok := strings.CutPrefix(name, "trimmed-mean"); ok {
		if s == "" {
			return 5.0, true
		}
		p, err := strconv.ParseFloat(s, 64)
		return p, err == nil
	}
	return 0, false
}

// percentilePercent maps percentile names to the kept-slots percent.
func percentilePercent(name string) (float64, bool) {
	if s, ok := strings.CutPrefix(name, "percentile"); ok {
		if s == "" {
			return 95.0, true
		}
		p, err := strconv.ParseFloat(s, 64)
		return p, err == nil
	}
	return 0, false
}

// tgMedian mirrors tg_median_flush (median.h): sort, trim by VALUE RANGE
// (delta = (max-min)*pct), then the R-7 quantile of the surviving slots.
func tgMedian(values []float64, defPercent float64, options string) TGResult {
	n := len(values)
	if n == 0 {
		return TGResult{Empty: true}
	}
	if n == 1 {
		return TGResult{Value: values[0]}
	}

	percent := defPercent
	if options != "" {
		p, err := strconv.ParseFloat(options, 64)
		if err != nil || math.IsNaN(p) || math.IsInf(p, 0) {
			p = 0.0
		}
		percent = math.Max(0.0, math.Min(50.0, p))
	}
	percent /= 100.0

	series := append([]float64(nil), values...)
	sort.Float64s(series)

	start, end := 0, n-1
	if percent > 0.0 {
		minV, maxV := series[0], series[n-1]
		delta := (maxV - minV) * percent
		wantedMin, wantedMax := minV+delta, maxV-delta
		for start = 0; start < n; start++ {
			if series[start] >= wantedMin {
				break
			}
		}
		for end = n - 1; end > start; end-- {
			if series[end] <= wantedMax {
				break
			}
		}
	}

	var v float64
	if start == end {
		v = series[start]
	} else {
		v = percentileOnSorted(series[start:end+1], 0.5)
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return TGResult{Empty: true}
	}
	return TGResult{Value: v}
}

// tgSlotWindowMean mirrors tg_trimmed_mean_flush / tg_percentile_flush:
// the MEAN of a window of sorted slots (percent of the available slots,
// with fractional-slot interpolation). percentile mode anchors the window
// at the low end (or the high end when any value is negative); trimmed
// mode centers it. min==max short-circuits to min.
func tgSlotWindowMean(values []float64, percentArg float64, percentileMode bool) TGResult {
	n := len(values)
	if n == 0 {
		return TGResult{Empty: true}
	}
	if n == 1 {
		return TGResult{Value: values[0]}
	}

	percent := math.Max(0.0, math.Min(100.0, percentArg)) / 100.0

	series := append([]float64(nil), values...)
	sort.Float64s(series)

	minV, maxV := series[0], series[n-1]
	if minV == maxV {
		return TGResult{Value: minV}
	}

	slotsToUse := int(float64(n) * percent)
	if slotsToUse == 0 {
		slotsToUse = 1
	}
	percentToUse := float64(slotsToUse) / float64(n)
	percentDelta := percent - percentToUse

	var percentInterpolationSlot, percentLastSlot float64
	if percentDelta > 0.0 {
		percentToUsePlus1 := float64(slotsToUse+1) / float64(n)
		percent1Slot := percentToUsePlus1 - percentToUse
		percentInterpolationSlot = percentDelta / percent1Slot
		percentLastSlot = 1 - percentInterpolationSlot
	}

	var startSlot, stopSlot, step, lastSlot, interpolationSlot int
	if minV >= 0.0 && maxV >= 0.0 {
		if percentileMode {
			startSlot = 0
		} else {
			startSlot = (n - slotsToUse) / 2
		}
		stopSlot = startSlot + slotsToUse
		lastSlot = stopSlot - 1
		interpolationSlot = stopSlot
		step = 1
	} else {
		if percentileMode {
			startSlot = n - 1
		} else {
			startSlot = n - 1 - (n-slotsToUse)/2
		}
		stopSlot = startSlot - slotsToUse
		lastSlot = stopSlot + 1
		interpolationSlot = stopSlot
		step = -1
	}

	value := 0.0
	for slot := startSlot; slot != stopSlot; slot += step {
		value += series[slot]
	}
	counted := slotsToUse
	if percentInterpolationSlot > 0.0 && interpolationSlot >= 0 && interpolationSlot < n {
		value += series[interpolationSlot] * percentInterpolationSlot
		value += series[lastSlot] * percentLastSlot
		counted++
	}
	value /= float64(counted)

	if math.IsNaN(value) || math.IsInf(value, 0) {
		return TGResult{Empty: true}
	}
	return TGResult{Value: value}
}

// parseCountifOptions mirrors tg_countif_create's grammar.
func parseCountifOptions(options string) (cmp string, target float64) {
	s := strings.TrimLeft(options, " \t")
	if s == "" {
		return "==", 0.0
	}
	switch s[0] {
	case '!':
		cmp = "!="
		if len(s) > 1 && (s[1] == '=' || s[1] == ':') {
			s = s[2:]
		} else {
			s = s[1:]
		}
	case '>':
		if len(s) > 1 && (s[1] == '=' || s[1] == ':') {
			cmp = ">="
			s = s[2:]
		} else {
			cmp = ">"
			s = s[1:]
		}
	case '<':
		if len(s) > 1 && s[1] == '>' {
			cmp = "!="
			s = s[2:]
		} else if len(s) > 1 && (s[1] == '=' || s[1] == ':') {
			cmp = "<="
			s = s[2:]
		} else {
			cmp = "<"
			s = s[1:]
		}
	case '=':
		cmp = "=="
		if len(s) > 1 && s[1] == '=' {
			s = s[2:]
		} else {
			s = s[1:]
		}
	default:
		// ':' compares EQUAL; for a bare number tg_countif_create's
		// post-switch advance consumes one char even though no operator
		// matched, so the first digit is swallowed ("40" targets 0,
		// "440" targets 40) — pinned quirk. Both paths skip one char.
		cmp = "=="
		s = s[1:]
	}
	s = strings.TrimLeft(s, " \t")
	target, _ = strconv.ParseFloat(s, 64)
	return cmp, target
}

// sesWindow mirrors tg_ses_window/tg_des_window: group for grouped views,
// points_wanted for identity views, capped at 15 (stock config).
func sesWindow(group, pointsWanted int) float64 {
	points := float64(group)
	if group == 1 {
		points = float64(pointsWanted)
	}
	if points > 15 {
		return 15
	}
	return points
}

// TGOracleSES computes per-bucket SES (ema/ewma) results: EMA with
// alpha = 2/(W+1) whose level RUNS ACROSS buckets (flush does not reset);
// a bucket with no values after data has flowed returns the running level.
func TGOracleSES(buckets [][]float64, group, pointsWanted int) []TGResult {
	alpha := 2.0 / (sesWindow(group, pointsWanted) + 1.0)
	level := 0.0
	count := 0
	out := make([]TGResult, 0, len(buckets))
	for _, bucket := range buckets {
		for _, v := range bucket {
			if count == 0 {
				level = v
			}
			level = alpha*v + (1.0-alpha)*level
			count++
		}
		if count == 0 || math.IsNaN(level) || math.IsInf(level, 0) {
			out = append(out, TGResult{Empty: true})
		} else {
			out = append(out, TGResult{Value: level})
		}
	}
	return out
}

// TGOracleDES computes per-bucket DES (Holt) results, mirroring
// tg_des_add exactly — including the compound update on the second value.
func TGOracleDES(buckets [][]float64, group, pointsWanted int) []TGResult {
	w := sesWindow(group, pointsWanted)
	alpha := 2.0 / (w + 1.0)
	beta := 2.0 / (w + 1.0)
	var level, trend float64
	count := 0
	out := make([]TGResult, 0, len(buckets))
	for _, bucket := range buckets {
		for _, v := range bucket {
			if count > 0 {
				if count == 1 {
					trend = v - trend
					level = v
				}
				lastLevel := level
				level = alpha*v + (1.0-alpha)*(level+trend)
				trend = beta*(level-lastLevel) + (1.0-beta)*trend
			} else {
				level, trend = v, v
			}
			count++
		}
		if count == 0 || math.IsNaN(level) || math.IsInf(level, 0) {
			out = append(out, TGResult{Empty: true})
		} else {
			out = append(out, TGResult{Value: level})
		}
	}
	return out
}

// TGOracleIncrementalSum computes per-bucket incremental-sum results:
// bucket value = last - first, where first carries from the previous
// bucket's last; an empty bucket (or a leading single-value bucket)
// yields EMPTY and resets the carry.
func TGOracleIncrementalSum(buckets [][]float64) []TGResult {
	first := math.NaN()
	out := make([]TGResult, 0, len(buckets))
	for _, bucket := range buckets {
		last := math.NaN()
		count := 0
		for _, v := range bucket {
			if count == 0 {
				if math.IsNaN(first) {
					first = v
				} else {
					last = v
				}
			} else {
				last = v
			}
			count++
		}
		if count == 0 || math.IsNaN(first) || math.IsNaN(last) {
			out = append(out, TGResult{Empty: true})
		} else {
			out = append(out, TGResult{Value: last - first})
		}
		first = last
	}
	return out
}

// TierFetchValue mirrors the registry's tier_query_fetch mapping
// (query-group-over-time.c + query-execute.c): on tier>=1 data, min/max/
// sum consume the tier point's min/max/sum, every other family consumes
// the per-point AVERAGE (sum/count).
func TierFetchValue(name string, tp TierPoint) float64 {
	switch name {
	case "min":
		return tp.Min
	case "max":
		return tp.Max
	case "sum":
		return tp.Sum
	default:
		return tp.Sum / float64(tp.Count)
	}
}

// TGOracle computes per-bucket results for any registry time grouping.
// group/pointsWanted describe the view (for the ses/des window).
func TGOracle(name, options string, buckets [][]float64, group, pointsWanted int) []TGResult {
	switch name {
	case "ses", "ema", "ewma":
		return TGOracleSES(buckets, group, pointsWanted)
	case "des":
		return TGOracleDES(buckets, group, pointsWanted)
	case "incremental-sum":
		return TGOracleIncrementalSum(buckets)
	}
	out := make([]TGResult, 0, len(buckets))
	for _, bucket := range buckets {
		out = append(out, tgSimple(name, options, bucket))
	}
	return out
}
