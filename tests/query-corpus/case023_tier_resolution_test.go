// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"math"
	"sort"
	"strconv"
	"testing"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

const (
	c023ResolutionUE          = 10
	c023ResolutionTier1       = c023ResolutionUE * tier1Gran
	c023ResolutionTier2       = c023ResolutionUE * tier2Gran
	c023ResolutionWindows     = 4
	c023ResolutionPushWindows = 5 // the fifth tier-2 window flushes the fourth
	c023ResolutionResetAt     = 2*3600 + 31
)

func c023ResolutionBase() int64 {
	return fixture.T0 - fixture.T0%c023ResolutionTier2
}

// c023ResolutionFixture gives every candidate implementation a different
// answer. The first four tier-2 windows contain:
//
//   - availability: all-one; all-gap; alternating one/gap; then 15 zero
//     and 45 one samples in every tier-1 record;
//   - counter: one real restart inside the third tier-2 window;
//   - nonbinary: 20 zero, 20 five, and 20 ten samples per tier-1 record.
func c023ResolutionFixture() fixture.Chart {
	ch := fixture.Chart{
		ID: "fixture.c023resolution", Title: "fleet grouping tier resolution",
		Units: "units", Family: "fixture", Context: "fixture.c023resolution",
		UpdateEvery: c023ResolutionUE,
		// settleAndVerify compares the storage_number round trip before the
		// matrix; decimal reciprocal noise is below 1e-12.
		ValueTolerance: 1e-12,
		Dimensions: []fixture.Dimension{
			{ID: "availability"},
			{ID: "counter"},
			{ID: "nonbinary"},
		},
	}

	base := c023ResolutionBase()
	samples := c023ResolutionPushWindows * 3600
	for i := 1; i <= samples; i++ {
		ts := base + int64(i*c023ResolutionUE)
		window := (i - 1) / 3600
		inTier1 := (i - 1) % 60

		availability := fixture.Point{T: ts, Flags: stream.FlagNotAnomalous}
		switch window {
		case 0:
			availability.Collected = "1"
		case 1:
			availability.Flags = stream.FlagEmpty
		case 2:
			if inTier1%2 == 0 {
				availability.Flags = stream.FlagEmpty
			} else {
				availability.Collected = "1"
			}
		case 3:
			if inTier1 < 15 {
				availability.Collected = "0"
			} else {
				availability.Collected = "1"
			}
		default:
			availability.Collected = "1"
		}
		ch.Dimensions[0].Points = append(ch.Dimensions[0].Points, availability)

		counter := i
		if i >= c023ResolutionResetAt {
			counter = i - c023ResolutionResetAt + 1
		}
		ch.Dimensions[1].Points = append(ch.Dimensions[1].Points, fixture.Point{
			T: ts, Collected: strconv.Itoa(counter), Flags: stream.FlagNotAnomalous,
		})

		// Neighboring tier1 and tier2 records deliberately have different
		// averages. That makes a view finer than storage distinguish the
		// engine's interpolation from blindly repeating one stored average.
		tier2Window := window
		tier1Window := ((i - 1) % 3600) / 60
		zeroes, fives := 40, 10
		switch {
		case tier2Window%2 == 0 && tier1Window%2 == 1:
			zeroes, fives = 30, 20
		case tier2Window%2 == 1 && tier1Window%2 == 0:
			zeroes, fives = 10, 20
		case tier2Window%2 == 1 && tier1Window%2 == 1:
			zeroes, fives = 10, 10
		}
		nonbinary := 10
		if inTier1 < zeroes {
			nonbinary = 0
		} else if inTier1 < zeroes+fives {
			nonbinary = 5
		}
		ch.Dimensions[2].Points = append(ch.Dimensions[2].Points, fixture.Point{
			T: ts, Collected: strconv.Itoa(nonbinary), Flags: stream.FlagNotAnomalous,
		})
	}
	return ch
}

