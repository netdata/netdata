// SPDX-License-Identifier: GPL-3.0-or-later

// Layer 4 part (d) — THREE tiers joined inside one query.
//
// Part (c) proves a query can be served by two plans. Production runs three:
// a parent keeps minutes of per-second detail, hours of the first rollup and
// months of the second, and a "last 30 days" dashboard reads all three in
// one answer. The seam between tier2 and tier1 is a different code path from
// the seam between tier1 and tier0 (the plan walk switches forward through
// the list), and nothing pinned the case where BOTH seams are crossed.
//
// Building it needs each tier to rotate at a different depth. Retention TIME
// is unusable at the fixed 2023 epoch, so rotation is driven by VOLUME - and
// at the default 60 iterations per tier, tier1 would need ~60x more data
// than tier0 before it filled a quota of its own. Bringing the tiers closer
// together (1s / 5s / 10s) makes all three fill from one fixture: tier0
// rotates hardest, tier1 outlives it, tier2 outlives them both.
//
// Retention is DISCOVERED from db.per_tier, never predicted - the point is
// that the join works wherever the boundaries land.
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

const (
	// tier1 = 5s and tier2 = 10s, from TierGrouping below
	c4dTier2Granularity = 10
	c4dDims             = 50
	c4dRows             = 400_000
	c4dContext          = "fixture.l4d"
)

