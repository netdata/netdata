// SPDX-License-Identifier: GPL-3.0-or-later

package fixture

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Source-derived time-grouping arithmetic used by layer 3.
//
// Source: netdata/netdata @ 043f50ec075441010c1495250871d37a8ac69f8d
//   - considered-equal:
//     src/libnetdata/storage_number/storage_number.h:60-61
//   - percentile interpolation:
//     src/libnetdata/statistical/statistical.c:171-189
//   - average/sum/min/max:
//     src/web/api/queries/average/average.h:34-59
//     src/web/api/queries/sum/sum.h:31-53
//     src/web/api/queries/min/min.h:31-56
//     src/web/api/queries/max/max.h:31-56
//   - latest/extremes/stddev and coefficient-of-variation:
//     src/web/api/queries/latest/latest.h:35-60
//     src/web/api/queries/extremes/extremes.h:37-98
//     src/web/api/queries/stddev/stddev.h:33-117
//   - median/trimmed-mean/percentile:
//     src/web/api/queries/median/median.h:17-34,81-144
//     src/web/api/queries/trimmed_mean/trimmed_mean.h:17-34,78-170
//     src/web/api/queries/percentile/percentile.h:17-34,81-173
//   - SES/DES:
//     src/web/api/queries/ses/ses.h:28-89
//     src/web/api/queries/des/des.h:33-135
//   - countif numeric add/flush:
//     src/web/api/queries/countif/countif.h:104-150
//
// Inputs are the numeric sample values of each view bucket in timestamp
// order, already SNRoundTrip'd as tier 0 feeds them to the grouping.

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

	case "latest":
		// latest.h: the last (chronologically newest) non-gap value of
		// the bucket; empty when nothing was collected in it
		if n == 0 {
			return TGResult{Empty: true}
		}
		return TGResult{Value: values[n-1]}

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
		cmp, target, err := parseCountifOptions(options)
		if err != nil {
			panic("fixture: invalid countif expression: " + err.Error())
		}
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

func namePercent(name, prefix string, defaultPercent float64) (float64, bool) {
	suffix, ok := strings.CutPrefix(name, prefix)
	if !ok {
		return 0, false
	}
	if suffix == "" {
		return defaultPercent, true
	}
	percent, err := strconv.ParseFloat(suffix, 64)
	return percent, err == nil
}

// medianPercent maps median-family names to their default trim percent.
func medianPercent(name string) (float64, bool) {
	if name == "median" {
		return 0.0, true
	}
	return namePercent(name, "trimmed-median", 5.0)
}

// meanWindowPercent maps trimmed-mean names to the kept-slots percent.
func meanWindowPercent(name string) (float64, bool) {
	return namePercent(name, "trimmed-mean", 5.0)
}

// percentilePercent maps percentile names to the kept-slots percent.
func percentilePercent(name string) (float64, bool) {
	return namePercent(name, "percentile", 95.0)
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

// parseCountifOptions is the approved finite-numeric contract subset, not a
// port of the historical fail-open C parser. An absent or blank expression is
// ==0, and an operator without an operand applies to zero. Malformed,
// trailing-junk and non-finite operands are invalid. CASE-023 owns the
// gap-token and predecessor grammar.
func parseCountifOptions(options string) (cmp string, target float64, err error) {
	s := strings.TrimSpace(options)
	if s == "" {
		return "==", 0, nil
	}

	type operator struct {
		spelling string
		cmp      string
	}
	operators := [...]operator{
		{"!=", "!="},
		{"<>", "!="},
		{">=", ">="},
		{">:", ">="},
		{"<=", "<="},
		{"<:", "<="},
		{"==", "=="},
		{">", ">"},
		{"<", "<"},
		{"=", "=="},
		{":", "=="},
	}
	cmp = "=="
	operand := s
	for _, operator := range operators {
		if strings.HasPrefix(s, operator.spelling) {
			cmp = operator.cmp
			operand = strings.TrimSpace(s[len(operator.spelling):])
			break
		}
	}
	if operand == "" {
		return cmp, 0, nil
	}

	target, err = parseFiniteDecimal(operand)
	if err != nil {
		return "", 0, fmt.Errorf("expression %q has invalid finite numeric operand", options)
	}
	return cmp, target, nil
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

// TGOracleIncrementalSum is a Class A telescoping/conservation oracle, not a
// port of the current C implementation
// (src/web/api/queries/incremental_sum/incremental_sum.h:17-68 at the checked
// revision above). It computes per-bucket incremental-sum results:
// bucket value = last - first, where first carries from the previous
// bucket's last. A bucket with nothing to measure against yields EMPTY,
// but it does NOT throw the baseline away.
//
// The baseline has to survive a bucket that produced no answer, because
// the quantity is a CHANGE and change does not stop happening while the
// collector is silent. Dropping it loses everything that accumulated in
// the gap: over the canonical fixture (values 1..60, samples 21..30
// missing, buckets of 10) a dropped baseline totals 48 across the window
// while the series actually moved 59. Carrying it hands the gap's change
// to the first bucket that can see it - late, but never lost.
//
// It is the same reason a leading single-sample bucket keeps its sample:
// it has no PREDECESSOR to measure against, not nothing to offer its
// SUCCESSOR. Dropping it there is what made a chart drawn at the
// collection interval come back blank for its whole length.
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
		if !math.IsNaN(last) {
			first = last
		}
	}
	return out
}

// TierFetchValue mirrors the registry's tier_query_fetch mapping at the
// checked revision above:
//   - src/web/api/queries/query-group-over-time.c:58-642
//   - src/web/api/queries/query-execute.c:284-308
//
// On tier>=1 data, min/max/sum consume the tier point's min/max/sum and every
// other family consumes the per-point average (sum/count).
func TierFetchValue(name string, tp TierPoint) float64 {
	if tp.Empty || tp.Count == 0 {
		panic("fixture: TierFetchValue called on an empty tier window")
	}

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
