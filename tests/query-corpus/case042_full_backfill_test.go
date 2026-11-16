// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

func TestCase042FullHistoricalBackfill(t *testing.T) {
	for _, invariant := range []string{"average", "sum", "min", "max", "anomaly-rate", "partial"} {
		registerContract(t, "CASE-042/full-backfill-"+invariant)
	}
	base := int64(fixture.T0 - fixture.T0%60)
	const host, context = "full-backfill", "fixture.full_backfill"
	dd := startDedicatedStorageDaemon(t, daemon.Options{StorageTiers: 1, TierGrouping: [3]int{0, 60, 0}})
	conn, err := stream.Connect(dd.Addr, dd.StreamKey, stream.HostInfo{
		Hostname: host, MachineGUID: guid(9980),
	}, stream.CapsLive)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	ch := fixture.Series(context, context, base, 2, 10, func(int) string { return "7" }, notAnom)
	push := func() {
		ch.Define(conn)
		ch.PushLive(conn)
		if err := conn.Flush(); err != nil {
			t.Fatal(err)
		}
	}
	push()
	if _, err := dd.WaitRetention(host, context, base+10, base+20, 15*time.Second); err != nil {
		t.Fatal(err)
	}
	ch.UpdateEvery = 1
	ch.Dimensions[0].Points = []fixture.Point{
		{T: base + 21, Collected: "7", Flags: stream.FlagAnomalous},
		{T: base + 22, Collected: "0", Flags: stream.FlagEmpty},
		{T: base + 23, Collected: "7", Flags: stream.FlagNotAnomalous},
	}
	push()
	if _, err := dd.WaitRetention(host, context, base+10, base+23, 15*time.Second); err != nil {
		t.Fatal(err)
	}
	c040Restart(t, dd, conn.Close, 2, "full")
	// Backfill runs after the triggering tier0 write. Empty triggers beyond
	// the asserted record preserve its four original numeric observations.
	ch.Dimensions[0].Points = []fixture.Point{
		{T: base + 61, Collected: "0", Flags: stream.FlagEmpty},
		{T: base + 121, Collected: "0", Flags: stream.FlagEmpty},
	}
	pushDedicatedChart(t, dd, host, guid(9980), ch)
	waitDedicatedTierLastEntry(t, dd, host, context, 1, base+60)

	for _, invariant := range []string{"average", "sum", "min", "max", "anomaly-rate", "partial"} {
		t.Run(invariant, func(t *testing.T) {
			trackContract(t, "CASE-042/full-backfill-"+invariant)
			group := invariant
			if invariant == "anomaly-rate" || invariant == "partial" {
				group = "average"
			}
			p := daemon.DataParamsTier(context, 1, base, base+60, 1, group)
			p.Set("options", "jsonwrap|unaligned|virtual-points")
			doc, err := dd.DataV3(host, p)
			if err != nil {
				t.Fatal(err)
			}
			cols, err := canon.Columns(doc)
			if err != nil {
				t.Fatal(err)
			}
			if !assertOnlyColumn(t, cols, "load") || !assertTierPresence(t, doc, []bool{false, true}) ||
				!queryTimestampGridExact(t, doc, queryExpectedVirtualGrid(t, base, base+60, 1, false)) {
				t.Fatal("full backfill control changed its column, tier, or grid")
			}
			var held bool
			switch invariant {
			case "anomaly-rate":
				held = assertExactColumnMetadata(t, cols, "load", []expectedColumnPoint{wantNumberWithARPAt(base+60, 7, 25)})
			case "partial":
				held = assertExactColumnMetadata(t, cols, "load", []expectedColumnPoint{wantNumberWithPAAt(base+60, 7, canon.AnnotationPartial)})
			default:
				want := 7.0
				if group == "sum" {
					want *= 4
				}
				held = assertExactColumnValues(t, cols, "load", []expectedColumnPoint{wantNumberAt(base+60, want)}, 0)
			}
			if !held {
				t.Error("historical full backfill changed fixture-derived " + invariant)
			}
		})
	}
}
