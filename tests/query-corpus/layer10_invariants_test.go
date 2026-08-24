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

	// why it is excused from a rule its kind would otherwise carry
	why                   string
	firstBucketMayBeEmpty bool
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
	"RRDR_GROUPING_INCREMENTAL_SUM": {kind: kindOther, firstBucketMayBeEmpty: true,
		why: "a difference between the ends of a bucket, which is not a value in it. " +
			"The opening single-point bucket may be EMPTY because no preceding baseline exists, " +
			"but its last sample must seed the carried chain for every later bucket"},
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

var (
	l10Ready        bool
	l10FixtureChart fixture.Chart
)

type l10QuerySpec struct {
	host, context                  string
	requestedGroup, canonicalGroup string
	condition, scopeDimensions     string
	extraOptions                   string
	responseDeclaredGrid           bool
	tier                           int
	after, before, points          int64
	expectedDimensions             []string
}

type l10QueryResult struct {
	doc   map[string]any
	cols  map[string][]canon.Pt
	grid  []int64
	valid bool
}

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
	l10FixtureChart = ch
	l10Ready = true
}

func TestL10QueryResultGuards(t *testing.T) {
	spec := l10QuerySpec{
		requestedGroup: "number-of-times",
		canonicalGroup: "number-of-times",
		condition:      ">0",
		tier:           0,
		after:          0,
		before:         20,
		points:         2,
		expectedDimensions: []string{
			"value",
		},
	}
	build := func() map[string]any {
		return map[string]any{
			"request": map[string]any{"aggregations": map[string]any{"time": map[string]any{
				"time_group":         "number-of-times",
				"time_group_options": ">0",
			}}},
			"view": map[string]any{
				"after": float64(1), "before": float64(20), "update_every": float64(10),
			},
			"db": map[string]any{"per_tier": []any{
				map[string]any{"tier": float64(0), "points": float64(2)},
				map[string]any{"tier": float64(1), "points": float64(0)},
				map[string]any{"tier": float64(2), "points": float64(0)},
			}},
			"result": map[string]any{
				"labels": []any{"time", "value"},
				"point":  map[string]any{"value": float64(0), "arp": float64(1), "pa": float64(2)},
				"data": []any{
					[]any{float64(10), []any{float64(1), float64(0), float64(0)}},
					[]any{float64(20), []any{float64(2), float64(0), float64(0)}},
				},
			},
		}
	}
	if result := validateL10Response(t, spec, build()); !result.valid ||
		len(result.grid) != 2 || result.grid[0] != 10 || result.grid[1] != 20 {
		t.Fatal("L10 response guard rejected the valid control")
	}

	mutations := map[string]func(map[string]any){
		"missing-row": func(doc map[string]any) {
			result := doc["result"].(map[string]any)
			result["data"] = result["data"].([]any)[:1]
		},
		"duplicate-timestamp": func(doc map[string]any) {
			rows := doc["result"].(map[string]any)["data"].([]any)
			rows[1].([]any)[0] = float64(10)
		},
		"shifted-timestamp": func(doc map[string]any) {
			rows := doc["result"].(map[string]any)["data"].([]any)
			rows[0].([]any)[0] = float64(11)
		},
		"out-of-order": func(doc map[string]any) {
			rows := doc["result"].(map[string]any)["data"].([]any)
			rows[0], rows[1] = rows[1], rows[0]
		},
		"wrong-group": func(doc map[string]any) {
			timeGroup := doc["request"].(map[string]any)["aggregations"].(map[string]any)["time"].(map[string]any)
			timeGroup["time_group"] = "average"
		},
		"wrong-condition": func(doc map[string]any) {
			timeGroup := doc["request"].(map[string]any)["aggregations"].(map[string]any)["time"].(map[string]any)
			timeGroup["time_group_options"] = "<0"
		},
		"wrong-tier": func(doc map[string]any) {
			tiers := doc["db"].(map[string]any)["per_tier"].([]any)
			tiers[0].(map[string]any)["points"] = float64(0)
			tiers[1].(map[string]any)["points"] = float64(2)
		},
		"missing-dimension": func(doc map[string]any) {
			result := doc["result"].(map[string]any)
			result["labels"] = []any{"time"}
			result["data"] = []any{}
		},
		"extra-dimension": func(map[string]any) {},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			doc := build()
			if name == "extra-dimension" {
				result := doc["result"].(map[string]any)
				result["labels"] = []any{"time", "value", "extra"}
				rows := result["data"].([]any)
				for i, rowAny := range rows {
					rows[i] = append(rowAny.([]any),
						[]any{float64(0), float64(0), float64(0)})
				}
			} else {
				mutate(doc)
			}
			if validateL10Response(t, spec, doc).valid {
				t.Errorf("L10 response guard accepted the %s mutation", name)
			}
		})
	}

	t.Run("response-declared-grid-cannot-drop-leading-bucket", func(t *testing.T) {
		doc := build()
		view := doc["view"].(map[string]any)
		view["after"] = float64(11)
		result := doc["result"].(map[string]any)
		result["data"] = result["data"].([]any)[1:]
		declared := spec
		declared.responseDeclaredGrid = true
		if validateL10Response(t, declared, doc).valid {
			t.Error("response-declared grid accepted a missing leading bucket")
		}
	})
	t.Run("ordinary-nondivisible-grid-cannot-drop-leading-bucket", func(t *testing.T) {
		doc := build()
		view := doc["view"].(map[string]any)
		view["after"] = float64(11)
		result := doc["result"].(map[string]any)
		result["data"] = result["data"].([]any)[1:]
		nondivisible := spec
		nondivisible.points = 3
		if validateL10Response(t, nondivisible, doc).valid {
			t.Error("ordinary nondivisible grid accepted a missing leading bucket")
		}
	})
	t.Run("ordinary-single-bucket-exclusive-lower-bound", func(t *testing.T) {
		doc := build()
		doc["view"] = map[string]any{
			"after": float64(1), "before": float64(20), "update_every": float64(20),
		}
		result := doc["result"].(map[string]any)
		result["data"] = []any{
			[]any{float64(20), []any{float64(2), float64(0), float64(0)}},
		}
		single := spec
		single.points = 1
		if !validateL10Response(t, single, doc).valid {
			t.Error("ordinary single-bucket validator rejected the exact (after,before] view")
		}

		doc = build()
		single = spec
		single.points = 1
		if validateL10Response(t, single, doc).valid {
			t.Error("ordinary single-bucket validator accepted a two-row response")
		}
	})

	value, other := 1.0, 2.0
	count, otherCount := int64(3), int64(4)
	hidden, otherHidden := 4.0, 5.0
	base := canon.Pt{T: 10, Value: &value, ARP: 5, PA: 6, Count: &count, Hidden: &hidden}
	if !samePoint(base, base) {
		t.Fatal("complete point equality rejected the valid control")
	}
	pointMutations := map[string]canon.Pt{
		"value":          {T: 10, Value: &other, ARP: 5, PA: 6, Count: &count, Hidden: &hidden},
		"arp":            {T: 10, Value: &value, ARP: 7, PA: 6, Count: &count, Hidden: &hidden},
		"pa":             {T: 10, Value: &value, ARP: 5, PA: 7, Count: &count, Hidden: &hidden},
		"missing-count":  {T: 10, Value: &value, ARP: 5, PA: 6, Hidden: &hidden},
		"count":          {T: 10, Value: &value, ARP: 5, PA: 6, Count: &otherCount, Hidden: &hidden},
		"missing-hidden": {T: 10, Value: &value, ARP: 5, PA: 6, Count: &count},
		"hidden":         {T: 10, Value: &value, ARP: 5, PA: 6, Count: &count, Hidden: &otherHidden},
	}
	for name, point := range pointMutations {
		t.Run("point/"+name, func(t *testing.T) {
			if samePoint(base, point) {
				t.Errorf("complete point equality accepted the %s mutation", name)
			}
		})
	}

	zero, ten := 0.0, 10.0
	if !l10EnvelopeHolds("average", [][]float64{{0, 10}}, []*float64{&zero}, 0) ||
		!l10EnvelopeHolds("average", [][]float64{{0, 10}}, []*float64{&ten}, 0) {
		t.Fatal("L10 envelope rejected an exact lower or upper bound")
	}
	below, above := -0.1, 10.1
	if l10EnvelopeHolds("average", [][]float64{{0, 10}}, []*float64{&below}, 0) ||
		l10EnvelopeHolds("average", [][]float64{{0, 10}}, []*float64{&above}, 0) {
		t.Fatal("L10 envelope accepted a value outside the bucket source range")
	}
	if l10EnvelopeHolds("average", [][]float64{{}}, []*float64{nil}, 0) {
		t.Fatal("L10 envelope accepted an empty source and null answer")
	}
	fifty := 50.0
	if !l10EnvelopeHolds("ses", [][]float64{{0}, {100}}, []*float64{&zero, &fifty}, 0) {
		t.Fatal("L10 SES envelope rejected a value inside its cumulative source range")
	}
	if l10EnvelopeHolds("average", [][]float64{{0}, {100}}, []*float64{&zero, &fifty}, 0) {
		t.Fatal("L10 bucket-local envelope accepted a value outside its current bucket")
	}

	t.Run("gap-total-slack-is-only-for-cut-tier1-records", func(t *testing.T) {
		alignedAfter := int64(fixture.T0 + 40 + tier1Gran)
		alignedBefore := alignedAfter + 20*tier1Gran
		if alignedAfter%tier1Gran != 0 || alignedBefore%tier1Gran != 0 {
			t.Fatal("L10 exact-span control is not aligned to the tier1 grid")
		}

		const oneRecord = float64(l10GapCollected * l10GapValue)
		for _, tc := range []struct {
			name          string
			tier          int
			after, before int64
			want          float64
		}{
			{name: "tier0 aligned", tier: 0, after: alignedAfter, before: alignedBefore},
			{name: "tier0 off-grid", tier: 0, after: alignedAfter + 17, before: alignedBefore + 17},
			{name: "tier1 aligned", tier: 1, after: alignedAfter, before: alignedBefore},
			{name: "tier1 cut lower edge", tier: 1, after: alignedAfter + 17, before: alignedBefore, want: oneRecord},
			{name: "tier1 cut upper edge", tier: 1, after: alignedAfter, before: alignedBefore + 17, want: oneRecord},
			{name: "tier1 cut both edges", tier: 1, after: alignedAfter + 17, before: alignedBefore + 17, want: oneRecord},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if got := l10GapTotalSlack(tc.tier, tc.after, tc.before); got != tc.want {
					t.Fatalf("gap total slack = %v, want %v", got, tc.want)
				}
			})
		}

		for _, delta := range []float64{-oneRecord, oneRecord} {
			if math.Abs(delta) <= l10GapTotalSlack(0, alignedAfter, alignedBefore) ||
				math.Abs(delta) <= l10GapTotalSlack(1, alignedAfter, alignedBefore) {
				t.Fatalf("an exact path accepted a one-record total mutation of %+v", delta)
			}
		}
		if math.Abs(oneRecord) > l10GapTotalSlack(1, alignedAfter+17, alignedBefore+17) {
			t.Fatal("cut tier1 span rejected its one-record allowance")
		}
		if math.Abs(oneRecord+1) <= l10GapTotalSlack(1, alignedAfter+17, alignedBefore+17) {
			t.Fatal("cut tier1 span accepted more than one record of slack")
		}
	})
}