func c023ResolutionDimension(ch fixture.Chart, id string) fixture.Dimension {
	for _, d := range ch.Dimensions {
		if d.ID == id {
			return d
		}
	}
	panic("CASE-023 resolution fixture has no dimension " + id)
}

func c023PointValue(p fixture.Point) (float64, bool) {
	return p.CollectedValue("CASE-023 resolution fixture")
}

func c023BucketIndex(ts, after, step int64, points int) int {
	if ts <= after || ts > after+int64(points)*step {
		return -1
	}
	return int((ts - after - 1) / step)
}

func c023WindowRecords(d fixture.Dimension, granularity int64) []fixture.TierPoint {
	// The rollup's nominal source slots follow this fixture's 10-second cadence.
	windows := d.TierWindows(granularity, c023ResolutionUE)
	out := make([]fixture.TierPoint, 0, len(windows))
	for _, w := range windows {
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EndT < out[j].EndT })
	return out
}

func TestC023ResolutionWindowRecordsUseFixtureCadence(t *testing.T) {
	d := c023ResolutionDimension(c023ResolutionFixture(), "availability")
	records := c023WindowRecords(d, c023ResolutionTier1)
	if len(records) == 0 {
		t.Fatal("CASE-023 resolution oracle produced no tier1 records")
	}
	for _, record := range records {
		if got := record.Count + record.GapCount; got != int(tier1Gran) {
			t.Fatalf("tier1 record ending %d models %d source slots, want %d at update_every=%d",
				record.EndT, got, tier1Gran, c023ResolutionUE)
		}
	}
}

func c023Overlaps(w fixture.TierPoint, granularity, from, to int64) bool {
	return w.EndT > from && w.EndT-granularity < to
}

func c023Overlap(w fixture.TierPoint, granularity, from, to int64) int64 {
	lo := w.EndT - granularity
	if lo < from {
		lo = from
	}
	hi := w.EndT
	if hi > to {
		hi = to
	}
	if hi <= lo {
		return 0
	}
	return hi - lo
}

// c023WindowFractionEqual is the deliberate higher-tier two-point model. It
// is Class B because a rollup preserves min/max/average but not the interior
// distribution.
//
// Source: netdata/netdata @ c8f9ce4d5622767ea752a2877bf1049a0bc85a46
// src/web/api/queries/tg-expression.h:366-436
// tg_expression_window_fraction()
func c023WindowFractionEqual(w fixture.TierPoint, target float64) float64 {
	if w.Empty || w.Count == 0 {
		return 0
	}
	avg := w.Sum / float64(w.Count)
	if w.Max <= w.Min {
		if avg == target {
			return 1
		}
		return 0
	}
	weightMax := (avg - w.Min) / (w.Max - w.Min)
	if target == w.Min {
		return 1 - weightMax
	}
	if target == w.Max {
		return weightMax
	}
	return 0
}

func c023RawPercentageEqual(
	d fixture.Dimension,
	after, before int64,
	points int,
	target float64,
) []expectedColumnPoint {
	step := (before - after) / int64(points)
	matched := make([]float64, points)
	total := make([]float64, points)
	for _, p := range d.Points {
		bucket := c023BucketIndex(p.T, after, step, points)
		if bucket < 0 {
			continue
		}
		value, collected := c023PointValue(p)
		total[bucket] += c023ResolutionUE
		if collected && value == target {
			matched[bucket] += c023ResolutionUE
		}
	}

	out := make([]expectedColumnPoint, points)
	for i := range out {
		end := after + int64(i+1)*step
		if total[i] == 0 {
			out[i] = wantEmptyAt(end)
		} else {
			out[i] = wantNumberAt(end, 100*matched[i]/total[i])
		}
	}
	return out
}

