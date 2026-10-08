// SPDX-License-Identifier: GPL-3.0-or-later

// Layer 9 — the window/API surface: the virtual-points view oracle
// (fixture/viewpoints.go, the port of rrd2rrdr_query_execute's
// three-point loop) makes the boundary-interpolation semantics EXACT —
// upgrading the envelope pins the update_every sweep banked:
//   - a grid whose boundaries cut sample intervals serves an
//     interpolated boundary point per bucket, consuming the straddling
//     sample (its remainder never reaches the next bucket);
//   - off-grid charts re-time onto the absolute grid with interpolated
//     values.
//
// Plus the window normalization knobs: relative windows resolve
// against `before`, the (0,0) sentinels resolve to the default live window,
// a points request beyond the db resolution serves natural points (no
// upsampling), time_resampling forces the bucket size up (v1 gtime =
// v2 time_resampling), and /api/v2/data answers identically to
// /api/v3/data (shared implementation pinned).
package corpus

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

// l9 fixture: ue=30 at the RAW epoch — T0%30=20, so sample ends sit
// OFF the absolute 30s grid and every default-mode bucket boundary
// cuts a sample interval: the interpolation path runs on every bucket.
const (
	l9Context = "fixture.l9interp"
	l9UE      = 30
	l9N       = 400
)

func l9Fixture() fixture.Chart {
	return fixture.Series(l9Context, l9Context, fixture.T0, l9N, l9UE, func(i int) string {
		return strconv.Itoa(i % 1000)
	}, func(i int) string {
		if i >= 90 && i <= 170 {
			return stream.FlagEmpty
		}
		return stream.FlagNotAnomalous
	})
}

func l9Settle(t *testing.T) fixture.Chart {
	t.Helper()
	ch := l9Fixture()
	if _, err := td.WaitRetention("l9-interp", l9Context, ch.FirstT(), ch.LastT(), 2*time.Second); err != nil {
		pushLiveBurst(t, "l9-interp", guid(164), ch)
		if _, err := td.WaitRetention("l9-interp", l9Context, ch.FirstT(), ch.LastT(), 15*time.Second); err != nil {
			t.Fatal(err)
		}
	}
	return ch
}

// TestLayer9InterpolatedBuckets: the DEFAULT (aligned) grid over the
// off-phase fixture, EXACT values via the view oracle — the S5 sweep
// pinned only the envelope; this is the full contract.
func TestLayer9InterpolatedBuckets(t *testing.T) {
	trackContractComponent(t, "L9/virtual-points", "interpolated-buckets")

	ch := l9Settle(t)

	const group = 10
	span := int64(group * l9UE) // 300s buckets
	// after rounds UP to the absolute grid
	anchor := fixture.T0 + (span-fixture.T0%span)%span
	// before extends UP to a grid multiple past the data
	last := fixture.T0 + int64(l9N*l9UE)
	end := last + (span-last%span)%span
	lines := int((end - anchor) / span)

	dbPoints := ch.Dimensions[0].DBPoints(l9UE)
	buckets := fixture.ViewBuckets(dbPoints, anchor, span, lines)

	for _, tg := range []string{"average", "sum", "min", "max", "stddev"} {
		t.Run(tg, func(t *testing.T) {
			params := daemon.DataParams(l9Context, fixture.T0, last, int64(l9N/group))
			params.Set("time_group", tg)
			doc, err := td.DataV3("l9-interp", params)
			if err != nil {
				t.Fatal(err)
			}
			cols, err := canon.Columns(doc)
			if err != nil {
				t.Fatal(err)
			}
			col := cols[ch.Dimensions[0].ID]
			if len(col) != lines {
				t.Fatalf("got %d buckets, want %d", len(col), lines)
			}
			// sum is a volume, not a reading of the level at the
			// bucket's end - see fixture.ViewSumVolume
			exp := fixture.TGOracle(tg, "", buckets, group, lines)
			if tg == "sum" {
				exp = fixture.ViewSumVolume(dbPoints, anchor, span, lines)
			}
			for i, pt := range col {
				want := exp[i]
				bucketT := anchor + int64(i+1)*span
				if pt.T != bucketT {
					t.Errorf("%s bucket %d: time t0%+d, want t0%+d", tg, i, pt.T-fixture.T0, bucketT-fixture.T0)
					continue
				}
				switch {
				case want.Empty && pt.Value != nil:
					t.Errorf("%s bucket t0%+d: value %v, want null", tg, pt.T-fixture.T0, *pt.Value)
				case !want.Empty && pt.Value == nil:
					t.Errorf("%s bucket t0%+d: null, want %v", tg, pt.T-fixture.T0, want.Value)
				case !want.Empty && !tierValueMatch(*pt.Value, want.Value, 1e-9):
					t.Errorf("%s bucket t0%+d: value %v, want %v", tg, pt.T-fixture.T0, *pt.Value, want.Value)
				}
			}
		})
	}
}