func l10EnvelopeHolds(group string, sources [][]float64, got []*float64, tolerance float64) bool {
	if len(sources) == 0 || len(got) != len(sources) || tolerance < 0 {
		return false
	}
	cumulative := group == "ses"
	lo, hi := math.Inf(1), math.Inf(-1)
	for i, source := range sources {
		if len(source) == 0 || got[i] == nil ||
			math.IsNaN(*got[i]) || math.IsInf(*got[i], 0) {
			return false
		}
		if !cumulative {
			lo, hi = math.Inf(1), math.Inf(-1)
		}
		for _, value := range source {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return false
			}
			lo, hi = math.Min(lo, value), math.Max(hi, value)
		}
		if *got[i] < lo-tolerance || *got[i] > hi+tolerance {
			return false
		}
	}
	return true
}

func l10NumericPoints(column []canon.Pt) int {
	count := 0
	for _, point := range column {
		if point.Value != nil {
			count++
		}
	}
	return count
}

func l10InvalidEmptyBuckets(column []canon.Pt, firstBucketMayBeEmpty bool) (count, first int) {
	for i, point := range column {
		if point.Value != nil || (i == 0 && firstBucketMayBeEmpty) {
			continue
		}
		if count == 0 {
			first = i + 1
		}
		count++
	}
	return count, first
}

