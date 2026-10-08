// SPDX-License-Identifier: GPL-3.0-or-later

package fixture

import (
	"testing"
)

func TestChartFirstAndLastTScanEveryPoint(t *testing.T) {
	chart := Chart{Dimensions: []Dimension{
		{Points: []Point{{T: -5}, {T: -10}, {T: 0}}},
		{Points: []Point{{T: 8}, {T: 3}, {T: 12}}},
	}}

	if got := chart.FirstT(); got != -10 {
		t.Errorf("FirstT() = %d, want -10", got)
	}
	if got := chart.LastT(); got != 12 {
		t.Errorf("LastT() = %d, want 12", got)
	}

	negative := Chart{Dimensions: []Dimension{
		{Points: []Point{{T: -4}, {T: -9}}},
	}}
	if got := negative.LastT(); got != -4 {
		t.Errorf("all-negative LastT() = %d, want -4", got)
	}

	withZero := Chart{Dimensions: []Dimension{
		{Points: []Point{{T: 0}}},
		{Points: []Point{{T: 5}}},
	}}
	if got := withZero.FirstT(); got != 0 {
		t.Errorf("zero FirstT() = %d, want 0", got)
	}
}

func TestChartRowTimesAreTheTimestampUnion(t *testing.T) {
	chart := Chart{Dimensions: []Dimension{
		{Points: []Point{{T: 3}, {T: 1}}},
		{Points: []Point{{T: 2}, {T: 3}}},
	}}

	got := chart.rowTimes()
	want := []int64{1, 2, 3}
	if len(got) != len(want) {
		t.Fatalf("rowTimes() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rowTimes() = %v, want %v", got, want)
		}
	}
	if rows := chart.ReplayWindow(0, 3); len(rows) != len(want) {
		t.Fatalf("ReplayWindow served %d rows, want timestamp union size %d", len(rows), len(want))
	}
}

func TestDimensionOraclesFollowIngestionOrder(t *testing.T) {
	d := Dimension{ID: "load", Points: []Point{
		{T: 3, Collected: "3", Flags: "A"},
		{T: 1, Collected: "1", Flags: "A"},
		{T: 2, Collected: "2", Flags: "A"},
	}}

	expected := d.Expected()
	db := d.DBPoints(1)
	for i, wantT := range []int64{1, 2, 3} {
		if expected[i].T != wantT || db[i].End != wantT {
			t.Fatalf("oracle row %d ends at Expected/DBPoints %d/%d, want %d",
				i, expected[i].T, db[i].End, wantT)
		}
	}
}

func TestFixtureOraclesRejectInvalidCollectedValues(t *testing.T) {
	oracles := map[string]func(Dimension){
		"Expected":    func(d Dimension) { _ = d.Expected() },
		"TierWindows": func(d Dimension) { _ = d.TierWindows(60, 1) },
		"DBPoints":    func(d Dimension) { _ = d.DBPoints(1) },
	}
	values := map[string]string{
		"malformed":         "not-a-number",
		"nan":               "NaN",
		"positive-infinity": "+Inf",
		"negative-infinity": "-Inf",
	}

	for oracle, invoke := range oracles {
		for name, value := range values {
			t.Run(oracle+"/"+name, func(t *testing.T) {
				d := Dimension{ID: "load", Points: []Point{{
					T: 1, Collected: value, Flags: "A",
				}}}
				defer func() {
					if recover() == nil {
						t.Fatalf("%s accepted collected value %q", oracle, value)
					}
				}()
				invoke(d)
			})
		}
	}
}

func TestFixtureConsumersRejectDuplicateDimensionTimestamps(t *testing.T) {
	d := Dimension{ID: "load", Points: []Point{
		{T: 1, Collected: "1", Flags: "A"},
		{T: 1, Collected: "2", Flags: "A"},
	}}
	chart := Chart{Dimensions: []Dimension{d}}
	consumers := map[string]func(){
		"chart-index":  func() { _ = chart.pointsByTime() },
		"replay":       func() { _ = chart.ReplayWindow(0, 1) },
		"expected":     func() { _ = d.Expected() },
		"tier-windows": func() { _ = d.TierWindows(60, 1) },
		"db-points":    func() { _ = d.DBPoints(1) },
	}

	for name, consume := range consumers {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("fixture consumer accepted duplicate dimension timestamps")
				}
			}()
			consume()
		})
	}
}

