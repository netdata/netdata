// SPDX-License-Identifier: GPL-3.0-or-later

// Layer 8 — post-processing options over the query result:
//
//   - percentage: per ROW each dimension becomes value*100/row_total
//     (EMPTY cells excluded from the total; total==0 keeps values as-is
//     via the divisor-1 guard) — non-raw only;
//   - absolute: |value| applied at FETCH time, before any grouping;
//   - nonzero: all-zero dimensions are dropped — unless every dimension
//     is zero, in which case the option neutralizes itself;
//   - null2zero: gap cells become 0 values;
//   - cardinality_limit=N: keep the top N-1 dimensions by |view sum| and
//     fold the rest into one "remaining X dimensions" slot (per-row sum
//     of the folded values).
//
// Live-edge partial trimming needs near-now fixtures that conflict with
// the fixed 2023 epoch — deferred to layer 9 (window/API semantics).
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
	l8Context = "fixture.l8"
	l8Rows    = 40
)

// l8Value is the fixture generator: pos 1..7, neg -1..-5, zero 0,
// gappy 1..3 with a gap at rows 11-20.
func l8Value(dim string, i int) (v float64, gap bool) {
	switch dim {
	case "pos":
		return float64(i%7 + 1), false
	case "neg":
		return float64(-(i%5 + 1)), false
	case "zero":
		return 0, false
	case "gappy":
		return float64(i%3 + 1), i >= 11 && i <= 20
	}
	panic("unknown dim " + dim)
}

var l8Dims = []string{"pos", "neg", "zero", "gappy"}

func l8Fixture() fixture.Chart {
	ch := fixture.Chart{
		ID: l8Context, Title: "post-processing", Units: "units", Family: "fixture",
		Context: l8Context, UpdateEvery: 1,
	}
	for _, dim := range l8Dims {
		d := fixture.Dimension{ID: dim}
		for i := 1; i <= l8Rows; i++ {
			v, gap := l8Value(dim, i)
			p := fixture.Point{T: fixture.T0 + int64(i), Collected: strconv.FormatFloat(v, 'f', -1, 64), Flags: stream.FlagNotAnomalous}
			if gap {
				p.Flags = stream.FlagEmpty
			}
			d.Points = append(d.Points, p)
		}
		ch.Dimensions = append(ch.Dimensions, d)
	}
	return ch
}

