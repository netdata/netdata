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