func l10SourceBuckets(
	t *testing.T,
	query l10QueryResult,
	group string,
	tier int,
	dimension string,
) [][]float64 {
	t.Helper()
	var fixtureDimension *fixture.Dimension
	for i := range l10FixtureChart.Dimensions {
		if l10FixtureChart.Dimensions[i].ID == dimension {
			fixtureDimension = &l10FixtureChart.Dimensions[i]
			break
		}
	}
	if fixtureDimension == nil {
		t.Fatalf("L10 fixture has no dimension %q", dimension)
	}

	view := query.doc["view"].(map[string]any)
	viewAfter, afterOK := queryInteger(view["after"])
	updateEvery, everyOK := queryInteger(view["update_every"])
	if !afterOK || !everyOK || updateEvery <= 0 || len(query.grid) == 0 {
		t.Fatalf("L10 response has no usable view for source oracle: %v", query.doc["view"])
	}

	var dbPoints []fixture.DBPoint
	switch tier {
	case 0:
		dbPoints = fixtureDimension.DBPoints(1)
	case 1:
		windows := fixtureDimension.TierWindows(tier1Gran, int64(l10FixtureChart.UpdateEvery))
		ends := make([]int64, 0, len(windows))
		for end := range windows {
			ends = append(ends, end)
		}
		sort.Slice(ends, func(i, j int) bool { return ends[i] < ends[j] })
		dbPoints = make([]fixture.DBPoint, 0, len(ends))
		for _, end := range ends {
			point := windows[end]
			dbPoint := fixture.DBPoint{
				Start: end - tier1Gran,
				End:   end,
				Gap:   point.Empty,
			}
			if !point.Empty {
				dbPoint.Value = fixture.TierFetchValue(group, point)
			}
			dbPoints = append(dbPoints, dbPoint)
		}
	default:
		t.Fatalf("L10 source oracle does not model tier %d", tier)
	}
	return fixture.ViewBuckets(dbPoints, viewAfter-1, updateEvery, len(query.grid))
}

