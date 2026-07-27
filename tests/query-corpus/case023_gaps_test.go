// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-023 gap accounting — RED: what "no data" is worth.
//
// A condition that names a gap is the only way uncollected time enters a
// query at all, so the two things that decide its answer have to be right:
//
//   - how FAR the accounting runs. The query engine stops walking a few
//     buckets after a dimension's storage is exhausted and lets the caller
//     fill the rest with EMPTY. For every other aggregation that is the
//     same answer; for `==gap` the remaining buckets ARE the answer, and
//     stopping early silently under-reports an outage.
//   - how MUCH a gap weighs. `percentage-of-samples` counts samples, so a
//     gap has to be counted in stored slots. Measuring it against the
//     query grid (1s for an ordinary query) makes one missing 10s slot
//     outweigh ten collected ones.
//
// Expectations below come from the fixture: which slots were pushed and
// which were not.
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

// A dimension that stops being collected while the chart keeps going: the
// chart's retention covers the whole window, but this dimension's storage
// runs out early, which is the path that gives up after a handful of
// buckets. Every bucket past the last sample is uncollected time and has to
// be counted as such, all the way to the end of the requested window.
func TestCase023TrailingGapsRunToTheEnd(t *testing.T) {
	const (
		samples = 600 // the chart keeps collecting for the whole window
		stops   = 120 // ...but this dimension stops here
	)

	ch := fixture.Chart{
		ID: "fixture.c023trail", Title: "trailing gaps", Units: "units",
		Family: "fixture", Context: "fixture.c023trail", UpdateEvery: 1,
		Dimensions: []fixture.Dimension{{ID: "always"}, {ID: "stops"}},
	}
	for i := 1; i <= samples; i++ {
		ts := fixture.T0 + int64(i)
		ch.Dimensions[0].Points = append(ch.Dimensions[0].Points,
			fixture.Point{T: ts, Collected: "1", Flags: stream.FlagNotAnomalous})
		// NOTHING is pushed for this dimension after `stops` - not even an
		// empty slot. Stored gaps would keep the storage query alive; this
		// case needs the dimension's storage to actually run OUT, which is
		// what makes the engine stop walking.
		if i <= stops {
			ch.Dimensions[1].Points = append(ch.Dimensions[1].Points,
				fixture.Point{T: ts, Collected: "1", Flags: stream.FlagNotAnomalous})
		}
	}

	pushLiveBurst(t, "c023trail", guid(214), ch)
	if _, err := td.WaitRetention("c023trail", ch.Context, ch.FirstT(), ch.LastT(), 20*time.Second); err != nil {
		t.Fatal(err)
	}

	ok := true
	check := func(cond bool, what string, args ...any) {
		t.Helper()
		if !cond {
			t.Logf("trailing gap contract not met: "+what, args...)
			ok = false
		}
	}

	// 30 buckets of 20s over the whole window: six carry the stopped
	// dimension's data, twenty-four are entirely past it - far more than
	// the handful the engine used to walk
	const (
		bucketSpan = 20
		buckets    = 30
	)
	after := int64(fixture.T0)
	before := after + buckets*bucketSpan

	params := daemon.DataParams(ch.Context, after, before, buckets)
	params.Set("time_group", "percentage-of-time")
	params.Set("time_group_options", "==gap")
	doc, err := td.DataV3("c023trail", params)
	if err != nil {
		t.Fatal(err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}

	col := cols["stops"]
	if len(col) == 0 {
		t.Fatalf("no rows for the stopped dimension")
	}

	tail := 0
	for _, pt := range col {
		// a bucket that starts after the last collected sample is entirely
		// uncollected, whatever the engine decided to walk
		if pt.T-bucketSpan < fixture.T0+stops {
			continue
		}
		tail++
		if pt.Value == nil {
			check(false, "bucket t0%+d past the last sample is EMPTY, want 100%% gap", pt.T-fixture.T0)
			continue
		}
		check(math.Abs(*pt.Value-100) < 1e-6,
			"bucket t0%+d past the last sample reads %v, want 100", pt.T-fixture.T0, *pt.Value)
	}

	// the case only proves something if it reaches well beyond the point
	// the walk used to stop
	if tail < 20 {
		t.Fatalf("only %d trailing buckets were examined, want at least 20", tail)
	}

	expectAgentStatus(t, "CASE-023/trailing-gaps", ok)
}

// A gap covers stored SLOTS, not seconds. On a metric collected every 10s,
// a 100s hole is ten missing samples - not a hundred - so it cannot
// outweigh the collected samples around it by the collection interval.
func TestCase023GapWeightFollowsCollectionInterval(t *testing.T) {
	const (
		ue      = 10
		samples = 40 // 20 collected, 20 uncollected, in equal halves
	)

	base := fixture.T0 - fixture.T0%int64(ue)
	ch := fixture.Chart{
		ID: "fixture.c023weight", Title: "gap weight", Units: "units",
		Family: "fixture", Context: "fixture.c023weight", UpdateEvery: ue,
		Dimensions: []fixture.Dimension{{ID: "d"}},
	}
	for i := 1; i <= samples; i++ {
		ts := base + int64(i*ue)
		if (i-1)%20 < 10 {
			ch.Dimensions[0].Points = append(ch.Dimensions[0].Points,
				fixture.Point{T: ts, Collected: strconv.Itoa(i), Flags: stream.FlagNotAnomalous})
		} else {
			ch.Dimensions[0].Points = append(ch.Dimensions[0].Points,
				fixture.Point{T: ts, Flags: stream.FlagEmpty})
		}
	}

	pushLiveBurst(t, "c023weight", guid(215), ch)
	if _, err := td.WaitRetention("c023weight", ch.Context, ch.FirstT(), ch.LastT(), 20*time.Second); err != nil {
		t.Fatal(err)
	}

	// one bucket per 20 slots: ten collected and ten uncollected, so the
	// share of samples that are gaps is exactly half
	const perBucket = 20
	after := base
	before := base + int64(samples*ue)
	buckets := int64(samples / perBucket)

	params := daemon.DataParams(ch.Context, after, before, buckets)
	params.Set("time_group", "percentage-of-samples")
	params.Set("time_group_options", "==gap")
	doc, err := td.DataV3("c023weight", params)
	if err != nil {
		t.Fatal(err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}

	ok := true
	checked := 0
	for _, pt := range cols["d"] {
		if pt.Value == nil {
			continue
		}
		checked++
		// half the slots of every bucket were never collected. Counting a
		// gap in seconds instead of slots would read ~91% here, because one
		// missing 10s slot would count as ten.
		if math.Abs(*pt.Value-50) >= 1e-6 {
			t.Logf("gap weight contract not met: bucket t0%+d reads %v%%, want 50%% "+
				"(ten collected slots and ten missing ones)", pt.T-fixture.T0, *pt.Value)
			ok = false
		}
	}

	if checked == 0 {
		t.Fatalf("no buckets carried a value")
	}

	expectAgentStatus(t, "CASE-023/gap-weight", ok)
}

// The denominator of percentage-of-time is the SELECTED duration, not the
// collected part of it.
//
// Uncollected time is time during which the condition did not hold. One
// collected sample reading 1, followed by ninety-nine seconds with nothing
// collected, is 1% of the window at `==1` — not 100% of the only sample
// that happened to be there. Reporting the latter turns a node that went
// silent into a node that is perfectly healthy, which is the exact
// opposite of what an availability query is for.
//
// This is what separates percentage-of-time from percentage-of-samples:
// the latter answers about the samples it was given and stays blind to
// gaps unless the condition names one.
func TestCase023PercentageOfTimeCountsTheWholeWindow(t *testing.T) {
	// one collected second, then ninety-nine with nothing pushed at all
	const (
		collected = 1
		window    = 100
	)

	ch := fixture.Chart{
		ID: "fixture.c023denom", Title: "denominator", Units: "units",
		Family: "fixture", Context: "fixture.c023denom", UpdateEvery: 1,
		Dimensions: []fixture.Dimension{{ID: "keeps"}, {ID: "stops"}},
	}
	for i := 1; i <= window; i++ {
		ts := fixture.T0 + int64(i)
		// one dimension keeps the chart alive for the whole window
		ch.Dimensions[0].Points = append(ch.Dimensions[0].Points,
			fixture.Point{T: ts, Collected: "1", Flags: stream.FlagNotAnomalous})
		if i <= collected {
			ch.Dimensions[1].Points = append(ch.Dimensions[1].Points,
				fixture.Point{T: ts, Collected: "1", Flags: stream.FlagNotAnomalous})
		}
	}

	pushLiveBurst(t, "c023denom", guid(219), ch)
	if _, err := td.WaitRetention("c023denom", ch.Context, ch.FirstT(), ch.LastT(), 20*time.Second); err != nil {
		t.Fatal(err)
	}

	ok := true
	ask := func(group, options string) *float64 {
		t.Helper()
		params := daemon.DataParams(ch.Context, fixture.T0, fixture.T0+window, 1)
		params.Set("time_group", group)
		params.Set("time_group_options", options)
		params.Set("options", "jsonwrap|unaligned")
		doc, err := td.DataV3("c023denom", params)
		if err != nil {
			t.Fatal(err)
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}
		col := cols["stops"]
		if len(col) != 1 {
			t.Fatalf("%s(%s): got %d buckets, want 1", group, options, len(col))
		}
		return col[0].Value
	}

	// one second of the hundred satisfied the condition
	if v := ask("percentage-of-time", "==1"); v == nil {
		t.Logf("denominator contract not met: percentage-of-time ==1 is empty")
		ok = false
	} else if math.Abs(*v-1) >= 1e-6 {
		t.Logf("denominator contract not met: percentage-of-time ==1 reads %v%%, want 1%% "+
			"(1 collected second satisfying it, %d seconds of window)", *v, window)
		ok = false
	}

	// and the ninety-nine uncollected seconds are the rest of it
	if v := ask("percentage-of-time", "==gap"); v == nil {
		t.Logf("denominator contract not met: percentage-of-time ==gap is empty")
		ok = false
	} else if math.Abs(*v-99) >= 1e-6 {
		t.Logf("denominator contract not met: percentage-of-time ==gap reads %v%%, want 99%%", *v)
		ok = false
	}

	// percentage-of-samples keeps its own contract: it answers about the
	// samples it was handed, so the single collected one is all of them
	if v := ask("percentage-of-samples", "==1"); v == nil {
		t.Logf("denominator contract not met: percentage-of-samples ==1 is empty")
		ok = false
	} else if math.Abs(*v-100) >= 1e-6 {
		t.Logf("denominator contract not met: percentage-of-samples ==1 reads %v%%, want 100%% "+
			"(this grouping counts samples, not time)", *v)
		ok = false
	}

	expectAgentStatus(t, "CASE-023/percentage-of-time-denominator", ok)
}

// The limit of the trailing-gap contract: a window that does not touch the
// dimension's retention AT ALL.
//
// A node that went silent three days ago, asked "what share of the last
// hour were you unreachable", must answer 100% - the same answer it gives
// for the tail of a window it partly covers. The engine's metric selection
// is what stands in the way: a metric whose retention misses the window is
// normally not worth querying, because it can only answer "nothing here",
// and an absent dimension already says that. For a grouping that ACCOUNTS
// for uncollected time, "nothing here" is not the absence of an answer -
// it IS the answer, and dropping the metric turns a total outage into an
// empty chart, which reads like a healthy node nobody asked about.
func TestCase023WindowOutsideRetention(t *testing.T) {
	const samples = 300

	ch := fixture.Chart{
		ID: "fixture.c023gone", Title: "gone before the window", Units: "units",
		Family: "fixture", Context: "fixture.c023gone", UpdateEvery: 1,
		Dimensions: []fixture.Dimension{{ID: "gone"}},
	}
	for i := 1; i <= samples; i++ {
		ch.Dimensions[0].Points = append(ch.Dimensions[0].Points,
			fixture.Point{T: fixture.T0 + int64(i), Collected: "1", Flags: stream.FlagNotAnomalous})
	}

	pushLiveBurst(t, "c023gone", guid(215), ch)
	if _, err := td.WaitRetention("c023gone", ch.Context, ch.FirstT(), ch.LastT(), 20*time.Second); err != nil {
		t.Fatal(err)
	}

	ok := true

	// a window that starts well after the last stored sample and never
	// overlaps retention by a single second
	const (
		bucketSpan = 20
		buckets    = 30
	)
	after := fixture.T0 + int64(samples) + 600
	before := after + buckets*bucketSpan

	params := daemon.DataParams(ch.Context, after, before, buckets)
	params.Set("time_group", "percentage-of-time")
	params.Set("time_group_options", "==gap")
	doc, err := td.DataV3("c023gone", params)
	if err != nil {
		t.Fatal(err)
	}
	// the dimension is dropped from the result entirely when the bug is
	// present, so there are no columns to read at all - that IS the red
	// condition, not a harness failure
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Logf("a window entirely past the dimension's retention returned no dimension columns "+
			"at all (%v), want %d buckets of 100%% gap", err, buckets)
		expectAgentStatus(t, "CASE-023/window-outside-retention", false)
		return
	}

	col, has := cols["gone"]
	if !has || len(col) == 0 {
		t.Logf("a window entirely past the dimension's retention returned no rows for it at all, "+
			"want %d buckets of 100%% gap", buckets)
		expectAgentStatus(t, "CASE-023/window-outside-retention", false)
		return
	}

	for _, pt := range col {
		if pt.Value == nil {
			t.Logf("bucket t0%+d, entirely past retention, is EMPTY, want 100%% gap", pt.T-fixture.T0)
			ok = false
			continue
		}
		if math.Abs(*pt.Value-100) > 1e-6 {
			t.Logf("bucket t0%+d, entirely past retention, reads %v, want 100", pt.T-fixture.T0, *pt.Value)
			ok = false
		}
	}

	expectAgentStatus(t, "CASE-023/window-outside-retention", ok)
}
