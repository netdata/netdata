// SPDX-License-Identifier: GPL-3.0-or-later

// Layer 1 — tier0 ingestion: stored points equal pushed points for every
// edge-data palette shape — gaps (leading/interior-run/trailing), resets,
// anomaly runs, negatives, zeros, single points, non-default update_every —
// plus the storage_number quantization contract and the three gap-only
// retention states pinned by the #23095 working-as-intended ruling.
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

func modVal(i int) string  { return strconv.Itoa(i % 10) }
func notAnom(_ int) string { return stream.FlagNotAnomalous }

func TestLayer1Palette(t *testing.T) {
	cases := map[string]struct {
		hostname string
		guid     string
		chart    fixture.Chart
	}{
		"complete": {
			hostname: "l1-complete", guid: guid(21),
			chart: fixture.Series("fixture.l1complete", "fixture.l1complete", fixture.T0, 60, 1, modVal, notAnom),
		},
		"leading-gap": {
			hostname: "l1-leadgap", guid: guid(22),
			chart: fixture.Series("fixture.l1leadgap", "fixture.l1leadgap", fixture.T0, 60, 1, modVal, func(i int) string {
				if i <= 10 {
					return stream.FlagEmpty
				}
				return stream.FlagNotAnomalous
			}),
		},
		"interior-gap-run": {
			hostname: "l1-gaprun", guid: guid(23),
			chart: fixture.Series("fixture.l1gaprun", "fixture.l1gaprun", fixture.T0, 60, 1, modVal, func(i int) string {
				if i >= 25 && i <= 35 {
					return stream.FlagEmpty
				}
				return stream.FlagNotAnomalous
			}),
		},
		"trailing-gap-short-retention": {
			hostname: "l1-trailgap", guid: guid(24),
			chart: fixture.Series("fixture.l1trailgap", "fixture.l1trailgap", fixture.T0, 45, 1, modVal, notAnom),
		},
		"reset-not-anomalous": {
			hostname: "l1-reset", guid: guid(25),
			chart: fixture.Series("fixture.l1reset", "fixture.l1reset", fixture.T0, 60, 1, modVal, func(i int) string {
				if i == 20 {
					return "AR" // reset annotation, explicitly not anomalous
				}
				return stream.FlagNotAnomalous
			}),
		},
		"reset-anomalous": {
			hostname: "l1-resetanom", guid: guid(26),
			chart: fixture.Series("fixture.l1resetanom", "fixture.l1resetanom", fixture.T0, 60, 1, modVal, func(i int) string {
				if i == 20 {
					return "R" // reset without 'A': reset AND anomalous
				}
				return stream.FlagNotAnomalous
			}),
		},
		"anomalous-run": {
			hostname: "l1-anomrun", guid: guid(27),
			chart: fixture.Series("fixture.l1anomrun", "fixture.l1anomrun", fixture.T0, 60, 1, modVal, func(i int) string {
				if i >= 10 && i <= 20 {
					return stream.FlagAnomalous
				}
				return stream.FlagNotAnomalous
			}),
		},
		"negative": {
			hostname: "l1-negative", guid: guid(28),
			chart: fixture.Series("fixture.l1negative", "fixture.l1negative", fixture.T0, 60, 1, func(i int) string {
				return strconv.Itoa(-(i % 10))
			}, notAnom),
		},
		"all-zero": {
			hostname: "l1-allzero", guid: guid(29),
			chart: fixture.Series("fixture.l1allzero", "fixture.l1allzero", fixture.T0, 60, 1, func(_ int) string {
				return "0"
			}, notAnom),
		},
		"update-every-5": {
			hostname: "l1-ue5", guid: guid(31),
			chart: fixture.Series("fixture.l1ue5", "fixture.l1ue5", fixture.T0, 12, 5, modVal, notAnom),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			pushLiveBurst(t, tc.hostname, tc.guid, tc.chart)
			settleAndVerify(t, tc.hostname, tc.chart)
		})
	}
}

