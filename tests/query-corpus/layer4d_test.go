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
// together (1s / 5s / 15s) makes all three fill from one fixture: tier0
// rotates hardest, tier1 outlives it, tier2 outlives them both.
//
// Retention is DISCOVERED from db.per_tier, never predicted - the point is
// that the join works wherever the boundaries land.
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
	// tier1 = 5s and tier2 = 15s, from TierGrouping below
	c4dTier1Grouping    = 5
	c4dTier2Grouping    = 3
	c4dTier2Granularity = c4dTier1Grouping * c4dTier2Grouping
	c4dDims             = 250
	c4dRows             = 80_000
	c4dContext          = "fixture.l4d"

	c4dAvailabilityDim = "availability"
	c4dCounterDim      = "counter"
	c4dRateDim         = "rate"
	c4dConditionEpoch  = int64(fixture.T0 - fixture.T0%c4dTier2Granularity)
	c4dAvailabilityUp  = int64(4)
	c4dAvailabilityN   = int64(5)
	c4dCounterPeriod   = int64(300)

	c4dOldRate             = 20
	c4dNewRate             = 100
	c4dCadenceTransitionIn = 7
	c4dNewCadence          = 10
	c4dMeasuredTailRows    = 400
	c4dSettlingTailRows    = 20
)