func validateL10Response(t *testing.T, spec l10QuerySpec, doc map[string]any) l10QueryResult {
	t.Helper()
	result := l10QueryResult{doc: doc, valid: true}

	request, requestOK := doc["request"].(map[string]any)
	aggregations, aggregationsOK := request["aggregations"].(map[string]any)
	timeAggregation, timeOK := aggregations["time"].(map[string]any)
	group, groupOK := timeAggregation["time_group"].(string)
	if !requestOK || !aggregationsOK || !timeOK || !groupOK || group != spec.canonicalGroup {
		t.Logf("%s request time_group echo is %v, want %q",
			spec.requestedGroup, timeAggregation["time_group"], spec.canonicalGroup)
		result.valid = false
	}
	if spec.condition != "" {
		condition, ok := timeAggregation["time_group_options"].(string)
		if !ok || condition != spec.condition {
			t.Logf("%s request condition echo is %v, want %q",
				spec.requestedGroup, timeAggregation["time_group_options"], spec.condition)
			result.valid = false
		}
	}

	view, viewOK := doc["view"].(map[string]any)
	viewAfter, afterOK := queryInteger(view["after"])
	viewBefore, beforeOK := queryInteger(view["before"])
	updateEvery, everyOK := queryInteger(view["update_every"])
	if !viewOK || !afterOK || !beforeOK || !everyOK ||
		updateEvery <= 0 || viewBefore < viewAfter {
		t.Logf("%s view is malformed: %v", spec.requestedGroup, doc["view"])
		result.valid = false
	} else {
		span := viewBefore - viewAfter + 1
		if span%updateEvery != 0 {
			t.Logf("%s view span %d is not divisible by update_every %d",
				spec.requestedGroup, span, updateEvery)
			result.valid = false
		} else {
			rows := span / updateEvery
			result.grid = make([]int64, rows)
			for i := range result.grid {
				result.grid[i] = viewAfter + updateEvery - 1 + int64(i)*updateEvery
			}
		}
		if !spec.responseDeclaredGrid && spec.points > 0 &&
			(spec.before-spec.after)%spec.points == 0 {
			wantEvery := (spec.before - spec.after) / spec.points
			if viewAfter != spec.after+1 || viewBefore != spec.before ||
				updateEvery != wantEvery || int64(len(result.grid)) != spec.points {
				t.Logf("%s view is %d/%d/%d with %d rows, want %d/%d/%d with %d rows",
					spec.requestedGroup, viewAfter, viewBefore, updateEvery, len(result.grid),
					spec.after+1, spec.before, wantEvery, spec.points)
				result.valid = false
			}
		} else {
			if viewBefore != spec.before {
				t.Logf("%s nondivisible request ends at %d, want requested before %d",
					spec.requestedGroup, viewBefore, spec.before)
				result.valid = false
			}
			if viewAfter > spec.after+updateEvery {
				t.Logf("%s response-derived grid starts at %d, after the first requested bucket ending at %d",
					spec.requestedGroup, viewAfter, spec.after+updateEvery)
				result.valid = false
			}
		}
	}

	if err := queryRawTimestampsExact(doc, result.grid); err != nil {
		t.Logf("%s raw result grid: %v", spec.requestedGroup, err)
		result.valid = false
	}

	cols, err := canon.Columns(doc)
	if err != nil {
		t.Logf("%s canonical decode failed: %v", spec.requestedGroup, err)
		result.valid = false
	} else {
		result.cols = cols
		if !assertExactColumnSet(t, cols, spec.expectedDimensions) {
			result.valid = false
		}
		for _, dimension := range spec.expectedDimensions {
			column := cols[dimension]
			if len(column) != len(result.grid) {
				t.Logf("%s dimension %q has %d rows, want %d",
					spec.requestedGroup, dimension, len(column), len(result.grid))
				result.valid = false
				continue
			}
			for i, point := range column {
				if point.T != result.grid[i] {
					t.Logf("%s dimension %q row %d ends at %d, want %d",
						spec.requestedGroup, dimension, i, point.T, result.grid[i])
					result.valid = false
				}
			}
		}
	}
	if !assertSelectedTier(t, doc, spec.tier) {
		result.valid = false
	}
	return result
}

// l10Query runs and validates one grouping query without a second API call.
func l10Query(t *testing.T, spec l10QuerySpec) l10QueryResult {
	t.Helper()
	if spec.host == "" {
		spec.host = "l10"
	}
	if spec.context == "" {
		spec.context = l10Context
	}
	if spec.canonicalGroup == "" {
		spec.canonicalGroup = spec.requestedGroup
	}
	if len(spec.expectedDimensions) == 0 {
		spec.expectedDimensions = l10Dims
	}
	if spec.tier < 0 {
		t.Fatalf("%s: L10 validated queries require an explicit tier", spec.requestedGroup)
	}

	params := daemon.DataParamsTier(
		spec.context, spec.tier, spec.after, spec.before, spec.points, spec.requestedGroup)
	options := "jsonwrap|unaligned|flip|debug"
	if spec.extraOptions != "" {
		options += "|" + spec.extraOptions
	}
	params.Set("options", options)
	if spec.condition != "" {
		params.Set("time_group_options", spec.condition)
	}
	if spec.scopeDimensions != "" {
		params.Set("scope_dimensions", spec.scopeDimensions)
	}

	doc, err := td.DataV3(spec.host, params)
	if err != nil {
		t.Fatalf("%s: %v", spec.requestedGroup, err)
	}
	return validateL10Response(t, spec, doc)
}

