// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-030 a metric that changes how often it is collected does not change
// what its history held.
//
// Netdata supports a metric's update_every changing while it runs. For
// complete records with no collection gaps, a volume over an OLD window is
// a property of that window's records - it cannot move because the metric is
// sampled differently today.
//
// This is the case that separates "the interval this record's samples were
// collected at" from "the interval this metric uses now". They are the same
// number until the interval changes, which is exactly why a fixture with one
// uniform interval cannot tell a correct implementation from one reading
// current metadata.
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

func TestCase030IntervalChangeKeepsHistory(t *testing.T) {
	const (
		rate = 10 // units per second, throughout
		fast = 1  // the two collection intervals exercised in both directions
		slow = 10
	)

	// aligned to the COARSEST tier2 grid in play (the ten-second metric's
	// tier2 records are 36000s wide and land on multiples of that), so a
	// window of whole tier2 records cuts no record at any tier
	base := int64(fixture.T0) - int64(fixture.T0)%36000

	cases := map[string]struct {
		contract           string
		first, then        int
		samples1, samples2 int
		guidIdx            int
	}{
		"speeds-up":  {contract: "CASE-030/interval-change-speeding-up", first: slow, then: fast, samples1: 28800, samples2: 600, guidIdx: 280},
		"slows-down": {contract: "CASE-030/interval-change-slowing-down", first: fast, then: slow, samples1: 28800, samples2: 600, guidIdx: 281},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			trackContract(t, tc.contract)

			ctx := "fixture.c030_" + name
			host := "c030-" + name

			// history, at the first interval
			ch1 := fixture.Series(ctx, ctx, base, tc.samples1, tc.first,
				func(int) string { return strconv.Itoa(rate) },
				func(int) string { return stream.FlagNotAnomalous })
			ch1.Dimensions[0].Algorithm = "incremental"
			ch1.Units = "units/s"
			// both phases go over ONE connection: the chart is redefined
			// with the new update_every, the way a collector whose interval
			// changes does it
			conn := connect(t, host, guid(tc.guidIdx), stream.CapsLive)
			ch1.Define(conn)
			ch1.PushLive(conn)
			if err := conn.Flush(); err != nil {
				t.Fatal(err)
			}
			if _, err := td.WaitRetention(host, ctx, ch1.FirstT(), ch1.LastT(), 30*time.Second); err != nil {
				t.Fatal(err)
			}

			// the metric changes how often it is collected, and keeps going
			histEnd := base + int64(tc.samples1*tc.first)
			ch2 := fixture.Series(ctx, ctx, histEnd, tc.samples2, tc.then,
				func(int) string { return strconv.Itoa(rate) },
				func(int) string { return stream.FlagNotAnomalous })
			ch2.Dimensions[0].Algorithm = "incremental"
			ch2.Units = "units/s"
			ch2.Define(conn)
			ch2.PushLive(conn)
			if err := conn.Flush(); err != nil {
				t.Fatal(err)
			}
			if _, err := td.WaitRetention(host, ctx, ch1.FirstT(), ch2.LastT(), 30*time.Second); err != nil {
				t.Fatal(err)
			}

			gran2 := int64(tc.first) * 3600
			// four whole tier2 records in the MIDDLE of the history - which
			// is also a whole number of tier1 records and of samples, so one
			// window serves every tier. Far enough from either end that each
			// record the query reads is complete and rolled up whatever else
			// the daemon is doing, and every second of it was measured at the
			// first interval.
			after := base + 2*gran2
			before := after + 4*gran2
			rowVolume := float64(gran2 * rate)
			want := make([]expectedColumnPoint, 4)
			for i := range want {
				want[i] = wantNumberAt(after+int64(i+1)*gran2, rowVolume)
			}
			dimension := ch1.Dimensions[0].ID

			ok := true
			for _, tier := range []int{0, 1, 2} {
				params := daemon.DataParamsTier(ctx, tier, after, before, 4, "sum")
				params.Set("options", "jsonwrap|unaligned")
				params.Set("scope_dimensions", dimension)
				doc, err := td.DataV3(host, params)
				if err != nil {
					t.Fatal(err)
				}
				cols, err := canon.Columns(doc)
				if err != nil {
					t.Logf("history contract not met: tier %d: %v", tier, err)
					ok = false
					continue
				}
				if !assertSelectedTier(t, doc, tier) {
					ok = false
				}
				if !assertOnlyColumn(t, cols, dimension) {
					ok = false
				}
				if !assertExactColumn(t, cols, dimension, want, 0) {
					t.Logf("history contract not met: %s tier %d must return four exact "+
						"%ds rows of %.0f units; how often the metric is sampled today "+
						"cannot change what its history held",
						name, tier, gran2, rowVolume)
					ok = false
				}
			}
			assertContract(t, tc.contract, ok)
		})
	}
}