func TestFixtureGapDoesNotParseCollectedPlaceholder(t *testing.T) {
	d := Dimension{ID: "load", Points: []Point{{
		T: 1, Collected: "not-a-number", Flags: "E",
	}}}

	expected := d.Expected()
	if len(expected) != 1 || expected[0].Value != nil {
		t.Fatalf("Expected() = %+v, want one gap", expected)
	}
	tier := d.TierWindows(60, 1)[60]
	if !tier.Empty || tier.Count != 0 || tier.GapCount != 60 {
		t.Fatalf("TierWindows() = %+v, want an empty stored 0/60 window", tier)
	}
	db := d.DBPoints(1)
	if len(db) != 1 || !db[0].Gap {
		t.Fatalf("DBPoints() = %+v, want one gap", db)
	}
}

func TestTierFetchValueRejectsEmptyWindows(t *testing.T) {
	for name, point := range map[string]TierPoint{
		"explicit-empty": {Empty: true},
		"zero-count":     {Sum: 1},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("TierFetchValue accepted an empty tier window")
				}
			}()
			TierFetchValue("average", point)
		})
	}
}

func TestNumericCollectedValueIsFinite(t *testing.T) {
	d := Dimension{ID: "load", Points: []Point{{
		T: 1, Collected: "1.25", Flags: "A",
	}}}

	expected := d.Expected()
	if expected[0].Value == nil || *expected[0].Value != SNRoundTrip(1.25) {
		t.Fatalf("Expected() = %+v", expected)
	}
	tier := d.TierWindows(60, 1)[60]
	if tier.Empty || tier.Count != 1 || tier.GapCount != 59 || tier.Sum != float64(float32(1.25)) {
		t.Fatalf("TierWindows() = %+v", tier)
	}
	db := d.DBPoints(1)
	if db[0].Gap || db[0].Value != SNRoundTrip(1.25) {
		t.Fatalf("DBPoints() = %+v", db)
	}

	for _, collected := range []string{"0x1p2", "1_000"} {
		t.Run("reject/"+collected, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("CollectedValue accepted %q", collected)
				}
			}()
			Point{T: 1, Collected: collected, Flags: "A"}.CollectedValue("load")
		})
	}
}

func TestParseCountifOptionsUsesStrictExpressionContract(t *testing.T) {
	valid := map[string]struct {
		cmp    string
		target float64
	}{
		"":         {"==", 0},
		" ":        {"==", 0},
		"\t":       {"==", 0},
		">":        {">", 0},
		">=":       {">=", 0},
		"<":        {"<", 0},
		"<=":       {"<=", 0},
		"=":        {"==", 0},
		"==":       {"==", 0},
		":":        {"==", 0},
		"!=":       {"!=", 0},
		"<>":       {"!=", 0},
		"40":       {"==", 40},
		" = 40 ":   {"==", 40},
		"==40":     {"==", 40},
		":40":      {"==", 40},
		"!=1":      {"!=", 1},
		"<>1":      {"!=", 1},
		">30":      {">", 30},
		">=30":     {">=", 30},
		">:30":     {">=", 30},
		"<20":      {"<", 20},
		"<=20":     {"<=", 20},
		"<:20":     {"<=", 20},
		"> -1.5e2": {">", -150},
	}
	for expression, want := range valid {
		t.Run("valid/"+expression, func(t *testing.T) {
			cmp, target, err := parseCountifOptions(expression)
			if err != nil {
				t.Fatal(err)
			}
			if cmp != want.cmp || target != want.target {
				t.Fatalf("parseCountifOptions(%q) = %q,%v; want %q,%v",
					expression, cmp, target, want.cmp, want.target)
			}
		})
	}

	for _, expression := range []string{
		"!1", "!:1", ">word", ">1junk", ">1e309", "NaN", "+Inf", "-Inf",
		"0x1p2", "1_000",
	} {
		t.Run("invalid/"+expression, func(t *testing.T) {
			if _, _, err := parseCountifOptions(expression); err == nil {
				t.Fatalf("parseCountifOptions(%q) accepted malformed expression", expression)
			}
		})
	}
}
