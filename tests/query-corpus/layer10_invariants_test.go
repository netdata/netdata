// SPDX-License-Identifier: GPL-3.0-or-later

// Layer 10 — the invariants EVERY time-grouping owes, swept across all of them.
//
// Every layer above this one pins a specific answer for a specific grouping.
// That is how the corpus found the bugs it found, and also why it kept finding
// the same SHAPE of bug in a grouping nobody had written a case for yet:
//
//   - `percentage-of-time(<previous)` re-decided a re-delivered window and
//     reported a reboot in two thirds of the buckets of a counter that never
//     restarted;
//   - `number-of-flaps` and `number-of-times` dropped the sample count of a
//     re-delivered point and left 16 of 24 buckets EMPTY;
//   - a NUMBER condition on an upsampled tier-0 sample changed its verdict
//     with the zoom.
//
// Three different symptoms, one cause: the answer moved when the only thing
// that moved was how finely the range was cut. Nothing in the corpus said
// that was forbidden - for any grouping - so each instance had to be found by
// hand, in the one grouping someone happened to be looking at.
//
// This layer states the rules instead. Each one is a property an aggregation
// owes by virtue of what it MEANS, not by virtue of what the engine does, and
// each is checked against every grouping the property applies to. The roster
// comes from the engine's own enum (roster.go), so a grouping added without a
// line in the table below fails here by name.
//
// A grouping is excused from a rule only with a reason from first principles
// - "an increment needs two samples" is a reason, "the engine returns EMPTY"
// is not.
package corpus

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

// What a grouping's answer IS, which is what decides the rules it owes.
type groupingKind int

const (
	// a central tendency or an order statistic: the answer is one of the
	// values that went in, or something between them. Never outside.
	kindInRange groupingKind = iota
	// a share of something, always in [0,100]
	kindPercent
	// a count of occurrences: >= 0, and it cannot grow just because the
	// same data was cut into more buckets
	kindCount
	// an accumulation or a dispersion: legitimately outside the sample
	// range (a sum exceeds every sample; a stddev is a distance, not a
	// value; a forecast extrapolates past both ends)
	kindOther
)

// One row per grouping. The constant is the enum name, so the compiler is not
// what keeps this honest - readGroupingRoster() is, by refusing to run a
// sweep that does not name every grouping the engine declares.
type groupingRule struct {
	kind groupingKind

	// A stored point wider than a result bucket is delivered to every
	// bucket it spans. For an aggregation that COUNTS deliveries this is
	// legitimate - a delivery is a sample and there are more of them - so
	// its total over a fixed span may move with the zoom. For one that
	// counts EVENTS it is not: the same event cannot happen twice because
	// a dashboard was zoomed in.
	countsDeliveries bool

	// The duration-weighted mean over a fixed span answers the same
	// question at any resolution. True only where the grouping's own
	// denominator is TIME; percentage-of-samples answers about samples, so
	// re-delivering one legitimately moves its answer.
	weightedMeanStable bool

	// The plain SUM over the buckets of a fixed span is the same number at
	// any resolution. True of an accumulation, which is exactly decomposable
	// - the total volume over an hour is a physical quantity and cannot
	// depend on how many columns the chart was drawn with.
	totalExact bool

	// A KNOWN defect, pinned red by L10/single-point-buckets-answer: this
	// grouping returns EMPTY for a bucket that holds exactly one point.
	// The general no-holes sweep steps around it so it keeps guarding the
	// other forty groupings instead of reporting the same bug forever.
	knownSinglePointEmpty bool

	// why it is excused from a rule its kind would otherwise carry
	why string
}

