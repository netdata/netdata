// SPDX-License-Identifier: GPL-3.0-or-later

// Layer 4 part (c) — plan switching across tiers with DIFFERENT retention:
// the production scenario where higher tiers earn queries.
//
// A dedicated daemon caps tier0 at the minimum dbengine quota (25MB,
// RRDENG_MIN_DISK_SPACE_MB), and a streaming replication fixture pushes
// enough incompressible samples that tier0's oldest datafiles rotate out
// while tier1 keeps the full history. The tier0 head boundary is not
// predicted — it is DISCOVERED from db.per_tier — and queries spanning it
// must be served by MULTIPLE plans: tier1 for the head, tier0 for the
// tail. Values stay oracle-driven per side of the boundary.
package corpus

import (
	"fmt"
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
	c4cDims    = 50
	c4cRows    = 200_000 // 55 dimensions x 200k rows = 11M samples, enough to rotate tier0
	c4cContext = "fixture.l4c"

	// A constant dimension alongside the random ones. Rollup keeps a
	// constant EXACTLY: every stored window holds 60 x c4cConstValue, and
	// the part of a window cut by a boundary is worth exactly its share of
	// it. There is nothing a coarser tier can lose about a flat line, so on
	// this dimension any difference between two answers over the same
	// seconds is arithmetic, never resolution.
	c4cConstDim   = "flat"
	c4cConstValue = 1000

	// A RATE dimension alongside them. Its volume is the only quantity whose
	// arithmetic depends on which tier answered - a rate's sum has to be
	// multiplied by the interval its samples were collected at, and above
	// tier 0 that interval is not in the record's value. A constant rate
	// makes the answer exact, so a seam that mislabels which tier a record
	// came from cannot hide behind rollup noise.
	c4cRateDim   = "rate"
	c4cRateValue = 20

	// Exact condition fixtures. Availability is up for seven of every ten
	// seconds. The counter resets inside every tier1 record, with each new
	// floor one lower than the last. The raw drop and the rollup's new minimum
	// therefore prove the same one event per minute at every tier, including
	// the record crossing any possible plan seam.
	c4cAvailabilityDim = "availability"
	c4cCounterDim      = "counter"
	c4cConditionEpoch  = int64(fixture.T0 + 40)
	c4cCounterEpoch    = int64(fixture.T0 - 50)
	c4cCounterBase     = int64(1_000_000)
	c4cAvailabilityUp  = int64(7)
	c4cAvailabilityN   = int64(10)
	c4cCounterPeriod   = int64(60)

	// A negative flat line exposes whether options=absolute was applied to
	// the point read from the incoming plan at a tier seam.
	c4cNegativeDim   = "negative"
	c4cNegativeValue = -37
)

// c4cValue is the deterministic sample generator: a 24-bit full-mantissa
// mix per (dim, offset), SN-exact as an integer and incompressible as a
// page — the quota must fill with real bytes for rotation to happen.
func c4cValue(dim, off int64) int64 {
	x := uint64(off)*2654435761 + uint64(dim)*0x9E3779B97F4A7C15
	x ^= x >> 29
	x *= 0xBF58476D1CE4E5B9
	x ^= x >> 32
	return int64(x & 0xFFFFFF)
}

func c4cDimID(d int) string { return fmt.Sprintf("d%02d", d) }

func c4cCounterValueAt(ts int64) int64 {
	elapsed := ts - c4cCounterEpoch
	cycle := elapsed / c4cCounterPeriod
	return c4cCounterBase - cycle + elapsed%c4cCounterPeriod
}

// perTierRetention extracts db.per_tier by its explicit tier ID. Missing,
// duplicate, fractional or malformed entries cannot silently become epoch zero.
func perTierRetention(t *testing.T, doc map[string]any) []daemon.Retention {
	t.Helper()

	db, ok := doc["db"].(map[string]any)
	if !ok {
		t.Fatal("response has no db object")
	}
	tiersAny, ok := db["per_tier"].([]any)
	if !ok || len(tiersAny) == 0 {
		t.Fatalf("db.per_tier is missing or empty: %v", db["per_tier"])
	}

	out := make([]daemon.Retention, len(tiersAny))
	seen := make([]bool, len(tiersAny))
	for i, raw := range tiersAny {
		entry, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("db.per_tier[%d] is malformed: %v", i, raw)
		}
		tierRaw, tierOK := entry["tier"].(float64)
		firstRaw, firstOK := entry["first_entry"].(float64)
		lastRaw, lastOK := entry["last_entry"].(float64)
		tier := int(tierRaw)
		first, last := int64(firstRaw), int64(lastRaw)
		if !tierOK || tier < 0 || tier >= len(out) || tierRaw != float64(tier) ||
			!firstOK || firstRaw != float64(first) ||
			!lastOK || lastRaw != float64(last) {
			t.Fatalf("db.per_tier[%d] has invalid tier/retention fields: %v", i, entry)
		}
		if seen[tier] {
			t.Fatalf("db.per_tier repeats tier %d", tier)
		}
		seen[tier] = true
		out[tier] = daemon.Retention{FirstEntry: first, LastEntry: last}
	}
	for tier, found := range seen {
		if !found {
			t.Fatalf("db.per_tier is missing tier %d", tier)
		}
	}
	return out
}

