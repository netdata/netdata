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
		Tier0DiskSpaceMB:       25, // RRDENG_MIN_DISK_SPACE_MB — the floor
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
	firstT := int64(fixture.T0)
	lastT := int64(fixture.T0 + c4cRows)
	conn.ChartDefinitionEnd(firstT, lastT, lastT)

	served, err := conn.ServeReplication(
		map[string]struct{ FirstT, LastT int64 }{c4cContext: {FirstT: firstT, LastT: lastT}},
		lastT,
		func(_ string, after, before int64) []stream.ReplayRow {
			rows := make([]stream.ReplayRow, 0, before-after)
			for ts := after + 1; ts <= before; ts++ {
				row := stream.ReplayRow{T: ts, Dims: make([]stream.ReplayValue, c4cDims)}
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

	// (1) SWITCHING query: 1s buckets straddling the boundary force tier0
	// as the selected tier (only it satisfies the density), and the head
	// must come from a second tier1 plan
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
			// tail: tier0 identity — exact sample values
			want := fixture.SNRoundTrip(float64(c4cValue(0, off)))
			if pt.Value == nil || !tierValueMatch(*pt.Value, want, 0) {
				t.Errorf("tail bucket t0%+d: got %v, want %v", off, pt.Value, want)
			}
		case pt.T < boundary-60 && (pt.T-(fixture.T0+40))%tier1Gran == 0:
			// head, on the tier1 grid: the bucket lands exactly on a tier1
			// point end, where interpolation is exact — value equals that
			// window's fetched average
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
			// head, off-grid: coarse points interpolate linearly — pin
			// only that data exists (tier1 serves the rotated range)
			if pt.Value == nil {
				t.Errorf("head bucket t0%+d: null — tier1 did not serve the rotated range", off)
			}
		}
	}

	// (2) HEAD-ONLY query: entirely inside the rotated range — tier1 must
	// serve alone, tier0 contributes nothing
	params = daemon.DataParams(c4cContext, boundary-7200, boundary-3600, 60)
	params.Set("scope_dimensions", c4cDimID(0))
	doc, err = dd.DataV3("l4c-child", params)
	if err != nil {
		t.Fatal(err)
	}
	ptp = perTierPoints(doc)
	if len(ptp) < 2 || ptp[0] != 0 || ptp[1] == 0 {
		t.Fatalf("head-only query: expected tier1 alone, per_tier points %v", ptp)
	}
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
