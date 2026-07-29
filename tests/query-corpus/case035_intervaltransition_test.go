// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-035 a higher-tier record that straddles a collection-interval change
// preserves the volume measured on both sides. The fixture changes both the
// interval and the rate, making cadence-blind arithmetic observable.
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

const (
	c035TransitionOffset = 3690
	c035FirstRate        = 10
	c035SecondRate       = 100
)

type c035Case struct {
	contract              string
	firstEvery, thenEvery int
	machineGUID           int
}

func c035Measured(base int64, tc c035Case, after, before int64) float64 {
	transition := base + c035TransitionOffset
	total := 0.0
	for ts := base + int64(tc.firstEvery); ts <= transition; ts += int64(tc.firstEvery) {
		if ts > after && ts <= before {
			total += c035FirstRate * float64(tc.firstEvery)
		}
	}
	for ts := transition + int64(tc.thenEvery); ts <= before; ts += int64(tc.thenEvery) {
		if ts > after {
			total += c035SecondRate * float64(tc.thenEvery)
		}
	}
	return total
}

func c035QueryVolume(t *testing.T, context, host string, tier int, after, before int64, want float64) bool {
	t.Helper()

	// Ten-second rows divide every window in the fixture exactly and avoid
	// the independent points=1 defect asserted by CASE-034.
	params := daemon.DataParamsTier(context, tier, after, before, (before-after)/10, "sum")
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
	for _, pt := range cols["load"] {
		if pt.Value != nil {
			total += *pt.Value
		}
	}
	ptp := perTierPoints(doc)
	usedExpectedTier := len(ptp) == 3
	for i, n := range ptp {
		if (i == tier && n == 0) || (i != tier && n != 0) {
			usedExpectedTier = false
		}
	}
	if !usedExpectedTier {
		t.Logf("tier%d query over (%d,%d] used per_tier %v", tier, after, before, ptp)
	}
	if math.IsNaN(total) || math.IsInf(total, 0) || math.Abs(total-want) > 1e-6 {
		t.Logf("tier%d query over (%d,%d] totals %.10g, want %.10g from the fixture's "+
			"measured rate x collection interval", tier, after, before, total, want)
		return false
	}
	return usedExpectedTier
}

func TestCase035RateRecordStraddlesIntervalChange(t *testing.T) {
	cases := map[string]c035Case{
		"slows-down": {
			contract:    "CASE-035/straddling-record-slowing-down",
			firstEvery:  1,
			thenEvery:   10,
			machineGUID: 296,
		},
		"speeds-up": {
			contract:    "CASE-035/straddling-record-speeding-up",
			firstEvery:  10,
			thenEvery:   1,
			machineGUID: 297,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			trackContract(t, tc.contract)

			base := int64(fixture.T0) - int64(fixture.T0)%36000
			context := "fixture.c035_" + name
			host := "c035-" + name
			firstSamples := c035TransitionOffset / tc.firstEvery
			ch1 := fixture.Series(context, context, base, firstSamples, tc.firstEvery,
				func(int) string { return strconv.Itoa(c035FirstRate) },
				func(int) string { return stream.FlagNotAnomalous })
			ch1.Dimensions[0].Algorithm = "incremental"
			ch1.Units = "units/s"

			conn := connect(t, host, guid(tc.machineGUID), stream.CapsLive)
			ch1.Define(conn)
			ch1.PushLive(conn)
			if err := conn.Flush(); err != nil {
				t.Fatal(err)
			}
			if _, err := td.WaitRetention(host, context, ch1.FirstT(), ch1.LastT(), 30*time.Second); err != nil {
				t.Fatal(err)
			}

			transition := base + c035TransitionOffset
			tier2Every := int64(tc.firstEvery) * tier2Gran
			tier2End := base + (int64(c035TransitionOffset)/tier2Every+1)*tier2Every
			secondSamples := int((tier2End-transition)/int64(tc.thenEvery)) + 4000
			ch2 := fixture.Series(context, context, transition, secondSamples, tc.thenEvery,
				func(int) string { return strconv.Itoa(c035SecondRate) },
				func(int) string { return stream.FlagNotAnomalous })
			ch2.Dimensions[0].Algorithm = "incremental"
			ch2.Units = "units/s"
			ch2.Define(conn)
			ch2.PushLive(conn)
			if err := conn.Flush(); err != nil {
				t.Fatal(err)
			}
			if _, err := td.WaitRetention(host, context, ch1.FirstT(), ch2.LastT(), 60*time.Second); err != nil {
				t.Fatal(err)
			}

			ok := true
			windows := map[int]struct{ after, before int64 }{}
			for tier, grouping := range map[int]int64{1: tier1Gran, 2: tier2Gran} {
				recordEvery := int64(tc.firstEvery) * grouping
				after := base + int64(c035TransitionOffset)/recordEvery*recordEvery
				windows[tier] = struct{ after, before int64 }{after: after, before: after + recordEvery}
			}
			for targetTier, window := range windows {
				want := c035Measured(base, tc, window.after, window.before)
				if !c035QueryVolume(t, context, host, 0, window.after, window.before, want) {
					ok = false
				}
				if !c035QueryVolume(t, context, host, targetTier, window.after, window.before, want) {
					ok = false
				}
			}
			assertContract(t, tc.contract, ok)
		})
	}
}
