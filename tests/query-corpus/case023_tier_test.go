// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-023 tier layer — RED: what the fleet groupings must answer ABOVE
// tier 0, where a stored point is no longer a sample but min/max/avg over
// many.
//
// This is the half of the contract the tier-0 cases cannot reach, and it is
// the half the feature exists for: tier-0 retention on a busy parent is
// minutes, so every SLO query over a day or a week reads tier 1+.
//
// Evaluating the condition on the stored average is not usable here. A
// minute that was up 30s and down 30s records avg=0.5, so `>=1` and `==0`
// would BOTH answer "never". The contract is the two-point mass model:
// treat the window as if the value took only min and max, weighted so
// their mean is the recorded average
//
//	weight(max) = (avg - min) / (max - min)
//
// which is EXACT for a 0/1 dimension at a steady collection cadence —
// the shape of the fixture below — because there the sample-weighted
// average is also the fraction of elapsed time at 1. A cadence change
// inside a rollup is pinned separately by
// CASE-023/cadence-change-availability-higher-tiers.
//
// Expectations below are derived from that definition and from the
// fixture, never from the engine.
package corpus

import (
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

func TestCase023TierEstimation(t *testing.T) {
	// A 0/1 availability signal changes its down-time pattern every 60
	// fixture samples. Storage windows sit on the absolute 60-second tier
	// grid, so the first window is partial and later windows can cross two
	// generator phases. The oracle below reads fixture.TierWindows() rather
	// than assuming fixture indices and tier windows are aligned.
	down := func(k int) int {
		switch k % 4 {
		case 0:
			return 0 // wholly up  -> min == max == 1, exact
		case 1:
			return 60 // wholly down -> min == max == 0, exact
		case 2:
			return 30 // half and half
		default:
			return 15 // a quarter down
		}
	}

	const samples = 2400 // 40 tier-1 windows at 60s granularity
	ch := fixture.Series("fixture.c023tier", "fixture.c023tier", fixture.T0, samples, 1,
		func(i int) string {
			if i%60 < down(i/60) {
				return "0"
			}
			return "1"
		}, func(int) string { return stream.FlagNotAnomalous })
	ch.ValueTolerance = 1e-9

	pushLiveBurst(t, "c023tier", guid(211), ch)
	if _, err := td.WaitRetention("c023tier", ch.Context, ch.FirstT(), ch.LastT(), 20*time.Second); err != nil {
		t.Fatal(err)
	}

	// one view bucket per tier-1 window, so each answer is one window's
	// estimate and nothing is mixed
	const (
		after      = fixture.T0 - 20
		bucketSpan = tier1Gran
		buckets    = 12
	)

	d := ch.Dimensions[0]
	windows := d.TierWindows(tier1Gran, int64(ch.UpdateEvery))

	// the contract, stated as the model rather than as the code
	fractionAtLeast := func(w fixture.TierPoint, threshold float64) float64 {
		avg := w.Sum / float64(w.Count)
		if w.Max <= w.Min {
			// a constant window is exact
			if avg >= threshold {
				return 1
			}
			return 0
		}
		if threshold <= w.Min {
			return 1
		}
		if threshold > w.Max {
			return 0
		}
		return (avg - w.Min) / (w.Max - w.Min)
	}
	fractionEqual := func(w fixture.TierPoint, target float64) float64 {
		avg := w.Sum / float64(w.Count)
		if w.Max <= w.Min {
			if avg == target {
				return 1
			}
			return 0
		}
		wt := (avg - w.Min) / (w.Max - w.Min)
		switch target {
		case w.Min:
			return 1 - wt
		case w.Max:
			return wt
		}
		return 0
	}

	query := func(t *testing.T, group, options string) (map[string]any, map[string][]canon.Pt) {
		t.Helper()
		params := daemon.DataParamsTier(ch.Context, 1, after, after+buckets*bucketSpan, buckets, group)
		if options != "" {
			params.Set("time_group_options", options)
		}
		doc, err := td.DataV3("c023tier", params)
		if err != nil {
			t.Fatal(err)
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}
		return doc, cols
	}

	near := func(got, want float64) bool { return math.Abs(got-want) < 1e-6 }
	expected := func(t *testing.T, value func(fixture.TierPoint) float64) []expectedColumnPoint {
		t.Helper()
		out := make([]expectedColumnPoint, buckets)
		for i := range out {
			end := int64(after + int64(i+1)*bucketSpan)
			w, has := windows[end]
			if !has || w.Count == 0 {
				t.Fatalf("fixture has no populated tier-1 window ending at %d", end)
			}
			out[i] = wantNumberAt(end, value(w))
		}
		return out
	}

	t.Run("source", func(t *testing.T) {
		trackContract(t, "CASE-023/tier-estimation-source")
		for _, tc := range []struct{ group, options string }{
			{"percentage-of-time", ">=1"},
			{"number-of-flaps", "==0"},
			{"number-of-times", "==0"},
			{"percentage-of-samples", "==0"},
		} {
			doc, cols := query(t, tc.group, tc.options)
			if !assertSelectedTier(t, doc, 1) || !assertOnlyColumn(t, cols, d.ID) {
				t.Fail()
			}
		}
	})

	t.Run("percentage-of-time", func(t *testing.T) {
		trackContract(t, "CASE-023/tier-estimation-percentage-of-time")

		// The share of each window that satisfied the condition is weighted
		// by the model above.
		for _, tc := range []struct {
			options string
			want    func(fixture.TierPoint) float64
		}{
			{">=1", func(w fixture.TierPoint) float64 { return fractionAtLeast(w, 1) * 100 }},
			{"==0", func(w fixture.TierPoint) float64 { return fractionEqual(w, 0) * 100 }},
			{"==1", func(w fixture.TierPoint) float64 { return fractionEqual(w, 1) * 100 }},
		} {
			_, cols := query(t, "percentage-of-time", tc.options)
			// The oracle is exact; printTol only accounts for json2's seven
			// fractional digits.
			if !assertOnlyColumn(t, cols, d.ID) || !assertExactColumn(t, cols, d.ID, expected(t, tc.want), printTol) {
				t.Errorf("percentage-of-time %s did not return the exact fixture-derived tier windows", tc.options)
			}
		}
	})

	// number-of-flaps: a window that is neither wholly true nor wholly
	// false changed at least once and counts as one; a constant window
	// only counts when it turns the state on
	t.Run("number-of-flaps", func(t *testing.T) {
		trackContract(t, "CASE-023/tier-estimation-number-of-flaps")
		_, cols := query(t, "number-of-flaps", "==0")
		state, hasState := false, false
		want := expected(t, func(w fixture.TierPoint) float64 {
			share := fractionEqual(w, 0)
			flaps := 0.0
			if share > 0 && share < 1 {
				flaps = 1
				state = true
			} else {
				now := share > 0
				if hasState && !state && now {
					flaps = 1
				}
				state = now
			}
			hasState = true
			return flaps
		})
		if !assertExactColumn(t, cols, d.ID, want, 0) {
			t.Error("number-of-flaps ==0 did not return one exact verdict per tier window")
		}
	})

	// number-of-times: occurrences inside one stored window collapse to
	// one, because the window keeps no ordering
	t.Run("number-of-times", func(t *testing.T) {
		trackContract(t, "CASE-023/tier-estimation-number-of-times")
		_, cols := query(t, "number-of-times", "==0")
		want := expected(t, func(w fixture.TierPoint) float64 {
			if fractionEqual(w, 0) > 0 {
				return 1
			}
			return 0
		})
		if !assertExactColumn(t, cols, d.ID, want, 0) {
			t.Error("number-of-times ==0 did not apply count-as-one exactly once per tier window")
		}
	})

	// percentage-of-samples keeps its historical tier behaviour (D6): the
	// stored point IS the sample, so the condition is evaluated on it
	t.Run("percentage-of-samples", func(t *testing.T) {
		trackContract(t, "CASE-023/tier-estimation-percentage-of-samples")
		_, cols := query(t, "percentage-of-samples", "==0")
		want := expected(t, func(w fixture.TierPoint) float64 {
			avg := w.Sum / float64(w.Count)
			if near(avg, 0) {
				return 100
			}
			return 0
		})
		if !assertExactColumn(t, cols, d.ID, want, 0) {
			t.Error("percentage-of-samples ==0 did not evaluate every delivered tier average")
		}
	})
}

// tierWindowEnd is the stored window a view bucket ending at t belongs to:
// the same rounding TierWindows() keys on. Tier windows sit on absolute
// multiples of the granularity, so a finer view grid puts several buckets
// inside one stored window.
func tierWindowEnd(t, granularity int64) int64 {
	if rem := t % granularity; rem != 0 {
		return t + granularity - rem
	}
	return t
}

// CASE-023 wide-point re-delivery — RED: what happens when the view grid is
// FINER than the stored data, which is the normal case for a dashboard
// zoomed into a window that only tier 1 still covers.
//
// A stored point wider than the view bucket is delivered to every bucket it
// covers, carrying its ORIGINAL start and an INTERPOLATED value, and only
// the first delivery is a new observation. So across the buckets of one
// stored window:
//
//   - the share of time answers the SAME estimate in each of them (the
//     model has no sub-window detail to distinguish them with);
//   - one window can produce at most ONE occurrence and ONE flap, because
//     a re-delivery is the same window seen again, not a second event.
//
// Counting the repeats would inflate an SLO by exactly the zoom factor,
// which is the failure this pins.
func TestCase023TierWidePointRedelivery(t *testing.T) {
	// the same 0/1 availability shape as the estimation case: every stored
	// window has min=0, max=1, and an average that IS the fraction of time up
	down := func(k int) int {
		switch k % 4 {
		case 0:
			return 0 // wholly up
		case 1:
			return 60 // wholly down
		case 2:
			return 30
		default:
			return 15
		}
	}

	const samples = 2400
	ch := fixture.Series("fixture.c023wide", "fixture.c023wide", fixture.T0, samples, 1,
		func(i int) string {
			if i%60 < down(i/60) {
				return "0"
			}
			return "1"
		}, func(int) string { return stream.FlagNotAnomalous })
	ch.ValueTolerance = 1e-9

	pushLiveBurst(t, "c023wide", guid(212), ch)
	if _, err := td.WaitRetention("c023wide", ch.Context, ch.FirstT(), ch.LastT(), 20*time.Second); err != nil {
		t.Fatal(err)
	}

	// the window is a whole number of stored windows, cut into three view
	// buckets each, so every stored point is re-delivered twice
	const (
		perWindow  = 3
		bucketSpan = tier1Gran / perWindow
		windows    = 8
	)
	after := int64(fixture.T0 + 40) // the first absolute multiple of tier1Gran
	before := after + windows*tier1Gran

	d := ch.Dimensions[0]
	stored := d.TierWindows(tier1Gran, int64(ch.UpdateEvery))

	query := func(t *testing.T, group, options string) (map[string]any, map[string][]canon.Pt) {
		t.Helper()
		params := daemon.DataParamsTier(ch.Context, 1, after, before, windows*perWindow, group)
		params.Set("time_group_options", options)
		doc, err := td.DataV3("c023wide", params)
		if err != nil {
			t.Fatal(err)
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}
		return doc, cols
	}

	t.Run("source", func(t *testing.T) {
		trackContract(t, "CASE-023/tier-wide-point-source")
		for _, tc := range []struct{ group, options string }{
			{"percentage-of-time", ">=1"},
			{"number-of-times", "==0"},
			{"number-of-flaps", "==0"},
		} {
			doc, cols := query(t, tc.group, tc.options)
			if !assertSelectedTier(t, doc, 1) || !assertOnlyColumn(t, cols, d.ID) {
				t.Fail()
			}
		}
	})

	// The wide-record duration share and the once-per-record event state below
	// are Class B ports.
	//
	// Source: netdata/netdata @ c8f9ce4d5622767ea752a2877bf1049a0bc85a46
	// src/web/api/queries/tg-expression.h:366-436,445-541
	// tg_expression_window_fraction(), tg_expression_share()
	// src/web/api/queries/percentage_of_time/percentage_of_time.h:57-75,86-105
	// tg_percentage_of_time_add_point(), tg_percentage_of_time_flush()
	// src/web/api/queries/number_of_times/number_of_times.h:46-70,79-98
	// tg_number_of_times_add_point(), tg_number_of_times_flush()
	// src/web/api/queries/number_of_flaps/number_of_flaps.h:53-86,95-114
	// tg_number_of_flaps_add_point(), tg_number_of_flaps_flush()
	// src/web/api/queries/query-execute.c:78-190,522-570
	// query_add_point_to_group(), inner point re-delivery loop
	wantTime := make([]expectedColumnPoint, windows*perWindow)
	for i := range wantTime {
		end := after + int64(i+1)*bucketSpan
		storedEnd := tierWindowEnd(end, tier1Gran)
		w, has := stored[storedEnd]
		if !has || w.Count == 0 {
			t.Fatalf("fixture window ending %d is not populated: %+v", storedEnd, w)
		}
		shareAtOne := 0.0
		avg := w.Sum / float64(w.Count)
		switch {
		case w.Max <= w.Min && avg >= 1:
			shareAtOne = 1
		case w.Max > w.Min && 1 <= w.Min:
			shareAtOne = 1
		case w.Max > w.Min && 1 <= w.Max:
			shareAtOne = (avg - w.Min) / (w.Max - w.Min)
		}
		wantTime[i] = wantNumberAt(end, shareAtOne*100)
	}
	t.Run("time-share", func(t *testing.T) {
		trackContract(t, "CASE-023/tier-wide-point-time-share")
		_, cols := query(t, "percentage-of-time", ">=1")
		if !assertOnlyColumn(t, cols, d.ID) || !assertExactColumn(t, cols, d.ID, wantTime, printTol) {
			t.Error("percentage-of-time changed or dropped a tier-window estimate during re-delivery")
		}
	})

	// an occurrence belongs to the window, not to the buckets it was
	// delivered into
	for _, tc := range []struct {
		name, contract, group, options string
	}{
		{"number-of-times", "CASE-023/tier-wide-point-number-of-times", "number-of-times", "==0"},
		{"number-of-flaps", "CASE-023/tier-wide-point-number-of-flaps", "number-of-flaps", "==0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			trackContract(t, tc.contract)
			want := make([]expectedColumnPoint, windows*perWindow)
			state, hasState := false, false
			for i := range want {
				end := after + int64(i+1)*bucketSpan
				value := 0.0
				if i%perWindow == 0 {
					storedEnd := tierWindowEnd(end, tier1Gran)
					w, has := stored[storedEnd]
					if !has || w.Count == 0 {
						t.Fatalf("fixture has no populated tier-1 window ending at %d", storedEnd)
					}
					share := c023WindowFractionEqual(w, 0)
					switch tc.group {
					case "number-of-times":
						if share > 0 {
							value = 1
						}
					case "number-of-flaps":
						if share > 0 && share < 1 {
							value = 1
							state = true
						} else {
							now := share > 0
							if hasState && !state && now {
								value = 1
							}
							state = now
						}
						hasState = true
					}
				}
				want[i] = wantNumberAt(end, value)
			}
			_, cols := query(t, tc.group, tc.options)
			if !assertOnlyColumn(t, cols, d.ID) || !assertExactColumn(t, cols, d.ID, want, 0) {
				t.Errorf("%s %s did not count each stored window exactly once across its three deliveries",
					tc.group, tc.options)
			}
		})
	}
}