func TestLayer4PlanSwitching(t *testing.T) {
	dd, err := daemon.Start(daemon.Options{
		Binary:                 netdataBinary,
		RunDir:                 t.TempDir(),
		TierRetentionMB:        [3]int{25}, // RRDENG_MIN_DISK_SPACE_MB — the floor
		ReplicationStepSeconds: 3600,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dd.Stop() })

	// stream the fixture: rows are generated per replication request —
	// materializing 11M points would cost hundreds of MB
	conn, err := stream.Connect(dd.Addr, daemon.StreamKey, stream.HostInfo{
		Hostname: "l4c-child", MachineGUID: guid(80),
	}, stream.CapsReplication)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	conn.DefineChart(stream.Chart{
		ID: c4cContext, Title: "plan switching", Units: "units",
		Family: "fixture", Context: c4cContext, UpdateEvery: 1,
	})
	for d := 0; d < c4cDims; d++ {
		conn.Dimension(c4cDimID(d), "", 1, 1)
	}
	conn.Dimension(c4cConstDim, "", 1, 1)
	conn.Dimension(c4cRateDim, "incremental", 1, 1)
	conn.Dimension(c4cAvailabilityDim, "", 1, 1)
	conn.Dimension(c4cCounterDim, "", 1, 1)
	conn.Dimension(c4cNegativeDim, "", 1, 1)
	firstT := int64(fixture.T0)
	lastT := int64(fixture.T0 + c4cRows)
	conn.ChartDefinitionEnd(firstT, lastT, lastT)

	served, err := conn.ServeReplication(
		map[string]struct{ FirstT, LastT int64 }{c4cContext: {FirstT: firstT, LastT: lastT}},
		lastT,
		func(_ string, after, before int64) []stream.ReplayRow {
			rows := make([]stream.ReplayRow, 0, before-after)
			for ts := after + 1; ts <= before; ts++ {
				row := stream.ReplayRow{T: ts, Dims: make([]stream.ReplayValue, c4cDims+5)}
				for d := 0; d < c4cDims; d++ {
					row.Dims[d] = stream.ReplayValue{
						ID:        c4cDimID(d),
						Collected: strconv.FormatInt(c4cValue(int64(d), ts-fixture.T0), 10),
						Flags:     stream.FlagNotAnomalous,
					}
				}
				row.Dims[c4cDims] = stream.ReplayValue{
					ID:        c4cConstDim,
					Collected: strconv.Itoa(c4cConstValue),
					Flags:     stream.FlagNotAnomalous,
				}
				row.Dims[c4cDims+1] = stream.ReplayValue{
					ID:        c4cRateDim,
					Collected: strconv.Itoa(c4cRateValue),
					Flags:     stream.FlagNotAnomalous,
				}
				row.Dims[c4cDims+2] = stream.ReplayValue{
					ID: c4cAvailabilityDim,
					Collected: strconv.FormatInt(
						c4AvailabilityValue(ts, c4cConditionEpoch, c4cAvailabilityN, c4cAvailabilityUp), 10),
					Flags: stream.FlagNotAnomalous,
				}
				row.Dims[c4cDims+3] = stream.ReplayValue{
					ID:        c4cCounterDim,
					Collected: strconv.FormatInt(c4cCounterValueAt(ts), 10),
					Flags:     stream.FlagNotAnomalous,
				}
				row.Dims[c4cDims+4] = stream.ReplayValue{
					ID:        c4cNegativeDim,
					Collected: strconv.Itoa(c4cNegativeValue),
					Flags:     stream.FlagNotAnomalous,
				}
				rows = append(rows, row)
			}
			return rows
		},
		10*time.Minute,
	)
	if err != nil {
		t.Fatalf("replication dialogue: %v (served %v)", err, served)
	}
	if served[c4cContext] != c4cRows {
		t.Fatalf("replication served %d rows, want %d", served[c4cContext], c4cRows)
	}

	// settle: the last sample must be queryable on tier0
	probe := func() []daemon.Retention {
		params := daemon.DataParams(c4cContext, lastT-60, lastT, 60)
		params.Set("scope_dimensions", c4cDimID(0))
		doc, err := dd.DataV3("l4c-child", params)
		if err != nil {
			return nil
		}
		return perTierRetention(t, doc)
	}
	deadline := time.Now().Add(5 * time.Minute)
	var tiers []daemon.Retention
	for {
		tiers = probe()
		if len(tiers) >= 2 && tiers[0].LastEntry >= lastT-60 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("ingest did not settle: per_tier %+v", tiers)
		}
		time.Sleep(time.Second)
	}

	// rotation: tier0's head must be gone, tier1 must keep the full history
	if tiers[0].FirstEntry <= firstT+3600 {
		t.Fatalf("tier0 did not rotate (first_entry t0%+d) — quota/volume sizing needs revisiting: per_tier %+v",
			tiers[0].FirstEntry-fixture.T0, tiers)
	}
	if tiers[1].FirstEntry > firstT+60 {
		t.Fatalf("tier1 lost its head (first_entry t0%+d): per_tier %+v", tiers[1].FirstEntry-fixture.T0, tiers)
	}
	boundary := tiers[0].FirstEntry
	t.Logf("tier0 head rotated to t0%+d; tier1 keeps t0%+d — plan-switch boundary discovered",
		boundary-fixture.T0, tiers[1].FirstEntry-fixture.T0)

	t.Run("plan-switching-seam", func(t *testing.T) {
		trackContractComponent(t, "L4/plan-switching", "seam")

		// 1s buckets straddling the boundary force tier0 as the selected
		// tier; the rotated head must come from a second tier1 plan.
		after, before := boundary-3600, boundary+3600
		params := daemon.DataParams(c4cContext, after, before, before-after)
		params.Set("scope_dimensions", c4cDimID(0))
		params.Set("options", "jsonwrap|unaligned")
		doc, err := dd.DataV3("l4c-child", params)
		if err != nil {
			t.Fatal(err)
		}
		if !assertTierPresence(t, doc, []bool{true, true, false}) {
			t.Fatal("expected exactly a tier1+tier0 plan switch")
		}
		if !assertExactView(t, doc, after, before, 1) {
			t.Error("plan-switching seam returned the wrong view grid")
		}

		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}
		dimension := c4cDimID(0)
		if !assertOnlyColumn(t, cols, dimension) {
			t.Error("plan-switching seam returned the wrong columns")
		}
		col := cols[dimension]
		if len(col) != int(before-after) {
			t.Fatalf("plan-switching seam returned %d rows, want exactly %d", len(col), before-after)
		}
		windows := map[int64]float64{} // tier1 window end → fetched average
		for i, pt := range col {
			wantT := after + int64(i+1)
			if pt.T != wantT {
				t.Errorf("plan-switching seam row %d ends at %d, want %d", i, pt.T, wantT)
			}
			if pt.Value == nil {
				t.Errorf("plan-switching seam row %d at %d is null inside retained history", i, pt.T)
				continue
			}
			if math.IsNaN(*pt.Value) || math.IsInf(*pt.Value, 0) {
				t.Errorf("plan-switching seam row %d at %d is non-finite: %v", i, pt.T, *pt.Value)
				continue
			}
			off := pt.T - fixture.T0
			switch {
			case pt.T > boundary+60:
				want := fixture.SNRoundTrip(float64(c4cValue(0, off)))
				if !tierValueMatch(*pt.Value, want, 0) {
					t.Errorf("tail bucket t0%+d: got %v, want %v", off, pt.Value, want)
				}
			case pt.T < boundary-60 && (pt.T-(fixture.T0+40))%tier1Gran == 0:
				if len(windows) == 0 {
					for end, tp := range tier1WindowsFor(0, after-tier1Gran, boundary) {
						windows[end] = tp
					}
				}
				want, ok := windows[pt.T]
				if !ok {
					continue
				}
				if !tierValueMatch(*pt.Value, want, 1e-9) {
					t.Errorf("head bucket t0%+d (tier1 grid): got %v, want %v", off, pt.Value, want)
				}
			}
		}
	})

	t.Run("plan-switching-head-only", func(t *testing.T) {
		trackContractComponent(t, "L4/plan-switching", "head-only")

		after, before := boundary-7200, boundary-3600
		params := daemon.DataParams(c4cContext, after, before, 60)
		dimension := c4cDimID(0)
		params.Set("scope_dimensions", dimension)
		params.Set("options", "jsonwrap|unaligned")
		doc, err := dd.DataV3("l4c-child", params)
		if err != nil {
			t.Fatal(err)
		}
		if !assertTierPresence(t, doc, []bool{false, true, false}) {
			t.Fatal("head-only query did not use tier1 alone")
		}
		if !assertExactView(t, doc, after, before, 60) {
			t.Error("head-only query returned the wrong view grid")
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}
		if !assertOnlyColumn(t, cols, dimension) {
			t.Error("head-only query returned the wrong columns")
		}
		col := cols[dimension]
		if len(col) != 60 {
			t.Fatalf("head-only query returned %d rows, want exactly 60", len(col))
		}
		for i, point := range col {
			wantT := after + int64(i+1)*60
			if point.T != wantT {
				t.Errorf("head-only row %d ends at %d, want %d", i, point.T, wantT)
			}
			if point.Value == nil || math.IsNaN(*point.Value) || math.IsInf(*point.Value, 0) {
				t.Errorf("head-only row %d at %d is missing or non-finite: %v", i, point.T, point.Value)
			}
		}
	})

	t.Run("sum-conservation", func(t *testing.T) {
		trackContract(t, "CASE-026/totals-survive-a-plan-switch")

		flatTiers := c4DimensionRetention(
			t, dd, c4cContext, "l4c-child", c4cConstDim, lastT, 2)
		flatBoundary := flatTiers[0].FirstEntry
		headAfter := c4cAlignTier1(flatBoundary - 10800)
		tailAfter := c4cAlignTier1(flatBoundary + 3600)
		seamAfter := c4cAlignTier1(flatBoundary - 3600)
		c4cRequireTwoTierWindow(t, "flat head-only", headAfter, headAfter+7200,
			flatTiers, [2]bool{false, true})
		c4cRequireTwoTierWindow(t, "flat tail-only", tailAfter, tailAfter+7200,
			flatTiers, [2]bool{true, false})
		c4cRequireTwoTierWindow(t, "flat straddling", seamAfter, seamAfter+7200,
			flatTiers, [2]bool{true, true})

		// The two single-plan windows are controls. If they conserve and
		// the straddling window does not, the seam is the difference.
		ok := c4cSumConserves(t, dd, "head-only", headAfter,
			[3]bool{false, true, false})
		ok = c4cSumConserves(t, dd, "tail-only", tailAfter,
			[3]bool{true, false, false}) && ok
		ok = c4cSumConserves(t, dd, "straddling", seamAfter,
			[3]bool{true, true, false}) && ok
		assertContract(t, "CASE-026/totals-survive-a-plan-switch", ok)
	})

	t.Run("rate-volume", func(t *testing.T) {
		trackContract(t, "CASE-031/rate-volume-across-an-automatic-seam")

		rateTiers := c4DimensionRetention(
			t, dd, c4cContext, "l4c-child", c4cRateDim, lastT, 2)
		rateBoundary := rateTiers[0].FirstEntry
		seamAfter, seamBefore := rateBoundary-3600, rateBoundary+3600
		headAfter, headBefore := c4cAlignTier1(rateBoundary-10800), c4cAlignTier1(rateBoundary-3600)
		tailAfter, tailBefore := c4cAlignTier1(rateBoundary+3600), c4cAlignTier1(rateBoundary+10800)
		c4cRequireTwoTierWindow(t, "rate seam", seamAfter, seamBefore,
			rateTiers, [2]bool{true, true})
		c4cRequireTwoTierWindow(t, "rate rotated head", headAfter, headBefore,
			rateTiers, [2]bool{false, true})
		c4cRequireTwoTierWindow(t, "rate tier0 tail", tailAfter, tailBefore,
			rateTiers, [2]bool{true, false})

		rok := c4cRateVolume(t, dd, "across-the-seam", seamAfter, seamBefore,
			[3]bool{false, true, false}, [3]bool{true, true, false})
		rok = c4cRateVolume(t, dd, "rotated-head-only", headAfter, headBefore,
			[3]bool{false, true, false}, [3]bool{false, true, false}) && rok
		rok = c4cRateVolume(t, dd, "tier0-only", tailAfter, tailBefore,
			[3]bool{false, true, false}, [3]bool{true, false, false}) && rok
		assertContract(t, "CASE-031/rate-volume-across-an-automatic-seam", rok)
	})

	t.Run("condition-groupings", func(t *testing.T) {
		trackContractComponent(t, "L4/plan-switching", "condition-groupings")

		const span = int64(7200)
		ok := true
		for _, dimension := range []struct {
			label            string
			id               string
			availabilityOnly bool
			counterOnly      bool
		}{
			{label: "availability", id: c4cAvailabilityDim, availabilityOnly: true},
			{label: "counter", id: c4cCounterDim, counterOnly: true},
		} {
			dimensionTiers := c4DimensionRetention(
				t, dd, c4cContext, "l4c-child", dimension.id, lastT, 2)
			dimensionBoundary := dimensionTiers[0].FirstEntry
			windowEpoch := c4cConditionEpoch
			if dimension.counterOnly {
				windowEpoch = c4cCounterEpoch
			}
			seamAfter := c4AlignDown(
				dimensionBoundary-3600, windowEpoch, c4cCounterPeriod)
			headAfter := c4AlignDown(
				dimensionBoundary-10800, windowEpoch, c4cCounterPeriod)
			tailAfter := c4AlignDown(
				dimensionBoundary+3600, windowEpoch, c4cCounterPeriod)
			c4cRequireTwoTierWindow(t, dimension.label+" seam", seamAfter, seamAfter+span,
				dimensionTiers, [2]bool{true, true})
			c4cRequireTwoTierWindow(t, dimension.label+" head", headAfter, headAfter+span,
				dimensionTiers, [2]bool{false, true})
			c4cRequireTwoTierWindow(t, dimension.label+" tail", tailAfter, tailAfter+span,
				dimensionTiers, [2]bool{true, false})

			for _, tc := range []struct {
				label        string
				after        int64
				rowSpan      int64
				expectedTier [3]bool
			}{
				{"seam-downsample", seamAfter, 300, [3]bool{false, true, false}},
				{"seam-identity", seamAfter, 60, [3]bool{false, true, false}},
				{"seam-upsample", seamAfter, 1, [3]bool{true, true, false}},
				{"head-downsample", headAfter, 300, [3]bool{false, true, false}},
				{"head-identity", headAfter, 60, [3]bool{false, true, false}},
				{"head-upsample", headAfter, 1, [3]bool{false, true, false}},
				{"tail-downsample", tailAfter, 300, [3]bool{false, true, false}},
				{"tail-identity", tailAfter, 60, [3]bool{false, true, false}},
				{"tail-upsample", tailAfter, 1, [3]bool{true, false, false}},
			} {
				if !c4ConditionExact(t, dd, c4ConditionSpec{
					label:              "layer4c-" + dimension.label + "-" + tc.label,
					context:            c4cContext,
					host:               "l4c-child",
					availabilityDim:    c4cAvailabilityDim,
					counterDim:         c4cCounterDim,
					after:              tc.after,
					before:             tc.after + span,
					rowSpan:            tc.rowSpan,
					selectedTier:       -1,
					expectedTiers:      tc.expectedTier,
					availabilityEpoch:  c4cConditionEpoch,
					availabilityPeriod: c4cAvailabilityN,
					availabilityUp:     c4cAvailabilityUp,
					counterPeriod:      c4cCounterPeriod,
					tier0First:         dimensionBoundary,
					availabilityOnly:   dimension.availabilityOnly,
					counterOnly:        dimension.counterOnly,
				}) {
					ok = false
				}
			}
		}
		if !ok {
			t.Errorf("condition groupings did not conserve exact fixture answers across the tier0/tier1 plan boundary")
		}
	})

	t.Run("absolute-across-plan-seam", func(t *testing.T) {
		trackContract(t, "CASE-036/absolute-across-plan-seam")

		const span = int64(7200)
		negativeTiers := c4DimensionRetention(
			t, dd, c4cContext, "l4c-child", c4cNegativeDim, lastT, 2)
		negativeBoundary := negativeTiers[0].FirstEntry
		if negativeBoundary <= negativeTiers[1].FirstEntry {
			t.Fatalf("negative dimension has no tier1-only history: %+v", negativeTiers)
		}
		seamAfter := c4AlignDown(negativeBoundary-3600, c4cConditionEpoch, c4cCounterPeriod)
		headAfter := c4AlignDown(negativeBoundary-10800, c4cConditionEpoch, c4cCounterPeriod)
		tailAfter := c4AlignDown(negativeBoundary+3600, c4cConditionEpoch, c4cCounterPeriod)
		c4cRequireTwoTierWindow(t, "absolute seam", seamAfter, seamAfter+span,
			negativeTiers, [2]bool{true, true})
		c4cRequireTwoTierWindow(t, "absolute rotated head", headAfter, headAfter+span,
			negativeTiers, [2]bool{false, true})
		c4cRequireTwoTierWindow(t, "absolute tier0 tail", tailAfter, tailAfter+span,
			negativeTiers, [2]bool{true, false})

		ok := c4cAbsoluteExact(t, dd, "seam", seamAfter, seamAfter+span,
			[3]bool{true, true, false})
		ok = c4cAbsoluteExact(t, dd, "rotated-head", headAfter, headAfter+span,
			[3]bool{false, true, false}) && ok
		ok = c4cAbsoluteExact(t, dd, "tier0-tail", tailAfter, tailAfter+span,
			[3]bool{true, false, false}) && ok
		assertContract(t, "CASE-036/absolute-across-plan-seam", ok)
	})
}