// Class B duration-weighted percentage-of-time delivery.
//
// Source: netdata/netdata @ c8f9ce4d5622767ea752a2877bf1049a0bc85a46
// src/web/api/queries/tg-expression.h:366-436
// tg_expression_window_fraction()
// src/web/api/queries/percentage_of_time/percentage_of_time.h:57-75,86-105
// tg_percentage_of_time_add_point(), tg_percentage_of_time_flush()
// src/web/api/queries/query-execute.c:78-190
// query_add_point_to_group()
func c023TierPercentageEqual(
	d fixture.Dimension,
	granularity, after, before int64,
	points int,
	target float64,
) []expectedColumnPoint {
	step := (before - after) / int64(points)
	records := c023WindowRecords(d, granularity)
	out := make([]expectedColumnPoint, points)
	for i := range out {
		from := after + int64(i)*step
		to := from + step
		matched, total := 0.0, 0.0
		for _, w := range records {
			if !c023Overlaps(w, granularity, from, to) {
				continue
			}
			duration := float64(c023Overlap(w, granularity, from, to))
			total += duration
			matched += c023WindowFractionEqual(w, target) * duration
		}
		if total < float64(step) {
			// Uncovered time is an explicit non-match in this grouping.
			total = float64(step)
		}
		end := to
		if total == 0 {
			out[i] = wantEmptyAt(end)
		} else {
			out[i] = wantNumberAt(end, 100*matched/total)
		}
	}
	return out
}

func c023PercentageEqual(
	d fixture.Dimension,
	tier int,
	after, before int64,
	points int,
	target float64,
) []expectedColumnPoint {
	switch tier {
	case 0:
		return c023RawPercentageEqual(d, after, before, points, target)
	case 1:
		return c023TierPercentageEqual(d, c023ResolutionTier1, after, before, points, target)
	case 2:
		return c023TierPercentageEqual(d, c023ResolutionTier2, after, before, points, target)
	default:
		panic("unsupported tier")
	}
}

func c023RawPercentageGreater(
	d fixture.Dimension,
	after, before int64,
	points int,
	target float64,
) []expectedColumnPoint {
	step := (before - after) / int64(points)
	matched := make([]float64, points)
	total := make([]float64, points)
	for _, p := range d.Points {
		bucket := c023BucketIndex(p.T, after, step, points)
		if bucket < 0 {
			continue
		}
		value, collected := c023PointValue(p)
		if !collected {
			continue
		}
		total[bucket]++
		if value > target {
			matched[bucket]++
		}
	}

	out := make([]expectedColumnPoint, points)
	for i := range out {
		end := after + int64(i+1)*step
		if total[i] == 0 {
			out[i] = wantEmptyAt(end)
		} else {
			out[i] = wantNumberAt(end, 100*matched[i]/total[i])
		}
	}
	return out
}

// c023TierDeliveredAverage is the Class B partial-delivery interpolation.
//
// Source: netdata/netdata @ 043f50ec075441010c1495250871d37a8ac69f8d
// src/web/api/queries/query-execute.c:59-75
// query_interpolate_point
func c023TierDeliveredAverage(
	w fixture.TierPoint,
	byEnd map[int64]fixture.TierPoint,
	granularity, deliveryEnd int64,
) float64 {
	average := w.Sum / float64(w.Count)
	if deliveryEnd >= w.EndT {
		return average
	}

	prior, ok := byEnd[w.EndT-granularity]
	if !ok || prior.Empty || prior.Count == 0 {
		return average
	}
	priorAverage := prior.Sum / float64(prior.Count)
	fraction := float64(deliveryEnd-(w.EndT-granularity)) / float64(granularity)
	return priorAverage + (average-priorAverage)*fraction
}