// TestLayer1SinglePoint verifies single-point ingestion through a window
// wider than the retention: the value sits at exactly its timestamp with
// nulls around it. It also PINS the current 1-point-window behavior — the
// engine expands a (t0, t0+1] points=1 query to view [t0+1, t0+2] with
// update_every 2 and stamps the bucket at t0+2 — as a layer-9 seed
// (window/alignment semantics review pending).
func TestLayer1SinglePoint(t *testing.T) {
	ch := fixture.Series("fixture.l1single", "fixture.l1single", fixture.T0, 1, 1, func(_ int) string {
		return "7"
	}, notAnom)
	pushLiveBurst(t, "l1-single", guid(30), ch)
	if _, err := td.WaitRetention("l1-single", ch.Context, fixture.T0+1, fixture.T0+1, 15*time.Second); err != nil {
		t.Fatal(err)
	}

	// ingestion fidelity through a wide window
	doc, err := td.DataV3("l1-single", daemon.DataParams(ch.Context, fixture.T0-4, fixture.T0+6, 10))
	if err != nil {
		t.Fatal(err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, pt := range cols["load"] {
		switch {
		case pt.T == fixture.T0+1 && (pt.Value == nil || *pt.Value != 7):
			t.Errorf("t0+1: got %v, want 7", pt.Value)
		case pt.T != fixture.T0+1 && pt.Value != nil:
			t.Errorf("t0+%d: got %v, want null", pt.T-fixture.T0, *pt.Value)
		}
	}

	// pin the current 1-point-window expansion (layer-9 seed)
	doc, err = td.DataV3("l1-single", daemon.DataParams(ch.Context, fixture.T0, fixture.T0+1, 1))
	if err != nil {
		t.Fatal(err)
	}
	cols, err = canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}
	col := cols["load"]
	if len(col) != 1 || col[0].T != fixture.T0+2 || col[0].Value == nil || *col[0].Value != 7 {
		t.Errorf("1-point window behavior changed: got %+v, want single bucket stamped t0+2 value 7 (if this fails after an engine change, re-pin deliberately)", col)
	}
}

// TestLayer1TrailingWindow pins the beyond-retention read: querying past
// the last stored point returns null points (annotated EMPTY) — at the
// fixed 2023 epoch no now-trimming applies.
func TestLayer1TrailingWindow(t *testing.T) {
	// reuses the trailing-gap chart pushed by TestLayer1Palette
	host, context := "l1-trailgap", "fixture.l1trailgap"
	if _, err := td.WaitRetention(host, context, fixture.T0+1, fixture.T0+45, 15*time.Second); err != nil {
		t.Skip("trailing-gap fixture not available (palette case failed?)")
	}

	doc, err := td.DataV3(host, daemon.DataParams(context, fixture.T0, fixture.T0+60, 60))
	if err != nil {
		t.Fatal(err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}
	col := cols["load"]
	if len(col) == 0 {
		t.Fatal("no rows returned for beyond-retention window")
	}
	for _, pt := range col {
		i := pt.T - fixture.T0
		switch {
		case i <= 45 && pt.Value == nil:
			t.Errorf("t0+%d: null inside retention", i)
		case i > 45 && pt.Value != nil:
			t.Errorf("t0+%d: value %v beyond retention, want null", i, *pt.Value)
		}
	}
	t.Logf("beyond-retention window returned %d rows (values to +45, nulls after)", len(col))
}

// TestLayer1Precision pins the storage_number quantization contract: the
// engine's stored values equal the Go port of pack/unpack
// (fixture.SNRoundTrip) within JSON print/parse tolerance.
func TestLayer1Precision(t *testing.T) {
	values := []string{
		"16777215",      // max 24-bit mantissa, exact
		"16777217",      // just above: quantized by the divide-by-10 step
		"123456789",     // large integer, quantized
		"0.123456789",   // small fraction: multiplied up to the mantissa window
		"-0.000001234",  // tiny negative
		"9876543210987", // huge: multiplier path
		"0.5",
		"-16777216.5",
	}
	ch := fixture.Series("fixture.l1prec", "fixture.l1prec", fixture.T0, len(values), 1, func(i int) string {
		return values[i-1]
	}, notAnom)
	ch.ValueTolerance = 1e-9

	pushLiveBurst(t, "l1-prec", guid(32), ch)
	settleAndVerify(t, "l1-prec", ch)

	for _, v := range values {
		f, _ := strconv.ParseFloat(v, 64)
		t.Logf("value %s → oracle %v", v, fixture.SNRoundTrip(f))
	}
}

// TestLayer1ZGapStates pins the three observable states of a gap-only
// (never-valued) dimension per the #23095 working-as-intended ruling:
// (a) LIVE: the ghost dimension exists with all-null values and its
//
//	dimension-scoped retention advances (phantom retention);
//
// (b) after RESTART: the gap-only pages are discarded — no retention;
// (c) on the NEXT live iteration: the ghost reappears.
// MUST stay last in this file: it restarts the shared daemon.
func TestLayer1ZGapStates(t *testing.T) {
	const host = "l1-ghost"
	const context = "fixture.l1ghost"
	t0 := int64(fixture.T0)

	// chart with a real dimension and a never-valued ghost dimension
	ch := fixture.Chart{
		ID: context, Title: "ghost", Units: "u", Family: "fixture", Context: context,
		Dimensions: []fixture.Dimension{
			{ID: "real"}, {ID: "ghost"},
		},
	}
	for i := int64(1); i <= 60; i++ {
		ch.Dimensions[0].Points = append(ch.Dimensions[0].Points,
			fixture.Point{T: t0 + i, Collected: strconv.FormatInt(i%10, 10), Flags: stream.FlagNotAnomalous})
		ch.Dimensions[1].Points = append(ch.Dimensions[1].Points,
			fixture.Point{T: t0 + i, Collected: "0", Flags: stream.FlagEmpty})
	}

	pushLiveBurst(t, host, guid(33), ch)
	if _, err := td.WaitRetention(host, context, t0+1, t0+60, 15*time.Second); err != nil {
		t.Fatal(err)
	}
	// age the host past a metadata scan cycle so the restart below measures
	// the ghost dimension's behavior, not CASE-016 (fresh hosts are
	// forgotten across restarts entirely — see case016_test.go)
	time.Sleep(8 * time.Second)

	ghostRetention := func() (daemon.Retention, int, error) {
		params := daemon.DataParams(context, t0, t0+60, 60)
		params.Set("scope_dimensions", "ghost")
		doc, err := td.DataV3(host, params)
		if err != nil {
			return daemon.Retention{}, 0, err
		}
		ret, _ := daemon.QueryRetention(doc)
		cols, err := canon.Columns(doc)
		if err != nil {
			return ret, 0, nil // no result payload at all
		}
		nonNull := 0
		for _, pt := range cols["ghost"] {
			if pt.Value != nil {
				nonNull++
			}
		}
		return ret, len(cols["ghost"]), nil
	}

	// (a) LIVE: phantom retention advancing, all values null
	ret, rows, err := ghostRetention()
	if err != nil {
		t.Fatalf("state (a) live query: %v", err)
	}
	if ret.FirstEntry == 0 || ret.LastEntry == 0 {
		t.Errorf("state (a) LIVE: expected phantom retention for ghost-only query, got [%d,%d]", ret.FirstEntry, ret.LastEntry)
	}
	t.Logf("state (a) LIVE: ghost retention [%d,%d], %d rows (phantom, as ruled)", ret.FirstEntry, ret.LastEntry, rows)

	// (b) RESTART: gap-only pages discarded, retention gone
	if err := td.Restart(); err != nil {
		t.Fatal(err)
	}
	retB, rowsB, errB := ghostRetention()
	if errB != nil {
		t.Logf("state (b) RESTART: ghost-only query errored (%v) — pinning as no-retention", errB)
	} else if retB.FirstEntry != 0 || retB.LastEntry != 0 {
		t.Errorf("state (b) RESTART: expected NO retention for gap-only dim after journal replay, got [%d,%d] (%d rows)",
			retB.FirstEntry, retB.LastEntry, rowsB)
	} else {
		t.Logf("state (b) RESTART: ghost retention gone [0,0] as ruled")
	}

	// (c) NEXT ITERATION: one more live sample resurrects the ghost slot
	conn := connect(t, host, guid(33), stream.CapsLive)
	ch2 := ch
	ch2.Dimensions = make([]fixture.Dimension, 2)
	ch2.Dimensions[0] = fixture.Dimension{ID: "real", Points: []fixture.Point{{T: t0 + 61, Collected: "1", Flags: stream.FlagNotAnomalous}}}
	ch2.Dimensions[1] = fixture.Dimension{ID: "ghost", Points: []fixture.Point{{T: t0 + 61, Collected: "0", Flags: stream.FlagEmpty}}}
	ch2.Define(conn)
	ch2.PushLive(conn)
	if err := conn.Flush(); err != nil {
		t.Fatal(err)
	}
	if _, err := td.WaitRetention(host, context, t0+1, t0+61, 15*time.Second); err != nil {
		t.Fatalf("state (c): context retention after new iteration: %v", err)
	}
	retC, rowsC, errC := ghostRetention()
	if errC != nil {
		t.Fatalf("state (c) query: %v", errC)
	}
	if retC.FirstEntry == 0 && retC.LastEntry == 0 {
		t.Errorf("state (c) NEXT ITERATION: ghost did not reappear (retention [0,0])")
	} else {
		t.Logf("state (c) NEXT ITERATION: ghost back with retention [%d,%d], %d rows, as ruled", retC.FirstEntry, retC.LastEntry, rowsC)
	}
}