func c4PositiveMod(value, modulus int64) int64 {
	remainder := value % modulus
	if remainder < 0 {
		remainder += modulus
	}
	return remainder
}

func c4AlignDown(ts, epoch, granularity int64) int64 {
	return ts - c4PositiveMod(ts-epoch, granularity)
}

func c4AlignUp(ts, epoch, granularity int64) int64 {
	down := c4AlignDown(ts, epoch, granularity)
	if down < ts {
		return down + granularity
	}
	return down
}

func c4AvailabilityValue(ts, epoch, period, up int64) int64 {
	if c4PositiveMod(ts-epoch-1, period) < up {
		return 1
	}
	return 0
}

// The value reaches period-1 and drops to zero exactly at each period end.
// A window (epoch+k*period, epoch+(k+n)*period] therefore contains n resets.
func c4CounterValue(ts, epoch, period int64) int64 {
	return c4PositiveMod(ts-epoch, period)
}

type c4ConditionSpec struct {
	label                  string
	context, host          string
	availabilityDim        string
	counterDim             string
	after, before, rowSpan int64
	selectedTier           int // -1 lets the planner switch tiers
	expectedTiers          [3]bool
	availabilityEpoch      int64
	availabilityPeriod     int64
	availabilityUp         int64
	counterPeriod          int64
	tier0First             int64
	availabilityOnly       bool
	counterOnly            bool
}