// Class B percentage-of-samples delivery over re-delivered stored records.
//
// Source: netdata/netdata @ c8f9ce4d5622767ea752a2877bf1049a0bc85a46
// src/web/api/queries/countif/countif.h:46-70,79-98
// tg_countif_add_point(), tg_countif_flush()
// src/web/api/queries/query-execute.c:60-76,522-570
// query_interpolate_point, inner point re-delivery loop
func c023TierPercentageGreater(
	d fixture.Dimension,
	granularity, after, before int64,
	points int,
	target float64,
) []expectedColumnPoint {
	step := (before - after) / int64(points)
	records := c023WindowRecords(d, granularity)
	byEnd := make(map[int64]fixture.TierPoint, len(records))
	for _, w := range records {
		byEnd[w.EndT] = w
	}

	out := make([]expectedColumnPoint, points)
	for i := range out {
		from := after + int64(i)*step
		to := from + step
		matched, total := 0.0, 0.0
		for _, w := range records {
			if w.Empty || w.Count == 0 || !c023Overlaps(w, granularity, from, to) {
				continue
			}
			total++
			if c023TierDeliveredAverage(w, byEnd, granularity, to) > target {
				matched++
			}
		}
		if total == 0 {
			out[i] = wantEmptyAt(to)
		} else {
			out[i] = wantNumberAt(to, 100*matched/total)
		}
	}
	return out
}

func c023PercentageGreater(
	d fixture.Dimension,
	tier int,
	after, before int64,
	points int,
	target float64,
) []expectedColumnPoint {
	switch tier {
	case 0:
		return c023RawPercentageGreater(d, after, before, points, target)
	case 1:
		return c023TierPercentageGreater(d, c023ResolutionTier1, after, before, points, target)
	case 2:
		return c023TierPercentageGreater(d, c023ResolutionTier2, after, before, points, target)
	default:
		panic("unsupported tier")
	}
}

func c023RawNumberTimes(
	d fixture.Dimension,
	after, before int64,
	points int,
	target float64,
	gaps bool,
) []expectedColumnPoint {
	step := (before - after) / int64(points)
	count := make([]float64, points)
	contributed := make([]bool, points)
	for _, p := range d.Points {
		bucket := c023BucketIndex(p.T, after, step, points)
		if bucket < 0 {
			continue
		}
		value, collected := c023PointValue(p)
		if gaps {
			contributed[bucket] = true
			if !collected {
				count[bucket]++
			}
		} else if collected {
			contributed[bucket] = true
			if value == target {
				count[bucket]++
			}
		}
	}

	out := make([]expectedColumnPoint, points)
	for i := range out {
		end := after + int64(i+1)*step
		if contributed[i] {
			out[i] = wantNumberAt(end, count[i])
		} else {
			out[i] = wantEmptyAt(end)
		}
	}
	return out
}

// Class B once-per-stored-record occurrence delivery.
//
// Source: netdata/netdata @ c8f9ce4d5622767ea752a2877bf1049a0bc85a46
// src/web/api/queries/number_of_times/number_of_times.h:46-70,79-98
// tg_number_of_times_add_point(), tg_number_of_times_flush()
// src/web/api/queries/tg-expression.h:366-436,445-541
// tg_expression_window_fraction(), tg_expression_share()
// src/web/api/queries/query-execute.c:78-190
// query_add_point_to_group()
func c023TierNumberTimes(
	d fixture.Dimension,
	granularity, after, before int64,
	points int,
	target float64,
	gaps bool,
) []expectedColumnPoint {
	step := (before - after) / int64(points)
	records := c023WindowRecords(d, granularity)
	delivered := make(map[int64]bool, len(records))
	out := make([]expectedColumnPoint, points)
	for i := range out {
		from := after + int64(i)*step
		to := from + step
		count := 0.0
		contributed := false
		for _, w := range records {
			if !c023Overlaps(w, granularity, from, to) {
				continue
			}
			if w.Empty {
				if gaps {
					// Gap deliveries represent stored slots. The matrix asks
					// this only at rows at least as wide as a record.
					contributed = true
					count += float64(c023Overlap(w, granularity, from, to)) / float64(granularity)
				}
				continue
			}
			contributed = true
			if delivered[w.EndT] {
				continue
			}
			delivered[w.EndT] = true
			if !gaps && c023WindowFractionEqual(w, target) > 0 {
				count++
			}
		}
		if contributed {
			out[i] = wantNumberAt(to, count)
		} else {
			out[i] = wantEmptyAt(to)
		}
	}
	return out
}

