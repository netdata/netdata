// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-029 a slowly collected metric totals the same at every zoom, on
// tier 0 too.
//
// The zoom inflation `sum` used to have was not a property of tiers - it
// was a property of a stored record being WIDER than the row asking about
// it. Above tier 0 that is always true at fine resolutions, which is where
// it was found, but it is equally true at tier 0 for anything collected
// less often than once a second: a metric collected every ten seconds
// answers ten one-second rows from one stored record.
//
// So a total over a window must not change with the requested resolution
// on tier 0 either. Pinned here because it is a deliberate change to what
// tier 0 answers for slow metrics - a chart of a 10-second metric zoomed to
// one-second rows used to report ten times what was collected.
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

func TestCase029SlowMetricTotalsAtEveryZoom(t *testing.T) {
	const (
		ctx     = "fixture.c029slow"
		host    = "c029slow"
		ue      = 10
		value   = 7
		samples = 60
	)

	base := int64(fixture.T0) - int64(fixture.T0)%int64(ue)
	ch := fixture.Series(ctx, ctx, base, samples, ue,
		func(int) string { return strconv.Itoa(value) },
		func(int) string { return stream.FlagNotAnomalous })

	pushLiveBurst(t, host, guid(255), ch)
	if _, err := td.WaitRetention(host, ch.Context, ch.FirstT(), ch.LastT(), 20*time.Second); err != nil {
		t.Fatal(err)
	}

	after := base
	before := base + int64(samples*ue)

	// an absolute gauge: the window holds one reading of `value` per stored
	// record, and nothing about asking for narrower rows creates more of them
	want := float64(samples * value)

	ok := true
	for _, points := range []int64{
		samples,          // one row per stored record
		samples * 2,      // 5s rows: two per record
		samples * ue,     // 1s rows: ten per record
		samples * ue / 2, // 2s rows: five per record
	} {
		params := daemon.DataParamsTier(ctx, 0, after, before, points, "sum")
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
		for _, pt := range cols[ch.Dimensions[0].ID] {
			if pt.Value != nil {
				total += *pt.Value
			}
		}

		if math.Abs(total-want) > want*1e-6 {
			t.Logf("tier-0 zoom contract not met: at %d rows (%ds each) the window totals "+
				"%.4f, but the fixture stored %d readings of %d, which is %.4f - a row "+
				"narrower than a stored record owns part of it, not all of it",
				points, (before-after)/points, total, samples, value, want)
			ok = false
		}
	}

	assertContract(t, "CASE-029/tier0-slow-metric-totals-at-every-zoom", ok)
}