func c4DimensionRetention(
	t *testing.T,
	dd *daemon.Daemon,
	context, host, dimension string,
	last int64,
	wantTiers int,
) []daemon.Retention {
	t.Helper()

	params := daemon.DataParams(context, last-60, last, 60)
	params.Set("scope_dimensions", dimension)
	doc, err := dd.DataV3(host, params)
	if err != nil {
		t.Fatal(err)
	}
	tiers := perTierRetention(t, doc)
	if len(tiers) < wantTiers {
		t.Fatalf("%s retention has %d tiers, want at least %d: %+v",
			dimension, len(tiers), wantTiers, tiers)
	}
	for tier := 0; tier < wantTiers; tier++ {
		if tiers[tier].FirstEntry <= 0 || tiers[tier].LastEntry < tiers[tier].FirstEntry {
			t.Fatalf("%s tier%d retention is invalid: %+v", dimension, tier, tiers[tier])
		}
	}
	return tiers
}

// c4cRequireTwoTierWindow proves that a planned control or seam window has the
// claimed physical relation to this dimension's own retention.
func c4cRequireTwoTierWindow(
	t *testing.T,
	label string,
	after, before int64,
	tiers []daemon.Retention,
	want [2]bool,
) {
	t.Helper()

	if len(tiers) < 2 {
		t.Fatalf("%s has fewer than two retention tiers: %+v", label, tiers)
	}
	wantTier0, wantTier1 := want[0], want[1]
	switch {
	case wantTier0 && wantTier1:
		if !(after < tiers[0].FirstEntry && before > tiers[0].FirstEntry) {
			t.Fatalf("%s (%d,%d] does not cross tier0 first entry %d",
				label, after, before, tiers[0].FirstEntry)
		}
	case wantTier1:
		if before >= tiers[0].FirstEntry {
			t.Fatalf("%s (%d,%d] reaches tier0 first entry %d",
				label, after, before, tiers[0].FirstEntry)
		}
	case wantTier0:
		if after < tiers[0].FirstEntry {
			t.Fatalf("%s (%d,%d] starts before tier0 first entry %d",
				label, after, before, tiers[0].FirstEntry)
		}
	default:
		t.Fatalf("%s requests neither tier0 nor tier1", label)
	}
	if wantTier1 && (after < tiers[1].FirstEntry || before > tiers[1].LastEntry) {
		t.Fatalf("%s (%d,%d] exceeds tier1 retention [%d,%d]",
			label, after, before, tiers[1].FirstEntry, tiers[1].LastEntry)
	}
	if wantTier0 && before > tiers[0].LastEntry {
		t.Fatalf("%s (%d,%d] exceeds tier0 last entry %d",
			label, after, before, tiers[0].LastEntry)
	}
}