// Every grouping the engine offers. Adding one to RRDR_TIME_GROUPING without
// adding it here fails TestLayer10RosterIsComplete, by name.
var groupingRules = map[string]groupingRule{
	// ---- central tendencies and order statistics ----
	// each answers WITH one of the values it was given, or between two of
	// them, so no bucket may read outside the range of the samples
	"RRDR_GROUPING_AVERAGE":          {kind: kindInRange},
	"RRDR_GROUPING_MIN":              {kind: kindInRange},
	"RRDR_GROUPING_MAX":              {kind: kindInRange},
	"RRDR_GROUPING_MEDIAN":           {kind: kindInRange},
	"RRDR_GROUPING_TRIMMED_MEAN1":    {kind: kindInRange},
	"RRDR_GROUPING_TRIMMED_MEAN2":    {kind: kindInRange},
	"RRDR_GROUPING_TRIMMED_MEAN3":    {kind: kindInRange},
	"RRDR_GROUPING_TRIMMED_MEAN":     {kind: kindInRange},
	"RRDR_GROUPING_TRIMMED_MEAN10":   {kind: kindInRange},
	"RRDR_GROUPING_TRIMMED_MEAN15":   {kind: kindInRange},
	"RRDR_GROUPING_TRIMMED_MEAN20":   {kind: kindInRange},
	"RRDR_GROUPING_TRIMMED_MEAN25":   {kind: kindInRange},
	"RRDR_GROUPING_TRIMMED_MEDIAN1":  {kind: kindInRange},
	"RRDR_GROUPING_TRIMMED_MEDIAN2":  {kind: kindInRange},
	"RRDR_GROUPING_TRIMMED_MEDIAN3":  {kind: kindInRange},
	"RRDR_GROUPING_TRIMMED_MEDIAN":   {kind: kindInRange},
	"RRDR_GROUPING_TRIMMED_MEDIAN10": {kind: kindInRange},
	"RRDR_GROUPING_TRIMMED_MEDIAN15": {kind: kindInRange},
	"RRDR_GROUPING_TRIMMED_MEDIAN20": {kind: kindInRange},
	"RRDR_GROUPING_TRIMMED_MEDIAN25": {kind: kindInRange},
	"RRDR_GROUPING_PERCENTILE25":     {kind: kindInRange},
	"RRDR_GROUPING_PERCENTILE50":     {kind: kindInRange},
	"RRDR_GROUPING_PERCENTILE75":     {kind: kindInRange},
	"RRDR_GROUPING_PERCENTILE80":     {kind: kindInRange},
	"RRDR_GROUPING_PERCENTILE90":     {kind: kindInRange},
	"RRDR_GROUPING_PERCENTILE":       {kind: kindInRange},
	"RRDR_GROUPING_PERCENTILE97":     {kind: kindInRange},
	"RRDR_GROUPING_PERCENTILE98":     {kind: kindInRange},
	"RRDR_GROUPING_PERCENTILE99":     {kind: kindInRange},
	// whichever extreme is further from zero - still one of the samples
	"RRDR_GROUPING_EXTREMES": {kind: kindInRange},
	// the last value in the bucket, verbatim
	"RRDR_GROUPING_LATEST": {kind: kindInRange},
	// a weighted average of the samples seen so far: every weight is
	// positive and they sum to one, so it cannot leave their range
	"RRDR_GROUPING_SES": {kind: kindInRange},

	// ---- everything else ----
	"RRDR_GROUPING_SUM": {kind: kindOther, totalExact: true,
		why: "an accumulation exceeds every sample it accumulates"},
	"RRDR_GROUPING_INCREMENTAL_SUM": {kind: kindOther, knownSinglePointEmpty: true,
		why: "a difference between the ends of a bucket, which is not a value in it. " +
			"Returns EMPTY for every bucket holding a single point - a known defect, " +
			"pinned by L10/single-point-buckets-answer"},
	"RRDR_GROUPING_STDDEV": {kind: kindOther,
		why: "a distance, not a value: zero for a constant series whose samples are 7"},
	"RRDR_GROUPING_CV": {kind: kindOther,
		why: "a dispersion relative to the mean, in percent of it - not bounded by 100"},
	"RRDR_GROUPING_DES": {kind: kindOther,
		why: "double exponential smoothing carries a TREND term and extrapolates, " +
			"so a rising series forecasts past its own maximum by design"},

	// ---- the condition family ----
	"RRDR_GROUPING_PERCENTAGE_OF_TIME": {kind: kindPercent, weightedMeanStable: true},
	"RRDR_GROUPING_COUNTIF": {kind: kindPercent,
		countsDeliveries: true,
		why: "percentage-of-samples answers about the samples it was handed, and a " +
			"re-delivered point IS another sample to it - so its answer moves with " +
			"the zoom by contract, unlike percentage-of-time whose denominator is time"},
	"RRDR_GROUPING_NUMBER_OF_FLAPS": {kind: kindCount},
	"RRDR_GROUPING_NUMBER_OF_TIMES": {kind: kindCount},
}

const (
	l10Context = "fixture.l10"
	l10Samples = 2400 // 40 tier-1 windows
	l10Flat    = 7    // the constant dimension's value

	// The zoom invariants compare a COARSE reading of a span against finer
	// ones, and "one bucket per stored window" only means that when the
	// coarse grid lines up with the stored windows. Tier-1 points end on
	// the absolute grid (≡ T0+40 mod 60), so a span starting anywhere else
	// gives the coarse reading buckets that straddle two stored windows -
	// a different set of points, not a different answer about the same
	// ones. Layer 4d pins the alignment itself; here it is a precondition.
	l10GridAfter = fixture.T0 + 40 + tier1Gran

	// The span every sweep reads. It starts on the tier-1 grid and ends
	// well inside the collected range, so no bucket straddles the edge of
	// the data: outward alignment legitimately puts a boundary bucket
	// outside retention (layer 4d pins that), and a sweep that tripped over
	// it would be reporting the grid, not the grouping.
	l10SpanAfter  = l10GridAfter
	l10SpanBefore = l10GridAfter + 30*tier1Gran
)

// l10Value is the fixture oracle: what each dimension holds at offset i.
//
//	ramp  - strictly increasing, so `<previous` never fires and every
//	        tier window has min < max
//	wave  - swings inside EVERY tier-1 window, so a rollup of it is never
//	        a degenerate one-value window
//	flat  - constant, the degenerate case: min == max, which is still a
//	        window and not a sample
func l10Value(dim string, i int) float64 {
	switch dim {
	case "ramp":
		return float64(i)
	case "wave":
		return float64(i % 60)
	default:
		return l10Flat
	}
}

var l10Dims = []string{"ramp", "wave", "flat"}

