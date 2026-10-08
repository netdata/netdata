// SPDX-License-Identifier: GPL-3.0-or-later

// Layer 11 — the slicing matrix.
//
// The engine turns stored records into chart points, and the two never line
// up: one record can cover many points, one point can hold many records, and
// a record can straddle the edge between two points. Every bug this corpus
// has found in the aggregations was that division going wrong in one of three
// ways - a record counted twice, a record lost, or a record credited to the
// wrong point.
//
// Layer 10 sweeps every AGGREGATION and holds everything else still. That is
// how three defects walked through it: they needed a window that started off
// the storage grid, or data with holes in it, or an option flag. The bug was
// never in the aggregation the sweep varied; it was in a knob the sweep did
// not turn.
//
// This layer turns the knobs instead. It rests on one property that needs no
// oracle at all:
//
//	ADDITIVITY - cut a window in two and the halves must total the whole.
//
// The split introduces exactly one new edge, so any record straddling it is
// the one under test: counted in both halves the parts exceed the whole,
// dropped from both they fall short. It holds at any tier, any alignment, any
// shape of data, for anything that accumulates - and it is checked by asking
// the engine three questions and comparing them to each other, so nothing has
// to know what the right answer is.
//
// On top of that, CONSERVATION against the fixture's own arithmetic, where
// the window aligns with stored records and the expected total is exact.
package corpus

