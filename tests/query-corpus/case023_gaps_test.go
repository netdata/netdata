// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-023 gap accounting — RED: what "no data" is worth.
//
// A condition that names a gap is the only way uncollected time enters a
// query at all, so the two things that decide its answer have to be right:
//
//   - how FAR the accounting runs. The query engine stops walking a few
//     buckets after a dimension's storage is exhausted and lets the caller
//     fill the rest with EMPTY. For every other aggregation that is the
//     same answer; after the metric has qualified by overlapping the query,
//     for `==gap` the remaining buckets ARE the answer, and stopping early
//     silently under-reports the visible gap.
//   - how MUCH a gap weighs. `percentage-of-samples` counts samples, so a
//     gap has to be counted in stored slots. Measuring it against the
//     query grid (1s for an ordinary query) makes one missing 10s slot
//     outweigh ten collected ones.
//
// Expectations below come from the fixture: which slots were pushed and
// which were not.
package corpus

import (
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

// An instance whose retention overlaps the query but whose collection stops
// before the query ends. Once the instance participates in the query, every
// bucket past its last sample is visible uncollected time and has to be
// counted as such, all the way to the end of the requested window.
func TestCase023TrailingGapsRunToTheEnd(t *testing.T) {
	trackContract(t, "CASE-023/trailing-gaps")

	const (
		samples = 600 // the context keeps collecting for the whole window
		stops   = 120 // ...but this instance stops here
	)

	const context = "fixture.c023trail"
	always := fixture.Chart{
		ID: context + "_always", Title: "always collected", Units: "units",
		Family: "fixture", Context: context, UpdateEvery: 1,
		Dimensions: []fixture.Dimension{{ID: "value"}},
	}
	stopped := fixture.Chart{
		ID: context + "_stopped", Title: "collection stopped", Units: "units",
		Family: "fixture", Context: context, UpdateEvery: 1,
		Dimensions: []fixture.Dimension{{ID: "value"}},
	}
	for i := 1; i <= samples; i++ {
		ts := fixture.T0 + int64(i)
		always.Dimensions[0].Points = append(always.Dimensions[0].Points,
			fixture.Point{T: ts, Collected: "1", Flags: stream.FlagNotAnomalous})
		// NOTHING is pushed for this instance after `stops` - not even an
		// empty slot. Stored gaps would keep the storage query alive; this
		// case needs the instance's metric storage to actually run OUT, which is
		// what makes the engine stop walking.
		if i <= stops {
			stopped.Dimensions[0].Points = append(stopped.Dimensions[0].Points,
				fixture.Point{T: ts, Collected: "1", Flags: stream.FlagNotAnomalous})
		}
	}

	conn := connect(t, "c023trail", guid(214), stream.CapsLive)
	for _, ch := range []fixture.Chart{always, stopped} {
		ch.Define(conn)
		ch.PushLive(conn)
	}
	if err := conn.Flush(); err != nil {
		t.Fatal(err)
	}
	if _, err := td.WaitRetention("c023trail", context, always.FirstT(), always.LastT(), 20*time.Second); err != nil {
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

	params := daemon.DataParams(context, after, before, buckets)
	params.Set("time_group", "percentage-of-time")
	params.Set("time_group_options", "==gap")
	params.Set("scope_instances", stopped.ID)
	params.Set("group_by", "instance")
	doc, err := td.DataV3("c023trail", params)
	if err != nil {
		t.Fatal(err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}

	want := make([]expectedColumnPoint, 0, buckets)
	for bucket := 1; bucket <= buckets; bucket++ {
		value := 0.0
		if bucket > stops/bucketSpan {
			value = 100
		}
		want = append(want,
			wantNumberWithMetadataAt(after+int64(bucket*bucketSpan), value, 0, 0))
	}
	stoppedColumn := stopped.ID + "@" + guid(214)
	if !assertExactView(t, doc, after, before, bucketSpan) ||
		!assertOnlyColumn(t, cols, stoppedColumn) ||
		!assertExactColumn(t, cols, stoppedColumn, want, 1e-6) {
		check(false, "overlapping stopped instance did not return the exact 30-row 0%%/100%% grid")
	}

	assertContract(t, "CASE-023/trailing-gaps", ok)
}

// A leading gap belongs before the first real sample. Reordering it to the
// row suffix changes an order-sensitive expression from gap->numeric to
// numeric->gap and invents a false-to-true flap.
func TestCase023LeadingGapPrecedesFirstRealSample(t *testing.T) {
	trackContract(t, "CASE-023/leading-gap-chronology")

	const (
		window = 100
		real   = 10
	)
	ch := fixture.Chart{
		ID: "fixture.c023lead", Title: "late start", Units: "units",
		Family: "fixture", Context: "fixture.c023lead", UpdateEvery: 1,
		Dimensions: []fixture.Dimension{{ID: "value"}},
	}
	for i := window - real + 1; i <= window; i++ {
		ch.Dimensions[0].Points = append(ch.Dimensions[0].Points,
			fixture.Point{T: fixture.T0 + int64(i), Collected: "1", Flags: stream.FlagNotAnomalous})
	}

	pushLiveBurst(t, "c023lead", guid(294), ch)
	if _, err := td.WaitRetention("c023lead", ch.Context, ch.FirstT(), ch.LastT(), 20*time.Second); err != nil {
		t.Fatal(err)
	}

	params := daemon.DataParams(ch.Context, fixture.T0, fixture.T0+window, 1)
	params.Set("time_group", "number-of-flaps")
	params.Set("time_group_options", "==gap")
	params.Set("options", "jsonwrap|unaligned")
	params.Set("scope_dimensions", "value")
	doc, err := td.DataV3("c023lead", params)
	if err != nil {
		t.Fatal(err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}

	points := cols["value"]
	ok := assertOnlyColumn(t, cols, "value") && len(points) == 1 &&
		points[0].Value != nil && *points[0].Value == 0
	if !ok {
		got := any(nil)
		if len(points) == 1 && points[0].Value != nil {
			got = *points[0].Value
		}
		t.Logf("leading gap chronology returned value %v in %d row(s), want one numeric zero-flap result",
			got, len(points))
	}
	assertContract(t, "CASE-023/leading-gap-chronology", ok)
}

// A gap covers stored SLOTS, not seconds. On a metric collected every 10s,
// a 100s hole is ten missing samples - not a hundred - so it cannot
// outweigh the collected samples around it by the collection interval.
func TestCase023GapWeightFollowsCollectionInterval(t *testing.T) {
	trackContract(t, "CASE-023/gap-weight")

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

	want := make([]expectedColumnPoint, 0, buckets)
	for bucket := int64(1); bucket <= buckets; bucket++ {
		want = append(want,
			wantNumberWithMetadataAt(after+bucket*perBucket*ue, 50, 0, 0))
	}
	ok := assertExactView(t, doc, after, before, perBucket*ue) &&
		assertOnlyColumn(t, cols, "d") &&
		assertExactColumn(t, cols, "d", want, 1e-6)

	assertContract(t, "CASE-023/gap-weight", ok)
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
	for _, contract := range []string{
		"CASE-023/percentage-of-time-denominator",
		"CASE-023/percentage-of-samples-denominator",
	} {
		registerContract(t, contract)
	}

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

	ask := func(t *testing.T, group, options string, want float64) bool {
		t.Helper()
		params := daemon.DataParams(ch.Context, fixture.T0, fixture.T0+window, 1)
		params.Set("time_group", group)
		params.Set("time_group_options", options)
		params.Set("options", "jsonwrap|unaligned")
		params.Set("scope_dimensions", "stops")
		doc, err := td.DataV3("c023denom", params)
		if err != nil {
			t.Fatal(err)
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}
		return assertViewFields(t, doc, fixture.T0, fixture.T0+window, window+1) &&
			assertOnlyColumn(t, cols, "stops") &&
			assertExactColumn(t, cols, "stops", []expectedColumnPoint{
				wantNumberWithMetadataAt(fixture.T0+window, want, 0, 0),
			}, 1e-6)
	}

	t.Run("percentage-of-time", func(t *testing.T) {
		trackContract(t, "CASE-023/percentage-of-time-denominator")

		// one second of the hundred satisfied the condition, and the
		// ninety-nine uncollected seconds are the rest of it.
		if !ask(t, "percentage-of-time", "==1", 1) {
			t.Error("percentage-of-time ==1 is not the exact 1% answer")
		}
		if !ask(t, "percentage-of-time", "==gap", 99) {
			t.Error("percentage-of-time ==gap is not the exact 99% answer")
		}
	})

	t.Run("percentage-of-samples", func(t *testing.T) {
		trackContract(t, "CASE-023/percentage-of-samples-denominator")

		// This grouping answers about the samples it was handed, so the
		// single collected one is all of them.
		if !ask(t, "percentage-of-samples", "==1", 100) {
			t.Error("percentage-of-samples ==1 is not the exact 100% answer")
		}
	})
}

// An instance whose retention does not overlap the requested window is not a
// participant in that query, even when another instance of the same context
// does overlap. Synthesizing the expired instance as an all-gap series would
// resurrect every expired ephemeral instance and flood fleet queries with
// noise unrelated to the selected time range.
func TestCase023WindowOutsideRetention(t *testing.T) {
	trackContract(t, "CASE-023/window-outside-retention")

	const (
		context    = "fixture.c023visibility"
		pastRows   = 300
		bucketSpan = 20
		buckets    = 30
	)
	past := fixture.Chart{
		ID: context + "_past", Title: "gone before the window", Units: "units",
		Family: "fixture", Context: context, UpdateEvery: 1,
		Dimensions: []fixture.Dimension{{ID: "value"}},
	}
	present := fixture.Chart{
		ID: context + "_present", Title: "present in the window", Units: "units",
		Family: "fixture", Context: context, UpdateEvery: 1,
		Dimensions: []fixture.Dimension{{ID: "value"}},
	}
	for i := 1; i <= pastRows; i++ {
		past.Dimensions[0].Points = append(past.Dimensions[0].Points,
			fixture.Point{T: fixture.T0 + int64(i), Collected: "1", Flags: stream.FlagNotAnomalous})
	}

	// The selected window starts well after `past` ends. `present` shares the
	// context and fills the window, making absence of the specific expired
	// instance observable without relying only on a wholly empty response.
	after := fixture.T0 + int64(pastRows) + 600
	before := after + buckets*bucketSpan
	for ts := after + 1; ts <= before; ts++ {
		present.Dimensions[0].Points = append(present.Dimensions[0].Points,
			fixture.Point{T: ts, Collected: "1", Flags: stream.FlagNotAnomalous})
	}

	conn := connect(t, "c023visibility", guid(291), stream.CapsLive)
	for _, ch := range []fixture.Chart{past, present} {
		ch.Define(conn)
		ch.PushLive(conn)
	}
	if err := conn.Flush(); err != nil {
		t.Fatal(err)
	}
	if _, err := td.WaitRetention("c023visibility", context, past.FirstT(), present.LastT(), 20*time.Second); err != nil {
		t.Fatal(err)
	}

	params := daemon.DataParams(context, after, before, buckets)
	params.Set("time_group", "percentage-of-time")
	params.Set("time_group_options", "==gap")
	params.Set("group_by", "instance")
	doc, err := td.DataV3("c023visibility", params)
	if err != nil {
		t.Fatal(err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}
	presentColumn := present.ID + "@" + guid(291)
	wantPresent := make([]expectedColumnPoint, 0, buckets)
	for bucket := 1; bucket <= buckets; bucket++ {
		wantPresent = append(wantPresent,
			wantNumberWithMetadataAt(after+int64(bucket*bucketSpan), 0, 0, 0))
	}
	if !assertExactView(t, doc, after, before, bucketSpan) ||
		!assertOnlyColumn(t, cols, presentColumn) ||
		!assertExactColumn(t, cols, presentColumn, wantPresent, 1e-6) {
		t.Logf("window outside past retention did not return only the overlapping instance")
		assertContract(t, "CASE-023/window-outside-retention", false)
		return
	}

	// Even an explicit request for the expired instance cannot turn a window
	// with no retention overlap into a synthetic all-gap result.
	pastOnly := daemon.DataParams(context, after, before, buckets)
	pastOnly.Set("time_group", "percentage-of-time")
	pastOnly.Set("time_group_options", "==gap")
	pastOnly.Set("scope_instances", past.ID)
	pastOnly.Set("group_by", "instance")
	pastDoc, err := td.DataV3("c023visibility", pastOnly)
	if err != nil {
		t.Fatal(err)
	}
	if !canon.EmptyResult(pastDoc) {
		t.Logf("explicitly selected instance outside retention returned a result: %v", pastDoc["result"])
		assertContract(t, "CASE-023/window-outside-retention", false)
		return
	}

	assertContract(t, "CASE-023/window-outside-retention", true)
}
