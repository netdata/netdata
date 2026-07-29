// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-023 re-delivery and visibility — RED.
//
// Three contracts that all follow from ONE fact: the query engine hands the
// same stored point to every result bucket it spans, and the groupings
// disagree about what that means.
//
//   - `percentage-of-samples` treats a delivered point AS a sample and has
//     always counted every delivery. Skipping repeats leaves EMPTY holes in
//     buckets that used to carry a value.
//   - the counting groupings must NOT count a repeat: it is the same window
//     seen again, not a second event.
//   - `<previous` must not compare a window against itself, and one reset
//     must count once no matter how many windows follow it.
//
// Plus the visibility rule these groupings need: they answer a question
// ABOUT the samples, so `options=nonzero` has to judge them by the ANSWER.
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

// A view grid three times finer than the stored data. Every stored point is
// delivered to three buckets, so the two groupings that disagree about a
// repeat are both exercised on the same data.
func TestCase023RedeliveryAcrossGroupings(t *testing.T) {
	trackContract(t, "CASE-023/redelivery")

	const samples = 2400 // 40 tier-1 windows

	// a 0/1 signal that varies inside every window, so no window is
	// constant and every re-delivery carries an interpolated value
	ch := fixture.Series("fixture.c023redl", "fixture.c023redl", fixture.T0, samples, 1,
		func(i int) string {
			if i%60 < 20 {
				return "0"
			}
			return "1"
		}, func(int) string { return stream.FlagNotAnomalous })
	ch.ValueTolerance = 1e-9

	pushLiveBurst(t, "c023redl", guid(216), ch)
	if _, err := td.WaitRetention("c023redl", ch.Context, ch.FirstT(), ch.LastT(), 20*time.Second); err != nil {
		t.Fatal(err)
	}

	ok := true
	check := func(cond bool, what string, args ...any) {
		t.Helper()
		if !cond {
			t.Logf("re-delivery contract not met: "+what, args...)
			ok = false
		}
	}

	const (
		perWindow = 3
		windows   = 8
	)
	after := int64(fixture.T0 + 40) // the first absolute multiple of tier1Gran
	before := after + windows*tier1Gran

	query := func(group, options string) []canon.Pt {
		t.Helper()
		params := daemon.DataParamsTier(ch.Context, 1, after, before, windows*perWindow, group)
		params.Set("time_group_options", options)
		doc, err := td.DataV3("c023redl", params)
		if err != nil {
			t.Fatal(err)
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}
		return cols[ch.Dimensions[0].ID]
	}

	// percentage-of-samples answers in EVERY bucket: the engine has always
	// delivered one point per bucket to this grouping, and a bucket that
	// used to carry a value must not become EMPTY
	{
		col := query("percentage-of-samples", ">=1")
		if len(col) < windows*perWindow {
			t.Fatalf("view grid is not finer than the stored data: %d buckets", len(col))
		}
		empty := 0
		for _, pt := range col {
			if pt.Value == nil {
				empty++
			}
		}
		check(empty == 0,
			"percentage-of-samples left %d of %d buckets EMPTY — repeats were skipped",
			empty, len(col))
	}

	// the counting groupings answer at most once per stored window, however
	// many buckets that window was delivered into - and they answer in
	// every one of those buckets.
	//
	// "counted once" and "answered everywhere" are different contracts and
	// both have to hold. A bucket a wide point covers on its own carries no
	// occurrence, but it is not EMPTY either: nothing happened there, which
	// is a zero. Returning EMPTY instead punches holes into a chart wherever
	// the user zooms past the stored resolution.
	for _, group := range []string{"number-of-times", "number-of-flaps"} {
		col := query(group, "==0")
		perStored := map[int64]float64{}
		empty := 0
		for _, pt := range col {
			if pt.Value == nil {
				empty++
				continue
			}
			perStored[tierWindowEnd(pt.T, tier1Gran)] += *pt.Value
		}
		for end, total := range perStored {
			check(total <= 1,
				"%s totalled %v across the buckets of the stored window ending %d, want at most 1",
				group, total, end)
		}
		check(empty == 0,
			"%s left %d of %d buckets EMPTY — the buckets a wide stored point covers on its own "+
				"report 'nothing here' instead of 'nothing happened'",
			group, empty, len(col))
	}

	assertContract(t, "CASE-023/redelivery", ok)
}