// The roster guard. Everything else in this layer is only as complete as this
// is: a grouping the engine offers and this file does not classify is a
// grouping no invariant is checked against.
func TestLayer10RosterIsComplete(t *testing.T) {
	trackContract(t, "L10/roster-is-complete")

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
// A bucket whose whole width was collected has something to say about it;
// answering EMPTY says "there is no data here", which is what an outage looks
// like. The sole exception is the opening incremental-sum bucket: without a
// predecessor its delta is undefined, but it must seed every later bucket.
// This is the rule `number-of-flaps` and `number-of-times` broke for every
// bucket a re-delivered point covered on its own.
//
// The buckets below are wide enough to hold several samples, so a grouping
// that genuinely needs two of them (an increment, a dispersion) is not being
// asked for the impossible.
func TestLayer10NoHolesInsideData(t *testing.T) {
	trackContract(t, "L10/no-holes-inside-data")

	l10Fixture(t)
	r, err := readGroupingRoster()
	if err != nil {
		t.Fatal(err)
	}

	after, before := int64(l10SpanAfter), int64(l10SpanBefore)

	ok := true
	for _, c := range r.Order {
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
			result := l10Query(t, l10QuerySpec{
				requestedGroup: group,
				tier:           probe.tier,
				after:          after,
				before:         before,
				points:         probe.points,
			})
			if !result.valid {
				ok = false
			}
			cols := result.cols
			for _, dim := range l10Dims {
				col := cols[dim]
				if len(col) == 0 {
					t.Logf("invariant not met: %s (%s) returned no rows for %q", group, probe.what, dim)
					ok = false
					continue
				}
				empty, firstEmpty := l10InvalidEmptyBuckets(col, groupingRules[c].firstBucketMayBeEmpty)
				if empty > 0 {
					t.Logf("invariant not met: %s (%s) left %d of %d invalid buckets EMPTY for %q "+
						"inside a span that was collected end to end (first at bucket %d)",
						group, probe.what, empty, len(col), dim, firstEmpty)
					ok = false
				}
			}
		}
	}

	assertContract(t, "L10/no-holes-inside-data", ok)
}

// INV-1c: a bucket NARROWER than the stored data still answers.
//
// The zoomed-in regime, and the one that matters most: above tier 0 a stored
// point covers many seconds, so a dashboard drawn finer than that gives the
// engine buckets that a single re-delivered point covers on its own. Every
// grouping must still answer in them, apart from the same opening
// incremental-sum bucket whose predecessor is outside the query.
//
// This is where `number-of-flaps` and `number-of-times` punched holes - they
// dropped the repeat entirely, sample count and all, so the buckets between
// stored points came back EMPTY. INV-1 does not reach it: it deliberately
// uses buckets wide enough to hold several samples, so that a grouping which
// genuinely needs two of them is not asked for the impossible. That fairness
// is exactly what hides this case, so it gets its own sweep.
func TestLayer10BucketsFinerThanStoredDataAnswer(t *testing.T) {
	trackContract(t, "L10/buckets-finer-than-stored-data-answer")

	l10Fixture(t)
	r, err := readGroupingRoster()
	if err != nil {
		t.Fatal(err)
	}

	after, before := int64(l10SpanAfter), int64(l10SpanAfter)+20*tier1Gran

	ok := true
	for _, c := range r.Order {
		group := r.Canonical[c]

		// 3, 5 and 10 buckets per 60s stored window: every bucket after the
		// first of each window is covered by a re-delivery alone
		for _, perWindow := range []int64{3, 5, 10} {
			result := l10Query(t, l10QuerySpec{
				requestedGroup: group,
				tier:           1,
				after:          after,
				before:         before,
				points:         20 * perWindow,
			})
			if !result.valid {
				ok = false
			}
			cols := result.cols
			for _, dim := range l10Dims {
				col := cols[dim]
				if len(col) == 0 {
					t.Logf("invariant not met: %s returned no rows for %q at %d buckets per stored window",
						group, dim, perWindow)
					ok = false
					continue
				}
				empty, firstEmpty := l10InvalidEmptyBuckets(col, groupingRules[c].firstBucketMayBeEmpty)
				if empty > 0 {
					t.Logf("invariant not met: %s left %d of %d invalid buckets EMPTY for %q at %d buckets per "+
						"stored window - the buckets a re-delivered point covers on its own report "+
						"'no data here' instead of what the point says (first at bucket %d)",
						group, empty, len(col), dim, perWindow, firstEmpty)
					ok = false
				}
			}
		}
	}

	assertContract(t, "L10/buckets-finer-than-stored-data-answer", ok)
}

