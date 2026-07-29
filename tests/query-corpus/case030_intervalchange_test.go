// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-030 a metric that changes how often it is collected does not change
// what its history held.
//
// Netdata supports a metric's update_every changing while it runs, and the
// database keeps every historical record at the interval it was collected
// at. A volume over an OLD window is therefore a property of that window's
// own records - it cannot move because the metric is sampled differently
// today.
//
// This is the case that separates "the interval this record's samples were
// collected at" from "the interval this metric uses now". They are the same
// number until the interval changes, which is exactly why a fixture with one
// uniform interval cannot tell a correct implementation from one reading
// current metadata.
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

func TestCase030IntervalChangeKeepsHistory(t *testing.T) {
	const (
		rate = 10 // units per second, throughout
		fast = 1  // the interval the history was collected at
		slow = 10 // and the one the metric moves to
	)

	// aligned to the COARSEST tier2 grid in play (the ten-second metric's
	// tier2 records are 36000s wide and land on multiples of that), so a
	// window of whole tier2 records cuts no record at any tier
	base := int64(fixture.T0) - int64(fixture.T0)%36000

	for _, tc := range []struct {
		name, contract     string
		first, then        int
		samples1, samples2 int
	}{
		{"speeds-up", "CASE-030/interval-change-speeding-up", slow, fast, 28800, 600},
		{"slows-down", "CASE-030/interval-change-slowing-down", fast, slow, 28800, 600},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := "fixture.c030_" + tc.name
			host := "c030-" + tc.name

			// history, at the first interval
			ch1 := fixture.Series(ctx, ctx, base, tc.samples1, tc.first,
				func(int) string { return strconv.Itoa(rate) },
				func(int) string { return stream.FlagNotAnomalous })
			ch1.Dimensions[0].Algorithm = "incremental"
			ch1.Units = "units/s"
			// both phases go over ONE connection: the chart is redefined
			// with the new update_every, the way a collector whose interval
			// changes does it
			conn := connect(t, host, guid(280+len(tc.name)), stream.CapsLive)
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

			// a window of whole tier1 records, entirely inside the HISTORY -
			// every second of it was measured at the first interval
			gran1 := int64(tc.first) * 60
			gran2 := int64(tc.first) * 3600
			// four whole tier2 records in the MIDDLE of the history - which
			// is also a whole number of tier1 records and of samples, so one
			// window serves every tier. Far enough from either end that each
			// record the query reads is complete and rolled up whatever else
			// the daemon is doing, and every second of it was measured at the
			// first interval.
			after := base + 2*gran2
			before := after + 4*gran2
			_ = gran1
			want := float64(before-after) * float64(rate)

			ok := true
			for _, tier := range []int{0, 1, 2} {
				params := daemon.DataParamsTier(ctx, tier, after, before, 4, "sum")
				params.Set("options", "jsonwrap|unaligned")
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
				total := 0.0
				for _, pt := range cols[ch1.Dimensions[0].ID] {
					if pt.Value != nil {
						total += *pt.Value
					}
				}
				t.Logf("%s tier %d: %.4f (want %.4f)", tc.name, tier, total, want)
				if math.Abs(total-want) > 1e-6 {
					t.Logf("history contract not met: %s tier %d totals %.4f over %ds of a "+
						"%d/s rate collected every %ds, which is %.4f - how often the metric "+
						"is sampled TODAY cannot change what its history held",
						tc.name, tier, total, before-after, rate, tc.first, want)
					ok = false
				}
			}
			assertContract(t, tc.contract, ok)
		})
	}
}