// CASE-023 anomaly bit above tier 0 — RED: with options=anomaly-bit the
// value a grouping receives is the anomaly RATE of the stored window, while
// min/max still describe the metric. Estimating across them would mix two
// unrelated domains, so the condition is answered on the rate itself: a
// window either satisfied it or it did not.
func TestCase023TierAnomalyBit(t *testing.T) {
	trackContract(t, "CASE-023/tier-anomaly-bit")

	// values sweep a wide metric range, so a leak of the metric extrema
	// into the estimate shows up as a fractional answer
	const samples = 600
	ch := fixture.Series("fixture.c023anom", "fixture.c023anom", fixture.T0, samples, 1,
		func(i int) string { return strconv.Itoa(i) },
		func(i int) string {
			// the anomalous share rises window by window, so the fixture
			// straddles the threshold below
			if i%60 < (i/60)*6 {
				return stream.FlagAnomalous
			}
			return stream.FlagNotAnomalous
		})
	ch.ValueTolerance = 1e-9

	pushReplication(t, "c023anom", guid(213), ch)
	if _, err := td.WaitRetention("c023anom", ch.Context, ch.FirstT(), ch.LastT(), 20*time.Second); err != nil {
		t.Fatal(err)
	}

	ok := true
	check := func(cond bool, what string, args ...any) {
		t.Helper()
		if !cond {
			t.Logf("anomaly-bit tier contract not met: "+what, args...)
			ok = false
		}
	}

	const (
		firstEnd = fixture.T0 + 40
		lastEnd  = fixture.T0 + 520
	)
	after := int64(firstEnd - tier1Gran)
	points := (lastEnd - after) / tier1Gran

	d := ch.Dimensions[0]
	stored := d.TierWindows(tier1Gran, int64(ch.UpdateEvery))

	query := func(options string) map[string][]canon.Pt {
		t.Helper()
		params := daemon.DataParamsTier(ch.Context, 1, after, lastEnd, points, "percentage-of-time")
		params.Set("options", "jsonwrap|anomaly-bit")
		params.Set("time_group_options", options)
		doc, err := td.DataV3("c023anom", params)
		if err != nil {
			t.Fatal(err)
		}
		if !assertSelectedTier(t, doc, 1) {
			ok = false
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}
		return cols
	}

	atLeastCols := query(">=50")
	belowCols := query("<50")
	if !assertOnlyColumn(t, atLeastCols, d.ID) || !assertOnlyColumn(t, belowCols, d.ID) {
		ok = false
	}
	atLeast, below := atLeastCols[d.ID], belowCols[d.ID]

	atLeastWant := make([]expectedColumnPoint, 0, points)
	belowWant := make([]expectedColumnPoint, 0, points)
	for row := int64(1); row <= points; row++ {
		ts := after + row*tier1Gran
		window, has := stored[ts]
		if !has || window.Count == 0 {
			t.Fatalf("fixture has no stored window at %d", ts)
		}
		rate := 100 * float64(window.AnomalyCount) / float64(window.Count)
		atLeastValue := 0.0
		if rate >= 50 {
			atLeastValue = 100
		}
		atLeastWant = append(atLeastWant, wantNumberAt(ts, atLeastValue))
		belowWant = append(belowWant, wantNumberAt(ts, 100-atLeastValue))
	}
	if !assertExactColumn(t, atLeastCols, d.ID, atLeastWant, 1e-6) ||
		!assertExactColumn(t, belowCols, d.ID, belowWant, 1e-6) {
		ok = false
	}

	checked, above, under := 0, 0, 0
	for i, pt := range atLeast {
		w, has := stored[pt.T]
		if !has || w.Count == 0 {
			t.Fatalf("response timestamp %d has no fixture window", pt.T)
		}
		if pt.Value == nil {
			ok = false
			continue
		}
		checked++
		rate := 100 * float64(w.AnomalyCount) / float64(w.Count)
		if rate >= 50 {
			above++
		} else {
			under++
		}

		want := 0.0
		if rate >= 50 {
			want = 100
		}
		check(math.Abs(*pt.Value-want) < 1e-6,
			"window t=%d (anomaly rate %.4f, metric min=%v max=%v): >=50 answered %v, want %v",
			pt.T, rate, w.Min, w.Max, *pt.Value, want)

		// the pair has to partition the window: anything else means the
		// answer came from the metric extrema, not the rate
		if i < len(below) && below[i].Value != nil && below[i].T == pt.T {
			check(math.Abs(*pt.Value+*below[i].Value-100) < 1e-6,
				"window t=%d: >=50 and <50 answered %v + %v, want 100",
				pt.T, *pt.Value, *below[i].Value)
		}
	}

	// the fixture has to straddle the threshold, or "always 0" would pass
	if checked < int(points)-1 || above == 0 || under == 0 {
		t.Fatalf("fixture does not straddle the threshold: %d windows checked, %d at or above 50%%, %d below",
			checked, above, under)
	}

	assertContract(t, "CASE-023/tier-anomaly-bit", ok)
}