// INV-2: an order statistic answers WITH the data, not past it.
//
// Applies to every grouping whose answer is one of the values it was given or
// something between them. A value outside that range means the aggregation
// invented a number - from an interpolation it should not have used, or from
// state left behind by another point.
func TestLayer10OrderStatisticsStayInRange(t *testing.T) {
	trackContract(t, "L10/order-statistics-stay-in-range")

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
			result := l10Query(t, l10QuerySpec{
				requestedGroup: group,
				tier:           tier,
				after:          after,
				before:         before,
				points:         (before - after) / 30,
			})
			if !result.valid {
				ok = false
				continue
			}
			for _, dim := range l10Dims {
				sources := l10SourceBuckets(t, result, group, tier, dim)
				values := make([]*float64, len(result.cols[dim]))
				for i := range result.cols[dim] {
					values[i] = result.cols[dim][i].Value
				}
				if !l10EnvelopeHolds(group, sources, values, printTol) {
					t.Logf("invariant not met: %s (tier %d) returned an empty value or left the "+
						"fixture-derived source envelope for %q", group, tier, dim)
					ok = false
					continue
				}
				checked++
			}
		}
	}

	if checked == 0 {
		t.Fatalf("no order-statistic buckets were checked")
	}
	t.Logf("%d grouping/tier/dimension combinations checked against exact source envelopes", checked)
	assertContract(t, "L10/order-statistics-stay-in-range", ok)
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
	trackContract(t, "L10/dimensions-are-independent")

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
			togetherResult := l10Query(t, l10QuerySpec{
				requestedGroup: group,
				tier:           tier,
				after:          after,
				before:         before,
				points:         points,
			})
			if !togetherResult.valid {
				ok = false
			}
			together := togetherResult.cols

			for _, dim := range l10Dims {
				aloneResult := l10Query(t, l10QuerySpec{
					requestedGroup:  group,
					tier:            tier,
					after:           after,
					before:          before,
					points:          points,
					scopeDimensions: dim,
					expectedDimensions: []string{
						dim,
					},
				})
				if !aloneResult.valid {
					ok = false
				}
				alone := aloneResult.cols

				a, b := alone[dim], together[dim]
				if l10NumericPoints(a) == 0 || l10NumericPoints(b) == 0 {
					t.Logf("invariant not met: %s (tier %d) returned no numeric buckets for %q "+
						"alone or alongside its neighbours", group, tier, dim)
					ok = false
					continue
				}
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

	assertContract(t, "L10/dimensions-are-independent", ok)
}

// INV-4: an event does not happen twice because the chart was zoomed in.
//
// Applies to the counting groupings. Above tier 0 a stored point covers many
// seconds; ask for buckets narrower than that and the engine hands the SAME
// point to each one. A grouping that counts occurrences must not count them
// again - the total over a fixed span may collapse as a rollup loses ordering,
// but it may never grow.
func TestLayer10CountsDoNotInflateWithZoom(t *testing.T) {
	trackContract(t, "L10/counts-do-not-inflate-with-zoom")

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
			result := l10Query(t, l10QuerySpec{
				requestedGroup: group,
				canonicalGroup: group,
				condition:      ">30",
				tier:           1,
				after:          after,
				before:         before,
				points:         20 * perWindow,
			})
			if !result.valid {
				ok = false
			}
			sum := 0.0
			numeric := 0
			for _, pt := range result.cols["wave"] {
				if pt.Value != nil {
					sum += *pt.Value
					numeric++
				}
			}
			if numeric == 0 {
				t.Logf("invariant not met: %s returned no numeric count buckets at %d buckets per window",
					group, perWindow)
				ok = false
			}
			return sum
		}

		base := total(1)
		if base <= 0 {
			t.Logf("invariant not met: %s count baseline is %v, want a positive discriminator", group, base)
			ok = false
		}
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

	assertContract(t, "L10/counts-do-not-inflate-with-zoom", ok)
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
	trackContract(t, "L10/totals-are-exact-across-zoom")

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
				result := l10Query(t, l10QuerySpec{
					requestedGroup: group,
					tier:           tier,
					after:          after,
					before:         before,
					points:         points,
				})
				if !result.valid {
					ok = false
				}
				got := 0.0
				for _, pt := range result.cols["flat"] {
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

	assertContract(t, "L10/totals-are-exact-across-zoom", ok)
}

