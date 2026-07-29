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
	t.Cleanup(func() { _ = dd.Stop() })

	conn, err := stream.Connect(dd.Addr, daemon.StreamKey, stream.HostInfo{
		Hostname: "l4d-child", MachineGUID: guid(81),
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
		map[string]struct{ FirstT, LastT int64 }{c4dContext: {FirstT: firstT, LastT: lastT}},
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

	t.Run("retention-and-condition-contracts", func(t *testing.T) {
		trackContract(t, "L4/three-tier-join")

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

			cols, err := canon.Columns(doc)
			if err != nil {
				fail("points=%d: %v", points, err)
				continue
			}
			grid := c4dExpectedAlignedGrid(t, after, before, points)
			empty, gridOK := c4dAlignedResultExact(
				t, doc, cols, c4cDimID(0), after, before, grid)
			if !gridOK {
				fail("points=%d: aligned result grid or retained-span values are wrong", points)
			}

			t.Logf("points=%-6d bucket=%-6ds rows=%-6d empty=%-6d per_tier=%v",
				points, grid.updateEvery, grid.rows, empty, pts)
		}

		// Exact availability answers and conserved reset counts in every
		// retention region and across both automatic seams. The fixed regions
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
			c4dMax(availabilityTiers[2].FirstEntry, counterTiers[2].FirstEntry),
			c4dMin(availabilityTiers[1].FirstEntry, counterTiers[1].FirstEntry),
			conditionSpan)
		tier1After := c4dRegionStart(
			t, "tier1-only",
			c4dMax(availabilityTiers[1].FirstEntry, counterTiers[1].FirstEntry),
			c4dMin(availabilityTiers[0].FirstEntry, counterTiers[0].FirstEntry),
			conditionSpan)
		tier0First := c4dMax(availabilityTiers[0].FirstEntry, counterTiers[0].FirstEntry)
		tier0After := c4dRegionStart(
			t, "tier0-only", tier0First,
			c4dMin(availabilityTiers[0].LastEntry, counterTiers[0].LastEntry),
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
			if !c4ConditionExact(t, dd, c4ConditionSpec{
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
				if !c4ConditionExact(t, dd, c4ConditionSpec{
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

		assertContract(t, "L4/three-tier-join", ok)
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
		if err == nil {
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

type c4dAlignedGrid struct {
	after, before, updateEvery int64
	rows                       int
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := c4dExpectedAlignedGrid(t, 0, tc.duration, tc.points)
			if got.after != tc.after || got.before != tc.before ||
				got.updateEvery != tc.every || got.rows != tc.rows {
				t.Errorf("grid = after:%d before:%d every:%d rows:%d, want %d/%d/%d/%d",
					got.after, got.before, got.updateEvery, got.rows,
					tc.after, tc.before, tc.every, tc.rows)
			}
		})
	}
}

// c4dExpectedAlignedGrid is the Class B port of the no-resampling,
// one-second-granularity path in query-window.c:214-268, 297-302 and 333-364.
// It independently fixes the exact row count and timestamps expected from each
// broad three-tier query instead of trusting the returned view metadata.
func c4dExpectedAlignedGrid(
	t *testing.T,
	after, before, requestedPoints int64,
) c4dAlignedGrid {
	t.Helper()

	duration := before - after
	if duration <= 0 || requestedPoints <= 0 || requestedPoints > duration ||
		requestedPoints > 86400 {
		t.Fatalf("unsupported aligned-grid fixture: after=%d before=%d points=%d",
			after, before, requestedPoints)
	}

	// query-window treats both endpoint seconds as available at one-second
	// granularity, while required coverage remains the exclusive/inclusive
	// request duration.
	available := duration + 1
	points := requestedPoints
	if points > available {
		points = available
	}
	group := available / points
	if group == 0 {
		group = 1
	}
	if available%points > points/2 {
		group++
	}
	if points*group < duration {
		points = (available + group - 1) / group
	}

	alignedBefore := before
	if remainder := alignedBefore % group; remainder != 0 {
		alignedBefore += group - remainder
	}
	viewAfter := alignedBefore - ((points-1)*group + group - 1)
	return c4dAlignedGrid{
		after:       viewAfter,
		before:      alignedBefore,
		updateEvery: group,
		rows:        int(points),
	}
}

func c4dAlignedResultExact(
	t *testing.T,
	doc map[string]any,
	cols map[string][]canon.Pt,
	dimension string,
	requestedAfter, requestedBefore int64,
	grid c4dAlignedGrid,
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

func c4dMin(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func c4dMax(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func c4dRegionStart(t *testing.T, label string, first, last, span int64) int64 {
	t.Helper()
	after := c4AlignUp(first+c4dCounterPeriod, c4dConditionEpoch, c4dCounterPeriod)
	if after+span > last-c4dCounterPeriod {
		t.Fatalf("%s retention region (%d,%d] is too narrow for an exact %ds condition window",
			label, first, last, span)
	}
	return after
}