import (
	"fmt"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

// The knobs. None of them may change what a total comes to.
type sliceAxes struct {
	Shape       string // how the data itself is laid out
	UE          int    // the collection interval
	Tier        int    // which tier answers
	PointsPer   int    // chart points per stored record: <1 coarser, 1 equal, >1 finer
	StartOffset int64  // how far the window starts from the storage grid
	Option      string // an option flag that claims not to change the numbers
}

func (a sliceAxes) String() string {
	return fmt.Sprintf("shape=%s ue=%d tier=%d pointsPerRecord=%d startOffset=%d option=%q",
		a.Shape, a.UE, a.Tier, a.PointsPer, a.StartOffset, a.Option)
}

var (
	sliceShapes  = []string{"dense", "gaps", "sparse"}
	sliceUEs     = []int{1, 10}
	sliceTiers   = []int{0, 1}
	slicePoints  = []int{1, 3, 10} // chart points per stored record
	sliceOffsets = []int64{0, 17, 30}
	sliceOptions = []string{"", "natural-points", "absolute"}
)

const (
	sliceValue   = 7
	sliceSamples = 1800
)

func TestL11SlicingOracleGuards(t *testing.T) {
	for _, ue := range []int{1, 10} {
		base := sliceBase(ue)
		tier0 := sliceAxes{Shape: "dense", UE: ue, Tier: 0}
		if got := sliceRecordContent(tier0, base+int64(ue)); got != 7 {
			t.Errorf("dense tier0 ue%d record content = %v, want 7", ue, got)
		}
		tier1 := sliceAxes{Shape: "dense", UE: ue, Tier: 1}
		end := (base + int64(ue)*600) / sliceRecordDuration(tier1) * sliceRecordDuration(tier1)
		if got := sliceRecordContent(tier1, end); got != 420 {
			t.Errorf("dense tier1 ue%d record content = %v, want 420", ue, got)
		}
		for shape, want := range map[string]float64{"gaps": 210, "sparse": 42} {
			tier1.Shape = shape
			if got := sliceRecordContent(tier1, end); got != want {
				t.Errorf("%s tier1 ue%d record content = %v, want %v", shape, ue, got, want)
			}
		}
	}

	a := sliceAxes{Shape: "dense", UE: 1, Tier: 1}
	duration := sliceRecordDuration(a)
	aligned := (int64(fixture.T0) + 600) / duration * duration
	if _, _, crosses := sliceCrossingRecord(a, aligned); crosses {
		t.Fatal("aligned edge reported a crossing record")
	}
	end, content, crosses := sliceCrossingRecord(a, aligned+17)
	if !crosses || end != aligned+duration || content != 420 {
		t.Fatalf("off-grid edge crossing = end %d content %v crosses %v, want %d/420/true",
			end, content, crosses, aligned+duration)
	}
	if got := sliceEdgeAllowance(a, aligned+17, aligned+30); got != 420 {
		t.Fatalf("two cuts through the same record allow %v, want one record content 420", got)
	}
	if got := sliceEdgeAllowance(a, aligned, aligned+duration); got != 420 {
		t.Fatalf("aligned lower/upper endpoint allowance = %v, want only the lower record content 420", got)
	}
	if !sliceWithinTolerance(420, 420, 0) || sliceWithinTolerance(420.1, 420, 0) {
		t.Fatal("edge tolerance does not accept its exact bound or rejects a meaningful excess")
	}
	if sliceOneCutPartition(2) || !sliceOneCutPartition(1) {
		t.Fatal("one-cut partition guard accepted multiple shared records or rejected one shared edge record")
	}

	c := randomSliceCase{
		Shape: "gaps", UE: 10, Tier: 1, Option: "absolute",
		StartSample: 200, Offset: 17, Records: 8, Buckets: 24,
	}
	baseRequest := c.materialize()
	candidates := shrinkCandidates(c)
	if len(candidates) == 0 {
		t.Fatal("random slicing case produced no shrink candidates")
	}
	for _, candidate := range candidates {
		if candidate.materialize() == baseRequest {
			t.Errorf("shrink candidate did not change the materialized request: %v", candidate)
		}
		if !sliceCaseSimpler(candidate, c) {
			t.Errorf("shrink candidate is not strictly simpler than its input: %v -> %v", c, candidate)
		}
	}

	c.Buckets = 1
	for _, candidate := range shrinkCandidates(c) {
		if !sliceCaseSimpler(candidate, c) {
			t.Errorf("one-bucket shrink candidate can cycle back to a more complex case: %v -> %v", c, candidate)
		}
	}

	for _, tc := range []struct {
		option string
		points int64
		want   bool
	}{
		{option: "", points: 2},
		{option: "absolute", points: 2},
		{option: "natural-points", points: 2, want: true},
		{option: "", points: 1, want: true},
	} {
		if got := sliceResponseDeclaredGrid(tc.option, tc.points); got != tc.want {
			t.Errorf("option %q points %d response-declared grid permission = %v, want %v",
				tc.option, tc.points, got, tc.want)
		}
	}

	tiers := []daemon.Retention{
		{FirstEntry: 10, LastEntry: 100},
		{FirstEntry: 20, LastEntry: 90},
	}
	if !sliceRetentionCovers(tiers, 1, 20, 90) ||
		sliceRetentionCovers(tiers, 1, 19, 90) ||
		sliceRetentionCovers(tiers, 1, 20, 91) ||
		sliceRetentionCovers(tiers, 2, 20, 90) {
		t.Fatal("tier-retention coverage guard accepted an uncovered window or rejected the exact covered window")
	}

	coverage := map[string]bool{}
	for _, shape := range sliceShapes {
		for _, tier := range sliceTiers {
			coverage[sliceCoverageKey(sliceAxes{Shape: shape, UE: sliceUEs[0], Tier: tier})] = true
		}
	}
	missing := missingSliceCoverage(coverage)
	if len(missing) != len(sliceShapes)*len(sliceTiers) {
		t.Fatalf("shape/tier-only random coverage leaves %d combinations missing, want %d: %v",
			len(missing), len(sliceShapes)*len(sliceTiers), missing)
	}
}

// sliceCollected says whether the fixture pushed sample i of a shape.
func sliceCollected(shape string, i int) bool {
	switch shape {
	case "gaps":
		// half of every minute silent - the only way to reach a record the
		// engine does not trim, because it trims only when the record before
		// it is adjacent and numeric
		return i%60 < 30
	case "sparse":
		// one sample per ten, so a chart point usually holds exactly one
		return i%10 == 5
	default:
		return true
	}
}

func sliceBase(ue int) int64 {
	return int64(fixture.T0) - int64(fixture.T0)%int64(ue)
}

var slicePrefixes = map[string][]int{}

func slicePrefix(shape string, ue int) []int {
	key := fmt.Sprintf("%s/%d", shape, ue)
	if prefix := slicePrefixes[key]; prefix != nil {
		return prefix
	}
	prefix := make([]int, sliceSamples+1)
	for i := 1; i <= sliceSamples; i++ {
		prefix[i] = prefix[i-1]
		if sliceCollected(shape, i) {
			prefix[i]++
		}
	}
	slicePrefixes[key] = prefix
	return prefix
}

func sliceCollectedCount(shape string, ue int, after, before int64) int {
	if before <= after {
		return 0
	}
	base := sliceBase(ue)
	first := (after-base)/int64(ue) + 1
	last := (before - base) / int64(ue)
	if first < 1 {
		first = 1
	}
	if last > sliceSamples {
		last = sliceSamples
	}
	if first > last || last < 1 || first > sliceSamples {
		return 0
	}
	prefix := slicePrefix(shape, ue)
	return prefix[last] - prefix[first-1]
}

func sliceExpected(shape string, ue int, after, before int64) float64 {
	return float64(sliceValue * sliceCollectedCount(shape, ue, after, before))
}

func sliceRecordDuration(a sliceAxes) int64 {
	duration := int64(a.UE)
	if a.Tier > 0 {
		duration *= tier1Gran
	}
	return duration
}

func sliceRecordContent(a sliceAxes, recordEnd int64) float64 {
	duration := sliceRecordDuration(a)
	return sliceExpected(a.Shape, a.UE, recordEnd-duration, recordEnd)
}

func sliceCrossingRecord(a sliceAxes, edge int64) (end int64, content float64, crosses bool) {
	duration := sliceRecordDuration(a)
	if duration <= 0 || edge%duration == 0 {
		return 0, 0, false
	}
	end = (edge/duration + 1) * duration
	return end, sliceRecordContent(a, end), true
}

func sliceEdgeAllowance(a sliceAxes, lowerEdge, upperEdge int64) float64 {
	records := make(map[int64]float64, 2)
	addCrossing := func(edge int64) {
		end, content, crosses := sliceCrossingRecord(a, edge)
		if crosses {
			records[end] = content
		}
	}
	addCrossing(lowerEdge)
	addCrossing(upperEdge)
	if duration := sliceRecordDuration(a); duration > 0 && lowerEdge%duration == 0 {
		// Released query windows can include the stored record ending at an
		// aligned lower endpoint. An aligned upper endpoint is already inside.
		records[lowerEdge] = sliceRecordContent(a, lowerEdge)
	}
	total := 0.0
	for _, content := range records {
		total += content
	}
	return total
}

func sliceWithinTolerance(difference, edgeAllowance float64, numericRows int) bool {
	if edgeAllowance < 0 || numericRows < 0 {
		return false
	}
	return math.Abs(difference) <= edgeAllowance+printTol*float64(numericRows)
}

func sliceOneCutPartition(sharedRecords int) bool {
	return sharedRecords <= 1
}

func sliceViewBounds(result sliceQueryResult) (after, before int64, ok bool) {
	view, ok := result.doc["view"].(map[string]any)
	if !ok {
		return 0, 0, false
	}
	after, afterOK := queryInteger(view["after"])
	before, beforeOK := queryInteger(view["before"])
	return after, before, afterOK && beforeOK && before >= after
}

// sliceSharedRecords returns the exact fixture content of stored records that
// can contribute to both response-declared grids. More than one shared record
// means the independently normalized subqueries are not a one-cut partition,
// so the one-boundary additivity law does not apply to that generated shape.
func sliceSharedRecords(a sliceAxes, left, right sliceQueryResult) (content float64, records int, ok bool) {
	leftAfter, leftBefore, leftOK := sliceViewBounds(left)
	rightAfter, rightBefore, rightOK := sliceViewBounds(right)
	if !leftOK || !rightOK {
		return 0, 0, false
	}
	after := max(leftAfter, rightAfter)
	before := min(leftBefore, rightBefore)
	if before < after {
		return 0, 0, true
	}

	duration := sliceRecordDuration(a)
	firstEnd := after
	if remainder := firstEnd % duration; remainder != 0 {
		firstEnd += duration - remainder
	}
	lastEnd := before
	if remainder := lastEnd % duration; remainder != 0 {
		lastEnd += duration - remainder
	}
	for end := firstEnd; end <= lastEnd; end += duration {
		content += sliceRecordContent(a, end)
		records++
	}
	return content, records, true
}

var (
	sliceReady     = map[string]bool{}
	sliceRetention = map[string][]daemon.Retention{}
	sliceGUIDIndex = map[string]int{
		"dense/1": 232, "dense/10": 233,
		"gaps/1": 234, "gaps/10": 235,
		"sparse/1": 236, "sparse/10": 237,
	}
)

func sliceResponseDeclaredGrid(option string, points int64) bool {
	// The released one-point API geometry includes both endpoint instants and
	// may therefore widen the sole bucket. Natural mode always owns its grid.
	return option == "natural-points" || points == 1
}

func sliceCoverageKey(a sliceAxes) string {
	return fmt.Sprintf("%s/%d/%d", a.Shape, a.UE, a.Tier)
}

func missingSliceCoverage(coverage map[string]bool) []string {
	var missing []string
	for _, shape := range sliceShapes {
		for _, ue := range sliceUEs {
			for _, tier := range sliceTiers {
				key := sliceCoverageKey(sliceAxes{Shape: shape, UE: ue, Tier: tier})
				if !coverage[key] {
					missing = append(missing, key)
				}
			}
		}
	}
	return missing
}

func sliceContext(shape string, ue int) string {
	return fmt.Sprintf("fixture.l11%s%d", shape, ue)
}

func sliceFixture(t *testing.T, shape string, ue int) string {
	t.Helper()
	ctx := sliceContext(shape, ue)
	if sliceReady[ctx] {
		return ctx
	}

	ch := fixture.Chart{
		ID: ctx, Title: "slicing", Units: "units",
		Family: "fixture", Context: ctx, UpdateEvery: ue,
		Dimensions: []fixture.Dimension{{ID: "v"}},
	}
	base := sliceBase(ue)
	for i := 1; i <= sliceSamples; i++ {
		ts := base + int64(i*ue)
		if sliceCollected(shape, i) {
			ch.Dimensions[0].Points = append(ch.Dimensions[0].Points,
				fixture.Point{T: ts, Collected: strconv.Itoa(sliceValue), Flags: stream.FlagNotAnomalous})
		} else {
			ch.Dimensions[0].Points = append(ch.Dimensions[0].Points,
				fixture.Point{T: ts, Flags: stream.FlagEmpty})
		}
	}

	host := "l11-" + shape + strconv.Itoa(ue)
	guidIndex, ok := sliceGUIDIndex[shape+"/"+strconv.Itoa(ue)]
	if !ok {
		t.Fatalf("slicing fixture has no GUID index for shape=%q update_every=%d", shape, ue)
	}
	pushLiveBurst(t, host, guid(guidIndex), ch)
	if _, err := td.WaitRetention(host, ctx, ch.FirstT(), ch.LastT(), 30*time.Second); err != nil {
		t.Fatal(err)
	}
	sliceReady[ctx] = true
	return ctx
}

func sliceHost(shape string, ue int) string { return "l11-" + shape + strconv.Itoa(ue) }

type sliceQueryResult struct {
	total             float64
	rows, numericRows int
	doc               map[string]any
	cols              map[string][]canon.Pt
	validGrid         bool
}

func sliceQuery(t *testing.T, a sliceAxes, after, before, points int64) sliceQueryResult {
	t.Helper()
	query := l10Query(t, l10QuerySpec{
		host:                 sliceHost(a.Shape, a.UE),
		context:              sliceContext(a.Shape, a.UE),
		requestedGroup:       "sum",
		canonicalGroup:       "sum",
		extraOptions:         a.Option,
		responseDeclaredGrid: sliceResponseDeclaredGrid(a.Option, points),
		tier:                 a.Tier,
		after:                after,
		before:               before,
		points:               points,
		expectedDimensions:   []string{"v"},
	})
	result := sliceQueryResult{
		rows:      len(query.grid),
		doc:       query.doc,
		cols:      query.cols,
		validGrid: query.valid && len(query.grid) > 0,
	}
	for _, point := range query.cols["v"] {
		if point.Value != nil {
			result.total += *point.Value
			result.numericRows++
		}
	}
	return result
}

// sliceTotal asks for a window and adds up what came back.
func sliceTotal(t *testing.T, a sliceAxes, after, before int64) sliceQueryResult {
	t.Helper()
	// one chart point per stored record, times the zoom
	recordEvery := sliceRecordDuration(a)
	points := (before - after) / recordEvery * int64(a.PointsPer)
	if points < 1 {
		points = 1
	}
	return sliceQuery(t, a, after, before, points)
}

func sliceRetentionCovers(tiers []daemon.Retention, tier int, after, before int64) bool {
	if tier < 0 || tier >= len(tiers) {
		return false
	}
	r := tiers[tier]
	return r.FirstEntry > 0 && r.FirstEntry <= after && r.LastEntry >= before
}

// sliceTierCovers refreshes any cached snapshot that does not cover the
// requested window. A negative rollup snapshot must not become permanent:
// higher tiers are built asynchronously while the fixture lands.
func sliceTierCovers(t *testing.T, a sliceAxes, after, before int64) bool {
	t.Helper()
	context := sliceContext(a.Shape, a.UE)
	if tiers := sliceRetention[context]; sliceRetentionCovers(tiers, a.Tier, after, before) {
		return true
	}

	params := daemon.DataParamsTier(context, 0, after, before, 1, "average")
	params.Set("options", "jsonwrap|unaligned")
	doc, err := td.DataV3(sliceHost(a.Shape, a.UE), params)
	if err != nil {
		return false
	}
	tiers := perTierRetention(t, doc)
	if sliceRetentionCovers(tiers, a.Tier, after, before) {
		sliceRetention[context] = tiers
		return true
	}
	delete(sliceRetention, context)
	return false
}

func requireSliceTierCovers(t *testing.T, a sliceAxes, after, before int64) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if sliceTierCovers(t, a, after, before) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("fixture prerequisite not met: %s tier %d does not cover (%d,%d]",
				sliceContext(a.Shape, a.UE), a.Tier, after, before)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// sliceWindow is a window well inside the fixture, offset from the grid.
func sliceWindow(a sliceAxes) (after, before int64) {
	base := int64(fixture.T0) - int64(fixture.T0)%int64(a.UE)
	// start a few records in, so nothing touches the edge of retention
	after = base + int64(a.UE)*300 + a.StartOffset
	before = after + int64(a.UE)*600
	return after, before
}

// checkAdditive is the whole layer in one function: the halves of a window
// must total the whole of it. No oracle - the engine is compared with itself,
// so a wrong answer has to be wrong CONSISTENTLY to escape, and a slicing
// error at the split is by definition not.
func checkAdditive(t *testing.T, a sliceAxes) (ok, exercised bool, detail string) {
	t.Helper()
	after, before := sliceWindow(a)
	requireSliceTierCovers(t, a, after, before)
	mid := after + (before-after)/2

	whole := sliceTotal(t, a, after, before)
	// The released API can include the split endpoint in both halves. The
	// exact content of that one shared stored record is the allowed overlap.
	left := sliceTotal(t, a, after, mid)
	right := sliceTotal(t, a, mid, before)
	if !whole.validGrid || !left.validGrid || !right.validGrid {
		return false, false, fmt.Sprintf("%s: one or more queries returned an invalid response grid", a)
	}
	if sliceExpected(a.Shape, a.UE, after, before) <= 0 {
		return true, false, ""
	}
	if whole.numericRows == 0 || left.numericRows+right.numericRows == 0 {
		return false, false, fmt.Sprintf(
			"%s: fixture has data but whole/halves numeric coverage is %d/%d+%d",
			a, whole.numericRows, left.numericRows, right.numericRows)
	}
	edgeAllowance, sharedRecords, overlapOK := sliceSharedRecords(a, left, right)
	if !overlapOK {
		return false, false, fmt.Sprintf("%s: subquery views are malformed", a)
	}
	if !sliceOneCutPartition(sharedRecords) {
		return true, false, ""
	}
	difference := left.total + right.total - whole.total
	numericRows := whole.numericRows + left.numericRows + right.numericRows
	if sliceWithinTolerance(difference, edgeAllowance, numericRows) {
		return true, true, ""
	}
	return false, true, fmt.Sprintf(
		"%s: whole=%.1f but halves total %.1f (left=%.1f right=%.1f, difference %.1f, "+
			"split-edge allowance %.1f)",
		a, whole.total, left.total+right.total, left.total, right.total, difference, edgeAllowance)
}

// sliceMatrix builds a set of configurations covering every PAIR of axis
// values. Every defect this corpus has found in the slicing was triggered by
// one or two knobs together - none needed three - so covering all pairs is
// the coverage target, and it costs a few dozen configurations instead of the
// hundreds the full cross-product would.
func sliceMatrix() []sliceAxes {
	var all []sliceAxes
	// a straightforward covering construction: walk the longest axis and
	// rotate the others through it, which lands every pair together at least
	// once for axes this small
	n := len(sliceShapes) * len(slicePoints)
	if len(sliceOffsets)*len(sliceOptions) > n {
		n = len(sliceOffsets) * len(sliceOptions)
	}
	for i := 0; i < n; i++ {
		for si, shape := range sliceShapes {
			for pi, pts := range slicePoints {
				all = append(all, sliceAxes{
					Shape:       shape,
					UE:          sliceUEs[(i+si)%len(sliceUEs)],
					Tier:        sliceTiers[(i+pi)%len(sliceTiers)],
					PointsPer:   pts,
					StartOffset: sliceOffsets[(i+si+pi)%len(sliceOffsets)],
					Option:      sliceOptions[(i+pi)%len(sliceOptions)],
				})
			}
		}
	}
	return all
}

// The halves of a window total the whole of it, across the matrix.
func TestLayer11SlicingIsAdditive(t *testing.T) {
	trackContract(t, "L11/slicing-is-additive")

	configs := sliceMatrix()

	// push every fixture the matrix needs, once
	for _, shape := range sliceShapes {
		for _, ue := range sliceUEs {
			sliceFixture(t, shape, ue)
		}
	}

	ok := true
	reported := 0
	coverage := map[string]bool{}
	for _, a := range configs {
		good, exercised, detail := checkAdditive(t, a)
		if exercised {
			coverage[fmt.Sprintf("%s/%d/%d", a.Shape, a.UE, a.Tier)] = true
		}
		if !good {
			ok = false
			if reported < 12 {
				t.Logf("additivity not met: %s", detail)
				reported++
			}
		}
	}
	for _, shape := range sliceShapes {
		for _, ue := range sliceUEs {
			for _, tier := range sliceTiers {
				key := sliceCoverageKey(sliceAxes{Shape: shape, UE: ue, Tier: tier})
				if !coverage[key] {
					t.Logf("additivity coverage missing for %s", key)
					ok = false
				}
			}
		}
	}
	t.Logf("%d configurations checked", len(configs))

	assertContract(t, "L11/slicing-is-additive", ok)
}

// Conservation against the fixture's own arithmetic.
//
// This one has a precondition, and it is not a convenience: it only applies
// while the chart points are at least as wide as the COLLECTION INTERVAL.
// Ask for points narrower than that and the engine is no longer dividing
// stored records - it is manufacturing values between them, interpolating a
// sub-sample series that was never collected. Adding those up is a different
// quantity from adding up what was collected, and layer 9 owns that contract.
//
// The precondition is on the collection interval, not on the stored record:
// at tier 1 a 1-second chart point is still one point per collected sample,
// and that regime is precisely where sum-over-time multiplies a total by the
// zoom - so excluding it would throw away the check that matters most.
func TestLayer11TotalsMatchWhatWasPushed(t *testing.T) {
	trackContract(t, "L11/totals-match-what-was-pushed")

	ok := true
	checked := 0
	coverage := map[string]bool{}
	for _, shape := range sliceShapes {
		for _, ue := range sliceUEs {
			sliceFixture(t, shape, ue)

			for _, tier := range []int{0, 1} {
				for _, pointsPer := range slicePoints {
					a := sliceAxes{Shape: shape, UE: ue, Tier: tier, PointsPer: pointsPer}

					// bucket width = the stored record's width / the zoom
					recordEvery := ue
					if tier > 0 {
						recordEvery = ue * int(tier1Gran)
					}
					if recordEvery/pointsPer < ue {
						continue // upsampling - see above
					}
					after, before := sliceWindow(a)
					requireSliceTierCovers(t, a, after, before)
					checked++

					want := sliceExpected(shape, ue, after, before)
					got := sliceTotal(t, a, after, before)
					if !got.validGrid {
						t.Logf("conservation not met: %s returned an invalid response grid", a)
						ok = false
						continue
					}
					if want <= 0 {
						continue
					}
					if got.numericRows == 0 {
						t.Logf("conservation not met: %s has fixture content %.1f but no numeric rows", a, want)
						ok = false
						continue
					}
					coverage[sliceCoverageKey(a)] = true
					edgeAllowance := sliceEdgeAllowance(a, after, before)
					if !sliceWithinTolerance(got.total-want, edgeAllowance, got.numericRows) {
						t.Logf("conservation not met: %s totals %.1f, but the fixture pushed %.1f "+
							"into that window (edge-record allowance %.1f)",
							a, got.total, want, edgeAllowance)
						ok = false
					}
				}
			}
		}
	}

	if checked == 0 {
		t.Fatalf("no configuration met the precondition - the check tested nothing")
	}
	for _, key := range missingSliceCoverage(coverage) {
		t.Logf("deterministic conservation coverage missing for %s", key)
		ok = false
	}
	t.Logf("%d configurations checked (the rest upsample)", checked)
	assertContract(t, "L11/totals-match-what-was-pushed", ok)
}

// fixture_T0 is the fixed corpus epoch, as an int64.
func fixture_T0() int64 { return int64(fixture.T0) }