// INV-1b: a bucket holding ONE collected sample still answers.
//
// The companion to INV-1, split out because it is the case a grouping is most
// tempted to treat as "not enough data": one bucket per sample interval, the
// most natural resolution there is. A dashboard drawn at the collection
// interval must not come back blank.
//
// `incremental-sum` may legitimately leave the opening bucket empty because
// the query has no predecessor yet. Its flush path carries the bucket's last
// sample forward, so every subsequent one-sample bucket has a baseline and
// must answer; losing that carry makes the whole remaining chain empty.
func TestLayer10SinglePointBucketsAnswer(t *testing.T) {
	trackContract(t, "L10/single-point-buckets-answer")

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
		result := l10Query(t, l10QuerySpec{
			requestedGroup: group,
			tier:           0,
			after:          after,
			before:         before,
			points:         before - after,
		})
		if !result.valid {
			ok = false
		}
		cols := result.cols

		for _, dim := range l10Dims {
			col := cols[dim]
			if len(col) == 0 {
				t.Logf("invariant not met: %s returned no rows for %q at one bucket per sample", group, dim)
				ok = false
				continue
			}
			invalidEmpty, firstInvalidEmpty := l10InvalidEmptyBuckets(
				col, groupingRules[c].firstBucketMayBeEmpty)
			if invalidEmpty != 0 {
				t.Logf("invariant not met: %s left %d/%d buckets EMPTY for %q at one bucket "+
					"per sample (first at bucket %d)", group, invalidEmpty, len(col), dim,
					firstInvalidEmpty)
				ok = false
			}
			if l10NumericPoints(col) == 0 {
				t.Logf("invariant not met: %s returned no numeric one-sample buckets for %q", group, dim)
				ok = false
			}
		}
	}

	assertContract(t, "L10/single-point-buckets-answer", ok)
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
	trackContract(t, "L10/time-shares-stable-across-zoom")

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
					result := l10Query(t, l10QuerySpec{
						requestedGroup: group,
						canonicalGroup: group,
						condition:      cond,
						tier:           1,
						after:          after,
						before:         before,
						points:         20 * perWindow,
					})
					if !result.valid {
						ok = false
					}
					col := result.cols[dim]
					if len(col) == 0 {
						t.Fatalf("%s(%s): no rows for %q", group, cond, dim)
					}
					sum := 0.0
					for _, pt := range col {
						if pt.Value == nil {
							t.Logf("invariant not met: %s(%s) returned an EMPTY bucket for %q",
								group, cond, dim)
							ok = false
							continue
						}
						sum += *pt.Value
					}
					return sum / float64(len(col))
				}

				base := mean(1)
				if cond == ">=0" && base <= 0 {
					t.Logf("invariant not met: %s(%s) positive control for %q is %v",
						group, cond, dim, base)
					ok = false
				}
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

	assertContract(t, "L10/time-shares-stable-across-zoom", ok)
}

