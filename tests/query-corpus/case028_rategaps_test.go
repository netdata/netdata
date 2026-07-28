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
// control: with nothing missing every candidate agrees, so a failure there is
// measuring something else.
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

func TestCase028RateWithGapsTotalsWhatWasMeasured(t *testing.T) {
	ok := true
	g := 260

	for _, ue := range []int{1, 10} {
		for _, gapped := range []bool{false, true} {
			shape := "nogaps"
			if gapped {
				shape = "gapped"
			}
			name := fmt.Sprintf("ue%d-%s", ue, shape)
			ctx := "fixture.c028_" + name
			host := "c028-" + name

			g++
			ch := c028Chart(ctx, ue, gapped)
			pushLiveBurst(t, host, guid(g), ch)

			// the series opens and closes on a collected sample, so the
			// retention barrier is the whole span either way
			ret, err := td.WaitRetention(host, ctx, ch.FirstT(), ch.LastT(), 60*time.Second)
			if err != nil {
				t.Fatalf("%s: %v (retention %+v)", name, err, ret)
			}
			t.Logf("%s: retention [%d,%d] = t0%+d..t0%+d", name, ret.FirstEntry, ret.LastEntry,
				ret.FirstEntry-int64(fixture.T0), ret.LastEntry-int64(fixture.T0))

			gran1 := int64(ue) * 60
			gran2 := int64(ue) * 3600
			base := c028Base(ue)
			// the second tier2 window: whole records at every tier
			after := base + gran2
			before := base + 2*gran2
			want := c028Measured(ue, gapped, after, before)

			for _, tier := range []int{0, 1, 2} {
				// one bucket for the whole window, and one per tier1 window:
				// both divide the span exactly, so the engine covers the
				// window asked for rather than a rounded one
				for _, points := range []int64{1, (before - after) / gran1} {
					params := daemon.DataParamsTier(ctx, tier, after, before, points, "sum")
					params.Set("options", "jsonwrap|unaligned")
					doc, err := td.DataV3(host, params)
					if err != nil {
						t.Fatal(err)
					}
					cols, err := canon.Columns(doc)
					if err != nil {
						t.Logf("volume contract not met: %s tier %d at %d buckets: %v",
							name, tier, points, err)
						ok = false
						continue
					}

					col, has := cols["rate"]
					if !has || len(col) == 0 {
						t.Logf("volume contract not met: %s tier %d at %d buckets returned no "+
							"data - the tier does not hold this window", name, tier, points)
						ok = false
						continue
					}

					total := 0.0
					for _, pt := range col {
						if pt.Value != nil {
							total += *pt.Value
						}
					}

					// a stored record straddling either edge is legitimately
					// split, so allow one of them at each end rather than
					// demanding the exact integer. The defect this case is
					// built for doubles the answer; edge slack cannot hide it.
					slack := float64(int64(c028Rate) * gran1 * 2)
					if math.Abs(total-want) > slack {
						t.Logf("volume contract not met: %s tier %d at %d buckets totals %.4f, "+
							"but the fixture measured %.0f seconds of %d/s in this window, "+
							"which is %.4f - seconds nobody measured cannot be added to a volume",
							name, tier, points, total, want/float64(c028Rate), c028Rate, want)
						ok = false
					}
				}
			}
		}
	}

	assertContract(t, "CASE-028/rate-with-gaps-totals-what-was-measured", ok)
}
