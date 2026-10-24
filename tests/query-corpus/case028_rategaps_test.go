// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-028 a rate with holes in it totals what was measured, on every tier.
//
// A rate metric is stored as units-per-second, so a volume over a window is
// the rate integrated over the seconds it was actually measured. At tier 0
// that is what the engine answers: each stored sample stands for its own
// collection interval, and a second nobody collected contributes nothing.
//
// Above tier 0 the stored record no longer carries those seconds one by one.
// It carries a sum, a count, and a wall-clock width - and where some seconds
// under it were never collected, the width and the measured time are two
// different numbers. Using the width says "whatever we saw, we saw all the
// way through", which invents volume for time nobody watched and makes the
// answer depend on which tier retention happens to leave available.
//
// The matrix separates the candidate arithmetics rather than being
// exhaustive. A metric collected once per second cannot tell `sum x interval`
// from `sum` alone, because the interval is 1 - so every case runs at
// update_every 1 AND 10. A single higher tier cannot tell `sum x collection
// interval` from `sum x this tier's own stride` - so every case runs at tier
// 1 AND tier 2, whose strides differ by sixty. The no-gap rows are the
// control: with nothing missing, the measured time and the record's width
// are the same number, so an implementation that confuses them still answers
// correctly there - a failure in those rows is measuring something else.
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

const (
	c028Rate    = 10    // units per second, on every second that was measured
	c028Samples = 17200 // enough whole tier2 windows to roll up (cf. L2/tier2)
)

// c028Base anchors the series on the tier2 grid for this update_every, so a
// query window of whole tier2 windows cuts no stored record at any tier.
func c028Base(ue int) int64 {
	gran2 := int64(ue) * 3600
	return int64(fixture.T0) - int64(fixture.T0)%gran2
}

// c028Chart builds a constant-rate incremental series. The stored value is
// the rate itself (the streaming path stores what the child calculated) and
// the dimension is declared incremental, so the query engine reads it as a
// rate. When gapped, only every other sample is sent - the seconds between
// them were never measured by anyone.
func c028Chart(ctx string, ue int, gapped bool) fixture.Chart {
	base := c028Base(ue)
	ch := fixture.Chart{
		ID: ctx, Title: "rate with holes", Units: "units/s",
		Family: "fixture", Context: ctx, UpdateEvery: ue,
		Dimensions: []fixture.Dimension{{ID: "rate", Algorithm: "incremental"}},
	}
	for i := 1; i <= c028Samples; i++ {
		ts := base + int64(i*ue)
		if gapped && i%2 == 0 && i != c028Samples {
			ch.Dimensions[0].Points = append(ch.Dimensions[0].Points,
				fixture.Point{T: ts, Flags: stream.FlagEmpty})
			continue
		}
		ch.Dimensions[0].Points = append(ch.Dimensions[0].Points,
			fixture.Point{T: ts, Collected: strconv.Itoa(c028Rate), Flags: stream.FlagNotAnomalous})
	}
	return ch
}

// c028Measured is the oracle: every sample that WAS collected stands for its
// own collection interval, and nothing else contributes.
func c028Measured(ue int, gapped bool, after, before int64) float64 {
	base := c028Base(ue)
	seconds := 0
	for i := 1; i <= c028Samples; i++ {
		if gapped && i%2 == 0 && i != c028Samples {
			continue
		}
		ts := base + int64(i*ue)
		if ts > after && ts <= before {
			seconds += ue
		}
	}
	return float64(seconds * c028Rate)
}

// c028Fixtures pushes the four shapes once, whichever case asks first.
var c028Ready = map[string]bool{}

var c028GUIDByFixture = map[string]int{
	"ue1-nogaps":  256,
	"ue1-gapped":  257,
	"ue10-nogaps": 258,
	"ue10-gapped": 259,
}

func c028Fixture(t *testing.T, ue int, gapped bool) (ctx, host string) {
	t.Helper()
	shape := "nogaps"
	if gapped {
		shape = "gapped"
	}
	name := fmt.Sprintf("ue%d-%s", ue, shape)
	ctx, host = "fixture.c028_"+name, "c028-"+name
	if c028Ready[name] {
		return ctx, host
	}

	g, ok := c028GUIDByFixture[name]
	if !ok {
		t.Fatalf("CASE-028 fixture %q has no machine GUID", name)
	}
	ch := c028Chart(ctx, ue, gapped)
	pushLiveBurst(t, host, guid(g), ch)
	if _, err := td.WaitRetention(host, ctx, ch.FirstT(), ch.LastT(), 60*time.Second); err != nil {
		t.Fatal(err)
	}
	c028Ready[name] = true
	return ctx, host
}