func c4TierVectorExact(t *testing.T, doc map[string]any, want [3]bool) bool {
	t.Helper()
	return assertTierPresence(t, doc, want[:])
}

func c4ExpectedAvailability(spec c4ConditionSpec) []expectedColumnPoint {
	points := int((spec.before - spec.after) / spec.rowSpan)
	out := make([]expectedColumnPoint, points)
	higherShare := float64(spec.availabilityUp) / float64(spec.availabilityPeriod)
	hasTier0 := spec.expectedTiers[0]
	hasHigherTier := spec.expectedTiers[1] || spec.expectedTiers[2]

	rawAt := func(ts int64) bool {
		if spec.selectedTier == 0 {
			return true
		}
		if spec.selectedTier > 0 || !hasTier0 {
			return false
		}
		if !hasHigherTier {
			return true
		}
		return ts >= spec.tier0First
	}

	for i := range out {
		rowStart := spec.after + int64(i)*spec.rowSpan
		rowEnd := rowStart + spec.rowSpan
		matched := 0.0
		for ts := rowStart + 1; ts <= rowEnd; ts++ {
			if rawAt(ts) {
				matched += float64(c4AvailabilityValue(
					ts, spec.availabilityEpoch, spec.availabilityPeriod, spec.availabilityUp))
			} else {
				// Higher tiers retain min/max/sum/count, so the window
				// estimator is sample-weighted. These fixtures have a
				// constant sample cadence and aligned periods, making the
				// estimate exact.
				matched += higherShare
			}
		}
		out[i] = wantNumberAt(rowEnd, 100*matched/float64(spec.rowSpan))
	}
	return out
}