func c023NumberTimes(
	d fixture.Dimension,
	tier int,
	after, before int64,
	points int,
	target float64,
	gaps bool,
) []expectedColumnPoint {
	switch tier {
	case 0:
		return c023RawNumberTimes(d, after, before, points, target, gaps)
	case 1:
		return c023TierNumberTimes(d, c023ResolutionTier1, after, before, points, target, gaps)
	case 2:
		return c023TierNumberTimes(d, c023ResolutionTier2, after, before, points, target, gaps)
	default:
		panic("unsupported tier")
	}
}

func TestC023TierNumberTimesPartialGapOverlap(t *testing.T) {
	ch := c023ResolutionFixture()
	d := c023ResolutionDimension(ch, "availability")
	base := c023ResolutionBase()
	after := base + c023ResolutionTier2 + c023ResolutionTier2/2
	before := after + c023ResolutionTier2

	got := c023TierNumberTimes(d, c023ResolutionTier2, after, before, 1, 0, true)
	if len(got) != 1 || got[0].Value == nil || *got[0].Value != 0.5 {
		t.Fatalf("half-overlapped empty tier record = %v, want exactly 0.5", got)
	}
}

func c023RawFlaps(
	d fixture.Dimension,
	after, before int64,
	points int,
	target float64,
) []expectedColumnPoint {
	step := (before - after) / int64(points)
	flaps := make([]float64, points)
	contributed := make([]bool, points)
	state, hasState := false, false
	for _, p := range d.Points {
		bucket := c023BucketIndex(p.T, after, step, points)
		if bucket < 0 {
			continue
		}
		value, collected := c023PointValue(p)
		if !collected {
			continue
		}
		contributed[bucket] = true
		now := value == target
		if hasState && !state && now {
			flaps[bucket]++
		}
		state, hasState = now, true
	}

	out := make([]expectedColumnPoint, points)
	for i := range out {
		end := after + int64(i+1)*step
		if contributed[i] {
			out[i] = wantNumberAt(end, flaps[i])
		} else {
			out[i] = wantEmptyAt(end)
		}
	}
	return out
}

// Class B once-per-stored-record flap delivery.
//
// Source: netdata/netdata @ c8f9ce4d5622767ea752a2877bf1049a0bc85a46
// src/web/api/queries/number_of_flaps/number_of_flaps.h:53-86,95-114
// tg_number_of_flaps_add_point(), tg_number_of_flaps_flush()
// src/web/api/queries/tg-expression.h:366-436,445-541
// tg_expression_window_fraction(), tg_expression_share()
// src/web/api/queries/query-execute.c:78-190
// query_add_point_to_group()
func c023TierFlaps(
	d fixture.Dimension,
	granularity, after, before int64,
	points int,
	target float64,
) []expectedColumnPoint {
	step := (before - after) / int64(points)
	records := c023WindowRecords(d, granularity)
	delivered := make(map[int64]bool, len(records))
	state, hasState := false, false
	out := make([]expectedColumnPoint, points)
	for i := range out {
		from := after + int64(i)*step
		to := from + step
		flaps := 0.0
		contributed := false
		for _, w := range records {
			if w.Empty || !c023Overlaps(w, granularity, from, to) {
				continue
			}
			contributed = true
			if delivered[w.EndT] {
				continue
			}
			delivered[w.EndT] = true
			share := c023WindowFractionEqual(w, target)
			if share > 0 && share < 1 {
				flaps++
				state = true
			} else {
				now := share > 0
				if hasState && !state && now {
					flaps++
				}
				state = now
			}
			hasState = true
		}
		if contributed {
			out[i] = wantNumberAt(to, flaps)
		} else {
			out[i] = wantEmptyAt(to)
		}
	}
	return out
}

