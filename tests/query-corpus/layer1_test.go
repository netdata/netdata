// SPDX-License-Identifier: GPL-3.0-or-later

// Layer 1 — tier0 ingestion: stored points equal pushed points for every
// edge-data palette shape — gaps (leading/interior-run/trailing), resets,
// anomaly runs, negatives, zeros, single points, non-default update_every —
// plus the storage_number quantization contract and the three gap-only
// retention states pinned by the #23095 working-as-intended ruling.
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

func modVal(i int) string  { return strconv.Itoa(i % 10) }
func notAnom(_ int) string { return stream.FlagNotAnomalous }

func TestLayer1Palette(t *testing.T) {
	trackContract(t, "L1/palette")

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
					return stream.FlagNotAnomalous + stream.FlagReset
				}
				return stream.FlagNotAnomalous
			}),
		},
		"reset-anomalous": {
			hostname: "l1-resetanom", guid: guid(26),
			chart: fixture.Series("fixture.l1resetanom", "fixture.l1resetanom", fixture.T0, 60, 1, modVal, func(i int) string {
				if i == 20 {
					return stream.FlagReset // reset without 'A': reset AND anomalous
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
// nulls around it. CASE-034 separately pins the public timestamp grid,
// including queries asking for one result bucket.
func TestLayer1SinglePoint(t *testing.T) {
	trackContract(t, "L1/single-point")

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
	want := make([]expectedColumnPoint, 0, 10)
	for ts := int64(fixture.T0 - 3); ts <= fixture.T0+6; ts++ {
		if ts == fixture.T0+1 {
			want = append(want, wantNumberAt(ts, 7))
		} else {
			want = append(want, wantEmptyAt(ts))
		}
	}
	if !assertOnlyColumn(t, cols, "load") ||
		!assertExactColumn(t, cols, "load", want, 0) {
		t.Fatal("single-point query did not return its exact ten-row grid")
	}
}

// TestLayer1TrailingWindow pins the beyond-retention read: querying past
// the last stored point returns null points (annotated EMPTY) — at the
// fixed 2023 epoch no now-trimming applies.
func TestLayer1TrailingWindow(t *testing.T) {
	trackContract(t, "L1/trailing-window")

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
	want := make([]expectedColumnPoint, 0, 60)
	for i := int64(1); i <= 60; i++ {
		if i <= 45 {
			want = append(want, wantNumberAt(fixture.T0+i, float64(i%10)))
		} else {
			want = append(want, wantEmptyAt(fixture.T0+i))
		}
	}
	if !assertOnlyColumn(t, cols, "load") ||
		!assertExactColumn(t, cols, "load", want, 0) {
		t.Fatal("beyond-retention query did not return 45 numeric and 15 EMPTY rows")
	}
}

// TestLayer1Precision pins the storage_number quantization contract: the
// engine's stored values equal the Go port of pack/unpack
// (fixture.SNRoundTrip) within JSON print/parse tolerance.
func TestLayer1Precision(t *testing.T) {
	trackContract(t, "L1/precision")

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
	trackContract(t, "L1/gap-states")

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

	type ghostObservation struct {
		ret   daemon.Retention
		empty bool
		cols  map[string][]canon.Pt
	}
	ghostRetention := func(after, before int64) (ghostObservation, error) {
		params := daemon.DataParams(context, after, before, before-after)
		params.Set("scope_dimensions", "ghost")
		doc, err := td.DataV3(host, params)
		if err != nil {
			return ghostObservation{}, err
		}
		ret, ok := daemon.QueryRetention(doc)
		if !ok {
			return ghostObservation{}, fmt.Errorf("ghost query has no retention metadata")
		}
		if canon.EmptyResult(doc) {
			return ghostObservation{ret: ret, empty: true}, nil
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			return ghostObservation{}, fmt.Errorf("decode ghost result: %w", err)
		}
		return ghostObservation{ret: ret, cols: cols}, nil
	}
	assertGhostGrid := func(state string, after int64, observation ghostObservation) bool {
		t.Helper()
		if observation.empty {
			t.Errorf("state %s: exact empty result, want the ghost column", state)
			return false
		}
		want := make([]expectedColumnPoint, 0, 60)
		for i := int64(1); i <= 60; i++ {
			want = append(want,
				wantEmptyWithMetadataAt(after+i, 0, canon.AnnotationEmpty))
		}
		return assertOnlyColumn(t, observation.cols, "ghost") &&
			assertExactColumn(t, observation.cols, "ghost", want, 0)
	}

	// (a) LIVE: phantom retention advancing, all values null
	live, err := ghostRetention(t0, t0+60)
	if err != nil {
		t.Fatalf("state (a) live query: %v", err)
	}
	if live.ret.FirstEntry == 0 || live.ret.LastEntry == 0 {
		t.Errorf("state (a) LIVE: expected phantom retention for ghost-only query, got [%d,%d]",
			live.ret.FirstEntry, live.ret.LastEntry)
	}
	if !assertGhostGrid("(a) LIVE", t0, live) {
		t.Fail()
	}
	t.Logf("state (a) LIVE: ghost retention [%d,%d], %d rows (phantom, as ruled)",
		live.ret.FirstEntry, live.ret.LastEntry, len(live.cols["ghost"]))

	// (b) RESTART: gap-only pages discarded, retention gone
	if err := td.Restart(); err != nil {
		t.Fatal(err)
	}
	restarted, errB := ghostRetention(t0, t0+60)
	if errB != nil {
		t.Logf("state (b) RESTART: ghost-only query errored (%v) — pinning as no-retention", errB)
	} else if !restarted.empty {
		t.Errorf("state (b) RESTART: got a nonempty result with retention [%d,%d], want query error or exact empty result",
			restarted.ret.FirstEntry, restarted.ret.LastEntry)
	} else if restarted.ret.FirstEntry != 0 || restarted.ret.LastEntry != 0 {
		t.Errorf("state (b) RESTART: exact empty result has retention [%d,%d], want [0,0]",
			restarted.ret.FirstEntry, restarted.ret.LastEntry)
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
	nextAfter := t0 + 1
	next, errC := ghostRetention(nextAfter, t0+61)
	if errC != nil {
		t.Fatalf("state (c) query: %v", errC)
	}
	if next.ret.FirstEntry == 0 && next.ret.LastEntry == 0 {
		t.Errorf("state (c) NEXT ITERATION: ghost did not reappear (retention [0,0])")
	} else {
		if !assertGhostGrid("(c) NEXT ITERATION", nextAfter, next) {
			t.Fail()
		}
		t.Logf("state (c) NEXT ITERATION: ghost back with retention [%d,%d], %d rows, as ruled",
			next.ret.FirstEntry, next.ret.LastEntry, len(next.cols["ghost"]))
	}
}