func TestLayer4ThreeTierJoin(t *testing.T) {
	completeSetup := trackInfrastructureSetup(
		t, infrastructureFailures, "layer4d-shared-fixture/setup")

	dd, err := daemon.Start(daemon.Options{
		Binary: netdataBinary,
		RunDir: t.TempDir(),
		// every tier at the engine's floor, so each one rotates on its own
		TierRetentionMB: [3]int{25, 25, 25},
		// 1s / 5s / 15s instead of 1s / 60s / 3600s.
		//
		// The sizing is arithmetic, not guesswork. The 250 incompressible
		// dimensions drive rotation; the two condition dimensions add only
		// a small margin. The random dimensions cost tier0 ~1000 B/s
		// (250 x 4B). A tier1
		// point is min/max/sum/count/anomaly-count, ~16B, so tier1 costs
		// 250 x 16 / G B/s. For tier1 to fill 25MiB within this fixture but
		// still outlive tier0, G has to sit between 4 and ~6; 5 gives
		// 800 B/s against tier0's 1000. Tier2 at 3 more iterations costs
		// about 267 B/s, leaving enough margin to outlive tier1 after the
		// exact-rate dimension is added.
		TierGrouping:           [3]int{0, c4dTier1Grouping, c4dTier2Grouping},
		ReplicationStepSeconds: 720,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := infrastructureFailures.run(
			"layer4d-shared-fixture/shutdown", dd.Stop); err != nil {
			t.Errorf("stop dedicated daemon: %v", err)
		}
	})

	conn, err := stream.Connect(dd.Addr, dd.StreamKey, stream.HostInfo{
		Hostname: "l4d-child", MachineGUID: guid(341),
	}, stream.CapsReplication)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	c4dDefineAtCadence(conn, 1)
	firstT := int64(fixture.T0)
	lastT := int64(fixture.T0 + c4dRows)
	conn.ChartDefinitionEnd(firstT, lastT, lastT)

	// same incompressible generator as part (c): the quotas must fill with
	// real bytes for rotation to happen
	served, err := conn.ServeReplication(
		map[string]stream.ReplayChart{
			c4dContext: {FirstT: firstT, LastT: lastT, UpdateEvery: 1},
		},
		lastT,
		func(_ string, after, before int64) []stream.ReplayRow {
			rows := make([]stream.ReplayRow, 0, before-after)
			for ts := after + 1; ts <= before; ts++ {
				row := stream.ReplayRow{T: ts, Dims: make([]stream.ReplayValue, c4dDims+3)}
				for d := 0; d < c4dDims; d++ {
					row.Dims[d] = stream.ReplayValue{
						ID:        c4cDimID(d),
						Collected: strconv.FormatInt(c4cValue(int64(d), ts-fixture.T0), 10),
						Flags:     stream.FlagNotAnomalous,
					}
				}
				row.Dims[c4dDims] = stream.ReplayValue{
					ID: c4dAvailabilityDim,
					Collected: strconv.FormatInt(
						c4AvailabilityValue(ts, c4dConditionEpoch, c4dAvailabilityN, c4dAvailabilityUp), 10),
					Flags: stream.FlagNotAnomalous,
				}
				row.Dims[c4dDims+1] = stream.ReplayValue{
					ID:        c4dCounterDim,
					Collected: strconv.FormatInt(c4CounterValue(ts, c4dConditionEpoch, c4dCounterPeriod), 10),
					Flags:     stream.FlagNotAnomalous,
				}
				row.Dims[c4dDims+2] = stream.ReplayValue{
					ID:        c4dRateDim,
					Collected: strconv.Itoa(c4dOldRate),
					Flags:     stream.FlagNotAnomalous,
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
		return perTierRetention(t, doc)
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
	completeSetup()

	t.Run("grid", func(t *testing.T) {
		const contract = "L4/three-tier-join-grid"
		trackContract(t, contract)

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
		// the requested density, so WHICH tier answers changes with the zoom.
		// The source-derived query-window grid and every interior row are exact.
		querySpan := before - after
		for _, points := range []int64{300, 3000, 10000, querySpan / 2, querySpan} {
			params := daemon.DataParams(c4dContext, after, before, points)
			params.Set("scope_dimensions", c4cDimID(0))
			params.Set("time_group", "average")
			doc, err := dd.DataV3("l4d-child", params)
			if err != nil {
				fail("points=%d: %v", points, err)
				continue
			}

			pts, tierVectorOK := strictTierPoints(t, doc)
			if !tierVectorOK {
				fail("points=%d: malformed db.per_tier", points)
				continue
			}
			if points == querySpan && !assertTierPresence(t, doc, []bool{true, true, true}) {
				fail("points=%d: the one-second query did not cross both seams in one plan walk", points)
			}

			grid := queryExpectedVirtualGrid(t, after, before, points, true)
			rawGrid := make([]int64, grid.rows)
			for i := range rawGrid {
				rawGrid[i] = grid.before - int64(i)*grid.updateEvery
			}
			if err := queryRawTimestampsExact(doc, rawGrid); err != nil {
				fail("points=%d: default newest-first wire order: %v", points, err)
			}

			cols, err := canon.Columns(doc)
			if err != nil {
				fail("points=%d: %v", points, err)
				continue
			}
			empty, gridOK := c4dAlignedResultExact(
				t, doc, cols, c4cDimID(0), after, before, grid)
			if !gridOK {
				fail("points=%d: aligned result grid or retained-span values are wrong", points)
			}

			t.Logf("points=%-6d bucket=%-6ds rows=%-6d empty=%-6d per_tier=%v",
				points, grid.updateEvery, grid.rows, empty, pts)
		}
		assertContract(t, contract, ok)
	})

	t.Run("condition-groupings", func(t *testing.T) {
		const contract = "L4/three-tier-condition-groupings"
		trackContract(t, contract)
		ok := true
		fail := func(what string, args ...any) {
			t.Helper()
			t.Logf("three-tier condition grouping contract not met: "+what, args...)
			ok = false
		}

		// Exact availability answers and the event contract in every retention
		// region and across both automatic seams. Forced tiers conserve reset
		// counts exactly; automatic seams preserve every finer-tier event and
		// allow only one crossing coarse representative. The fixed regions
		// force downsampling, identity and upsampling at tiers 1 and 2, plus
		// downsampling and identity at tier0. The seam rows are one second wide,
		// so selecting the wrong plan at either boundary is visible in the
		// strict per-tier vector.
		const conditionSpan = int64(3000)
		availabilityTiers := c4DimensionRetention(
			t, dd, c4dContext, "l4d-child", c4dAvailabilityDim, lastT, 3)
		counterTiers := c4DimensionRetention(
			t, dd, c4dContext, "l4d-child", c4dCounterDim, lastT, 3)
		tier2After := c4dRegionStart(
			t, "tier2-only",
			max(availabilityTiers[2].FirstEntry, counterTiers[2].FirstEntry),
			min(availabilityTiers[1].FirstEntry, counterTiers[1].FirstEntry),
			conditionSpan)
		tier1After := c4dRegionStart(
			t, "tier1-only",
			max(availabilityTiers[1].FirstEntry, counterTiers[1].FirstEntry),
			min(availabilityTiers[0].FirstEntry, counterTiers[0].FirstEntry),
			conditionSpan)
		tier0First := max(availabilityTiers[0].FirstEntry, counterTiers[0].FirstEntry)
		tier0After := c4dRegionStart(
			t, "tier0-only", tier0First,
			min(availabilityTiers[0].LastEntry, counterTiers[0].LastEntry),
			conditionSpan)

		for _, tc := range []struct {
			label        string
			after        int64
			rowSpan      int64
			selectedTier int
			expectedTier [3]bool
		}{
			{"tier2-downsample", tier2After, 30, 2, [3]bool{false, false, true}},
			{"tier2-identity", tier2After, 15, 2, [3]bool{false, false, true}},
			{"tier2-upsample", tier2After, 5, 2, [3]bool{false, false, true}},
			{"tier1-downsample", tier1After, 10, 1, [3]bool{false, true, false}},
			{"tier1-identity", tier1After, 5, 1, [3]bool{false, true, false}},
			{"tier1-upsample", tier1After, 1, 1, [3]bool{false, true, false}},
			{"tier0-downsample", tier0After, 5, 0, [3]bool{true, false, false}},
			{"tier0-identity", tier0After, 1, 0, [3]bool{true, false, false}},
		} {
			if !c4ConditionContract(t, dd, c4ConditionSpec{
				label:              "layer4d-" + tc.label,
				context:            c4dContext,
				host:               "l4d-child",
				availabilityDim:    c4dAvailabilityDim,
				counterDim:         c4dCounterDim,
				after:              tc.after,
				before:             tc.after + conditionSpan,
				rowSpan:            tc.rowSpan,
				selectedTier:       tc.selectedTier,
				expectedTiers:      tc.expectedTier,
				availabilityEpoch:  c4dConditionEpoch,
				availabilityPeriod: c4dAvailabilityN,
				availabilityUp:     c4dAvailabilityUp,
				counterPeriod:      c4dCounterPeriod,
				tier0First:         tier0First,
			}) {
				fail("%s condition grouping matrix failed", tc.label)
			}
		}

		for _, dimension := range []struct {
			label            string
			id               string
			availabilityOnly bool
			counterOnly      bool
		}{
			{label: "availability", id: c4dAvailabilityDim, availabilityOnly: true},
			{label: "counter", id: c4dCounterDim, counterOnly: true},
		} {
			dimensionTiers := c4DimensionRetention(
				t, dd, c4dContext, "l4d-child", dimension.id, lastT, 3)
			for _, seam := range []struct {
				label        string
				boundary     int64
				newerTier    int
				expectedTier [3]bool
			}{
				{"tier2-tier1-seam", dimensionTiers[1].FirstEntry, 1, [3]bool{false, true, true}},
				{"tier1-tier0-seam", dimensionTiers[0].FirstEntry, 0, [3]bool{true, true, false}},
			} {
				after := c4AlignDown(seam.boundary-conditionSpan/2,
					c4dConditionEpoch, c4dCounterPeriod)
				c4dRequireSeamWindow(
					t, dimension.label+" "+seam.label,
					after, after+conditionSpan, dimensionTiers, seam.newerTier)
				if !c4ConditionContract(t, dd, c4ConditionSpec{
					label:              "layer4d-" + dimension.label + "-" + seam.label,
					context:            c4dContext,
					host:               "l4d-child",
					availabilityDim:    c4dAvailabilityDim,
					counterDim:         c4dCounterDim,
					after:              after,
					before:             after + conditionSpan,
					rowSpan:            1,
					selectedTier:       -1,
					expectedTiers:      seam.expectedTier,
					availabilityEpoch:  c4dConditionEpoch,
					availabilityPeriod: c4dAvailabilityN,
					availabilityUp:     c4dAvailabilityUp,
					counterPeriod:      c4dCounterPeriod,
					tier0First:         dimensionTiers[0].FirstEntry,
					availabilityOnly:   dimension.availabilityOnly,
					counterOnly:        dimension.counterOnly,
				}) {
					fail("%s %s condition grouping failed", dimension.label, seam.label)
				}
			}
		}
		if !c4dFineEventGroupingsAuthoritative(t, dd, lastT) {
			fail("tier2-to-tier1 seam hid an event or flap emitted by the finer tier")
		}

		assertContract(t, contract, ok)
	})

	t.Run("rate-volume-across-cadence-and-three-tier-seams", func(t *testing.T) {
		const contract = "CASE-037/rate-volume-across-three-tier-cadence-query"
		trackContract(t, contract)
		assertContract(t, contract, c4dRateAcrossCadenceAndSeams(t, dd, conn, lastT))
	})
}

func c4dDefineAtCadence(conn *stream.Conn, updateEvery int) {
	conn.DefineChart(stream.Chart{
		ID: c4dContext, Title: "three tier join", Units: "units",
		Family: "fixture", Context: c4dContext, UpdateEvery: updateEvery,
	})
	for d := 0; d < c4dDims; d++ {
		conn.Dimension(c4cDimID(d), "", 1, 1)
	}
	conn.Dimension(c4dAvailabilityDim, "", 1, 1)
	conn.Dimension(c4dCounterDim, "", 1, 1)
	conn.Dimension(c4dRateDim, "incremental", 1, 1)
}

func c4dRateAcrossCadenceAndSeams(
	t *testing.T,
	dd *daemon.Daemon,
	conn *stream.Conn,
	lastReplicated int64,
) bool {
	t.Helper()

	// Move seven old-cadence samples past the replicated range so the
	// collection-interval change is deliberately inside both the 5-second
	// and 15-second rollup grids.
	transition := lastReplicated + c4dCadenceTransitionIn
	for ts := lastReplicated + 1; ts <= transition; ts++ {
		conn.Begin2(c4dContext, 1, ts)
		conn.Set2(c4dRateDim, strconv.Itoa(c4dOldRate), stream.FlagNotAnomalous)
		conn.End2()
	}

	c4dDefineAtCadence(conn, c4dNewCadence)
	rows := c4dMeasuredTailRows + c4dSettlingTailRows
	for i := 1; i <= rows; i++ {
		ts := transition + int64(i*c4dNewCadence)
		conn.Begin2(c4dContext, c4dNewCadence, ts)
		conn.Set2(c4dRateDim, strconv.Itoa(c4dNewRate), stream.FlagNotAnomalous)
		conn.End2()
	}
	if err := conn.Flush(); err != nil {
		t.Fatal(err)
	}

	newTier2Granularity := int64(c4dNewCadence * c4dTier1Grouping * c4dTier2Grouping)
	queryBefore := c4AlignDown(
		transition+int64(c4dMeasuredTailRows*c4dNewCadence),
		0, newTier2Granularity)
	pushedLast := transition + int64(rows*c4dNewCadence)
	var tiers []daemon.Retention
	deadline := time.Now().Add(2 * time.Minute)
	for {
		params := daemon.DataParams(c4dContext, queryBefore-100, queryBefore, 100)
		params.Set("scope_dimensions", c4dRateDim)
		doc, err := dd.DataV3("l4d-child", params)
		if err != nil {
			tiers = nil
		} else {
			tiers = perTierRetention(t, doc)
		}
		settled := len(tiers) >= 3
		for tier := 0; settled && tier < 3; tier++ {
			settled = tiers[tier].LastEntry >= queryBefore
		}
		if settled {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("cadence tail did not settle through %d after pushing through %d: per_tier %+v",
				queryBefore, pushedLast, tiers)
		}
		time.Sleep(time.Second)
	}

	if !(tiers[0].FirstEntry > tiers[1].FirstEntry &&
		tiers[1].FirstEntry > tiers[2].FirstEntry) {
		t.Fatalf("rate dimension has no three-tier retention ladder: %+v", tiers)
	}

	after := c4AlignUp(tiers[2].FirstEntry+c4dTier2Granularity,
		0, c4dTier2Granularity)
	if after >= tiers[1].FirstEntry || queryBefore <= tiers[0].FirstEntry {
		t.Fatalf("rate query cannot cross both automatic seams: after=%d before=%d per_tier=%+v",
			after, queryBefore, tiers)
	}
	oldRows, newRows := transition-after, queryBefore-transition
	if oldRows <= 0 || newRows <= 0 {
		t.Fatalf("rate query has no samples on both cadence sides: old=%d new=%d",
			oldRows, newRows)
	}

	params := daemon.DataParams(c4dContext, after, queryBefore, queryBefore-after)
	params.Set("time_group", "sum")
	params.Set("scope_dimensions", c4dRateDim)
	params.Set("options", "jsonwrap|unaligned")
	doc, err := dd.DataV3("l4d-child", params)
	if err != nil {
		t.Fatal(err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}

	want := make([]expectedColumnPoint, queryBefore-after)
	for i := range want {
		ts := after + int64(i+1)
		value := float64(c4dOldRate)
		if ts > transition {
			value = c4dNewRate
		}
		want[i] = wantNumberAt(ts, value)
	}

	ok := assertExactView(t, doc, after, queryBefore, 1)
	if !assertTierPresence(t, doc, []bool{true, true, true}) {
		t.Logf("cadence query did not use all three automatic tier plans")
		ok = false
	}
	if !assertOnlyColumn(t, cols, c4dRateDim) {
		ok = false
	}
	if !assertExactColumn(t, cols, c4dRateDim, want, 0) {
		t.Logf("cadence query did not preserve every fixture-derived one-second rate-volume row")
		ok = false
	}
	return ok
}

func TestC4DAlignedGridOracleGuardsOffByOne(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		duration, points     int64
		after, before, every int64
		rows                 int
	}{
		{
			name:     "coverage-recomputes-from-available",
			duration: 1002, points: 300,
			after: -2, before: 1002, every: 3, rows: 335,
		},
		{
			name:     "group-rounds-from-available",
			duration: 60150, points: 300,
			after: 1, before: 60300, every: 201, rows: 300,
		},
		{
			name:     "requested-points-clamp-to-available",
			duration: 10, points: 20,
			after: 0, before: 10, every: 1, rows: 11,
		},
		{
			name:     "requested-points-clamp-to-production-cap",
			duration: 1_000_000, points: 100_000,
			after: -36_791, before: 1_000_008, every: 12, rows: 86_400,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := queryExpectedVirtualGrid(t, 0, tc.duration, tc.points, true)
			if got.after != tc.after || got.before != tc.before ||
				got.updateEvery != tc.every || got.rows != tc.rows {
				t.Errorf("grid = after:%d before:%d every:%d rows:%d, want %d/%d/%d/%d",
					got.after, got.before, got.updateEvery, got.rows,
					tc.after, tc.before, tc.every, tc.rows)
			}
		})
	}
}

func c4dAlignedResultExact(
	t *testing.T,
	doc map[string]any,
	cols map[string][]canon.Pt,
	dimension string,
	requestedAfter, requestedBefore int64,
	grid queryExpectedGrid,
) (int, bool) {
	t.Helper()

	ok := true
	const maxDetailedFailures = 10
	reported, suppressed := 0, 0
	report := func(format string, args ...any) {
		if reported < maxDetailedFailures {
			t.Logf(format, args...)
			reported++
		} else {
			suppressed++
		}
	}

	view, hasView := doc["view"].(map[string]any)
	if !hasView {
		report("response has no view object")
		ok = false
	} else {
		gotAfter, afterOK := view["after"].(float64)
		gotBefore, beforeOK := view["before"].(float64)
		gotEvery, everyOK := view["update_every"].(float64)
		if !afterOK || !beforeOK || !everyOK ||
			gotAfter != float64(grid.after) ||
			gotBefore != float64(grid.before) ||
			gotEvery != float64(grid.updateEvery) {
			report("aligned view after=%v before=%v update_every=%v, want %d/%d/%d",
				view["after"], view["before"], view["update_every"],
				grid.after, grid.before, grid.updateEvery)
			ok = false
		}
	}

	if !assertOnlyColumn(t, cols, dimension) {
		ok = false
	}
	col, hasColumn := cols[dimension]
	if !hasColumn {
		report("dimension %q is missing", dimension)
		return 0, false
	}
	if len(col) != grid.rows {
		report("dimension %q has %d rows, want exactly %d", dimension, len(col), grid.rows)
		ok = false
	}

	empty := 0
	for i, point := range col {
		wantT := grid.after + int64(i+1)*grid.updateEvery - 1
		if point.T != wantT {
			report("dimension %q row %d ends at %d, want %d", dimension, i, point.T, wantT)
			ok = false
		}
		if point.T <= requestedAfter || point.T-grid.updateEvery >= requestedBefore {
			continue
		}
		if point.Value == nil {
			empty++
			report("dimension %q row %d at %d is empty inside retained history",
				dimension, i, point.T)
			ok = false
			continue
		}
		if point.PA&canon.AnnotationEmpty != 0 {
			report("dimension %q row %d at %d is numeric but marked EMPTY",
				dimension, i, point.T)
			ok = false
		}
		if math.IsNaN(*point.Value) || math.IsInf(*point.Value, 0) ||
			*point.Value < 0 || *point.Value > float64(0xFFFFFF) {
			report("dimension %q row %d at %d is outside the fixture range: %v",
				dimension, i, point.T, *point.Value)
			ok = false
		}
	}
	if suppressed > 0 {
		t.Logf("dimension %q has %d additional aligned-grid failures not shown",
			dimension, suppressed)
	}
	return empty, ok
}

type c4dFineSeam struct {
	dimension     string
	boundary      int64
	coarseEnd     int64
	flapThreshold int64
}

func c4dFindFineSeam(t *testing.T, dd *daemon.Daemon, lastT int64) (c4dFineSeam, bool) {
	t.Helper()

	for d := 0; d < c4dDims; d++ {
		candidate := c4cDimID(d)
		retention := c4DimensionRetention(
			t, dd, c4dContext, "l4d-child", candidate, lastT, 3)
		candidateBoundary := retention[1].FirstEntry
		candidateCoarseEnd := c4AlignUp(
			candidateBoundary, c4dConditionEpoch, c4dTier2Granularity)
		fineSuffix := candidateCoarseEnd - candidateBoundary
		if fineSuffix < c4dTier1Grouping || fineSuffix >= c4dTier2Granularity {
			continue
		}

		// The automatic fine plan starts after candidateBoundary. Find a
		// threshold strictly inside every retained tier1 record under the
		// crossing tier2 record; one non-vacuous fine record is sufficient.
		commonMin, commonMax := int64(0), int64(0xFFFFFF)
		for end := candidateBoundary + c4dTier1Grouping; end <= candidateCoarseEnd; end += c4dTier1Grouping {
			recordMin := c4cValue(int64(d), end-c4dTier1Grouping+1-fixture.T0)
			recordMax := recordMin
			for ts := end - c4dTier1Grouping + 2; ts <= end; ts++ {
				value := c4cValue(int64(d), ts-fixture.T0)
				if value < recordMin {
					recordMin = value
				}
				if value > recordMax {
					recordMax = value
				}
			}
			if recordMin > commonMin {
				commonMin = recordMin
			}
			if recordMax < commonMax {
				commonMax = recordMax
			}
		}
		if commonMax-commonMin < 2 {
			continue
		}

		threshold := commonMin + (commonMax-commonMin)/2
		if threshold <= commonMin || threshold >= commonMax {
			continue
		}
		return c4dFineSeam{
			dimension:     candidate,
			boundary:      candidateBoundary,
			coarseEnd:     candidateCoarseEnd,
			flapThreshold: threshold,
		}, true
	}

	t.Log("no dimension has an authoritative tier1 record with a usable flap threshold under its crossing tier2 record")
	return c4dFineSeam{}, false
}

type c4dFineAuthoritySpec struct {
	label         string
	group         string
	expression    string
	minimumEvents int
}

// c4dFineAuthority compares the tier1 suffix of an automatic tier2-to-tier1
// seam with a forced-tier1 control. Tier1 is still rolled up, but it is the
// finest retained evidence for that interval and every event it emits must
// survive the join with tier2.
func c4dFineAuthority(
	t *testing.T,
	dd *daemon.Daemon,
	seam c4dFineSeam,
	spec c4dFineAuthoritySpec,
) bool {
	t.Helper()

	after, before := seam.coarseEnd-30, seam.coarseEnd+30
	points := before - after
	autoParams := daemon.DataParams(c4dContext, after, before, points)
	autoParams.Set("time_group", spec.group)
	autoParams.Set("time_group_options", spec.expression)
	autoParams.Set("scope_dimensions", seam.dimension)
	autoParams.Set("options", "jsonwrap|unaligned")
	autoDoc, err := dd.DataV3("l4d-child", autoParams)
	if err != nil {
		t.Fatal(err)
	}

	fineParams := daemon.DataParamsTier(
		c4dContext, 1, after, before, points, spec.group)
	fineParams.Set("time_group_options", spec.expression)
	fineParams.Set("scope_dimensions", seam.dimension)
	fineParams.Set("options", "jsonwrap|unaligned")
	fineDoc, err := dd.DataV3("l4d-child", fineParams)
	if err != nil {
		t.Fatal(err)
	}

	autoCols, err := canon.Columns(autoDoc)
	if err != nil {
		t.Fatal(err)
	}
	fineCols, err := canon.Columns(fineDoc)
	if err != nil {
		t.Fatal(err)
	}

	ok := true
	if !assertTierPresence(t, autoDoc, []bool{false, true, true}) {
		t.Logf("%s query did not cross the tier2-to-tier1 seam", spec.label)
		ok = false
	}
	if !assertExactView(t, autoDoc, after, before, 1) ||
		!assertExactView(t, fineDoc, after, before, 1) {
		t.Logf("%s query or control returned the wrong view grid", spec.label)
		ok = false
	}
	if !assertSelectedTier(t, fineDoc, 1) {
		t.Logf("%s control did not stay on tier1", spec.label)
		ok = false
	}
	if !assertOnlyColumn(t, autoCols, seam.dimension) ||
		!assertOnlyColumn(t, fineCols, seam.dimension) {
		return false
	}
	if !assertColumnExactGrid(t, autoCols, seam.dimension, after, before, 1) {
		t.Logf("%s automatic query returned an invalid event-row sequence", spec.label)
		ok = false
	}
	if !assertColumnExactGrid(t, fineCols, seam.dimension, after, before, 1) {
		t.Logf("%s forced-tier1 control returned an invalid event-row sequence", spec.label)
		ok = false
	}

	autoByTime := make(map[int64]*float64, len(autoCols[seam.dimension]))
	for _, point := range autoCols[seam.dimension] {
		autoByTime[point.T] = point.Value
	}

	fineRows, fineEvents := 0, 0
	for _, point := range fineCols[seam.dimension] {
		if point.T <= seam.boundary || point.T > seam.coarseEnd {
			continue
		}
		fineRows++
		got, found := autoByTime[point.T]
		if !found {
			t.Logf("automatic seam is missing tier1 result row %d", point.T)
			ok = false
			continue
		}
		if point.Value == nil || got == nil {
			if point.Value == nil && got != nil {
				t.Logf("automatic seam row %d is %.12g, tier1 is null", point.T, *got)
				ok = false
			} else if point.Value != nil {
				t.Logf("automatic seam row %d is null, tier1 emits %.12g", point.T, *point.Value)
				ok = false
			}
			continue
		}
		if math.IsNaN(*point.Value) || math.IsInf(*point.Value, 0) ||
			*point.Value < 0 || math.Trunc(*point.Value) != *point.Value {
			t.Logf("tier1 row %d has invalid event count %.12g", point.T, *point.Value)
			ok = false
			continue
		}
		if math.IsNaN(*got) || math.IsInf(*got, 0) ||
			*got < 0 || math.Trunc(*got) != *got {
			t.Logf("automatic seam row %d has invalid event count %.12g", point.T, *got)
			ok = false
			continue
		}
		if *got != *point.Value {
			t.Logf("automatic seam row %d is %.12g, tier1 emits %.12g",
				point.T, *got, *point.Value)
			ok = false
		}
		fineEvents += int(*point.Value)
	}
	wantFineRows := int(seam.coarseEnd - seam.boundary)
	if fineRows != wantFineRows {
		t.Logf("%s compared %d fine rows, want %d", spec.label, fineRows, wantFineRows)
		ok = false
	}
	if fineEvents < spec.minimumEvents {
		t.Logf("%s tier1 emits only %d events under the crossing tier2 record, want at least %d",
			spec.label, fineEvents, spec.minimumEvents)
		ok = false
	}

	t.Logf("%s: %s crossing tier2 record ends at %d; tier1 starts after %d; tier1 emits %d authoritative finer-tier events",
		spec.label, seam.dimension, seam.coarseEnd, seam.boundary, fineEvents)
	return ok
}

func c4dFineEventGroupingsAuthoritative(t *testing.T, dd *daemon.Daemon, lastT int64) bool {
	t.Helper()

	seam, found := c4dFindFineSeam(t, dd, lastT)
	if !found {
		return false
	}

	timesOK := c4dFineAuthority(t, dd, seam, c4dFineAuthoritySpec{
		label:         "number-of-times fine authority",
		group:         "number-of-times",
		expression:    ">=-1",
		minimumEvents: 1,
	})
	flapsOK := c4dFineAuthority(t, dd, seam, c4dFineAuthoritySpec{
		label:         "number-of-flaps fine authority",
		group:         "number-of-flaps",
		expression:    ">=" + strconv.FormatInt(seam.flapThreshold, 10),
		minimumEvents: 1,
	})
	return timesOK && flapsOK
}

func c4dRequireSeamWindow(
	t *testing.T,
	label string,
	after, before int64,
	tiers []daemon.Retention,
	newerTier int,
) {
	t.Helper()

	olderTier := newerTier + 1
	if newerTier < 0 || olderTier >= len(tiers) {
		t.Fatalf("%s has invalid seam tier %d for retention %+v", label, newerTier, tiers)
	}
	boundary := tiers[newerTier].FirstEntry
	if !(after < boundary && before > boundary) {
		t.Fatalf("%s (%d,%d] does not cross tier%d first entry %d",
			label, after, before, newerTier, boundary)
	}
	if after < tiers[olderTier].FirstEntry {
		t.Fatalf("%s (%d,%d] starts before tier%d retention %d",
			label, after, before, olderTier, tiers[olderTier].FirstEntry)
	}
	if before > tiers[newerTier].LastEntry || before > tiers[olderTier].LastEntry {
		t.Fatalf("%s (%d,%d] exceeds seam-tier retention: %+v",
			label, after, before, tiers)
	}
	if newerTier > 0 && before >= tiers[newerTier-1].FirstEntry {
		t.Fatalf("%s (%d,%d] reaches tier%d first entry %d",
			label, after, before, newerTier-1, tiers[newerTier-1].FirstEntry)
	}
}

func c4dRegionStart(t *testing.T, label string, first, last, span int64) int64 {
	t.Helper()
	// DBENGINE may continue retiring the oldest volume-quota pages while the
	// matrix runs. Stay half a test window inside the observed boundary so a
	// later lookup still exercises the requested tier instead of stale gaps,
	// while retaining one complete window in the narrowest exclusive region.
	after := c4AlignUp(first+span/2, c4dConditionEpoch, c4dCounterPeriod)
	if after+span > last-c4dCounterPeriod {
		t.Fatalf("%s retention region (%d,%d] is too narrow for an exact %ds condition window",
			label, first, last, span)
	}
	return after
}
