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
// which is EXACT for a 0/1 dimension — the shape of every availability
// signal — because there the average IS the fraction of time at 1.
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
	// a 0/1 availability signal: `up` is 1 except for a run of zeros
	// inside each tier-1 window, so every window has min=0, max=1 and an
	// average that IS the fraction of time up.
	//
	// window k (k = 0..) covers samples [60k, 60k+59]. We make the first
	// `down(k)` samples of each window 0 and the rest 1, so
	//   avg(k)  = (60 - down(k)) / 60
	//   time at 0 = down(k)/60, time at 1 = 1 - down(k)/60
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

	ok := true
	check := func(cond bool, what string, args ...any) {
		t.Helper()
		if !cond {
			t.Logf("tier contract not met: "+what, args...)
			ok = false
		}
	}

	// one view bucket per tier-1 window, so each answer is one window's
	// estimate and nothing is mixed
	const (
		after      = fixture.T0 - 80
		bucketSpan = tier1Gran
		buckets    = 12
	)

	d := ch.Dimensions[0]
	windows := d.TierWindows(tier1Gran)

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

	query := func(group, options string) map[string][]canon.Pt {
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
		return cols
	}

	near := func(got, want float64) bool { return math.Abs(got-want) < 1e-6 }

	// percentage-of-time: the share of each window that satisfied the
	// condition, weighted by the model above
	for _, tc := range []struct {
		options string
		want    func(fixture.TierPoint) float64
	}{
		{">=1", func(w fixture.TierPoint) float64 { return fractionAtLeast(w, 1) * 100 }},
		{"==0", func(w fixture.TierPoint) float64 { return fractionEqual(w, 0) * 100 }},
		{"==1", func(w fixture.TierPoint) float64 { return fractionEqual(w, 1) * 100 }},
	} {
		cols := query("percentage-of-time", tc.options)
		col := cols[d.ID]
		for _, pt := range col {
			w, has := windows[pt.T]
			if !has || w.Count == 0 || pt.Value == nil {
				continue
			}
			want := tc.want(w)
			check(near(*pt.Value, want),
				"percentage-of-time %s at window t=%d (min=%v max=%v avg=%.4f): %v, want %.4f",
				tc.options, pt.T, w.Min, w.Max, w.Sum/float64(w.Count), *pt.Value, want)
		}
	}

	// the failure this layer exists to prevent: evaluating the condition on
	// the stored average makes a half-and-half window answer "never" in
	// BOTH directions
	{
		up := query("percentage-of-time", ">=1")[d.ID]
		dn := query("percentage-of-time", "==0")[d.ID]
		for i := range up {
			if i >= len(dn) || up[i].Value == nil || dn[i].Value == nil {
				continue
			}
			w, has := windows[up[i].T]
			if !has || w.Count == 0 || w.Max <= w.Min {
				continue
			}
			check(*up[i].Value > 0 || *dn[i].Value > 0,
				"window t=%d was neither up nor down (%v / %v) — the condition was evaluated on the average",
				up[i].T, *up[i].Value, *dn[i].Value)
			check(near(*up[i].Value+*dn[i].Value, 100),
				"window t=%d: up%%+down%% = %v, want 100", up[i].T, *up[i].Value+*dn[i].Value)
		}
	}

	// number-of-flaps: a window that is neither wholly true nor wholly
	// false changed at least once and counts as one; a constant window
	// only counts when it turns the state on
	{
		cols := query("number-of-flaps", "==0")
		for _, pt := range cols[d.ID] {
			w, has := windows[pt.T]
			if !has || w.Count == 0 || pt.Value == nil {
				continue
			}
			if w.Max > w.Min {
				check(*pt.Value >= 1,
					"number-of-flaps at mixed window t=%d = %v, want at least 1", pt.T, *pt.Value)
			}
		}
	}

	// number-of-times: occurrences inside one stored window collapse to
	// one, because the window keeps no ordering
	{
		cols := query("number-of-times", "==0")
		for _, pt := range cols[d.ID] {
			w, has := windows[pt.T]
			if !has || w.Count == 0 || pt.Value == nil {
				continue
			}
			check(*pt.Value <= 1,
				"number-of-times at window t=%d = %v, want at most 1 (count-as-1)", pt.T, *pt.Value)
			if w.Min == 0 && w.Max == 0 {
				check(*pt.Value == 1, "number-of-times at all-zero window t=%d = %v, want 1", pt.T, *pt.Value)
			}
		}
	}

	// percentage-of-samples keeps its historical tier behaviour (D6): the
	// stored point IS the sample, so the condition is evaluated on it
	{
		cols := query("percentage-of-samples", "==0")
		for _, pt := range cols[d.ID] {
			w, has := windows[pt.T]
			if !has || w.Count == 0 || pt.Value == nil {
				continue
			}
			avg := w.Sum / float64(w.Count)
			want := 0.0
			if avg == 0 {
				want = 100
			}
			check(near(*pt.Value, want),
				"percentage-of-samples ==0 at window t=%d (avg=%.4f) = %v, want %v (unchanged at tiers)",
				pt.T, avg, *pt.Value, want)
		}
	}

	expectAgentStatus(t, "CASE-023/tier-estimation", ok)
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

	ok := true
	check := func(cond bool, what string, args ...any) {
		t.Helper()
		if !cond {
			t.Logf("wide-point contract not met: "+what, args...)
			ok = false
		}
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
	stored := d.TierWindows(tier1Gran)

	query := func(group, options string) []canon.Pt {
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
		return cols[d.ID]
	}

	// group the answers by the stored window each bucket falls in
	byWindow := func(col []canon.Pt) map[int64][]float64 {
		out := make(map[int64][]float64)
		for _, pt := range col {
			if pt.Value == nil {
				continue
			}
			out[tierWindowEnd(pt.T, tier1Gran)] = append(out[tierWindowEnd(pt.T, tier1Gran)], *pt.Value)
		}
		return out
	}

	// the view grid has to actually be finer than the stored data, or this
	// case proves nothing
	checked := 0
	{
		col := query("percentage-of-time", ">=1")
		if len(col) < windows*perWindow {
			t.Fatalf("view grid is not finer than the stored data: %d buckets for %d stored windows",
				len(col), windows)
		}
		groups := byWindow(col)
		for end, vals := range groups {
			w, has := stored[end]
			if !has || w.Count == 0 {
				continue
			}
			checked++
			check(len(vals) == perWindow,
				"stored window t=%d was delivered to %d buckets, want %d", end, len(vals), perWindow)
			for i := 1; i < len(vals); i++ {
				check(math.Abs(vals[i]-vals[0]) < 1e-6,
					"stored window t=%d: bucket %d answered %v, bucket 0 answered %v — a re-delivery changed the estimate",
					end, i, vals[i], vals[0])
			}
		}
	}

	// an occurrence belongs to the window, not to the buckets it was
	// delivered into
	for _, tc := range []struct {
		group   string
		options string
	}{
		{"number-of-times", "==0"},
		{"number-of-flaps", "==0"},
	} {
		groups := byWindow(query(tc.group, tc.options))
		for end, vals := range groups {
			w, has := stored[end]
			if !has || w.Count == 0 {
				continue
			}
			total := 0.0
			for _, v := range vals {
				total += v
			}
			check(total <= 1,
				"%s %s: stored window t=%d totalled %v across %d buckets, want at most 1 — the repeats were counted",
				tc.group, tc.options, end, total, len(vals))
		}
	}

	// a case that silently checks nothing is worse than no case
	if checked < windows-1 {
		t.Fatalf("only %d of %d stored windows were reachable — the fixture and the query window disagree",
			checked, windows)
	}

	expectAgentStatus(t, "CASE-023/tier-wide-point", ok)
}

// CASE-023 anomaly bit above tier 0 — RED: with options=anomaly-bit the
// value a grouping receives is the anomaly RATE of the stored window, while
// min/max still describe the metric. Estimating across them would mix two
// unrelated domains, so the condition is answered on the rate itself: a
// window either satisfied it or it did not.
func TestCase023TierAnomalyBit(t *testing.T) {
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
	stored := d.TierWindows(tier1Gran)

	query := func(options string) []canon.Pt {
		t.Helper()
		params := daemon.DataParamsTier(ch.Context, 1, after, lastEnd, points, "percentage-of-time")
		params.Set("options", "jsonwrap|anomaly-bit")
		params.Set("time_group_options", options)
		doc, err := td.DataV3("c023anom", params)
		if err != nil {
			t.Fatal(err)
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}
		return cols[d.ID]
	}

	atLeast := query(">=50")
	below := query("<50")

	checked, above, under := 0, 0, 0
	for i, pt := range atLeast {
		w, has := stored[pt.T]
		if !has || w.Count == 0 || pt.Value == nil {
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

	expectAgentStatus(t, "CASE-023/tier-anomaly-bit", ok)
}