func c4ConditionExact(t *testing.T, dd *daemon.Daemon, spec c4ConditionSpec) bool {
	t.Helper()
	if spec.availabilityOnly && spec.counterOnly {
		t.Fatalf("condition query cannot be both availability-only and counter-only: %+v", spec)
	}
	runAvailability := !spec.counterOnly
	runCounter := !spec.availabilityOnly
	if spec.rowSpan <= 0 || spec.before <= spec.after ||
		(spec.before-spec.after)%spec.rowSpan != 0 {
		t.Fatalf("invalid exact condition window %+v", spec)
	}
	if runCounter && (spec.counterPeriod <= 0 ||
		(spec.before-spec.after)%spec.counterPeriod != 0) {
		t.Fatalf("invalid exact counter window %+v", spec)
	}
	if runAvailability && (spec.availabilityPeriod <= 0 ||
		spec.availabilityUp < 0 || spec.availabilityUp > spec.availabilityPeriod) {
		t.Fatalf("invalid exact availability window %+v", spec)
	}
	if runAvailability && spec.selectedTier < 0 && spec.expectedTiers[0] &&
		(spec.expectedTiers[1] || spec.expectedTiers[2]) && spec.tier0First <= 0 {
		t.Fatalf("mixed tier0/higher-tier condition query has no discovered tier0 boundary: %+v", spec)
	}
	points := (spec.before - spec.after) / spec.rowSpan
	ok := true
	for _, query := range []struct {
		group, expression, dimension string
		availability                 bool
	}{
		{
			group: "percentage-of-time", expression: "==1", dimension: spec.availabilityDim,
			availability: true,
		},
		{
			group: "number-of-times", expression: "<previous", dimension: spec.counterDim,
		},
	} {
		if query.availability && !runAvailability {
			continue
		}
		if !query.availability && !runCounter {
			continue
		}
		var params = daemon.DataParams(spec.context, spec.after, spec.before, points)
		if spec.selectedTier >= 0 {
			params = daemon.DataParamsTier(spec.context, spec.selectedTier,
				spec.after, spec.before, points, query.group)
		} else {
			params.Set("time_group", query.group)
		}
		params.Set("options", "jsonwrap|unaligned")
		params.Set("time_group_options", query.expression)
		params.Set("scope_dimensions", query.dimension)
		doc, err := dd.DataV3(spec.host, params)
		if err != nil {
			t.Fatal(err)
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}

		if !assertExactView(t, doc, spec.after, spec.before, spec.rowSpan) {
			t.Logf("%s %s(%s): view mismatch", spec.label, query.group, query.expression)
			ok = false
		}
		if !c4TierVectorExact(t, doc, spec.expectedTiers) {
			t.Logf("%s %s(%s): tier vector mismatch", spec.label, query.group, query.expression)
			ok = false
		}
		if spec.selectedTier >= 0 && !assertSelectedTier(t, doc, spec.selectedTier) {
			t.Logf("%s %s(%s): forced-tier proof failed", spec.label, query.group, query.expression)
			ok = false
		}
		if !assertOnlyColumn(t, cols, query.dimension) {
			t.Logf("%s %s(%s): result contains the wrong columns", spec.label, query.group, query.expression)
			ok = false
		}
		if query.availability {
			if !assertExactColumn(t, cols, query.dimension, c4ExpectedAvailability(spec), printTol) {
				t.Logf("%s %s(%s): exact availability oracle failed",
					spec.label, query.group, query.expression)
				ok = false
			}
		} else if !assertEventColumnShapeAndTotal(
			t, cols, query.dimension, spec.after, spec.before, spec.rowSpan,
			(spec.rowSpan+spec.counterPeriod-1)/spec.counterPeriod,
			float64((spec.before-spec.after)/spec.counterPeriod)) {
			// A higher-tier record proves that a reset happened but cannot
			// retain its exact sample timestamp. Pin conservation and reject
			// duplicated rows without making its within-record placement a
			// public contract.
			t.Logf("%s %s(%s): reset conservation oracle failed",
				spec.label, query.group, query.expression)
			ok = false
		}
	}
	return ok
}