func c023Flaps(
	d fixture.Dimension,
	tier int,
	after, before int64,
	points int,
	target float64,
) []expectedColumnPoint {
	switch tier {
	case 0:
		return c023RawFlaps(d, after, before, points, target)
	case 1:
		return c023TierFlaps(d, c023ResolutionTier1, after, before, points, target)
	case 2:
		return c023TierFlaps(d, c023ResolutionTier2, after, before, points, target)
	default:
		panic("unsupported tier")
	}
}

func c023RawPreviousDrops(
	d fixture.Dimension,
	after, before int64,
	points int,
) []expectedColumnPoint {
	step := (before - after) / int64(points)
	drops := make([]float64, points)
	contributed := make([]bool, points)
	previous, hasPrevious := 0.0, false
	for _, p := range d.Points {
		bucket := c023BucketIndex(p.T, after, step, points)
		if bucket < 0 {
			continue
		}
		value, collected := c023PointValue(p)
		if !collected {
			continue
		}
		contributed[bucket] = true
		if hasPrevious && value < previous {
			drops[bucket]++
		}
		previous, hasPrevious = value, true
	}

	out := make([]expectedColumnPoint, points)
	for i := range out {
		end := after + int64(i+1)*step
		if contributed[i] {
			out[i] = wantNumberAt(end, drops[i])
		} else {
			out[i] = wantEmptyAt(end)
		}
	}
	return out
}

func c023TierPreviousDrops(
	d fixture.Dimension,
	granularity, after, before int64,
	points int,
) []expectedColumnPoint {
	// The higher-tier <previous inference, post-drop floor, and once-per-record
	// occurrence delivery are Class B.
	//
	// Source: netdata/netdata @ c8f9ce4d5622767ea752a2877bf1049a0bc85a46
	// src/web/api/queries/tg-expression.h:445-541
	// tg_expression_share()
	// src/web/api/queries/number_of_times/number_of_times.h:46-70,79-98
	// tg_number_of_times_add_point(), tg_number_of_times_flush()
	step := (before - after) / int64(points)
	records := c023WindowRecords(d, granularity)
	delivered := make(map[int64]bool, len(records))
	previousMax, hasPrevious := 0.0, false
	out := make([]expectedColumnPoint, points)
	for i := range out {
		from := after + int64(i)*step
		to := from + step
		drops := 0.0
		contributed := false
		for _, w := range records {
			if w.Empty || !c023Overlaps(w, granularity, from, to) {
				continue
			}
			contributed = true
			if delivered[w.EndT] {
				continue
			}
			delivered[w.EndT] = true
			dropped := hasPrevious && w.Min < previousMax
			if dropped {
				drops++
				previousMax = w.Min
			} else {
				previousMax = w.Max
			}
			hasPrevious = true
		}
		if contributed {
			out[i] = wantNumberAt(to, drops)
		} else {
			out[i] = wantEmptyAt(to)
		}
	}
	return out
}

func c023PreviousDrops(
	d fixture.Dimension,
	tier int,
	after, before int64,
	points int,
) []expectedColumnPoint {
	switch tier {
	case 0:
		return c023RawPreviousDrops(d, after, before, points)
	case 1:
		return c023TierPreviousDrops(d, c023ResolutionTier1, after, before, points)
	case 2:
		return c023TierPreviousDrops(d, c023ResolutionTier2, after, before, points)
	default:
		panic("unsupported tier")
	}
}

type c023ResolutionQuerySpec struct {
	context   string
	tier      int
	after     int64
	before    int64
	points    int
	group     string
	options   string
	dimension string
}

