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
	for _, contract := range []string{
		"L8/percentage-post-processing",
		"L8/absolute-post-processing",
		"L8/nonzero-post-processing",
		"L8/null2zero-post-processing",
	} {
		registerContract(t, contract)
	}

	ch := l8Fixture()
	pushLiveBurst(t, "l8-post", guid(92), ch)
	if _, err := td.WaitRetention("l8-post", ch.Context, fixture.T0+1, fixture.T0+l8Rows, 15*time.Second); err != nil {
		t.Fatal(err)
	}

	t.Run("percentage", func(t *testing.T) {
		trackContract(t, "L8/percentage-post-processing")

		// v2/v3 FORCE options=absolute together with percentage
		// (api_v2_data.c) — the share is computed over |values|, signs
		// are erased at fetch time
		cols := l8Query(t, "l8-post", l8Context, "jsonwrap|percentage", nil)
		if !assertExactColumnSet(t, cols, l8Dims) {
			t.Fail()
		}
		for _, dim := range l8Dims {
			want := make([]expectedColumnPoint, 0, l8Rows)
			for i := 1; i <= l8Rows; i++ {
				total := 0.0
				for _, member := range l8Dims {
					if v, gap := l8Value(member, i); !gap {
						total += math.Abs(v)
					}
				}
				v, gap := l8Value(dim, i)
				if gap {
					want = append(want,
						wantEmptyWithMetadataAt(fixture.T0+int64(i), 0, canon.AnnotationEmpty))
					continue
				}
				divisor := total
				if divisor == 0 {
					divisor = 1
				}
				want = append(want,
					wantNumberWithMetadataAt(fixture.T0+int64(i), math.Abs(v)*100/divisor, 0, 0))
			}
			if !assertExactColumn(t, cols, dim, want, printTol) {
				t.Fail()
			}
		}
	})

	t.Run("absolute", func(t *testing.T) {
		trackContract(t, "L8/absolute-post-processing")

		cols := l8Query(t, "l8-post", l8Context, "jsonwrap|absolute", nil)
		if !assertExactColumnSet(t, cols, l8Dims) {
			t.Fail()
		}
		for _, dim := range l8Dims {
			want := make([]expectedColumnPoint, 0, l8Rows)
			for i := 1; i <= l8Rows; i++ {
				v, gap := l8Value(dim, i)
				if gap {
					want = append(want,
						wantEmptyWithMetadataAt(fixture.T0+int64(i), 0, canon.AnnotationEmpty))
					continue
				}
				want = append(want,
					wantNumberWithMetadataAt(fixture.T0+int64(i), math.Abs(v), 0, 0))
			}
			if !assertExactColumn(t, cols, dim, want, 0) {
				t.Fail()
			}
		}
	})

	t.Run("nonzero", func(t *testing.T) {
		trackContract(t, "L8/nonzero-post-processing")

		cols := l8Query(t, "l8-post", l8Context, "jsonwrap|nonzero", nil)
		dimensions := []string{"pos", "neg", "gappy"}
		if !assertExactColumnSet(t, cols, dimensions) {
			t.Fail()
		}
		for _, dim := range dimensions {
			want := make([]expectedColumnPoint, 0, l8Rows)
			for i := 1; i <= l8Rows; i++ {
				v, gap := l8Value(dim, i)
				if gap {
					want = append(want,
						wantEmptyWithMetadataAt(fixture.T0+int64(i), 0, canon.AnnotationEmpty))
				} else {
					want = append(want,
						wantNumberWithMetadataAt(fixture.T0+int64(i), v, 0, 0))
				}
			}
			if !assertExactColumn(t, cols, dim, want, 0) {
				t.Fail()
			}
		}
	})

	t.Run("null2zero", func(t *testing.T) {
		trackContract(t, "L8/null2zero-post-processing")

		cols := l8Query(t, "l8-post", l8Context, "jsonwrap|null2zero", nil)
		if !assertExactColumnSet(t, cols, l8Dims) {
			t.Fail()
		}
		for _, dim := range l8Dims {
			want := make([]expectedColumnPoint, 0, l8Rows)
			for i := 1; i <= l8Rows; i++ {
				v, gap := l8Value(dim, i)
				if gap {
					v = 0
					want = append(want,
						wantNumberWithMetadataAt(fixture.T0+int64(i), v, 0, canon.AnnotationEmpty))
					continue
				}
				want = append(want,
					wantNumberWithMetadataAt(fixture.T0+int64(i), v, 0, 0))
			}
			if !assertExactColumn(t, cols, dim, want, 0) {
				t.Fail()
			}
		}
	})
}

// TestLayer8NonzeroAllZero pins the self-neutralizing branch: when every
// dimension is zero, options=nonzero is dropped and all dimensions return.
func TestLayer8NonzeroAllZero(t *testing.T) {
	trackContract(t, "L8/nonzero-all-zero")

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
	dimensions := []string{"za", "zb"}
	if !assertExactColumnSet(t, cols, dimensions) {
		t.Fail()
	}
	for _, dimension := range dimensions {
		want := make([]expectedColumnPoint, 0, 10)
		for i := 1; i <= 10; i++ {
			want = append(want,
				wantNumberWithMetadataAt(fixture.T0+int64(i), 0, 0, 0))
		}
		if !assertExactColumn(t, cols, dimension, want, 0) {
			t.Fail()
		}
	}
}

// TestLayer8CardinalityLimit pins the fold: top limit-1 dimensions by
// |view sum| survive, the rest fold into "remaining X dimensions" whose
// per-row value is the SUM of the folded values.
func TestLayer8CardinalityLimit(t *testing.T) {
	trackContract(t, "L8/cardinality-limit")

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
	if !assertExactColumnSet(t, cols, keys2(want)) {
		t.Fail()
	}
	for name, wantV := range want {
		rows := make([]expectedColumnPoint, 0, 20)
		for i := 1; i <= 20; i++ {
			rows = append(rows,
				wantNumberWithMetadataAt(fixture.T0+int64(i), wantV, 0, 0))
		}
		if !assertExactColumn(t, cols, name, rows, 0) {
			t.Fail()
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
