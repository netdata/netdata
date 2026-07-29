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
	c4cRows    = 200_000 // ~10M samples ≈ 40MB of tier0 pages > the 25MB quota
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

// perTierRetention extracts db.per_tier[].{first,last}_entry.
func perTierRetention(doc map[string]any) []daemon.Retention {
	db, _ := doc["db"].(map[string]any)
	tiersAny, _ := db["per_tier"].([]any)
	out := make([]daemon.Retention, 0, len(tiersAny))
	for _, ta := range tiersAny {
		tier, _ := ta.(map[string]any)
		first, _ := tier["first_entry"].(float64)
		last, _ := tier["last_entry"].(float64)
		out = append(out, daemon.Retention{FirstEntry: int64(first), LastEntry: int64(last)})
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
	// materializing 10M points would cost hundreds of MB
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
	firstT := int64(fixture.T0)
	lastT := int64(fixture.T0 + c4cRows)
	conn.ChartDefinitionEnd(firstT, lastT, lastT)

	served, err := conn.ServeReplication(
		map[string]struct{ FirstT, LastT int64 }{c4cContext: {FirstT: firstT, LastT: lastT}},
		lastT,
		func(_ string, after, before int64) []stream.ReplayRow {
			rows := make([]stream.ReplayRow, 0, before-after)
			for ts := after + 1; ts <= before; ts++ {
				row := stream.ReplayRow{T: ts, Dims: make([]stream.ReplayValue, c4cDims+2)}
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
		return perTierRetention(doc)
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
		doc, err := dd.DataV3("l4c-child", params)
		if err != nil {
			t.Fatal(err)
		}
		ptp := perTierPoints(doc)
		if len(ptp) < 2 || ptp[0] == 0 || ptp[1] == 0 {
			t.Fatalf("expected a tier1+tier0 plan switch, per_tier points %v", ptp)
		}
		t.Logf("plan switch proven: per_tier points %v", ptp)

		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}
		col := cols[c4cDimID(0)]
		windows := map[int64]float64{} // tier1 window end → fetched average
		for _, pt := range col {
			off := pt.T - fixture.T0
			switch {
			case pt.T > boundary+60:
				want := fixture.SNRoundTrip(float64(c4cValue(0, off)))
				if pt.Value == nil || !tierValueMatch(*pt.Value, want, 0) {
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
				if pt.Value == nil || !tierValueMatch(*pt.Value, want, 1e-9) {
					t.Errorf("head bucket t0%+d (tier1 grid): got %v, want %v", off, pt.Value, want)
				}
			case pt.T < boundary-60:
				if pt.Value == nil {
					t.Errorf("head bucket t0%+d: null — tier1 did not serve the rotated range", off)
				}
			}
		}
	})

	t.Run("plan-switching-head-only", func(t *testing.T) {
		trackContractComponent(t, "L4/plan-switching", "head-only")

		params := daemon.DataParams(c4cContext, boundary-7200, boundary-3600, 60)
		params.Set("scope_dimensions", c4cDimID(0))
		doc, err := dd.DataV3("l4c-child", params)
		if err != nil {
			t.Fatal(err)
		}
		ptp := perTierPoints(doc)
		if len(ptp) < 2 || ptp[0] != 0 || ptp[1] == 0 {
			t.Fatalf("head-only query: expected tier1 alone, per_tier points %v", ptp)
		}
	})

	t.Run("sum-conservation", func(t *testing.T) {
		trackContract(t, "CASE-026/totals-survive-a-plan-switch")

		// The two single-plan windows are controls. If they conserve and
		// the straddling window does not, the seam is the difference.
		ok := c4cSumConserves(t, dd, "head-only", c4cAlignTier1(boundary-10800))
		ok = c4cSumConserves(t, dd, "tail-only", c4cAlignTier1(boundary+3600)) && ok
		ok = c4cSumConserves(t, dd, "straddling", c4cAlignTier1(boundary-3600)) && ok
		assertContract(t, "CASE-026/totals-survive-a-plan-switch", ok)
	})

	t.Run("rate-volume", func(t *testing.T) {
		trackContract(t, "CASE-031/rate-volume-across-an-automatic-seam")

		rok := c4cRateVolume(t, dd, "across-the-seam", boundary-3600, boundary+3600,
			[3]bool{false, true, false}, [3]bool{true, true, false})
		rok = c4cRateVolume(t, dd, "rotated-head-only",
			c4cAlignTier1(boundary-10800), c4cAlignTier1(boundary-3600),
			[3]bool{false, true, false}, [3]bool{false, true, false}) && rok
		rok = c4cRateVolume(t, dd, "tier0-only",
			c4cAlignTier1(boundary+3600), c4cAlignTier1(boundary+10800),
			[3]bool{false, true, false}, [3]bool{true, false, false}) && rok
		assertContract(t, "CASE-031/rate-volume-across-an-automatic-seam", rok)
	})
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
		col, has := cols[c4cRateDim]
		if !has || len(col) == 0 {
			t.Logf("rate volume not met: [%s] at %d buckets returned no data", label, points)
			ok = false
			continue
		}

		total := 0.0
		for _, pt := range col {
			if pt.Value != nil {
				total += *pt.Value
			}
		}
		ptp := perTierPoints(doc)
		t.Logf("rate volume [%s]: %d buckets (per_tier %v) total %.10g, want %.10g",
			label, points, ptp, total, want)

		wantTiers := wantAt60s
		if points == before-after {
			wantTiers = wantAt1s
		}
		if len(ptp) != len(wantTiers) {
			t.Logf("rate volume not met: [%s] per_tier has %d tiers, want %d: %v",
				label, len(ptp), len(wantTiers), ptp)
			ok = false
		} else {
			for tier, wantTier := range wantTiers {
				if got := ptp[tier] > 0; got != wantTier {
					t.Logf("rate volume not met: [%s] tier %d contributed=%v, want %v "+
						"at %d buckets (per_tier %v)",
						label, tier, got, wantTier, points, ptp)
					ok = false
				}
			}
		}

		if math.Abs(total-want) > 1e-6 {
			t.Logf("rate volume not met: [%s] at %d buckets totals %.10g over %ds of a "+
				"constant %d/s, which is %.10g - a rate's volume is the interval its samples "+
				"were collected at, and that is a property of the record, not of the tier "+
				"serving the query",
				label, points, total, before-after, c4cRateValue, want)
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
func c4cSumConserves(t *testing.T, dd *daemon.Daemon, label string, after int64) bool {
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

		col, has := cols[c4cConstDim]
		if !has {
			t.Logf("plan-switch conservation not met: %s at %d buckets returned no column", label, points)
			ok = false
			continue
		}

		total := 0.0
		for _, pt := range col {
			if pt.Value != nil {
				total += *pt.Value
			}
		}

		bucket := span / points
		t.Logf("plan-switch conservation [%s]: %ds buckets (%d points, per_tier %v) total %.10g, want %.10g",
			label, bucket, points, perTierPoints(doc), total, want)

		if math.Abs(total-want) > want*1e-6 {
			t.Logf("plan-switch conservation not met: [%s] %ds buckets total %.10g over a window "+
				"holding a flat %d for %ds, which is %.10g (off by %.4g seconds of data) - a flat "+
				"line has nothing a coarser tier can lose",
				label, bucket, total, c4cConstValue, span, want,
				(total-want)/float64(c4cConstValue))
			ok = false
		}
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
