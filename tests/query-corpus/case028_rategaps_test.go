// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-028 a rate with holes in it totals the same whichever tier answers.
//
// A rate metric is stored as units-per-second, so a volume over a window is
// the rate integrated over the seconds it was actually measured. Above tier
// 0 the stored record no longer carries those seconds one by one - it
// carries a sum, a count, and a wall-clock width - and when some seconds
// under it were never collected, the width and the measured time are two
// different numbers.
//
// Whichever of the two the arithmetic uses, the answer must not depend on
// which tier happens to serve the query: the same metric over the same
// window is the same amount. A tier that estimates through the holes while
// another one steps over them makes the answer a property of retention.
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

func TestCase028RateWithGapsIsTierIndependent(t *testing.T) {
	const (
		ctx     = "fixture.c028rate"
		host    = "c028rate"
		samples = 420 // seven whole tier1 windows
		perSec  = 10  // units per second, on every second that was measured
	)

	base := int64(fixture.T0) - int64(fixture.T0)%60

	// A rate of perSec units/s, measured on every OTHER second. The stored
	// value is the rate itself (the streaming v2 path stores what the child
	// calculated), and the dimension is declared incremental so the query
	// engine reads it as a rate: a volume over the window is the rate times
	// the seconds it was actually measured over.
	ch := fixture.Chart{
		ID: ctx, Title: "rate with holes", Units: "units/s",
		Family: "fixture", Context: ctx, UpdateEvery: 1,
		Dimensions: []fixture.Dimension{{ID: "rate", Algorithm: "incremental"}},
	}
	for i := 1; i <= samples; i++ {
		ts := base + int64(i)
		if i%2 == 0 {
			ch.Dimensions[0].Points = append(ch.Dimensions[0].Points,
				fixture.Point{T: ts, Collected: strconv.Itoa(perSec),
					Flags: stream.FlagNotAnomalous})
		} else {
			ch.Dimensions[0].Points = append(ch.Dimensions[0].Points,
				fixture.Point{T: ts, Flags: stream.FlagEmpty})
		}
	}

	pushLiveBurst(t, host, guid(254), ch)
	if _, err := td.WaitRetention(host, ch.Context, ch.FirstT(), ch.LastT(), 20*time.Second); err != nil {
		t.Fatal(err)
	}

	// a whole number of tier1 windows, well inside the data
	after := base + 60
	before := base + 360

	// every measured second holds perSec for one second; the seconds nobody
	// measured hold nothing anyone can know
	measured := 0
	for i := 1; i <= samples; i++ {
		ts := base + int64(i)
		if ts > after && ts <= before && i%2 == 0 {
			measured++
		}
	}
	want := float64(measured * perSec)

	totals := map[int]float64{}
	for _, tier := range []int{0, 1} {
		params := daemon.DataParamsTier(ctx, tier, after, before, (before-after)/60, "sum")
		params.Set("options", "jsonwrap|unaligned")
		doc, err := td.DataV3(host, params)
		if err != nil {
			t.Fatal(err)
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}
		total := 0.0
		for _, pt := range cols["rate"] {
			if pt.Value != nil {
				total += *pt.Value
			}
		}
		totals[tier] = total
		t.Logf("tier %d totals %.4f over %ds of a %d/s rate collected every other second",
			tier, total, before-after, perSec)
	}

	ok := true
	for _, tier := range []int{0, 1} {
		if math.Abs(totals[tier]-want) > want*1e-6 {
			t.Logf("volume contract not met: tier %d totals %.4f, but the fixture measured "+
				"%d seconds of %d/s in this window, which is %.4f - seconds nobody measured "+
				"cannot be added to a volume",
				tier, totals[tier], measured, perSec, want)
			ok = false
		}
	}
	if math.Abs(totals[0]-totals[1]) > math.Abs(totals[0])*1e-6 {
		t.Logf("tier-independence not met: the same window over the same metric totals "+
			"%.4f on tier 0 and %.4f on tier 1 - which tier retention leaves available "+
			"cannot change how much happened",
			totals[0], totals[1])
		ok = false
	}

	assertContract(t, "CASE-028/rate-with-gaps-is-tier-independent", ok)
}