// TestLayer9OffGridIdentity: identity (group=1) over the off-phase
// fixture — every slot value interpolates between the two samples the
// slot cuts; exact via the view oracle (upgrades the S5 envelope pin).
func TestLayer9OffGridIdentity(t *testing.T) {
	trackContractComponent(t, "L9/virtual-points", "off-grid-identity")

	ch := l9Settle(t)

	anchor := fixture.T0 + (int64(l9UE)-fixture.T0%int64(l9UE))%int64(l9UE)
	const lines = 60 // a slice is enough for the identity contract
	end := anchor + int64(lines*l9UE)

	buckets := fixture.ViewBuckets(ch.Dimensions[0].DBPoints(l9UE), anchor, int64(l9UE), lines)

	params := daemon.DataParams(l9Context, anchor, end, lines)
	doc, err := td.DataV3("l9-interp", params)
	if err != nil {
		t.Fatal(err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}
	col := cols[ch.Dimensions[0].ID]
	if len(col) != lines {
		t.Fatalf("got %d rows, want %d", len(col), lines)
	}
	// a line can collect more than one value (a whole-added sample plus
	// the boundary interpolation — the first line's usual shape); the
	// engine flushes the default average over them
	exp := fixture.TGOracle("average", "", buckets, 1, lines)
	for i, pt := range col {
		wantT := anchor + int64(i+1)*int64(l9UE)
		if pt.T != wantT {
			t.Errorf("row %d: time t0%+d, want t0%+d", i, pt.T-fixture.T0, wantT-fixture.T0)
			continue
		}
		want := exp[i]
		switch {
		case want.Empty && pt.Value != nil:
			t.Errorf("row t0%+d: value %v, want null", pt.T-fixture.T0, *pt.Value)
		case !want.Empty && pt.Value == nil:
			t.Errorf("row t0%+d: null, want %v", pt.T-fixture.T0, want.Value)
		case !want.Empty && !tierValueMatch(*pt.Value, want.Value, 1e-9):
			t.Errorf("row t0%+d: value %v, want %v", pt.T-fixture.T0, *pt.Value, want.Value)
		}
	}
}

// TestLayer9RelativeWindow: a negative `after` is relative to `before`
// — the response must be identical to the absolute equivalent.
func TestLayer9RelativeWindow(t *testing.T) {
	trackContractComponent(t, "L9/window-normalization", "relative-window")

	ch := l9Settle(t)
	_ = ch

	absolute := daemon.DataParams(l9Context, fixture.T0+6000, fixture.T0+9000, 10)
	relative := daemon.DataParams(l9Context, -3000, fixture.T0+9000, 10)

	docA, err := td.DataV3("l9-interp", absolute)
	if err != nil {
		t.Fatal(err)
	}
	docR, err := td.DataV3("l9-interp", relative)
	if err != nil {
		t.Fatal(err)
	}
	colsA, err := canon.Columns(docA)
	if err != nil {
		t.Fatal(err)
	}
	colsR, err := canon.Columns(docR)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(colsA, colsR) {
		t.Errorf("relative after=-3000 differs from the absolute window:\nabs: %v\nrel: %v", colsA, colsR)
	}
}

func l9DefaultWindowView(doc map[string]any) (int64, int64, error) {
	view, ok := doc["view"].(map[string]any)
	if !ok {
		return 0, 0, fmt.Errorf("view is missing or not an object: %v", doc["view"])
	}
	after, afterOK := queryInteger(view["after"])
	before, beforeOK := queryInteger(view["before"])
	if !afterOK || !beforeOK {
		return 0, 0, fmt.Errorf(
			"view after/before are not finite integers: %v/%v",
			view["after"], view["before"])
	}
	dimensions, ok := view["dimensions"].(map[string]any)
	if !ok {
		return 0, 0, fmt.Errorf(
			"view.dimensions is missing or not an object: %v", view["dimensions"])
	}
	ids, ok := dimensions["ids"].([]any)
	if !ok {
		return 0, 0, fmt.Errorf(
			"view.dimensions.ids is missing or not an array: %v", dimensions["ids"])
	}
	if len(ids) != 0 {
		return 0, 0, fmt.Errorf("view.dimensions.ids is %v, want empty", ids)
	}
	return after, before, nil
}

func TestL9DefaultWindowShapeGuards(t *testing.T) {
	build := func() map[string]any {
		return map[string]any{"view": map[string]any{
			"after": float64(100), "before": float64(700),
			"dimensions": map[string]any{"ids": []any{}},
		}}
	}
	if _, _, err := l9DefaultWindowView(build()); err != nil {
		t.Fatalf("valid default-window view rejected: %v", err)
	}
	for name, mutate := range map[string]func(map[string]any){
		"missing-ids": func(doc map[string]any) {
			delete(doc["view"].(map[string]any)["dimensions"].(map[string]any), "ids")
		},
		"wrong-ids-type": func(doc map[string]any) {
			doc["view"].(map[string]any)["dimensions"].(map[string]any)["ids"] = "none"
		},
		"nonempty-ids": func(doc map[string]any) {
			doc["view"].(map[string]any)["dimensions"].(map[string]any)["ids"] = []any{"fixture"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			doc := build()
			mutate(doc)
			if _, _, err := l9DefaultWindowView(doc); err == nil {
				t.Errorf("accepted %s default-window mutation", name)
			}
		})
	}
}

// TestLayer9DefaultRelativeWindow: 0 is a RELATIVE time — the (0,0)
// window resolves to the ~600s default window ENDING NOW (grid-aligned
// to the chosen view update_every), NOT the full retention. On the
// 2023 epoch fixture that window holds no data — the reason the
// harness settles via explicit windows.
func TestLayer9DefaultRelativeWindow(t *testing.T) {
	trackContractComponent(t, "L9/window-normalization", "default-relative-window")

	l9Settle(t)

	doc, err := td.DataV3("l9-interp", daemon.DataParams(l9Context, 0, 0, 10))
	if err != nil {
		t.Fatal(err)
	}
	// the empty-result response carries a flat view block
	after, before, err := l9DefaultWindowView(doc)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	if before < now-120 || before > now+120 {
		t.Errorf("(0,0) before resolved to %d, want ~now (%d)", before, now)
	}
	span := before - after
	if span < 480 || span > 660 {
		t.Errorf("(0,0) window span %d, want the ~600s default (grid-aligned)", span)
	}
}

// TestLayer9Upsampling: asking for more points than the db resolution
// serves INTERPOLATED sub-ue virtual slots (200 x 3s lines from 30s
// data) — exact via the view oracle's last-point reuse branch.
func TestLayer9Upsampling(t *testing.T) {
	trackContractComponent(t, "L9/virtual-points", "upsampling")

	ch := l9Settle(t)

	span := int64(600)
	anchor := fixture.T0 + (span-fixture.T0%span)%span
	const lines = 200 // 3s slots over 30s data
	ueView := span / lines

	// tier0 has NO backward plan expansion (the CASE-017 asymmetry):
	// the engine's stream starts at the first point ending AFTER the
	// window start, so the first straddler has no interpolation anchor
	// and serves raw — feed the oracle the same stream
	all := ch.Dimensions[0].DBPoints(l9UE)
	stream := all
	for i, p := range all {
		if p.End > anchor {
			stream = all[i:]
			break
		}
	}
	buckets := fixture.ViewBuckets(stream, anchor, ueView, lines)

	params := daemon.DataParams(l9Context, anchor, anchor+span, lines)
	doc, err := td.DataV3("l9-interp", params)
	if err != nil {
		t.Fatal(err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}
	col := cols["load"]
	if len(col) != lines {
		t.Fatalf("got %d rows, want %d upsampled slots", len(col), lines)
	}
	exp := fixture.TGOracle("average", "", buckets, 1, lines)
	for i, pt := range col {
		wantT := anchor + int64(i+1)*ueView
		if pt.T != wantT {
			t.Errorf("row %d: time t0%+d, want t0%+d", i, pt.T-fixture.T0, wantT-fixture.T0)
			continue
		}
		want := exp[i]
		switch {
		case want.Empty && pt.Value != nil:
			t.Errorf("slot t0%+d: value %v, want null", pt.T-fixture.T0, *pt.Value)
		case !want.Empty && pt.Value == nil:
			t.Errorf("slot t0%+d: null, want %v", pt.T-fixture.T0, want.Value)
		case !want.Empty && !tierValueMatch(*pt.Value, want.Value, 1e-9):
			t.Errorf("slot t0%+d: value %v, want %v", pt.T-fixture.T0, *pt.Value, want.Value)
		}
	}
}

// TestLayer9TimeResampling: v2/v3 time_resampling (v1: gtime) forces
// the bucket size to at least the requested seconds — with resampling
// 300 over a 3000s span and 100 requested points, buckets are 300s.
func TestLayer9TimeResampling(t *testing.T) {
	trackContractComponent(t, "L9/window-normalization", "time-resampling")

	ch := l9Settle(t)

	span := int64(3000)
	anchor := fixture.T0 + (span-fixture.T0%span)%span
	params := daemon.DataParams(l9Context, anchor, anchor+2*span, 200) // wants 30s buckets
	params.Set("time_resampling", "300")
	doc, err := td.DataV3("l9-interp", params)
	if err != nil {
		t.Fatal(err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}
	col := cols[ch.Dimensions[0].ID]
	if len(col) < 2 {
		t.Fatalf("got %d rows", len(col))
	}
	step := col[1].T - col[0].T
	if step < 300 {
		t.Errorf("time_resampling=300 produced %ds buckets, want >= 300s", step)
	}
}

// TestLayer9V2V3Parity: /api/v2/data and /api/v3/data share one
// implementation — identical params must produce identical results.
func TestLayer9V2V3Parity(t *testing.T) {
	trackContract(t, "L9/v2-v3-parity")

	l9Settle(t)

	params := daemon.DataParams(l9Context, fixture.T0, fixture.T0+int64(l9N*l9UE), 40)
	params.Set("time_group", "average")

	stripVolatile := func(doc map[string]any) map[string]any {
		delete(doc, "agents")
		delete(doc, "timings")
		delete(doc, "api") // 2 vs 3 by definition — the rest must match
		return doc
	}

	docs := map[string]map[string]any{}
	for _, api := range []string{"api/v2/data", "api/v3/data"} {
		doc, err := td.HostJSON("l9-interp", api, params)
		if err != nil {
			t.Fatal(err)
		}
		docs[api] = stripVolatile(doc)
	}

	a, _ := json.Marshal(docs["api/v2/data"])
	b, _ := json.Marshal(docs["api/v3/data"])
	if string(a) != string(b) {
		t.Errorf("v2 and v3 responses differ:\nv2: %.600s\nv3: %.600s", a, b)
	}
}

// TestLayer9NaturalPoints: options=natural-points serves the raw
// stored sample VALUES at db spacing — but the timestamps still snap
// onto the absolute ue grid (the same phase shift as every other
// view): "natural" means the count and the values, not the times.
func TestLayer9NaturalPoints(t *testing.T) {
	registerContract(t, "L9/natural-points-grid")
	registerContract(t, "L9/natural-points-values")

	ch := l9Settle(t)

	after := fixture.T0 + int64(3000)
	before := fixture.T0 + int64(6000)
	params := daemon.DataParams(l9Context, after, before, 100)
	params.Set("options", "jsonwrap|natural-points")
	doc, err := td.DataV3("l9-interp", params)
	if err != nil {
		t.Fatal(err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}
	col := cols["load"]

	// the natural samples in (after, before] — keep the full stream too:
	// the last row's interpolation partner sits OUTSIDE the window
	all := ch.Dimensions[0].DBPoints(l9UE)
	var want []fixture.DBPoint
	first := -1
	for i, p := range all {
		if p.End > after && p.End <= before {
			if first < 0 {
				first = i
			}
			want = append(want, p)
		}
	}
	// natural mode keeps the db count and spacing but the slot values
	// around region boundaries may be the RAW sample or its
	// phase-interpolation toward the next sample — pin the two-candidate
	// contract exactly (the full natural-mode slot selection is a
	// recorded deferral; the DEFAULT virtual-points mode is oracle-exact)
	snap := (int64(l9UE) - fixture.T0%int64(l9UE)) % int64(l9UE)
	phase := float64(snap) / float64(l9UE)

	t.Run("grid", func(t *testing.T) {
		trackContract(t, "L9/natural-points-grid")
		if len(col) != len(want) {
			t.Fatalf("got %d rows, want the %d natural samples", len(col), len(want))
		}
		for i, pt := range col {
			if pt.T != want[i].End+snap {
				t.Errorf("row %d: time t0%+d, want the grid-snapped t0%+d", i, pt.T-fixture.T0, want[i].End+snap-fixture.T0)
			}
		}
	})

	t.Run("values", func(t *testing.T) {
		trackContract(t, "L9/natural-points-values")
		if len(col) != len(want) {
			t.Fatalf("got %d rows, want the %d natural samples", len(col), len(want))
		}
		for i, pt := range col {
			if want[i].Gap {
				// the row at a gap's tail may already carry the next
				// sample's raw value (the boundary slot has no anchor)
				if pt.Value != nil {
					nextRaw := i+1 < len(want) && !want[i+1].Gap && tierValueMatch(*pt.Value, want[i+1].Value, 1e-9)
					if !nextRaw {
						t.Errorf("row t0%+d: value %v, want null (gap) or the next raw sample", pt.T-fixture.T0, *pt.Value)
					}
				}
				continue
			}
			if pt.Value == nil {
				t.Errorf("row t0%+d: null, want %v", pt.T-fixture.T0, want[i].Value)
				continue
			}
			raw := want[i].Value
			candidates := []float64{raw}
			if next := first + i + 1; next < len(all) && !all[next].Gap {
				candidates = append(candidates, raw+(all[next].Value-raw)*phase)
			}
			matched := false
			for _, c := range candidates {
				if tierValueMatch(*pt.Value, c, 1e-9) {
					matched = true
					break
				}
			}
			if !matched {
				t.Errorf("row t0%+d: value %v, want the raw %v or its phase-interpolation", pt.T-fixture.T0, *pt.Value, raw)
			}
		}
	})
}

// TestLayer9LiveEdgeGrid pins future-window normalization independently of the
// established near-live partial-data trimming exception. Any rows that survive
// wholly before retention remain explicit EMPTY rows.
func TestLayer9LiveEdgeGrid(t *testing.T) {
	registerContractComponent(t, "L9/window-normalization", "future-explicit-window")
	registerContract(t, "L9/live-edge-empty-outside-retention")

	const ue = 1
	const n = 65
	ctx := "fixture.l9edge"
	now := time.Now().Unix()
	base := now - n // rows at base+1 .. base+n ≈ now
	ch := fixture.Series(ctx, ctx, base, n, ue, strconv.Itoa, notAnom)
	pushLiveBurst(t, "l9-edge", guid(165), ch)
	if _, err := td.WaitRetention("l9-edge", ctx, ch.FirstT(), ch.LastT(), 15*time.Second); err != nil {
		t.Fatal(err)
	}

	// Ask past now. Clock normalization shifts the window back to NOW, but data
	// availability must not shorten the resulting grid at the collected edge.
	requestedBefore := now + 3600
	params := daemon.DataParams(ctx, base, requestedBefore, 10)
	params.Set("options", "jsonwrap|virtual-points|unaligned")
	queryStart := time.Now().Unix()
	doc, err := td.DataV3("l9-edge", params)
	queryEnd := time.Now().Unix()
	if err != nil {
		t.Fatal(err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}
	col := cols["load"]
	t.Run("future-window-normalization", func(t *testing.T) {
		trackContractComponent(t, "L9/window-normalization", "future-explicit-window")
		view := queryObject(t, doc, "view", "view")
		viewBefore, integer := queryInteger(view["before"])
		if !integer || viewBefore < queryStart-1 || viewBefore > queryEnd-1 {
			t.Fatalf("future request resolved before to %v, want query-time range [%d,%d]",
				view["before"], queryStart-1, queryEnd-1)
		}
		// Future normalization shifts both absolute endpoints by the same amount,
		// preserving the requested duration while ending at the query-time clock.
		// Returned rows may then be shortened by the separate near-live trimming
		// contract, so this component deliberately checks view geometry only.
		resolvedAfter := viewBefore - (requestedBefore - base)
		wantGrid := queryExpectedVirtualGrid(t, resolvedAfter, viewBefore, 10, false)
		if !assertViewFields(t, doc, wantGrid.after, wantGrid.before, wantGrid.updateEvery) {
			t.Fail()
		}
	})

	t.Run("empty-outside-retention", func(t *testing.T) {
		trackContract(t, "L9/live-edge-empty-outside-retention")
		emptyOutsideRetention := 0
		for _, point := range col {
			if point.T < ch.FirstT() {
				emptyOutsideRetention++
				if point.Value != nil || point.ARP != 0 || point.PA != canon.AnnotationEmpty {
					t.Errorf("pre-retention row at %d is value/arp/pa %v/%v/%d, want empty/0/%d",
						point.T, point.Value, point.ARP, point.PA, canon.AnnotationEmpty)
				}
			}
		}
		if emptyOutsideRetention == 0 {
			t.Error("fixture did not produce any grid row outside retention")
		}
	})
}
