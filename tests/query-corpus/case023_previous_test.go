// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-023 the predecessor across a re-delivered window — RED.
//
// Above tier 0 a stored point covers many seconds. Ask for buckets narrower
// than that and the SAME point is handed to every bucket it spans, each time
// with a value interpolated for that bucket.
//
// `<previous` is decided from the window's minimum against the PREVIOUS
// window's maximum. By the time the repeats arrive that maximum has already
// advanced to this window's own - so re-deciding them asks "is this window's
// minimum below its own maximum", which is true of every window that moved
// at all. A counter that never restarted then reports a reboot in every
// bucket after the first one it was delivered into.
//
// percentage-of-time is where this shows: it weighs every delivery by the
// time that delivery covers, so a wrong verdict on a repeat becomes wrong
// minutes on a dashboard. The counting groupings ignore repeats outright.
//
// Expectations come from the fixture: the counter below is monotone and
// never restarts, so no amount of zooming may find a reset in it.
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

func TestCase023PreviousSurvivesRedelivery(t *testing.T) {
	trackContract(t, "CASE-023/previous-survives-redelivery")

	const samples = 1800 // 30 tier-1 windows

	// a counter that only ever climbs - one step per second, no restart
	ch := fixture.Series("fixture.c023prev", "fixture.c023prev", fixture.T0, samples, 1,
		func(i int) string { return strconv.Itoa(i) },
		func(int) string { return stream.FlagNotAnomalous })
	ch.ValueTolerance = 1e-9

	pushLiveBurst(t, "c023prev", guid(220), ch)
	if _, err := td.WaitRetention("c023prev", ch.Context, ch.FirstT(), ch.LastT(), 20*time.Second); err != nil {
		t.Fatal(err)
	}

	const windows = 20
	after := int64(fixture.T0 + 40) // the first absolute multiple of tier1Gran
	before := after + windows*tier1Gran

	// buckets is how many result points the tier-1 window is cut into: 1 is
	// one delivery per window, anything above it re-delivers
	ok := true
	ask := func(buckets int64) []canon.Pt {
		t.Helper()
		params := daemon.DataParamsTier(ch.Context, 1, after, before, windows*buckets, "percentage-of-time")
		params.Set("time_group_options", "<previous")
		params.Set("options", "jsonwrap|unaligned")
		doc, err := td.DataV3("c023prev", params)
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
		if !assertOnlyColumn(t, cols, ch.Dimensions[0].ID) {
			ok = false
		}
		rowSpan := tier1Gran / buckets
		want := make([]expectedColumnPoint, 0, windows*buckets)
		for row := int64(1); row <= windows*buckets; row++ {
			want = append(want, wantNumberAt(after+row*rowSpan, 0))
		}
		if !assertExactColumn(t, cols, ch.Dimensions[0].ID, want, 1e-6) {
			ok = false
		}
		return cols[ch.Dimensions[0].ID]
	}

	for _, perWindow := range []int64{1, 3, 5} {
		col := ask(perWindow)

		worst, worstAt, nonzero := 0.0, int64(0), 0
		for _, pt := range col {
			if pt.Value == nil {
				continue
			}
			if *pt.Value > 1e-6 {
				nonzero++
				if *pt.Value > worst {
					worst, worstAt = *pt.Value, pt.T
				}
			}
		}

		if nonzero != 0 {
			t.Logf("predecessor contract not met: at %d buckets per stored window, "+
				"%d of %d buckets report time below the predecessor on a counter that only climbs "+
				"(worst %v%% at t0%+d)",
				perWindow, nonzero, len(col), worst, worstAt-fixture.T0)
			ok = false
		}
	}

	// and the case only means something if the finer grids really did
	// re-deliver: one point per stored window must produce fewer rows than
	// five per window
	if one, five := len(ask(1)), len(ask(5)); one >= five {
		t.Fatalf("the finer grid did not re-deliver: %d rows at 1/window vs %d at 5/window", one, five)
	}

	assertContract(t, "CASE-023/previous-survives-redelivery", ok)
}

// The mirror image: a counter that DID restart must still be found, and the
// whole window it restarted in must count as time below the predecessor -
// not just the first bucket that window was delivered into.
//
// Replaying the first delivery's verdict is what makes both true at once. A
// repeat that re-decides itself gets this backwards: it reports the reset in
// the buckets that follow it (where nothing happened) and, once the floor
// has been carried forward, stops reporting it in its own.
func TestCase023PreviousFindsARealDropAtEveryZoom(t *testing.T) {
	trackContract(t, "CASE-023/previous-drop-at-every-zoom")

	const (
		samples = 1800
		resetAt = 900 // one restart, in the middle
	)

	ch := fixture.Series("fixture.c023prevdrop", "fixture.c023prevdrop", fixture.T0, samples, 1,
		func(i int) string {
			v := i
			if i >= resetAt {
				v = i - resetAt
			}
			return strconv.Itoa(v)
		}, func(int) string { return stream.FlagNotAnomalous })
	ch.ValueTolerance = 1e-9

	pushLiveBurst(t, "c023prevdrop", guid(221), ch)
	if _, err := td.WaitRetention("c023prevdrop", ch.Context, ch.FirstT(), ch.LastT(), 20*time.Second); err != nil {
		t.Fatal(err)
	}

	const windows = 20
	after := int64(fixture.T0 + 40)
	before := after + windows*tier1Gran

	// the share of the whole span that reads "below the predecessor",
	// weighted by each bucket's own width - the quantity percentage-of-time
	// exists to report, and one that must not move with the zoom
	ok := true
	weighted := func(perWindow int64) float64 {
		t.Helper()
		params := daemon.DataParamsTier(ch.Context, 1, after, before, windows*perWindow, "percentage-of-time")
		params.Set("time_group_options", "<previous")
		params.Set("options", "jsonwrap|unaligned")
		doc, err := td.DataV3("c023prevdrop", params)
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
		col := cols[ch.Dimensions[0].ID]
		if len(col) == 0 {
			t.Fatalf("perWindow=%d returned no rows", perWindow)
		}

		sum := 0.0
		for _, pt := range col {
			if pt.Value != nil {
				sum += *pt.Value
			}
		}
		return sum / float64(len(col))
	}

	// one delivery per stored window: exactly one of the twenty windows
	// contains the restart, so it is 1/20 of the span
	base := weighted(1)
	if math.Abs(base-5) >= 0.5 {
		t.Logf("predecessor contract not met: at one bucket per stored window the restart covers %.2f%% "+
			"of the span, want ~5%% (one window in twenty)", base)
		ok = false
	}

	// and zooming in must not change the answer: the same restart, in the
	// same window, occupying the same share of the same span
	for _, perWindow := range []int64{3, 5} {
		got := weighted(perWindow)
		if math.Abs(got-base) >= 0.5 {
			t.Logf("predecessor contract not met: %d buckets per stored window report %.2f%% of the span "+
				"below the predecessor, but one bucket per window reports %.2f%% - the same restart",
				perWindow, got, base)
			ok = false
		}
	}

	assertContract(t, "CASE-023/previous-drop-at-every-zoom", ok)
}