func c023ResolutionQuery(
	t *testing.T,
	spec c023ResolutionQuerySpec,
) (map[string]any, map[string][]canon.Pt) {
	t.Helper()
	params := daemon.DataParamsTier(
		spec.context, spec.tier, spec.after, spec.before, int64(spec.points), spec.group)
	params.Set("options", "jsonwrap|unaligned")
	params.Set("time_group_options", spec.options)
	params.Set("scope_dimensions", spec.dimension)
	doc, err := td.DataV3("c023-resolution", params)
	if err != nil {
		t.Fatal(err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}
	return doc, cols
}

// TestCase023TierResolutionMatrix keeps this expensive matrix independent of
// the compact tier-estimation fixture, so either contract can fail or abort
// without silently preventing the other one from running.
func TestCase023TierResolutionMatrix(t *testing.T) {
	testCase023TierResolutionMatrix(t)
}

// Four tier-2 rows exercise tier0/tier1 downsampling and tier2 identity;
// 240 rows exercise tier1 identity and tier2 upsampling; 720 rows re-deliver
// every tier1 record three times.
func testCase023TierResolutionMatrix(t *testing.T) {
	t.Helper()
	const sourceContract = "CASE-023/tier-resolution-source"
	contracts := map[string]bool{
		sourceContract: true,
		"CASE-023/tier-resolution-percentage-of-time":     true,
		"CASE-023/tier-resolution-percentage-of-samples":  true,
		"CASE-023/tier-resolution-number-of-times":        true,
		"CASE-023/tier-resolution-number-of-flaps":        true,
		"CASE-023/tier-resolution-slow-metric-upsampling": true,
	}
	for contract := range contracts {
		registerContract(t, contract)
	}

	ch := c023ResolutionFixture()
	pushLiveBurst(t, "c023-resolution", guid(321), ch)
	settleAndVerify(t, "c023-resolution", ch)

	after := c023ResolutionBase()
	before := after + c023ResolutionWindows*c023ResolutionTier2
	availability := c023ResolutionDimension(ch, "availability")
	counter := c023ResolutionDimension(ch, "counter")
	nonbinary := c023ResolutionDimension(ch, "nonbinary")

	run := func(contract string, tier, points int, group, options, dimension string, want []expectedColumnPoint) {
		t.Helper()
		doc, cols := c023ResolutionQuery(t, c023ResolutionQuerySpec{
			context: ch.Context, tier: tier, after: after, before: before, points: points,
			group: group, options: options, dimension: dimension,
		})
		label := "tier " + strconv.Itoa(tier) + ", points " + strconv.Itoa(points) +
			", " + group + "(" + options + "), " + dimension
		if !assertSelectedTier(t, doc, tier) {
			t.Logf("%s: selected-tier proof failed", label)
			contracts[sourceContract] = false
		}
		if !assertOnlyColumn(t, cols, dimension) {
			t.Logf("%s: result contains the wrong columns", label)
			contracts[sourceContract] = false
		}

		tolerance := 0.0
		for _, p := range want {
			if p.Value != nil && math.Trunc(*p.Value) != *p.Value {
				// json2 prints seven fractional digits.
				tolerance = printTol
				break
			}
		}
		if !assertExactColumn(t, cols, dimension, want, tolerance) {
			t.Logf("%s: exact fixture oracle failed", label)
			contracts[contract] = false
		}
	}

	for _, tier := range []int{0, 1, 2} {
		// Four tier-2 buckets: tier0 and tier1 downsample; tier2 is identity.
		const points = 4
		run("CASE-023/tier-resolution-percentage-of-time", tier, points, "percentage-of-time", "==1", "availability",
			c023PercentageEqual(availability, tier, after, before, points, 1))
		run("CASE-023/tier-resolution-number-of-times", tier, points, "number-of-times", "==gap", "availability",
			c023NumberTimes(availability, tier, after, before, points, 0, true))
		run("CASE-023/tier-resolution-number-of-times", tier, points, "number-of-times", "==1", "availability",
			c023NumberTimes(availability, tier, after, before, points, 1, false))
		run("CASE-023/tier-resolution-number-of-flaps", tier, points, "number-of-flaps", "==1", "availability",
			c023Flaps(availability, tier, after, before, points, 1))
		run("CASE-023/tier-resolution-number-of-times", tier, points, "number-of-times", "<previous", "counter",
			c023PreviousDrops(counter, tier, after, before, points))
		run("CASE-023/tier-resolution-percentage-of-time", tier, points, "percentage-of-time", "==5", "nonbinary",
			c023PercentageEqual(nonbinary, tier, after, before, points, 5))
		run("CASE-023/tier-resolution-percentage-of-samples", tier, points, "percentage-of-samples", ">5", "nonbinary",
			c023PercentageGreater(nonbinary, tier, after, before, points, 5))

		for _, points := range []int{240, 720} {
			// 240: tier0 downsamples, tier1 is identity, tier2 upsamples.
			// 720: every tier1 point is re-delivered three times.
			run("CASE-023/tier-resolution-percentage-of-time", tier, points, "percentage-of-time", "==1", "availability",
				c023PercentageEqual(availability, tier, after, before, points, 1))
			run("CASE-023/tier-resolution-number-of-flaps", tier, points, "number-of-flaps", "==1", "availability",
				c023Flaps(availability, tier, after, before, points, 1))
			run("CASE-023/tier-resolution-number-of-times", tier, points, "number-of-times", "<previous", "counter",
				c023PreviousDrops(counter, tier, after, before, points))
			run("CASE-023/tier-resolution-percentage-of-time", tier, points, "percentage-of-time", "==5", "nonbinary",
				c023PercentageEqual(nonbinary, tier, after, before, points, 5))
			run("CASE-023/tier-resolution-percentage-of-samples", tier, points, "percentage-of-samples", ">5", "nonbinary",
				c023PercentageGreater(nonbinary, tier, after, before, points, 5))
		}
	}

	// A short forced-tier0 window makes the 10-second source records feed
	// 5-second result rows. This is the slow-metric upsampling shape that the
	// long matrix above cannot reach: every covered row must stay numeric,
	// while event groupings must not invent transitions or counter drops.
	upsampleAfter := after
	upsampleBefore := after + 120
	const upsamplePoints = 24
	upsampleWant := func(value float64) []expectedColumnPoint {
		want := make([]expectedColumnPoint, upsamplePoints)
		for i := range want {
			want[i] = wantNumberAt(upsampleAfter+int64(i+1)*5, value)
		}
		return want
	}
	for _, tc := range []struct {
		group, options, dimension string
		value                     float64
	}{
		{"percentage-of-samples", "==1", "availability", 100},
		{"percentage-of-time", "==1", "availability", 100},
		{"number-of-flaps", "==1", "availability", 0},
		{"number-of-times", "<previous", "counter", 0},
	} {
		const contract = "CASE-023/tier-resolution-slow-metric-upsampling"
		doc, cols := c023ResolutionQuery(t, c023ResolutionQuerySpec{
			context: ch.Context, tier: 0, after: upsampleAfter, before: upsampleBefore,
			points: upsamplePoints,
			group:  tc.group, options: tc.options, dimension: tc.dimension,
		})
		label := "tier 0 upsampling, " + tc.group + "(" + tc.options + "), " + tc.dimension
		if !assertSelectedTier(t, doc, 0) {
			t.Logf("%s: selected-tier proof failed", label)
			contracts[contract] = false
		}
		if !assertExactView(t, doc, upsampleAfter, upsampleBefore, 5) {
			t.Logf("%s: view grid is wrong", label)
			contracts[contract] = false
		}
		if !assertOnlyColumn(t, cols, tc.dimension) {
			t.Logf("%s: result contains the wrong columns", label)
			contracts[contract] = false
		}
		if !assertExactColumn(t, cols, tc.dimension, upsampleWant(tc.value), 0) {
			t.Logf("%s: exact covered-row oracle failed", label)
			contracts[contract] = false
		}
	}

	for _, contract := range []string{
		sourceContract,
		"CASE-023/tier-resolution-percentage-of-time",
		"CASE-023/tier-resolution-percentage-of-samples",
		"CASE-023/tier-resolution-number-of-times",
		"CASE-023/tier-resolution-number-of-flaps",
		"CASE-023/tier-resolution-slow-metric-upsampling",
	} {
		assertContract(t, contract, contracts[contract])
	}
}