// One counter reset, counted once — whatever the resolution asked for.
//
// Above tier 0 a reset is inferred from a window whose minimum falls below
// the previous window's maximum. Two things can turn one reboot into
// several: carrying the pre-reset PEAK forward, so the window after the
// reset looks like it dropped too; and re-delivering the reset window
// itself, so it is compared against the maximum its own first delivery
// stored.
func TestCase023ResetCountedOnceAcrossWindows(t *testing.T) {
	trackContract(t, "CASE-023/reset-counted-once")

	const (
		samples = 1800 // 30 tier-1 windows
		resetAt = 900  // one reset, in the middle
	)

	// a monotone counter that restarts exactly once
	ch := fixture.Series("fixture.c023reset", "fixture.c023reset", fixture.T0, samples, 1,
		func(i int) string {
			v := i
			if i >= resetAt {
				v = i - resetAt
			}
			return strconv.Itoa(v)
		}, func(int) string { return stream.FlagNotAnomalous })
	ch.ValueTolerance = 1e-9

	pushLiveBurst(t, "c023reset", guid(217), ch)
	if _, err := td.WaitRetention("c023reset", ch.Context, ch.FirstT(), ch.LastT(), 20*time.Second); err != nil {
		t.Fatal(err)
	}

	after := int64(fixture.T0 + 40)
	before := after + 20*tier1Gran

	// the fixture resets once inside this window
	total := func(buckets int64) float64 {
		t.Helper()
		params := daemon.DataParamsTier(ch.Context, 1, after, before, buckets, "number-of-times")
		params.Set("time_group_options", "<previous")
		params.Set("options", "jsonwrap|unaligned")
		doc, err := td.DataV3("c023reset", params)
		if err != nil {
			t.Fatal(err)
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}
		sum := 0.0
		for _, pt := range cols[ch.Dimensions[0].ID] {
			if pt.Value != nil {
				sum += *pt.Value
			}
		}
		return sum
	}

	ok := true

	// one bucket per stored window: the reset must be counted once, and the
	// window AFTER it must not be counted as a second one
	if got := total(20); math.Abs(got-1) >= 1e-6 {
		t.Logf("reset contract not met: one bucket per stored window counted %v reboots, want 1", got)
		ok = false
	}

	// three buckets per stored window: the same reset, re-delivered, still
	// counts once
	if got := total(60); math.Abs(got-1) >= 1e-6 {
		t.Logf("reset contract not met: a finer grid counted %v reboots, want 1", got)
		ok = false
	}

	assertContract(t, "CASE-023/reset-counted-once", ok)
}

// These groupings answer a question ABOUT the samples, so what makes a
// dimension worth showing is the ANSWER, not the values that went in. A
// dimension whose answer is zero everywhere has nothing to say and must be
// dropped by options=nonzero, even though every sample behind it is
// non-zero.
func TestCase023NonzeroFollowsTheAnswer(t *testing.T) {
	trackContract(t, "CASE-023/nonzero-follows-answer")

	// TWO dimensions, both collecting non-zero samples throughout. One
	// satisfies the condition below and one never does.
	//
	// Two dimensions are required, not decoration: when EVERY dimension
	// answers zero the engine drops the nonzero option altogether (the
	// all-zero self-neutralize rule, pinned by L8), so a single-dimension
	// fixture would be shown whatever the flag says and prove nothing.
	ch := fixture.Chart{
		ID: "fixture.c023nz", Title: "nonzero visibility", Units: "units",
		Family: "fixture", Context: "fixture.c023nz", UpdateEvery: 1,
		Dimensions: []fixture.Dimension{{ID: "high"}, {ID: "low"}},
	}
	for i := 1; i <= 120; i++ {
		ts := fixture.T0 + int64(i)
		ch.Dimensions[0].Points = append(ch.Dimensions[0].Points,
			fixture.Point{T: ts, Collected: "200", Flags: stream.FlagNotAnomalous})
		ch.Dimensions[1].Points = append(ch.Dimensions[1].Points,
			fixture.Point{T: ts, Collected: "1", Flags: stream.FlagNotAnomalous})
	}

	pushLiveBurst(t, "c023nz", guid(218), ch)
	if _, err := td.WaitRetention("c023nz", ch.Context, ch.FirstT(), ch.LastT(), 20*time.Second); err != nil {
		t.Fatal(err)
	}

	shown := func(queryOptions string) map[string][]canon.Pt {
		t.Helper()
		params := daemon.DataParams(ch.Context, fixture.T0, fixture.T0+120, 6)
		params.Set("time_group", "number-of-times")
		params.Set("time_group_options", ">100")
		params.Set("options", queryOptions)
		doc, err := td.DataV3("c023nz", params)
		if err != nil {
			t.Fatal(err)
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}
		return cols
	}

	ok := true

	// without the filter both dimensions are there, so the fixture is sound
	all := shown("jsonwrap")
	if _, has := all["high"]; !has {
		t.Fatalf("fixture broken: the matching dimension is missing without options=nonzero")
	}
	if _, has := all["low"]; !has {
		t.Fatalf("fixture broken: the non-matching dimension is missing without options=nonzero")
	}

	// with it, only the one whose ANSWER is non-zero survives. Both carry
	// non-zero samples, so a rule that looks at the samples keeps both.
	filtered := shown("jsonwrap|nonzero")
	if _, has := filtered["high"]; !has {
		t.Logf("nonzero contract not met: the dimension whose condition holds was dropped")
		ok = false
	}
	if _, has := filtered["low"]; has {
		t.Logf("nonzero contract not met: a dimension answering zero everywhere survived options=nonzero " +
			"(its SOURCE samples are non-zero, but the answer is what it reports)")
		ok = false
	}

	assertContract(t, "CASE-023/nonzero-follows-answer", ok)
}
