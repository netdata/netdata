// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-024 zoom on slowly-collected metrics.
//
// A metric collected once an hour is normal - SNMP walks, cloud billing,
// capacity counters. The dashboard still lets you zoom to a minute, and at
// that zoom the whole requested window sits INSIDE one sample's interval:
// the query asks for 60 points across 60 seconds from data that has one
// point per 3600.
//
// Whatever the engine does about resolution, it must not answer "no data"
// for a window that is fully covered by a stored sample. A chart that
// disappears when you zoom in is indistinguishable from an outage.
//
// The fixtures are CONSTANT on purpose: boundary interpolation between
// adjacent samples is then a no-op, so the expected value is the constant
// at every zoom level and the case tests availability, not the
// interpolation contract (which layer 9 owns).
package corpus

import (
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

func TestCase024ZoomIntoSlowMetrics(t *testing.T) {
	trackContract(t, "CASE-024/zoom-into-slow-metrics")

	// one chart per collection interval, each constant
	cases := map[string]struct {
		ue      int
		samples int
		guid    int
	}{
		"ue60":   {60, 240, 338},  // 4 hours at one sample per minute
		"ue600":  {600, 144, 339}, // 24 hours at one sample per 10 minutes
		"ue3600": {3600, 48, 222}, // 2 days at one sample per hour
	}

	const constant = 42.0

	ok := true
	fail := func(what string, args ...any) {
		t.Helper()
		t.Logf("zoom contract not met: "+what, args...)
		ok = false
	}

	for name, tc := range cases {
		// samples sit on the absolute update_every grid: storage keeps
		// pushed timestamps, but every view re-grids to absolute multiples
		base := fixture.T0 - fixture.T0%int64(tc.ue)
		ctx := "fixture.c024" + name
		ch := fixture.Series(ctx, ctx, base, tc.samples, tc.ue,
			func(int) string { return "42" },
			func(int) string { return stream.FlagNotAnomalous })

		host := "c024" + name
		pushLiveBurst(t, host, guid(tc.guid), ch)
		if _, err := td.WaitRetention(host, ch.Context, ch.FirstT(), ch.LastT(), 20*time.Second); err != nil {
			t.Fatal(err)
		}

		// a window well inside the collected span, so nothing here is about
		// retention edges
		mid := base + int64(tc.samples/2*tc.ue)

		// zoom levels: from a tenth of one collection interval up to ten of
		// them. The first three are windows SMALLER than a single sample.
		for _, span := range []int64{
			int64(tc.ue) / 10,
			int64(tc.ue) / 2,
			int64(tc.ue),
			int64(tc.ue) * 10,
		} {
			if span < 1 {
				continue
			}

			// always 60 points, the way a dashboard asks
			params := daemon.DataParams(ch.Context, mid-span, mid, 60)
			params.Set("time_group", "average")
			doc, err := td.DataV3(host, params)
			if err != nil {
				t.Fatal(err)
			}
			cols, err := canon.Columns(doc)
			if err != nil {
				t.Fatal(err)
			}

			col, has := cols[ch.Dimensions[0].ID]
			label := fmt.Sprintf("update_every=%ds window=%ds points=60", tc.ue, span)

			if !has || len(col) == 0 {
				fail("%s returned no rows at all", label)
				continue
			}

			values := 0
			for _, pt := range col {
				if pt.Value == nil {
					continue
				}
				values++
				if *pt.Value != constant {
					fail("%s: bucket t=%d reads %v, want %v (the series is constant)",
						label, pt.T, *pt.Value, constant)
				}
			}

			// the window is fully covered by collected data, so SOMETHING
			// has to carry a value - an all-null answer here is the chart
			// vanishing when the user zooms in
			if values == 0 {
				fail("%s returned %d rows but every one of them is empty, "+
					"although the window is inside the collected span", label, len(col))
			}
		}
	}

	assertContract(t, "CASE-024/zoom-into-slow-metrics", ok)
}