func c4cAbsoluteExact(
	t *testing.T,
	dd *daemon.Daemon,
	label string,
	after, before int64,
	expectedTiers [3]bool,
) bool {
	t.Helper()
	params := daemon.DataParams(c4cContext, after, before, before-after)
	params.Set("time_group", "average")
	params.Set("scope_dimensions", c4cNegativeDim)
	params.Set("options", "jsonwrap|unaligned|absolute")
	doc, err := dd.DataV3("l4c-child", params)
	if err != nil {
		t.Fatal(err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}
	want := make([]expectedColumnPoint, before-after)
	for i := range want {
		want[i] = wantNumberAt(after+int64(i+1), -float64(c4cNegativeValue))
	}
	ok := assertExactView(t, doc, after, before, 1)
	if !c4TierVectorExact(t, doc, expectedTiers) {
		t.Logf("absolute %s: tier vector mismatch", label)
		ok = false
	}
	if !assertOnlyColumn(t, cols, c4cNegativeDim) {
		t.Logf("absolute %s: result contains the wrong columns", label)
		ok = false
	}
	if !assertExactColumn(t, cols, c4cNegativeDim, want, 0) {
		t.Logf("absolute %s: negative flat line did not remain positive in every row", label)
		ok = false
	}
	return ok
}

// c4cRateVolume totals the rate dimension over one window, with no tier pin,
// and compares it against what the fixture pushed: a constant rate collected
// once a second holds rate x seconds, whichever tier answers.
//
// It also asserts WHICH tiers answered. Without that, a run where the
// planner quietly served everything from one tier would report the same
// number and prove nothing about seams at all.
//
// The expected presence vectors cover all configured tiers at both zooms.
func c4cRateVolume(t *testing.T, dd *daemon.Daemon, label string, after, before int64,
	wantAt60s, wantAt1s [3]bool) bool {
	t.Helper()

	want := float64(before-after) * float64(c4cRateValue)
	ok := true

	// CASE-034 covers the known single-bucket window-normalization defect.
	// These two zooms isolate rate conservation and tier selection.
	for _, points := range []int64{(before - after) / 60, before - after} {
		params := daemon.DataParams(c4cContext, after, before, points)
		params.Set("time_group", "sum")
		params.Set("scope_dimensions", c4cRateDim)
		params.Set("options", "jsonwrap|unaligned")
		doc, err := dd.DataV3("l4c-child", params)
		if err != nil {
			t.Fatal(err)
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}
		wantTiers := wantAt60s
		if points == before-after {
			wantTiers = wantAt1s
		}
		rowSpan := (before - after) / points
		if !assertExactView(t, doc, after, before, rowSpan) {
			t.Logf("rate volume not met: [%s] at %d buckets has the wrong view", label, points)
			ok = false
		}
		if !assertTierPresence(t, doc, wantTiers[:]) {
			t.Logf("rate volume not met: [%s] at %d buckets has the wrong tier vector", label, points)
			ok = false
		}
		if !assertOnlyColumn(t, cols, c4cRateDim) {
			t.Logf("rate volume not met: [%s] at %d buckets returned the wrong columns", label, points)
			ok = false
		}
		if !assertColumnShapeAndTotal(t, cols, c4cRateDim, after, before, rowSpan, want) {
			t.Logf("rate volume not met: [%s] at %d buckets did not preserve %.10g",
				label, points, want)
			ok = false
		}
	}
	return ok
}

// c4cAlignTier1 rounds a timestamp down onto the tier1 record grid, whose
// windows end at T0+40 (mod 60) — see tier1WindowsFor.
func c4cAlignTier1(ts int64) int64 {
	return ts - (ts-(fixture.T0+40))%tier1Gran
}

// c4cSumConserves totals a sum query over one grid-aligned window at several
// zooms and compares each against what the fixture actually pushed.
//
// The dimension is flat, so the answer is arithmetic: the window holds
// c4cConstValue for every one of its seconds, whichever tier serves them and
// however finely they are cut. Nothing here is read back from the engine to
// decide what is right - the fixture already decided.
func c4cSumConserves(
	t *testing.T,
	dd *daemon.Daemon,
	label string,
	after int64,
	wantAt1s [3]bool,
) bool {
	t.Helper()

	const span = 7200 // whole tier1 records, and divisible by every zoom below
	before := after + span
	want := float64(c4cConstValue) * float64(span)
	ok := true

	for _, points := range []int64{span, span / 10, span / 60, span / 300} {
		// no tier pin: the planner must be free to cross the boundary
		params := daemon.DataParams(c4cContext, after, before, points)
		params.Set("time_group", "sum")
		params.Set("scope_dimensions", c4cConstDim)
		params.Set("options", "jsonwrap|unaligned")
		doc, err := dd.DataV3("l4c-child", params)
		if err != nil {
			t.Fatal(err)
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}

		if !assertOnlyColumn(t, cols, c4cConstDim) {
			t.Logf("plan-switch conservation not met: %s at %d buckets returned the wrong columns",
				label, points)
			ok = false
			continue
		}

		rowSpan := span / points
		if !assertExactView(t, doc, after, before, rowSpan) {
			t.Logf("plan-switch conservation not met: %s at %d buckets returned the wrong view",
				label, points)
			ok = false
		}
		if !assertColumnShapeAndTotal(
			t, cols, c4cConstDim, after, before, rowSpan, want) {
			t.Logf("plan-switch conservation not met: [%s] %ds buckets did not preserve exact shape and total",
				label, rowSpan)
			ok = false
		}
		if points == span && !assertTierPresence(t, doc, wantAt1s[:]) {
			t.Logf("plan-switch conservation not met: %s did not use the expected one-second tier plans",
				label)
			ok = false
		}

		col := cols[c4cConstDim]
		total := 0.0
		for _, pt := range col {
			if pt.Value != nil {
				total += *pt.Value
			}
		}

		t.Logf("plan-switch conservation [%s]: %ds buckets (%d points, per_tier %v) total %.10g, want %.10g",
			label, rowSpan, points, perTierPoints(doc), total, want)
	}

	return ok
}

// tier1WindowsFor computes the tier1 fetched averages (sum/count of the
// ORIGINAL generated values, f32-rounded per the page format) for dim over
// aligned windows ending in (after, before].
func tier1WindowsFor(dim int64, after, before int64) map[int64]float64 {
	out := map[int64]float64{}
	first := int64(fixture.T0 + 1)
	// align the first candidate end UP to the tier grid (ends ≡ T0+40 mod 60)
	start := after - (after-(fixture.T0+40))%tier1Gran
	if start <= after {
		start += tier1Gran
	}
	for end := start; end <= before; end += tier1Gran {
		var sum float64
		var count int
		for ts := end - tier1Gran + 1; ts <= end; ts++ {
			if ts < first {
				continue
			}
			sum += float64(c4cValue(dim, ts-fixture.T0))
			count++
		}
		if count > 0 {
			out[end] = float64(float32(sum)) / float64(count)
		}
	}
	return out
}
