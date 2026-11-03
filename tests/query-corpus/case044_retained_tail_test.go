// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"testing"

	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
)

func c044Register(t *testing.T, component string) {
	t.Helper()
	for _, key := range []string{"fine-selected-resumption", "retention-gap-null"} {
		registerContractComponent(t, "CASE-044/"+key, component)
	}
}

func TestCase044RetainedTail(t *testing.T) {
	for i, tc := range []struct {
		name               string
		history            int
		fineFirst, gapLast int64
	}{
		{"alloc-long-gap", 6, 1495, 1495},
		{"alloc-short-gap", 596, 905, 1200},
		{"alloc-overlap", 901, 600, 1200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c044Register(t, tc.name)
			base := int64(fixture.T0 - fixture.T0%300)
			const host, context = "retained-tail", "fixture.retained_tail"
			dd := c040AllocDaemon(t, tc.history)
			ch := c040Series(context, base, 1, 900, c040Flat, c040NotAnomalous, 0)
			resumed := c040Series(context, base, 1201, 1501, func(int64) float64 { return 17 }, c040NotAnomalous, 0)
			ch.Dimensions[0].Points = append(ch.Dimensions[0].Points, resumed.Dimensions[0].Points...)
			// Sample1501 completes the next tier1 record and publishes record900
			// regardless of chart modulo. Keep the connection open afterward.
			pushDedicatedChart(t, dd, host, guid(10101+i), ch)
			waitDedicatedTierLastEntry(t, dd, host, context, 0, base+1501)
			waitDedicatedTierLastEntry(t, dd, host, context, 1, base+900)
			retention := []daemon.Retention{
				{FirstEntry: base + tc.fineFirst, LastEntry: base + 1501},
				{FirstEntry: base + 300, LastEntry: base + 900},
			}
			var want []expectedColumnPoint
			for off := tc.gapLast + 1; off <= 1501; off++ {
				want = append(want, wantNumberAt(base+off, 17))
			}
			c044Assert(t, dd, host, context, tc.name, base, base+1501, base+601, base+900,
				base+901, base+tc.gapLast, retention, want)
		})
	}
}

// Forced-tier values and retention establish independently that the automatic
// query has both an older island and a newer numeric tail available to read.
func c044Assert(t *testing.T, dd *daemon.Daemon, host, context, component string,
	after, before, oldFirst, oldLast, gapFirst, gapLast int64,
	retention []daemon.Retention, resumed []expectedColumnPoint,
) {
	t.Helper()
	forced, fine := c040Query(t, dd, host, context, after, before, before-after, "average", "0")
	c040Retention(t, forced, retention)
	if !assertTierPresence(t, forced, []bool{true, false}) ||
		!assertExactColumnValues(t, c040Rows(fine, gapLast+1, before), "load", resumed, 0) {
		t.Fatal("forced tier0 does not contain the fixture's resumed values")
	}
	forced, coarse := c040Query(t, dd, host, context, after, before, before-after, "average", "1")
	c040Retention(t, forced, retention)
	var old []expectedColumnPoint
	for ts := oldFirst; ts <= oldLast; ts++ {
		old = append(old, wantNumberAt(ts, 7))
	}
	if !assertTierPresence(t, forced, []bool{false, true}) ||
		!assertExactColumnValues(t, c040Rows(coarse, oldFirst, oldLast), "load", old, 0) {
		t.Fatal("forced tier1 does not contain the fixture's older values")
	}
	t.Run("resumption", func(t *testing.T) {
		trackContractComponent(t, "CASE-044/fine-selected-resumption", component)
		doc, rows := c040Query(t, dd, host, context, after, before, before-after, "average", "")
		c040Retention(t, doc, retention)
		valuesOK := assertExactColumnValues(t, c040Rows(rows, gapLast+1, before), "load", resumed, 0)
		if !assertTierPresence(t, doc, []bool{true, true}) || !valuesOK {
			t.Error("automatic query omitted newer retained values after its older tier ran out")
		}
	})
	t.Run("gap", func(t *testing.T) {
		trackContractComponent(t, "CASE-044/retention-gap-null", component)
		_, rows := c040Query(t, dd, host, context, after, before, before-after, "average", "")
		var want []expectedColumnPoint
		for ts := gapFirst; ts <= gapLast; ts++ {
			want = append(want, wantEmptyAt(ts))
		}
		if !assertExactColumnValues(t, c040Rows(rows, gapFirst, gapLast), "load", want, 0) {
			t.Error("automatic query fabricated data inside the retention gap")
		}
	})
}