func TestLayer4ThreeTierJoin(t *testing.T) {
	trackContract(t, "L4/three-tier-join")

	dd, err := daemon.Start(daemon.Options{
		Binary: netdataBinary,
		RunDir: t.TempDir(),
		// every tier at the engine's floor, so each one rotates on its own
		TierRetentionMB: [3]int{25, 25, 25},
		// 1s / 5s / 10s instead of 1s / 60s / 3600s.
		//
		// The sizing is arithmetic, not guesswork. 50 dimensions of
		// incompressible samples cost tier0 ~200 B/s (50 x 4B). A tier1
		// point is min/max/sum/count/anomaly-count, ~16B, so tier1 costs
		// 50 x 16 / G B/s. For tier1 to fill 25MiB within this fixture but
		// still outlive tier0, G has to sit between 4 and ~6; 5 gives
		// 160 B/s against tier0's 200. tier2 at 2 more iterations costs
		// 80 B/s and outlives them both.
		TierGrouping:           [3]int{0, 5, 2},
		ReplicationStepSeconds: 3600,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dd.Stop() })

	conn, err := stream.Connect(dd.Addr, daemon.StreamKey, stream.HostInfo{
		Hostname: "l4d-child", MachineGUID: guid(81),
	}, stream.CapsReplication)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	conn.DefineChart(stream.Chart{
		ID: c4dContext, Title: "three tier join", Units: "units",
		Family: "fixture", Context: c4dContext, UpdateEvery: 1,
	})
	for d := 0; d < c4dDims; d++ {
		conn.Dimension(c4cDimID(d), "", 1, 1)
	}
	firstT := int64(fixture.T0)
	lastT := int64(fixture.T0 + c4dRows)
	conn.ChartDefinitionEnd(firstT, lastT, lastT)

	// same incompressible generator as part (c): the quotas must fill with
	// real bytes for rotation to happen
	served, err := conn.ServeReplication(
		map[string]struct{ FirstT, LastT int64 }{c4dContext: {FirstT: firstT, LastT: lastT}},
		lastT,
		func(_ string, after, before int64) []stream.ReplayRow {
			rows := make([]stream.ReplayRow, 0, before-after)
			for ts := after + 1; ts <= before; ts++ {
				row := stream.ReplayRow{T: ts, Dims: make([]stream.ReplayValue, c4dDims)}
				for d := range row.Dims {
					row.Dims[d] = stream.ReplayValue{
						ID:        c4cDimID(d),
						Collected: strconv.FormatInt(c4cValue(int64(d), ts-fixture.T0), 10),
						Flags:     stream.FlagNotAnomalous,
					}
				}
				rows = append(rows, row)
			}
			return rows
		},
		15*time.Minute,
	)
	if err != nil {
		t.Fatalf("replication dialogue: %v (served %v)", err, served)
	}
	if served[c4dContext] != c4dRows {
		t.Fatalf("replication served %d rows, want %d", served[c4dContext], c4dRows)
	}

	probe := func() []daemon.Retention {
		params := daemon.DataParams(c4dContext, lastT-60, lastT, 60)
		params.Set("scope_dimensions", c4cDimID(0))
		doc, err := dd.DataV3("l4d-child", params)
		if err != nil {
			return nil
		}
		return perTierRetention(doc)
	}

	var tiers []daemon.Retention
	for i := 0; i < 120; i++ {
		tiers = probe()
		if len(tiers) >= 3 && tiers[0].LastEntry >= lastT-60 {
			break
		}
		time.Sleep(time.Second)
		if i == 119 {
			t.Fatalf("ingest did not settle: per_tier %+v", tiers)
		}
	}

	// the ladder the whole case rests on: each tier reaches further back
	// than the one below it. If this does not hold, the sizing needs
	// revisiting - it does NOT mean the join is broken.
	if len(tiers) < 3 {
		t.Fatalf("expected 3 tiers, got %d: %+v", len(tiers), tiers)
	}
	if !(tiers[0].FirstEntry > tiers[1].FirstEntry && tiers[1].FirstEntry > tiers[2].FirstEntry) {
		t.Fatalf("no three-tier ladder — tier0 first=t0%+d, tier1 first=t0%+d, tier2 first=t0%+d; "+
			"quota/grouping sizing needs revisiting",
			tiers[0].FirstEntry-fixture.T0, tiers[1].FirstEntry-fixture.T0, tiers[2].FirstEntry-fixture.T0)
	}
	t.Logf("three-tier ladder: tier2 from t0%+d, tier1 from t0%+d, tier0 from t0%+d, last t0%+d",
		tiers[2].FirstEntry-fixture.T0, tiers[1].FirstEntry-fixture.T0,
		tiers[0].FirstEntry-fixture.T0, tiers[0].LastEntry-fixture.T0)

	ok := true
	fail := func(what string, args ...any) {
		t.Helper()
		t.Logf("three-tier join contract not met: "+what, args...)
		ok = false
	}

	// the whole retained duration, in one query: it starts inside tier2's
	// exclusive range, crosses into tier1's, then into tier0's
	after := tiers[2].FirstEntry
	before := tiers[0].LastEntry

	// The whole retained duration, read at every resolution a dashboard
	// would ask for. The planner picks the coarsest tier that can supply
	// the requested density, so WHICH tier answers changes with the zoom -
	// what must not change is that the span is fully answerable.
	tiersSeen := map[int]bool{}
	for _, points := range []int64{300, 3000, 30000, 60000, 86000} {
		params := daemon.DataParams(c4dContext, after, before, points)
		params.Set("scope_dimensions", c4cDimID(0))
		params.Set("time_group", "average")
		doc, err := dd.DataV3("l4d-child", params)
		if err != nil {
			fail("points=%d: %v", points, err)
			continue
		}

		pts := perTierPoints(doc)
		for tier, n := range pts {
			if n > 0 {
				tiersSeen[tier] = true
			}
		}

		cols, err := canon.Columns(doc)
		if err != nil {
			fail("points=%d: %v", points, err)
			continue
		}
		col := cols[c4cDimID(0)]
		if len(col) == 0 {
			fail("points=%d returned no rows over a fully retained span", points)
			continue
		}

		// no holes and no time travel, whichever tiers served it: a gap
		// here means a seam dropped data
		// Holes are only a defect when the data can actually fill them.
		// Asking for buckets FINER than the coarsest tier serving the span
		// is upsampling: slots that no stored point ends inside are empty
		// by design, and layer 9 owns that contract. Above that width the
		// span is fully covered and a hole means a seam dropped data.
		bucket := float64(before-after) / float64(points)
		holesAreDefects := bucket >= float64(c4dTier2Granularity)

		empty, prevT, reported := 0, int64(0), 0
		for i, pt := range col {
			// Alignment rounds the grid OUTWARD, so the first buckets can
			// end before the requested start and precede retention
			// altogether. They are empty because there is nothing there,
			// which is not a seam.
			if pt.T <= after {
				prevT = pt.T
				continue
			}
			if pt.Value == nil {
				empty++
				if holesAreDefects && reported < 3 {
					fail("points=%d: bucket %d/%d t0%+d is empty inside the retained span",
						points, i, len(col), pt.T-fixture.T0)
					reported++
				}
			}
			if i > 0 && pt.T <= prevT {
				fail("points=%d: bucket %d went backwards: t0%+d after t0%+d",
					points, i, pt.T-fixture.T0, prevT-fixture.T0)
			}
			prevT = pt.T

			// the generator emits pseudo-random 24-bit integers, so an
			// average of them at ANY tier stays inside that range
			if pt.Value != nil && (*pt.Value < 0 || *pt.Value > float64(0xFFFFFF)) {
				fail("points=%d: bucket t0%+d reads %v, outside the generator's range",
					points, pt.T-fixture.T0, *pt.Value)
			}
		}
		if holesAreDefects && empty > 0 {
			fail("points=%d: %d of %d buckets are empty inside the retained span",
				points, empty, len(col))
		}

		t.Logf("points=%-6d bucket=%6.2fs rows=%-6d empty=%-6d per_tier=%v",
			points, bucket, len(col), empty, pts)
	}

	// the join itself: across those resolutions every tier answered at
	// least once, so both seams were crossed
	for tier := 0; tier < 3; tier++ {
		if !tiersSeen[tier] {
			fail("tier %d never contributed to any resolution over a span that crosses it", tier)
		}
	}

	assertContract(t, "L4/three-tier-join", ok)
}