func TestCase028RateWithGapsTotalsWhatWasMeasured(t *testing.T) {
	trackContract(t, "CASE-028/rate-with-gaps-totals-what-was-measured")

	ok := true

	for _, ue := range []int{1, 10} {
		for _, gapped := range []bool{false, true} {
			shape := "nogaps"
			if gapped {
				shape = "gapped"
			}
			name := fmt.Sprintf("ue%d-%s", ue, shape)
			ctx, host := c028Fixture(t, ue, gapped)

			gran1 := int64(ue) * 60
			gran2 := int64(ue) * 3600
			base := c028Base(ue)
			// the second tier2 window: whole records at every tier
			after := base + gran2
			before := base + 2*gran2
			want := c028Measured(ue, gapped, after, before)

			for _, tier := range []int{0, 1, 2} {
				// Zooms that divide the span exactly, so the engine covers
				// the window that was asked for rather than a rounded one.
				//
				// Keep this a multi-bucket matrix so it isolates whole-record
				// rate arithmetic over gaps from one-bucket value boundaries.
				for _, points := range []int64{(before - after) / gran1, (before - after) / (gran1 * 2)} {
					if !c028VolumeMatches(t, c028Query{
						context: ctx,
						host:    host,
						label:   name,
						tier:    tier,
						after:   after,
						before:  before,
						points:  points,
						want:    want,
						ue:      ue,
						gapped:  gapped,
					}) {
						ok = false
					}
				}
			}
		}
	}

	assertContract(t, "CASE-028/rate-with-gaps-totals-what-was-measured", ok)
}

// CASE-028b: a window that cuts stored records still totals what those
// seconds hold.
//
// The matrix above aligns every window to a record boundary, which is what
// makes its oracle exact - and it means a partial record is never asked for.
// A window that starts and ends INSIDE records is the ordinary case for a
// dashboard, and the seconds it covers are still countable from the fixture:
// a rate that never changes holds the same amount per second whichever part
// of a record the window takes.
//
// No gaps here. With holes in the record, the part of it inside the window
// cannot be counted from what is stored - that is the defect CASE-028
// already asserts, and it would drown this one out. This is about the
// boundary arithmetic alone.
func TestCase028PartialAndOffGridWindows(t *testing.T) {
	trackContract(t, "CASE-028/partial-and-off-grid-rate-windows")

	ok := true

	for _, ue := range []int{1, 10} {
		name := fmt.Sprintf("ue%d-nogaps", ue)
		ctx, host := c028Fixture(t, ue, false)

		gran1 := int64(ue) * 60
		base := c028Base(ue)

		// windows that begin and end inside a stored record, off the tier1
		// grid, and not a whole number of records wide
		for _, w := range []struct{ from, span int64 }{
			{gran1 + gran1/2, 4 * gran1},                 // starts mid-record, whole records wide
			{gran1 + gran1/3, 4*gran1 + gran1/2},         // both edges inside records
			{2*gran1 + 7*int64(ue), 3*gran1 + int64(ue)}, // exactly 181 samples
		} {
			after := base + w.from
			before := after + w.span
			want := c028Measured(ue, false, after, before)

			// bucket counts that divide the span exactly: a count that does
			// not makes the engine cover a rounded window, and the total then
			// differs for a reason this case did not put there
			zooms := []int64{w.span / int64(ue)}
			if w.span%(int64(ue)*2) == 0 {
				zooms = append(zooms, w.span/(int64(ue)*2))
			}

			for _, tier := range []int{0, 1} {
				for _, points := range zooms {
					if !c028VolumeMatches(t, c028Query{
						context: ctx,
						host:    host,
						label:   name,
						tier:    tier,
						after:   after,
						before:  before,
						points:  points,
						want:    want,
						ue:      ue,
					}) {
						ok = false
					}
				}
			}
		}
	}

	assertContract(t, "CASE-028/partial-and-off-grid-rate-windows", ok)
}

type c028Query struct {
	context, host, label  string
	tier                  int
	after, before, points int64
	want                  float64
	ue                    int
	gapped                bool
}

func c028VolumeMatches(t *testing.T, q c028Query) bool {
	t.Helper()

	params := daemon.DataParamsTier(q.context, q.tier, q.after, q.before, q.points, "sum")
	params.Set("options", "jsonwrap|unaligned")
	doc, err := td.DataV3(q.host, params)
	if err != nil {
		t.Fatal(err)
	}
	ok := assertSelectedTier(t, doc, q.tier)
	rowSpan := (q.before - q.after) / q.points
	if !assertExactView(t, doc, q.after, q.before, rowSpan) {
		ok = false
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Logf("volume contract not met: %s tier %d at %d buckets: %v",
			q.label, q.tier, q.points, err)
		return false
	}
	if !assertOnlyColumn(t, cols, "rate") ||
		!assertColumnExactGrid(t, cols, "rate", q.after, q.before, rowSpan) {
		ok = false
	}
	col := cols["rate"]
	wantRows := make([]expectedColumnPoint, q.points)
	for i := range wantRows {
		rowAfter := q.after + int64(i)*rowSpan
		rowBefore := rowAfter + rowSpan
		wantRows[i] = wantNumberAt(
			rowBefore, c028Measured(q.ue, q.gapped, rowAfter, rowBefore))
	}
	if !assertExactColumn(t, cols, "rate", wantRows, 1e-6) {
		ok = false
	}

	total := 0.0
	for _, pt := range col {
		if pt.Value != nil {
			total += *pt.Value
		}
	}

	if math.IsNaN(total) || math.IsInf(total, 0) || math.Abs(total-q.want) > 1e-6 {
		t.Logf("volume contract not met: %s tier %d over (t0%+d,t0%+d] at %d buckets "+
			"totals %.4f, but the fixture measured %.0f seconds of %d/s in it, which is %.4f - "+
			"unmeasured seconds contribute nothing",
			q.label, q.tier, q.after-int64(fixture.T0), q.before-int64(fixture.T0), q.points,
			total, q.want/float64(c028Rate), c028Rate, q.want)
		ok = false
	}
	return ok
}
