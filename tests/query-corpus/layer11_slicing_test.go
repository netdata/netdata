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
//     ADDITIVITY - cut a window in two and the halves must total the whole.
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

var sliceReady = map[string]bool{}

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
	base := fixture.T0 - fixture.T0%int64(ue)
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
	pushLiveBurst(t, host, guid(232+len(sliceReady)), ch)
	if _, err := td.WaitRetention(host, ctx, ch.FirstT(), ch.LastT(), 30*time.Second); err != nil {
		t.Fatal(err)
	}
	sliceReady[ctx] = true
	return ctx
}

func sliceHost(shape string, ue int) string { return "l11-" + shape + strconv.Itoa(ue) }

// sliceTotal asks for a window and adds up what came back.
func sliceTotal(t *testing.T, a sliceAxes, after, before int64) (float64, int) {
	t.Helper()
	ctx := sliceContext(a.Shape, a.UE)

	// one chart point per stored record, times the zoom
	recordEvery := int64(a.UE)
	if a.Tier > 0 {
		recordEvery = int64(a.UE) * tier1Gran
	}
	points := (before - after) / recordEvery * int64(a.PointsPer)
	if points < 1 {
		points = 1
	}

	params := daemon.DataParamsTier(ctx, a.Tier, after, before, points, "sum")
	opts := "jsonwrap|unaligned"
	if a.Option != "" {
		opts += "|" + a.Option
	}
	params.Set("options", opts)

	doc, err := td.DataV3(sliceHost(a.Shape, a.UE), params)
	if err != nil {
		t.Fatalf("%s: %v", a, err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatalf("%s: %v", a, err)
	}

	sum, n := 0.0, 0
	for _, pt := range cols["v"] {
		n++
		if pt.Value != nil {
			sum += *pt.Value
		}
	}
	return sum, n
}

// sliceTierCovers reports whether the tier being asked actually holds the
// whole window. A rollup is built as the data lands, so a tier that has not
// caught up yet answers with less than was pushed - which looks exactly like
// a conservation defect and is not one. Nothing is asserted against a tier
// that cannot answer.
func sliceTierCovers(t *testing.T, a sliceAxes, after, before int64) bool {
	t.Helper()
	params := daemon.DataParamsTier(sliceContext(a.Shape, a.UE), a.Tier, after, before, 1, "average")
	params.Set("options", "jsonwrap|unaligned")
	doc, err := td.DataV3(sliceHost(a.Shape, a.UE), params)
	if err != nil {
		return false
	}
	tiers := perTierRetention(doc)
	if a.Tier >= len(tiers) {
		return false
	}
	r := tiers[a.Tier]
	return r.FirstEntry > 0 && r.FirstEntry <= after && r.LastEntry >= before
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
func checkAdditive(t *testing.T, a sliceAxes) (ok bool, detail string) {
	t.Helper()
	after, before := sliceWindow(a)
	mid := after + (before-after)/2

	whole, _ := sliceTotal(t, a, after, before)
	left, _ := sliceTotal(t, a, after, mid)
	right, _ := sliceTotal(t, a, mid, before)

	// one stored record of slack: the split lands inside a record whose
	// content the two halves divide, and above tier 0 that division is
	// f32-rounded on each side
	recordContent := float64(sliceValue) * float64(a.UE)
	if a.Tier > 0 {
		recordContent *= float64(tier1Gran)
	}
	tolerance := recordContent * 1.05
	if tolerance < 1e-6 {
		tolerance = 1e-6
	}

	if math.Abs((left+right)-whole) <= tolerance {
		return true, ""
	}
	return false, fmt.Sprintf("%s: whole=%.1f but halves total %.1f (left=%.1f right=%.1f, "+
		"difference %.1f, one stored record holds %.1f)",
		a, whole, left+right, left, right, (left+right)-whole, recordContent)
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
	configs := sliceMatrix()

	// push every fixture the matrix needs, once
	for _, shape := range sliceShapes {
		for _, ue := range sliceUEs {
			sliceFixture(t, shape, ue)
		}
	}

	ok := true
	reported := 0
	for _, a := range configs {
		good, detail := checkAdditive(t, a)
		if !good {
			ok = false
			if reported < 12 {
				t.Logf("additivity not met: %s", detail)
				reported++
			}
		}
	}
	t.Logf("%d configurations checked", len(configs))

	expectAgentStatus(t, "L11/slicing-is-additive", ok)
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
	ok := true
	checked := 0
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
					if !sliceTierCovers(t, a, after, before) {
						t.Logf("skipped %s: tier %d does not cover the window yet", a, tier)
						continue
					}
					checked++

					want := 0.0
					base := int64(fixture.T0) - int64(fixture.T0)%int64(ue)
					for i := 1; i <= sliceSamples; i++ {
						ts := base + int64(i*ue)
						if ts > after && ts <= before && sliceCollected(shape, i) {
							want += sliceValue
						}
					}

					got, _ := sliceTotal(t, a, after, before)

					// a stored record straddling either end is legitimately
					// split, so allow one record at each edge
					recordContent := float64(sliceValue) * float64(ue)
					if tier > 0 {
						recordContent *= float64(tier1Gran)
					}
					if math.Abs(got-want) > recordContent*2.1 {
						t.Logf("conservation not met: %s totals %.1f, but the fixture pushed %.1f "+
							"into that window (one stored record holds %.1f)", a, got, want, recordContent)
						ok = false
					}
				}
			}
		}
	}

	if checked == 0 {
		t.Fatalf("no configuration met the precondition - the check tested nothing")
	}
	t.Logf("%d configurations checked (the rest upsample)", checked)
	expectAgentStatus(t, "L11/totals-match-what-was-pushed", ok)
}

// fixture_T0 is the fixed corpus epoch, as an int64.
func fixture_T0() int64 { return int64(fixture.T0) }

// sliceTotalPoints is sliceTotal with the point count given explicitly.
func sliceTotalPoints(t *testing.T, a sliceAxes, after, before, points int64) float64 {
	t.Helper()
	params := daemon.DataParamsTier(sliceContext(a.Shape, a.UE), a.Tier, after, before, points, "sum")
	opts := "jsonwrap|unaligned"
	if a.Option != "" {
		opts += "|" + a.Option
	}
	params.Set("options", opts)

	doc, err := td.DataV3(sliceHost(a.Shape, a.UE), params)
	if err != nil {
		t.Fatalf("%s: %v", a, err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatalf("%s: %v", a, err)
	}
	sum := 0.0
	for _, pt := range cols["v"] {
		if pt.Value != nil {
			sum += *pt.Value
		}
	}
	return sum
}