// INV-6: an alias is the same grouping, not a similar one.
//
// `avg` must answer exactly what `average` answers. The registry maps several
// names onto one implementation and nothing else checks that the mapping is
// what it claims - a copy-paste in the table would silently route `ewma` to
// the wrong aggregation.
func TestLayer10AliasesResolveToTheSameGrouping(t *testing.T) {
	trackContract(t, "L10/aliases-resolve-to-the-same-grouping")

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
		canonicalResult := l10Query(t, l10QuerySpec{
			requestedGroup: r.Canonical[c],
			tier:           0,
			after:          after,
			before:         before,
			points:         points,
		})
		if !canonicalResult.valid {
			ok = false
		}
		canonical := canonicalResult.cols

		for _, alias := range r.Aliases[c] {
			checked++
			gotResult := l10Query(t, l10QuerySpec{
				requestedGroup: alias,
				canonicalGroup: r.Canonical[c],
				tier:           0,
				after:          after,
				before:         before,
				points:         points,
			})
			if !gotResult.valid {
				ok = false
			}
			got := gotResult.cols
			for _, dim := range l10Dims {
				a, b := got[dim], canonical[dim]
				if l10NumericPoints(a) == 0 || l10NumericPoints(b) == 0 {
					t.Logf("invariant not met: alias %s or canonical %s returned no numeric rows for %q",
						alias, r.Canonical[c], dim)
					ok = false
					continue
				}
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
	assertContract(t, "L10/aliases-resolve-to-the-same-grouping", ok)
}

// INV-7: asking twice answers twice the same.
//
// Every grouping. The aggregations keep state for the length of a query; if
// any of it outlives the query - a static, an arena not cleared, a field the
// create() path does not initialise - the second answer differs from the
// first and every answer after that is a function of what was asked before
// it.
func TestLayer10QueriesAreDeterministic(t *testing.T) {
	trackContract(t, "L10/queries-are-deterministic")

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
		firstResult := l10Query(t, l10QuerySpec{
			requestedGroup: group,
			tier:           1,
			after:          after,
			before:         before,
			points:         points,
		})
		secondResult := l10Query(t, l10QuerySpec{
			requestedGroup: group,
			tier:           1,
			after:          after,
			before:         before,
			points:         points,
		})
		if !firstResult.valid || !secondResult.valid {
			ok = false
		}
		first, second := firstResult.cols, secondResult.cols

		for _, dim := range l10Dims {
			a, b := first[dim], second[dim]
			if l10NumericPoints(a) == 0 || l10NumericPoints(b) == 0 {
				t.Logf("invariant not met: %s returned no numeric rows for %q on one or both runs",
					group, dim)
				ok = false
				continue
			}
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

	assertContract(t, "L10/queries-are-deterministic", ok)
}

// INV-8: buckets come back in order, once each.
//
// Every grouping. A repeated or out-of-order timestamp means the grid walk
// lost its place, and every value after it is attributed to the wrong moment.
func TestLayer10BucketsAreOrderedAndUnique(t *testing.T) {
	trackContract(t, "L10/buckets-are-ordered-and-unique")

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
				result := l10Query(t, l10QuerySpec{
					requestedGroup: group,
					tier:           tier,
					after:          after,
					before:         before,
					points:         points,
				})
				if !result.valid {
					ok = false
				}
				cols := result.cols
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

	assertContract(t, "L10/buckets-are-ordered-and-unique", ok)
}

func samePoint(a, b canon.Pt) bool {
	if a.T != b.T {
		return false
	}
	equalFloat := func(x, y float64) bool {
		return x == y || (math.IsNaN(x) && math.IsNaN(y))
	}
	if (a.Value == nil) != (b.Value == nil) {
		return false
	}
	if a.Value != nil && !equalFloat(*a.Value, *b.Value) {
		return false
	}
	if !equalFloat(a.ARP, b.ARP) || a.PA != b.PA ||
		(a.Count == nil) != (b.Count == nil) ||
		(a.Hidden == nil) != (b.Hidden == nil) {
		return false
	}
	if a.Count != nil && *a.Count != *b.Count {
		return false
	}
	return a.Hidden == nil || equalFloat(*a.Hidden, *b.Hidden)
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
		Dimensions: []fixture.Dimension{{ID: "holes"}, {ID: "anom"}, {ID: "zero"}},
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
		// The independent zero-valued control has the same anomaly-rate input
		// as "anom". Every grouping must therefore return the same point,
		// including any mathematically legitimate EMPTY placement.
		ch.Dimensions[2].Points = append(ch.Dimensions[2].Points,
			fixture.Point{T: ts, Collected: "0", Flags: stream.FlagNotAnomalous})
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

func l10GapTotalSlack(tier int, after, before int64) float64 {
	if tier != 1 || (after%tier1Gran == 0 && before%tier1Gran == 0) {
		return 0
	}
	// Both-cut test spans have equal edge offsets and a whole-record duration.
	// The fixture repeats each record, so the two edge fragments complement
	// each other and together carry at most one record of uncertainty.
	return float64(l10GapCollected * l10GapValue)
}

// INV-4c: a total is exact over a span with HOLES in it, and over a span that
// does not start on the tier grid.
//
// Same rule as L10/totals-are-exact-across-zoom, on the two shapes that rule
// never sees. A sum is the sum of the samples that were collected: gaps add
// nothing, and where the span begins cannot change what is inside it.
func TestLayer10TotalsAreExactOverGapsAndOffGrid(t *testing.T) {
	trackContract(t, "L10/totals-exact-over-gaps-and-off-grid")

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
					result := l10Query(t, l10QuerySpec{
						host:               "l10gap",
						context:            l10GapContext,
						requestedGroup:     group,
						tier:               tier,
						after:              span.after,
						before:             span.before,
						points:             points,
						expectedDimensions: []string{"holes", "anom", "zero"},
					})
					if !result.valid {
						ok = false
					}

					got := 0.0
					for _, pt := range result.cols["holes"] {
						if pt.Value != nil {
							got += *pt.Value
						}
					}
					// Tier0 edges are exact sample boundaries. At tier1 only
					// an edge cutting a stored record may carry one record of
					// uncertainty because a rollup does not expose where its
					// interior gaps fall.
					slack := l10GapTotalSlack(tier, span.after, span.before)
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

	assertContract(t, "L10/totals-exact-over-gaps-and-off-grid", ok)
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
// The fixture makes that impossible to miss: a dimension holding 1000000 and
// an otherwise identical zero-valued control are both never anomalous. Their
// complete points must match under anomaly-bit. This permits a grouping such
// as CV to remain EMPTY for the undefined 0/0 case without permitting metric
// statistics to leak into the anomaly-rate answer.
func TestLayer10AnomalyBitAnswersAboutRates(t *testing.T) {
	trackContract(t, "L10/anomaly-bit-answers-about-rates")

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
				result := l10Query(t, l10QuerySpec{
					host:               "l10gap",
					context:            l10GapContext,
					requestedGroup:     group,
					extraOptions:       "anomaly-bit",
					tier:               tier,
					after:              after,
					before:             before,
					points:             points,
					expectedDimensions: []string{"holes", "anom", "zero"},
				})
				if !result.valid {
					ok = false
				}

				anom, zero := result.cols["anom"], result.cols["zero"]
				if len(anom) != len(zero) {
					t.Logf("invariant not met: %s (tier %d, points=%d) returned %d anomaly rows and %d zero-control rows",
						group, tier, points, len(anom), len(zero))
					ok = false
					continue
				}
				for i := range anom {
					if !samePoint(anom[i], zero[i]) {
						t.Logf("invariant not met: %s (tier %d, points=%d) anomaly bucket %d is %v, "+
							"zero-control bucket is %v; both delivered anomaly rate zero",
							group, tier, points, i, valueOf(anom[i]), valueOf(zero[i]))
						ok = false
						break
					}
				}
			}
		}
	}

	assertContract(t, "L10/anomaly-bit-answers-about-rates", ok)
}