func l8Query(t *testing.T, host, context, options string, extra map[string]string) map[string][]canon.Pt {
	t.Helper()
	params := daemon.DataParams(context, fixture.T0, fixture.T0+l8Rows, l8Rows)
	params.Set("options", options)
	for k, v := range extra {
		params.Set(k, v)
	}
	doc, err := td.DataV3(host, params)
	if err != nil {
		t.Fatal(err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}
	return cols
}

func TestLayer8PostProcessing(t *testing.T) {
	ch := l8Fixture()
	pushLiveBurst(t, "l8-post", guid(92), ch)
	if _, err := td.WaitRetention("l8-post", ch.Context, fixture.T0+1, fixture.T0+l8Rows, 15*time.Second); err != nil {
		t.Fatal(err)
	}

	t.Run("percentage", func(t *testing.T) {
		// v2/v3 FORCE options=absolute together with percentage
		// (api_v2_data.c) — the share is computed over |values|, signs
		// are erased at fetch time
		cols := l8Query(t, "l8-post", l8Context, "jsonwrap|percentage", nil)
		if len(cols) != len(l8Dims) {
			t.Fatalf("got %d dims %v, want %d", len(cols), keys2(cols), len(l8Dims))
		}
		for i := 1; i <= l8Rows; i++ {
			total := 0.0
			for _, dim := range l8Dims {
				if v, gap := l8Value(dim, i); !gap {
					total += math.Abs(v)
				}
			}
			divisor := total
			if divisor == 0 {
				divisor = 1.0
			}
			for _, dim := range l8Dims {
				v, gap := l8Value(dim, i)
				pt := cols[dim][i-1]
				if int(pt.T-fixture.T0) != i {
					t.Fatalf("row alignment broke: %d vs %d", pt.T-fixture.T0, i)
				}
				if gap {
					if pt.Value != nil {
						t.Errorf("%s row %d: value %v, want null", dim, i, *pt.Value)
					}
					continue
				}
				want := math.Abs(v) * 100.0 / divisor
				if pt.Value == nil || !tierValueMatch(*pt.Value, want, 1e-9) {
					t.Errorf("%s row %d: value %v, want %v (total %v)", dim, i, fmtPt(pt), want, total)
				}
			}
		}
	})

	t.Run("absolute", func(t *testing.T) {
		cols := l8Query(t, "l8-post", l8Context, "jsonwrap|absolute", nil)
		for _, dim := range l8Dims {
			for _, pt := range cols[dim] {
				i := int(pt.T - fixture.T0)
				v, gap := l8Value(dim, i)
				if gap {
					if pt.Value != nil {
						t.Errorf("%s row %d: value %v, want null", dim, i, *pt.Value)
					}
					continue
				}
				want := math.Abs(v)
				if pt.Value == nil || !tierValueMatch(*pt.Value, want, 0) {
					t.Errorf("%s row %d: value %v, want abs %v", dim, i, fmtPt(pt), want)
				}
			}
		}
	})

	t.Run("nonzero", func(t *testing.T) {
		cols := l8Query(t, "l8-post", l8Context, "jsonwrap|nonzero", nil)
		if _, ok := cols["zero"]; ok {
			t.Errorf("nonzero kept the all-zero dimension (have %v)", keys2(cols))
		}
		if len(cols) != len(l8Dims)-1 {
			t.Errorf("got %d dims %v, want %d", len(cols), keys2(cols), len(l8Dims)-1)
		}
	})

	t.Run("null2zero", func(t *testing.T) {
		cols := l8Query(t, "l8-post", l8Context, "jsonwrap|null2zero", nil)
		for _, pt := range cols["gappy"] {
			i := int(pt.T - fixture.T0)
			v, gap := l8Value("gappy", i)
			want := v
			if gap {
				want = 0 // the point of the option
			}
			if pt.Value == nil || !tierValueMatch(*pt.Value, want, 0) {
				t.Errorf("gappy row %d: value %v, want %v", i, pt.Value, want)
			}
		}
	})
}

// TestLayer8NonzeroAllZero pins the self-neutralizing branch: when every
// dimension is zero, options=nonzero is dropped and all dimensions return.
func TestLayer8NonzeroAllZero(t *testing.T) {
	const context = "fixture.l8zero"
	ch := fixture.Chart{
		ID: context, Title: "all zero", Units: "units", Family: "fixture",
		Context: context, UpdateEvery: 1,
	}
	for _, dim := range []string{"za", "zb"} {
		d := fixture.Dimension{ID: dim}
		for i := 1; i <= 10; i++ {
			d.Points = append(d.Points, fixture.Point{T: fixture.T0 + int64(i), Collected: "0", Flags: stream.FlagNotAnomalous})
		}
		ch.Dimensions = append(ch.Dimensions, d)
	}
	pushLiveBurst(t, "l8-zero", guid(93), ch)
	if _, err := td.WaitRetention("l8-zero", ch.Context, fixture.T0+1, fixture.T0+10, 15*time.Second); err != nil {
		t.Fatal(err)
	}

	params := daemon.DataParams(context, fixture.T0, fixture.T0+10, 10)
	params.Set("options", "jsonwrap|nonzero")
	doc, err := td.DataV3("l8-zero", params)
	if err != nil {
		t.Fatal(err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(cols) != 2 {
		t.Errorf("all-zero chart with nonzero: got %d dims %v, want both (option self-neutralizes)", len(cols), keys2(cols))
	}
}

// TestLayer8CardinalityLimit pins the fold: top limit-1 dimensions by
// |view sum| survive, the rest fold into "remaining X dimensions" whose
// per-row value is the SUM of the folded values.
func TestLayer8CardinalityLimit(t *testing.T) {
	const context = "fixture.l8card"
	const dims = 6
	ch := fixture.Chart{
		ID: context, Title: "cardinality", Units: "units", Family: "fixture",
		Context: context, UpdateEvery: 1,
	}
	for k := 1; k <= dims; k++ {
		d := fixture.Dimension{ID: fmt.Sprintf("d%d", k)}
		for i := 1; i <= 20; i++ {
			d.Points = append(d.Points, fixture.Point{T: fixture.T0 + int64(i), Collected: strconv.Itoa(k), Flags: stream.FlagNotAnomalous})
		}
		ch.Dimensions = append(ch.Dimensions, d)
	}
	pushLiveBurst(t, "l8-card", guid(94), ch)
	if _, err := td.WaitRetention("l8-card", ch.Context, fixture.T0+1, fixture.T0+20, 15*time.Second); err != nil {
		t.Fatal(err)
	}

	params := daemon.DataParams(context, fixture.T0, fixture.T0+20, 20)
	params.Set("cardinality_limit", "4")
	doc, err := td.DataV3("l8-card", params)
	if err != nil {
		t.Fatal(err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}

	// contributions: d6 > d5 > … > d1 — top 3 kept, d1+d2+d3 folded
	want := map[string]float64{
		"d6": 6, "d5": 5, "d4": 4,
		"remaining 3 dimensions": 1 + 2 + 3,
	}
	if len(cols) != len(want) {
		t.Fatalf("got %d dims %v, want %v", len(cols), keys2(cols), keys2(want))
	}
	for name, wantV := range want {
		col, ok := cols[name]
		if !ok {
			t.Errorf("column %q missing (have %v)", name, keys2(cols))
			continue
		}
		for _, pt := range col {
			if pt.Value == nil || !tierValueMatch(*pt.Value, wantV, 0) {
				t.Errorf("%q row t0%+d: value %v, want %v", name, pt.T-fixture.T0, pt.Value, wantV)
			}
		}
	}
}

// fmtPt renders a point value for error messages.
func fmtPt(pt canon.Pt) string {
	if pt.Value == nil {
		return "null"
	}
	return strconv.FormatFloat(*pt.Value, 'g', -1, 64)
}