// l10Range is the closed interval every in-range grouping must stay inside.
func l10Range(dim string) (lo, hi float64) {
	lo, hi = math.Inf(1), math.Inf(-1)
	for i := 1; i <= l10Samples; i++ {
		v := l10Value(dim, i)
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	return lo, hi
}

var l10Ready bool

// l10Fixture pushes the shared fixture once for the whole layer.
func l10Fixture(t *testing.T) {
	t.Helper()
	if l10Ready {
		return
	}

	ch := fixture.Chart{
		ID: l10Context, Title: "grouping invariants", Units: "units",
		Family: "fixture", Context: l10Context, UpdateEvery: 1,
	}
	for _, d := range l10Dims {
		ch.Dimensions = append(ch.Dimensions, fixture.Dimension{ID: d})
	}
	for i := 1; i <= l10Samples; i++ {
		ts := fixture.T0 + int64(i)
		for d := range ch.Dimensions {
			ch.Dimensions[d].Points = append(ch.Dimensions[d].Points,
				fixture.Point{
					T:         ts,
					Collected: strconv.FormatInt(int64(l10Value(l10Dims[d], i)), 10),
					Flags:     stream.FlagNotAnomalous,
				})
		}
	}

	pushLiveBurst(t, "l10", guid(230), ch)
	if _, err := td.WaitRetention("l10", ch.Context, ch.FirstT(), ch.LastT(), 30*time.Second); err != nil {
		t.Fatal(err)
	}
	l10Ready = true
}

// l10Query runs one grouping over the shared fixture.
func l10Query(t *testing.T, group string, tier int, after, before, points int64, dims string) map[string][]canon.Pt {
	t.Helper()

	var params = daemon.DataParams(l10Context, after, before, points)
	if tier >= 0 {
		params = daemon.DataParamsTier(l10Context, tier, after, before, points, group)
	} else {
		params.Set("time_group", group)
	}
	params.Set("options", "jsonwrap|unaligned")
	if dims != "" {
		params.Set("scope_dimensions", dims)
	}

	doc, err := td.DataV3("l10", params)
	if err != nil {
		t.Fatalf("%s: %v", group, err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatalf("%s: %v", group, err)
	}
	return cols
}

// The roster guard. Everything else in this layer is only as complete as this
// is: a grouping the engine offers and this file does not classify is a
// grouping no invariant is checked against.
func TestLayer10RosterIsComplete(t *testing.T) {
	r, err := readGroupingRoster()
	if err != nil {
		t.Fatal(err)
	}

	var missing []string
	for _, c := range r.Order {
		if _, ok := groupingRules[c]; !ok {
			missing = append(missing, fmt.Sprintf("%s (time_group=%s)", c, r.Canonical[c]))
		}
	}
	if len(missing) > 0 {
		t.Fatalf("the engine offers %d groupings that layer 10 does not classify:\n  %v\n\n"+
			"Add a row to groupingRules for each, choosing its kind from what its answer MEANS:\n"+
			"  kindInRange - a central tendency or order statistic; no bucket may read outside the samples\n"+
			"  kindPercent - a share, always in [0,100]\n"+
			"  kindCount   - a count of events; it may not grow because the range was cut more finely\n"+
			"  kindOther   - an accumulation, a dispersion or a forecast; say WHY it escapes the sample range\n\n"+
			"An unclassified grouping is an untested one.",
			len(missing), missing)
	}

	// The other direction is NOT a failure, and the asymmetry is deliberate.
	// A grouping the engine offers and this file does not classify is an
	// untested grouping - that is the dangerous one, and it fails above. A
	// classification the engine does not (yet) declare just means the corpus
	// is running ahead of the branch under test: the sweeps iterate the
	// engine's roster, so the extra rows are inert. Failing on them would
	// make the corpus unable to describe a grouping before it merges.
	var ahead []string
	known := map[string]bool{}
	for _, c := range r.Order {
		known[c] = true
	}
	for c := range groupingRules {
		if !known[c] {
			ahead = append(ahead, c)
		}
	}
	if len(ahead) > 0 {
		sort.Strings(ahead)
		t.Logf("%d classification(s) for groupings this engine does not declare (the corpus is "+
			"ahead of the branch under test, or they were removed): %v", len(ahead), ahead)
	}

	t.Logf("%d groupings classified", len(r.Order))
}

// INV-1: a bucket inside collected data carries a value.
//
// Every grouping, no exceptions. A bucket whose whole width was collected has
// something to say about it; answering EMPTY says "there is no data here",
// which is what an outage looks like. This is the rule `number-of-flaps` and
// `number-of-times` broke for every bucket a re-delivered point covered on
// its own.
//
// The buckets below are wide enough to hold several samples, so a grouping
// that genuinely needs two of them (an increment, a dispersion) is not being
// asked for the impossible.
func TestLayer10NoHolesInsideData(t *testing.T) {
	l10Fixture(t)
	r, err := readGroupingRoster()
	if err != nil {
		t.Fatal(err)
	}

	after, before := int64(l10SpanAfter), int64(l10SpanBefore)

	ok := true
	skipped := 0
	for _, c := range r.Order {
		if groupingRules[c].knownSinglePointEmpty {
			// its own red case owns this; leaving it in would keep the
			// general sweep red and blind to the next grouping that breaks
			skipped++
			continue
		}
		group := r.Canonical[c]
		for _, probe := range []struct {
			tier   int
			points int64
			what   string
		}{
			{tier: 0, points: (before - after) / 10, what: "tier 0, 10s buckets"},
			{tier: 0, points: (before - after) / 60, what: "tier 0, 60s buckets"},
			{tier: 1, points: (before - after) / 60, what: "tier 1, 60s buckets"},
			{tier: 1, points: (before - after) / 300, what: "tier 1, 300s buckets"},
			{tier: 1, points: (before - after) / 600, what: "tier 1, 600s buckets"},
		} {
			cols := l10Query(t, group, probe.tier, after, before, probe.points, "")
			for _, dim := range l10Dims {
				col := cols[dim]
				if len(col) == 0 {
					t.Logf("invariant not met: %s (%s) returned no rows for %q", group, probe.what, dim)
					ok = false
					continue
				}
				empty := 0
				for _, pt := range col {
					if pt.Value == nil {
						empty++
					}
				}
				if empty > 0 {
					t.Logf("invariant not met: %s (%s) left %d of %d buckets EMPTY for %q, "+
						"inside a span that was collected end to end",
						group, probe.what, empty, len(col), dim)
					ok = false
				}
			}
		}
	}

	if skipped > 0 {
		t.Logf("%d grouping(s) skipped as known-broken - see L10/single-point-buckets-answer", skipped)
	}
	expectAgentStatus(t, "L10/no-holes-inside-data", ok)
}

// INV-1c: a bucket NARROWER than the stored data still answers.
//
// The zoomed-in regime, and the one that matters most: above tier 0 a stored
// point covers many seconds, so a dashboard drawn finer than that gives the
// engine buckets that a single re-delivered point covers on its own. Every
// grouping must still answer in them.
//
// This is where `number-of-flaps` and `number-of-times` punched holes - they
// dropped the repeat entirely, sample count and all, so the buckets between
// stored points came back EMPTY. INV-1 does not reach it: it deliberately
// uses buckets wide enough to hold several samples, so that a grouping which
// genuinely needs two of them is not asked for the impossible. That fairness
// is exactly what hides this case, so it gets its own sweep.
func TestLayer10BucketsFinerThanStoredDataAnswer(t *testing.T) {
	l10Fixture(t)
	r, err := readGroupingRoster()
	if err != nil {
		t.Fatal(err)
	}

	after, before := int64(l10SpanAfter), int64(l10SpanAfter)+20*tier1Gran

	ok := true
	for _, c := range r.Order {
		if groupingRules[c].knownSinglePointEmpty {
			continue // its own red case owns it
		}
		group := r.Canonical[c]

		// 3, 5 and 10 buckets per 60s stored window: every bucket after the
		// first of each window is covered by a re-delivery alone
		for _, perWindow := range []int64{3, 5, 10} {
			cols := l10Query(t, group, 1, after, before, 20*perWindow, "")
			for _, dim := range l10Dims {
				col := cols[dim]
				if len(col) == 0 {
					t.Logf("invariant not met: %s returned no rows for %q at %d buckets per stored window",
						group, dim, perWindow)
					ok = false
					continue
				}
				empty := 0
				for _, pt := range col {
					if pt.Value == nil {
						empty++
					}
				}
				if empty > 0 {
					t.Logf("invariant not met: %s left %d of %d buckets EMPTY for %q at %d buckets per "+
						"stored window - the buckets a re-delivered point covers on its own report "+
						"'no data here' instead of what the point says",
						group, empty, len(col), dim, perWindow)
					ok = false
				}
			}
		}
	}

	expectAgentStatus(t, "L10/buckets-finer-than-stored-data-answer", ok)
}

// INV-2: an order statistic answers WITH the data, not past it.
//
// Applies to every grouping whose answer is one of the values it was given or
// something between them. A value outside that range means the aggregation
// invented a number - from an interpolation it should not have used, or from
// state left behind by another point.
func TestLayer10OrderStatisticsStayInRange(t *testing.T) {
	l10Fixture(t)
	r, err := readGroupingRoster()
	if err != nil {
		t.Fatal(err)
	}

	after, before := int64(l10SpanAfter), int64(l10SpanBefore)

	ok := true
	checked := 0
	for _, c := range r.Order {
		if groupingRules[c].kind != kindInRange {
			continue
		}
		group := r.Canonical[c]

		for _, tier := range []int{0, 1} {
			cols := l10Query(t, group, tier, after, before, (before-after)/30, "")
			for _, dim := range l10Dims {
				lo, hi := l10Range(dim)
				// the storage keeps f32, and a tier point is an average of
				// f32s - a hair outside the integer range is the format,
				// not the aggregation
				const tol = 1e-3
				for _, pt := range cols[dim] {
					if pt.Value == nil {
						continue
					}
					checked++
					if *pt.Value < lo-tol || *pt.Value > hi+tol {
						t.Logf("invariant not met: %s (tier %d) reads %v for %q at t0%+d, "+
							"outside the samples' own range [%v, %v]",
							group, tier, *pt.Value, dim, pt.T-fixture.T0, lo, hi)
						ok = false
					}
				}
			}
		}
	}

	if checked == 0 {
		t.Fatalf("no order-statistic buckets were checked")
	}
	t.Logf("%d bucket values checked against their dimension's range", checked)
	expectAgentStatus(t, "L10/order-statistics-stay-in-range", ok)
}

// INV-3: what a dimension answers does not depend on its neighbours.
//
// Every grouping. The aggregations carry state across buckets on purpose -
// the predecessor of a `<previous` condition, the flap state, the smoothing
// level - and the query engine walks dimensions one after another through the
// SAME grouping instance, resetting between them. A reset that misses a field
// makes one dimension's answer depend on which dimensions were queried
// alongside it, and on the order they happened to be walked in. Nothing else
// in the corpus would notice.
func TestLayer10DimensionsAreIndependent(t *testing.T) {
	l10Fixture(t)
	r, err := readGroupingRoster()
	if err != nil {
		t.Fatal(err)
	}

	after, before := int64(l10SpanAfter), int64(l10SpanBefore)
	points := (before - after) / 30

	ok := true
	for _, c := range r.Order {
		group := r.Canonical[c]

		for _, tier := range []int{0, 1} {
			together := l10Query(t, group, tier, after, before, points, "")

			for _, dim := range l10Dims {
				alone := l10Query(t, group, tier, after, before, points, dim)

				a, b := alone[dim], together[dim]
				if len(a) != len(b) {
					t.Logf("invariant not met: %s (tier %d) returned %d buckets for %q alone "+
						"and %d alongside its neighbours",
						group, tier, len(a), dim, len(b))
					ok = false
					continue
				}
				for i := range a {
					if !samePoint(a[i], b[i]) {
						t.Logf("invariant not met: %s (tier %d) answers %v for %q at t0%+d when asked alone "+
							"and %v when asked alongside its neighbours - state is surviving the "+
							"switch from one dimension to the next",
							group, tier, valueOf(a[i]), dim, a[i].T-fixture.T0, valueOf(b[i]))
						ok = false
						break
					}
				}
			}
		}
	}

	expectAgentStatus(t, "L10/dimensions-are-independent", ok)
}

// INV-4: an event does not happen twice because the chart was zoomed in.
//
// Applies to the counting groupings. Above tier 0 a stored point covers many
// seconds; ask for buckets narrower than that and the engine hands the SAME
// point to each one. A grouping that counts occurrences must not count them
// again - the total over a fixed span may collapse as a rollup loses ordering,
// but it may never grow.
func TestLayer10CountsDoNotInflateWithZoom(t *testing.T) {
	l10Fixture(t)
	r, err := readGroupingRoster()
	if err != nil {
		t.Fatal(err)
	}

	after, before := int64(l10SpanAfter), int64(l10SpanAfter)+20*tier1Gran

	ok := true
	for _, c := range r.Order {
		rule := groupingRules[c]
		if rule.kind != kindCount || rule.countsDeliveries {
			continue
		}
		group := r.Canonical[c]

		// a condition that the wave dimension satisfies inside every stored
		// window, so there is something to over-count
		total := func(perWindow int64) float64 {
			cols := l10QueryCond(t, group, 1, after, before, 20*perWindow, ">30")
			sum := 0.0
			for _, pt := range cols["wave"] {
				if pt.Value != nil {
					sum += *pt.Value
				}
			}
			return sum
		}

		base := total(1)
		for _, perWindow := range []int64{2, 3, 5} {
			got := total(perWindow)
			if got > base+1e-6 {
				t.Logf("invariant not met: %s counted %v occurrences at %d buckets per stored window "+
					"but %v at one - the same data cut more finely cannot contain more events",
					group, got, perWindow, base)
				ok = false
			}
		}
	}

	expectAgentStatus(t, "L10/counts-do-not-inflate-with-zoom", ok)
}

// INV-4b: a TOTAL over a fixed span is the same number at any resolution.
//
// Applies to the accumulations, which are exactly decomposable: the total
// volume over an hour is a physical quantity - how many bytes crossed the
// interface - and it cannot depend on how many columns the chart was drawn
// with. Summing the buckets must give the same answer whether the hour was
// cut into 20 pieces or 1200.
//
// This is the same fault the condition groupings had, in an aggregation
// nobody had written a case for: above tier 0 a stored point is delivered to
// every bucket it spans, and `sum` fetches the WINDOW'S OWN SUM for it
// (TIER_QUERY_FETCH_SUM), so each of those buckets is handed the total of the
// whole window and adds it again.
func TestLayer10TotalsAreExactAcrossZoom(t *testing.T) {
	l10Fixture(t)
	r, err := readGroupingRoster()
	if err != nil {
		t.Fatal(err)
	}

	after, before := int64(l10SpanAfter), int64(l10SpanAfter)+20*tier1Gran

	// `flat` is a constant 7, so the total over the span is arithmetic, not
	// an oracle anyone has to trust: 7 per second for the whole span
	want := float64(l10Flat) * float64(before-after)

	ok := true
	for _, c := range r.Order {
		if !groupingRules[c].totalExact {
			continue
		}
		group := r.Canonical[c]

		for _, tier := range []int{0, 1} {
			for _, points := range []int64{20, 60, 300, 1200} {
				cols := l10Query(t, group, tier, after, before, points, "")
				got := 0.0
				for _, pt := range cols["flat"] {
					if pt.Value != nil {
						got += *pt.Value
					}
				}
				if math.Abs(got-want) > want*1e-6 {
					t.Logf("invariant not met: %s (tier %d) totals %.0f over the span at %d buckets, "+
						"but the span holds a constant %d for %ds - %.0f, whatever it is cut into "+
						"(x%.1f)",
						group, tier, got, points, l10Flat, before-after, want, got/want)
					ok = false
				}
			}
		}
	}

	expectAgentStatus(t, "L10/totals-are-exact-across-zoom", ok)
}

// INV-1b: a bucket holding ONE collected sample still answers.
//
// The companion to INV-1, split out because it is the case a grouping is most
// tempted to treat as "not enough data": one bucket per sample interval, the
// most natural resolution there is. A dashboard drawn at the collection
// interval must not come back blank.
//
// An aggregation that genuinely needs two samples - an increment is a
// difference between two of them - has the previous bucket's last sample to
// work from, and `incremental-sum` is built to carry exactly that across the
// flush. Needing two samples is a reason to look backwards, not a reason to
// answer nothing.
func TestLayer10SinglePointBucketsAnswer(t *testing.T) {
	l10Fixture(t)
	r, err := readGroupingRoster()
	if err != nil {
		t.Fatal(err)
	}

	// one bucket per second, on data collected once per second
	after := int64(l10SpanAfter)
	before := after + 600

	ok := true
	for _, c := range r.Order {
		group := r.Canonical[c]
		cols := l10Query(t, group, 0, after, before, before-after, "")

		for _, dim := range l10Dims {
			col := cols[dim]
			if len(col) == 0 {
				t.Logf("invariant not met: %s returned no rows for %q at one bucket per sample", group, dim)
				ok = false
				continue
			}
			empty := 0
			for _, pt := range col {
				if pt.Value == nil {
					empty++
				}
			}
			// the FIRST bucket of a dimension has nothing behind it, so an
			// aggregation that looks backwards may legitimately have nothing
			// to say there - one bucket, not all of them
			if empty > 1 {
				t.Logf("invariant not met: %s left %d of %d buckets EMPTY for %q at one bucket per "+
					"collected sample - a chart drawn at the collection interval comes back blank",
					group, empty, len(col), dim)
				ok = false
			}
		}
	}

	expectAgentStatus(t, "L10/single-point-buckets-answer", ok)
}

// INV-5: a share of TIME over a fixed span is the same at every zoom.
//
// Applies to the groupings whose denominator is the selected duration. The
// share of a window that satisfied a condition is a property of the data and
// the window, not of how many buckets the window was drawn with - so the
// duration-weighted mean over a fixed span must not move. This is the rule
// `percentage-of-time(<previous)` broke: 5% of the span at one bucket per
// stored window, 77% at five.
//
// `percentage-of-samples` is deliberately NOT here: it answers about the
// samples it was handed and a re-delivery is another sample to it.
func TestLayer10TimeSharesAreStableAcrossZoom(t *testing.T) {
	l10Fixture(t)
	r, err := readGroupingRoster()
	if err != nil {
		t.Fatal(err)
	}

	after, before := int64(l10SpanAfter), int64(l10SpanAfter)+20*tier1Gran

	ok := true
	for _, c := range r.Order {
		if !groupingRules[c].weightedMeanStable {
			continue
		}
		group := r.Canonical[c]

		for _, cond := range []string{">30", "<previous", "==gap", ">=0"} {
			// EVERY dimension. `ramp` is the one that matters here: it only
			// ever climbs, so any time it reports below its predecessor is
			// time that did not happen. On the sawtooth a phantom drop and a
			// real one look identical, which is how a sweep reading only
			// `wave` would let the bug through.
			for _, dim := range l10Dims {
				mean := func(perWindow int64) float64 {
					cols := l10QueryCond(t, group, 1, after, before, 20*perWindow, cond)
					col := cols[dim]
					if len(col) == 0 {
						t.Fatalf("%s(%s): no rows for %q", group, cond, dim)
					}
					sum := 0.0
					for _, pt := range col {
						if pt.Value != nil {
							sum += *pt.Value
						}
					}
					return sum / float64(len(col))
				}

				base := mean(1)
				for _, perWindow := range []int64{2, 3, 5} {
					got := mean(perWindow)
					// the buckets are equal width, so the plain mean IS the
					// duration-weighted one; half a point of slack covers the
					// grid landing differently, not a different answer
					if math.Abs(got-base) >= 0.5 {
						t.Logf("invariant not met: %s(%s) reports %.2f%% of the span for %q at %d "+
							"buckets per stored window and %.2f%% at one - the same data, the same window",
							group, cond, got, dim, perWindow, base)
						ok = false
					}
				}
			}
		}
	}

	expectAgentStatus(t, "L10/time-shares-stable-across-zoom", ok)
}

// INV-6: an alias is the same grouping, not a similar one.
//
// `avg` must answer exactly what `average` answers. The registry maps several
// names onto one implementation and nothing else checks that the mapping is
// what it claims - a copy-paste in the table would silently route `ewma` to
// the wrong aggregation.
func TestLayer10AliasesResolveToTheSameGrouping(t *testing.T) {
	l10Fixture(t)
	r, err := readGroupingRoster()
	if err != nil {
		t.Fatal(err)
	}

	after, before := int64(l10SpanAfter), int64(l10SpanBefore)
	points := (before - after) / 30

	ok := true
	checked := 0
	for _, c := range r.Order {
		if len(r.Aliases[c]) == 0 {
			continue
		}
		canonical := l10Query(t, r.Canonical[c], 0, after, before, points, "")

		for _, alias := range r.Aliases[c] {
			checked++
			got := l10Query(t, alias, 0, after, before, points, "")
			for _, dim := range l10Dims {
				a, b := got[dim], canonical[dim]
				if len(a) != len(b) {
					t.Logf("invariant not met: time_group=%s returned %d buckets where %s returned %d",
						alias, len(a), r.Canonical[c], len(b))
					ok = false
					continue
				}
				for i := range a {
					if !samePoint(a[i], b[i]) {
						t.Logf("invariant not met: time_group=%s answers %v at t0%+d where its canonical "+
							"name %s answers %v - the alias resolves somewhere else",
							alias, valueOf(a[i]), a[i].T-fixture.T0, r.Canonical[c], valueOf(b[i]))
						ok = false
						break
					}
				}
			}
		}
	}

	if checked == 0 {
		t.Fatalf("no aliases were checked")
	}
	t.Logf("%d aliases checked against their canonical name", checked)
	expectAgentStatus(t, "L10/aliases-resolve-to-the-same-grouping", ok)
}

// INV-7: asking twice answers twice the same.
//
// Every grouping. The aggregations keep state for the length of a query; if
// any of it outlives the query - a static, an arena not cleared, a field the
// create() path does not initialise - the second answer differs from the
// first and every answer after that is a function of what was asked before
// it.
func TestLayer10QueriesAreDeterministic(t *testing.T) {
	l10Fixture(t)
	r, err := readGroupingRoster()
	if err != nil {
		t.Fatal(err)
	}

	after, before := int64(l10SpanAfter), int64(l10SpanBefore)
	points := (before - after) / 30

	ok := true
	for _, c := range r.Order {
		group := r.Canonical[c]
		first := l10Query(t, group, 1, after, before, points, "")
		second := l10Query(t, group, 1, after, before, points, "")

		for _, dim := range l10Dims {
			a, b := first[dim], second[dim]
			if len(a) != len(b) {
				t.Logf("invariant not met: %s returned %d buckets then %d for %q", group, len(a), len(b), dim)
				ok = false
				continue
			}
			for i := range a {
				if !samePoint(a[i], b[i]) {
					t.Logf("invariant not met: %s answered %v then %v for %q at t0%+d - "+
						"state outlived the query",
						group, valueOf(a[i]), valueOf(b[i]), dim, a[i].T-fixture.T0)
					ok = false
					break
				}
			}
		}
	}

	expectAgentStatus(t, "L10/queries-are-deterministic", ok)
}

// INV-8: buckets come back in order, once each.
//
// Every grouping. A repeated or out-of-order timestamp means the grid walk
// lost its place, and every value after it is attributed to the wrong moment.
func TestLayer10BucketsAreOrderedAndUnique(t *testing.T) {
	l10Fixture(t)
	r, err := readGroupingRoster()
	if err != nil {
		t.Fatal(err)
	}

	after, before := int64(l10SpanAfter), int64(l10SpanBefore)

	ok := true
	for _, c := range r.Order {
		group := r.Canonical[c]
		for _, tier := range []int{0, 1} {
			for _, points := range []int64{7, 60, 300} {
				cols := l10Query(t, group, tier, after, before, points, "")
				for _, dim := range l10Dims {
					prev := int64(math.MinInt64)
					for i, pt := range cols[dim] {
						if pt.T <= prev {
							t.Logf("invariant not met: %s (tier %d, points=%d) bucket %d of %q is t0%+d, "+
								"not after the previous t0%+d",
								group, tier, points, i, dim, pt.T-fixture.T0, prev-fixture.T0)
							ok = false
							break
						}
						prev = pt.T
					}
				}
			}
		}
	}

	expectAgentStatus(t, "L10/buckets-are-ordered-and-unique", ok)
}

// l10QueryCond runs a grouping that takes a condition.
func l10QueryCond(t *testing.T, group string, tier int, after, before, points int64, cond string) map[string][]canon.Pt {
	t.Helper()
	params := daemon.DataParamsTier(l10Context, tier, after, before, points, group)
	params.Set("time_group_options", cond)
	params.Set("options", "jsonwrap|unaligned")
	doc, err := td.DataV3("l10", params)
	if err != nil {
		t.Fatalf("%s(%s): %v", group, cond, err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatalf("%s(%s): %v", group, cond, err)
	}
	return cols
}

func samePoint(a, b canon.Pt) bool {
	if a.T != b.T {
		return false
	}
	if (a.Value == nil) != (b.Value == nil) {
		return false
	}
	if a.Value == nil {
		return true
	}
	// bit-identical, not "close": these are two runs of the same query over
	// the same data, so anything but equality is a defect
	return *a.Value == *b.Value || (math.IsNaN(*a.Value) && math.IsNaN(*b.Value))
}

func valueOf(p canon.Pt) any {
	if p.Value == nil {
		return "EMPTY"
	}
	return *p.Value
}

// ---------------------------------------------------------------------------
// The three shapes the sweep above does NOT reach, each added because a real
// defect walked straight through it.
//
// The sweep reads one span that starts ON the tier grid, over a fixture with
// no holes in it, never asking for anomaly rates. Every one of those choices
// was deliberate and every one of them hid a bug:
//
//   - an unaligned span puts the first stored point ACROSS the first bucket's
//     start, which is where an apportionment that clamps only against "what I
//     already accounted for" hands the bucket everything from the point's own
//     beginning instead of from the bucket's;
//   - a fixture with holes is the only way to reach a point the engine does
//     NOT trim - query_interpolate_point() trims a wide point to the bucket
//     end only when the point before it is adjacent and numeric, which after
//     a gap it is not;
//   - options=anomaly-bit replaces the delivered value with an anomaly RATE
//     while the stored statistics stay in the metric's own domain, so an
//     aggregation reading the wrong one answers in the wrong units entirely.

const (
	l10GapContext = "fixture.l10gap"
	l10GapSamples = 2400
	l10GapValue   = 7
	// collected for the first half of every minute, silent for the second
	l10GapCollected = 30
	l10GapPeriod    = 60
)

var l10GapReady bool

func l10GapFixture(t *testing.T) {
	t.Helper()
	if l10GapReady {
		return
	}

	ch := fixture.Chart{
		ID: l10GapContext, Title: "holes", Units: "units",
		Family: "fixture", Context: l10GapContext, UpdateEvery: 1,
		Dimensions: []fixture.Dimension{{ID: "holes"}, {ID: "anom"}},
	}
	for i := 1; i <= l10GapSamples; i++ {
		ts := fixture.T0 + int64(i)
		if i%l10GapPeriod < l10GapCollected {
			ch.Dimensions[0].Points = append(ch.Dimensions[0].Points,
				fixture.Point{T: ts, Collected: strconv.Itoa(l10GapValue), Flags: stream.FlagNotAnomalous})
		} else {
			ch.Dimensions[0].Points = append(ch.Dimensions[0].Points,
				fixture.Point{T: ts, Flags: stream.FlagEmpty})
		}
		// a large value that is NEVER anomalous: under options=anomaly-bit
		// every answer about it has to come from the rate (0), so anything
		// carrying this magnitude came from the metric instead
		ch.Dimensions[1].Points = append(ch.Dimensions[1].Points,
			fixture.Point{T: ts, Collected: "1000000", Flags: stream.FlagNotAnomalous})
	}

	pushLiveBurst(t, "l10gap", guid(231), ch)
	if _, err := td.WaitRetention("l10gap", ch.Context, ch.FirstT(), ch.LastT(), 30*time.Second); err != nil {
		t.Fatal(err)
	}
	l10GapReady = true
}

// l10GapCollectedIn counts the samples the fixture actually pushed in
// (after, before] - the oracle for what a sum over that span must be.
func l10GapCollectedIn(after, before int64) int {
	n := 0
	for i := 1; i <= l10GapSamples; i++ {
		ts := fixture.T0 + int64(i)
		if ts > after && ts <= before && i%l10GapPeriod < l10GapCollected {
			n++
		}
	}
	return n
}

// INV-4c: a total is exact over a span with HOLES in it, and over a span that
// does not start on the tier grid.
//
// Same rule as L10/totals-are-exact-across-zoom, on the two shapes that rule
// never sees. A sum is the sum of the samples that were collected: gaps add
// nothing, and where the span begins cannot change what is inside it.
func TestLayer10TotalsAreExactOverGapsAndOffGrid(t *testing.T) {
	l10GapFixture(t)
	r, err := readGroupingRoster()
	if err != nil {
		t.Fatal(err)
	}

	ok := true
	for _, span := range []struct {
		after, before int64
		what          string
	}{
		{int64(fixture.T0 + 40 + tier1Gran), int64(fixture.T0+40+tier1Gran) + 20*tier1Gran, "on the tier grid"},
		{int64(fixture.T0 + 40 + tier1Gran + 30), int64(fixture.T0+40+tier1Gran+30) + 20*tier1Gran, "half a stored point off the grid"},
		{int64(fixture.T0 + 40 + tier1Gran + 17), int64(fixture.T0+40+tier1Gran+17) + 20*tier1Gran, "seventeen seconds off the grid"},
	} {
		want := float64(l10GapCollectedIn(span.after, span.before) * l10GapValue)

		for _, c := range r.Order {
			if !groupingRules[c].totalExact {
				continue
			}
			group := r.Canonical[c]

			for _, tier := range []int{0, 1} {
				for _, points := range []int64{20, 60, 300, 1200} {
					params := daemon.DataParamsTier(l10GapContext, tier, span.after, span.before, points, group)
					params.Set("options", "jsonwrap|unaligned")
					doc, err := td.DataV3("l10gap", params)
					if err != nil {
						t.Fatalf("%s: %v", group, err)
					}
					cols, err := canon.Columns(doc)
					if err != nil {
						t.Fatalf("%s: %v", group, err)
					}

					got := 0.0
					for _, pt := range cols["holes"] {
						if pt.Value != nil {
							got += *pt.Value
						}
					}
					// a stored point straddling either end of the span is
					// legitimately split, so allow one stored point of slack
					// at each edge rather than demanding the exact integer
					slack := float64(l10GapCollected*l10GapValue) * 1.05
					if math.Abs(got-want) > slack {
						t.Logf("invariant not met: %s (tier %d, %s) totals %.0f at %d buckets, "+
							"but the span holds %.0f - the fixture collected %d samples of %d in it",
							group, tier, span.what, got, points, want,
							l10GapCollectedIn(span.after, span.before), l10GapValue)
						ok = false
					}
				}
			}
		}
	}

	expectAgentStatus(t, "L10/totals-exact-over-gaps-and-off-grid", ok)
}

// INV-9: options=anomaly-bit answers about ANOMALY RATES, never about the
// metric.
//
// The option replaces the delivered value with the stored window's anomaly
// rate, a percentage, while min/max/sum/count go on describing the metric. An
// aggregation that reaches past the delivered value into those statistics
// answers in the metric's units - and the two domains are unrelated, so the
// number it produces is not wrong by a little, it is meaningless.
//
// The fixture makes that impossible to miss: a dimension holding 1000000,
// never anomalous. Every answer about it under anomaly-bit is an answer about
// zero.
func TestLayer10AnomalyBitAnswersAboutRates(t *testing.T) {
	l10GapFixture(t)
	r, err := readGroupingRoster()
	if err != nil {
		t.Fatal(err)
	}

	after := int64(fixture.T0 + 40 + tier1Gran)
	before := after + 20*tier1Gran

	ok := true
	for _, c := range r.Order {
		// The condition groupings are excluded, and not as a concession: they
		// answer about a CONDITION applied to the rate, not about the rate.
		// A never-anomalous series has rate 0, so `countif(=0)` reads 100%
		// and is right to. Everything else answers WITH the rate, and the
		// rate here is zero everywhere.
		if k := groupingRules[c].kind; k == kindPercent || k == kindCount {
			continue
		}
		group := r.Canonical[c]

		for _, tier := range []int{0, 1} {
			for _, points := range []int64{20, 300, 1200} {
				params := daemon.DataParamsTier(l10GapContext, tier, after, before, points, group)
				params.Set("options", "jsonwrap|unaligned|anomaly-bit")
				doc, err := td.DataV3("l10gap", params)
				if err != nil {
					t.Fatalf("%s: %v", group, err)
				}
				cols, err := canon.Columns(doc)
				if err != nil {
					t.Fatalf("%s: %v", group, err)
				}

				for _, pt := range cols["anom"] {
					if pt.Value == nil {
						continue
					}
					// nothing in this series was ever anomalous, so every
					// grouping of its anomaly rate is zero. A value anywhere
					// near the metric's own magnitude came from the metric.
					if math.Abs(*pt.Value) > 1e-6 {
						t.Logf("invariant not met: %s (tier %d, points=%d) reads %v under "+
							"options=anomaly-bit at t0%+d, on a dimension that was never anomalous - "+
							"the metric itself holds %d, so this is an answer about the wrong domain",
							group, tier, points, *pt.Value, pt.T-fixture.T0, 1000000)
						ok = false
						break
					}
				}
			}
		}
	}

	expectAgentStatus(t, "L10/anomaly-bit-answers-about-rates", ok)
}
