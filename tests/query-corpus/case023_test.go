// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-023 — RED: the fleet time-aggregation contract, authored from the
// decided semantics BEFORE any engine code exists (the CASE-022 pattern).
//
// Four groupings answer the fleet questions the engine cannot express:
//
//	percentage-of-samples  share of samples matching  (canonical; `countif` alias)
//	percentage-of-time     share of TIME matching     (units "%")
//	number-of-flaps        false->true transitions    (units "flaps")
//	number-of-times        matching samples counted   (units "events")
//
// One shared expression grammar: the countif operators, plus gap tokens
// (nan|null|gap|empty) and the predecessor keywords (previous|last).
// Gaps stay invisible unless the expression NAMES a gap token; the
// predecessor is the previous COLLECTED sample, so a drop across a gap is
// still a drop, and the first sample of a query never matches.
//
// On an agent without the feature every new name is unknown and
// time_grouping_parse falls back to AVERAGE, so the echo reads "average"
// and the values are means — every assertion below fails loudly.
//
// Expectations are hand-derived from the fixture definition (Class A), not
// read back from the engine. Each block shows its derivation.
package corpus

import (
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

func c023FleetFixture(chart string) fixture.Chart {
	// bool is a 0/1 availability signal; counter resets at t4 and t9;
	// sparse has explicit gaps at t5..t8.
	boolV := []int{1, 1, 0, 0, 1, 0, 0, 0, 1, 1, 1, 0}
	counterV := []int{10, 20, 30, 5, 15, 25, 35, 45, 5, 15, 25, 35}
	sparseV := []int{1, 2, 3, 4, -1, -1, -1, -1, 9, 10, 11, 12}

	ch := fixture.Chart{
		ID: chart, Title: "fleet groupings", Units: "units", Family: "fixture",
		Context: chart, UpdateEvery: 1,
		Dimensions: []fixture.Dimension{{ID: "bool"}, {ID: "counter"}, {ID: "sparse"}},
	}
	for i := 1; i <= 12; i++ {
		ts := fixture.T0 + int64(i)
		ch.Dimensions[0].Points = append(ch.Dimensions[0].Points, fixture.Point{
			T: ts, Collected: strconv.Itoa(boolV[i-1]), Flags: stream.FlagNotAnomalous,
		})
		ch.Dimensions[1].Points = append(ch.Dimensions[1].Points, fixture.Point{
			T: ts, Collected: strconv.Itoa(counterV[i-1]), Flags: stream.FlagNotAnomalous,
		})
		if v := sparseV[i-1]; v < 0 {
			ch.Dimensions[2].Points = append(ch.Dimensions[2].Points, fixture.Point{
				T: ts, Flags: stream.FlagEmpty,
			})
		} else {
			ch.Dimensions[2].Points = append(ch.Dimensions[2].Points, fixture.Point{
				T: ts, Collected: strconv.Itoa(v), Flags: stream.FlagNotAnomalous,
			})
		}
	}
	return ch
}

func TestCase023FleetTimeGroupings(t *testing.T) {
	for _, contract := range []string{
		"CASE-023/fleet-grouping-echo",
		"CASE-023/percentage-of-samples",
		"CASE-023/percentage-of-time",
		"CASE-023/number-of-times",
		"CASE-023/number-of-flaps",
		"CASE-023/gap-slot-width",
		"CASE-023/fleet-grouping-units",
	} {
		registerContract(t, contract)
	}

	const chart = "fixture.c023"

	// the three fixture series, 12 samples at T0+1 .. T0+12
	//
	// bool     a 0/1 availability signal
	// counter  a monotone counter that RESETS twice (t4, t9) — reboots
	// sparse   collected only outside t5..t8, which are gaps
	ch := c023FleetFixture(chart)
	pushLiveBurst(t, "c023", guid(210), ch)
	if _, err := td.WaitRetention("c023", ch.Context, fixture.T0+1, fixture.T0+12, 15*time.Second); err != nil {
		t.Fatal(err)
	}

	type wholeResult struct {
		doc  map[string]any
		cols map[string][]canon.Pt
	}

	// one whole-window bucket: after..before covers every sample, points=1,
	// so each assertion is a single number derived by hand below.
	//
	// `unaligned` is MANDATORY here: with points=1 the group is the whole
	// 12s window, and the aligned path rounds `before` to a multiple of the
	// group, which slides the window off the fixture (measured: it served
	// t4..t12, nine samples, so every hand-derived count would be wrong).
	whole := func(t *testing.T, group, options string, extra map[string]string) wholeResult {
		t.Helper()
		opts := "unaligned"
		if extraOpts, has := extra["options"]; has {
			opts += "," + extraOpts
		}
		params := map[string][]string{
			"scope_contexts": {chart},
			"time_group":     {group},
			"format":         {"json2"},
			"group_by":       {"dimension"},
			"after":          {strconv.FormatInt(fixture.T0, 10)},
			"before":         {strconv.FormatInt(fixture.T0+12, 10)},
			"points":         {"1"},
			"options":        {opts},
		}
		if options != "" {
			params["time_group_options"] = []string{options}
		}
		for k, v := range extra {
			if k == "options" {
				continue
			}
			params[k] = []string{v}
		}
		resp, err := td.HostJSON("c023", "api/v3/data", params)
		if err != nil {
			t.Fatal(err)
		}
		cols, err := canon.Columns(resp)
		if err != nil {
			t.Fatal(err)
		}

		dimensions := []string{"bool", "counter", "sparse"}
		if scoped := extra["scope_dimensions"]; scoped != "" {
			dimensions = []string{scoped}
		}
		if !assertExactColumnSet(t, cols, dimensions) {
			t.Fail()
		}
		points := 1
		if raw := extra["points"]; raw != "" {
			points, err = strconv.Atoi(raw)
			if err != nil || points <= 0 || 12%points != 0 {
				t.Fatalf("invalid whole-window point count %q", raw)
			}
		}
		step := int64(12 / points)
		viewOK := false
		if points == 1 {
			// The one-bucket unaligned path reports its inclusive view
			// envelope, including the empty lower-bound second.
			viewOK = assertViewFields(t, resp, fixture.T0, fixture.T0+12, 13)
		} else {
			viewOK = assertExactView(t, resp, fixture.T0, fixture.T0+12, step)
		}
		if !viewOK {
			t.Fail()
		}
		for _, dimension := range dimensions {
			if !assertColumnExactGrid(t, cols, dimension, fixture.T0, fixture.T0+12, step) {
				t.Fail()
			}
		}
		return wholeResult{doc: resp, cols: cols}
	}
	// value of one dimension in the single whole-window bucket
	val := func(result wholeResult, dim string) (float64, bool) {
		column, has := result.cols[dim]
		if !has || len(column) != 1 || column[0].Value == nil {
			return 0, false
		}
		return *column[0].Value, true
	}
	// json2 rounds values to seven decimals, so an exact ratio like 2/12
	// arrives as 16.6666667
	near := func(got, want float64) bool { return math.Abs(got-want) < 1e-6 }
	expect := func(t *testing.T, result wholeResult, dimension string, want float64, what string) {
		t.Helper()
		got, numeric := val(result, dimension)
		if !numeric || !near(got, want) {
			t.Errorf("%s = %v (numeric=%v), want %v", what, got, numeric, want)
		}
	}

	t.Run("grouping-echo", func(t *testing.T) {
		trackContract(t, "CASE-023/fleet-grouping-echo")
		for _, name := range []string{
			"percentage-of-samples", "percentage-of-time", "number-of-flaps", "number-of-times",
		} {
			resp := whole(t, name, ">0", map[string]string{"options": "debug"})
			request, requestOK := resp.doc["request"].(map[string]any)
			aggregations, aggregationsOK := request["aggregations"].(map[string]any)
			timeAggregation, timeOK := aggregations["time"].(map[string]any)
			echo, echoOK := timeAggregation["time_group"].(string)
			if !requestOK || !aggregationsOK || !timeOK || !echoOK || echo != name {
				t.Errorf("echo for %s is %v, want %s", name, timeAggregation["time_group"], name)
			}
		}
		resp := whole(t, "countif", ">0", map[string]string{"options": "debug"})
		request, requestOK := resp.doc["request"].(map[string]any)
		aggregations, aggregationsOK := request["aggregations"].(map[string]any)
		timeAggregation, timeOK := aggregations["time"].(map[string]any)
		echo, echoOK := timeAggregation["time_group"].(string)
		if !requestOK || !aggregationsOK || !timeOK || !echoOK || echo != "percentage-of-samples" {
			t.Errorf("countif echo is %v, want percentage-of-samples", timeAggregation["time_group"])
		}
	})

	t.Run("percentage-of-samples", func(t *testing.T) {
		trackContract(t, "CASE-023/percentage-of-samples")
		expect(t, whole(t, "percentage-of-samples", "==0", nil), "bool", 50, "percentage-of-samples ==0 on bool")
		expect(t, whole(t, "percentage-of-samples", "<previous", nil), "counter", 200.0/12.0,
			"percentage-of-samples <previous on counter")
		for _, token := range []string{"gap", "nan", "null", "empty"} {
			expect(t, whole(t, "percentage-of-samples", "=="+token, nil), "sparse", 400.0/12.0,
				"percentage-of-samples =="+token+" on sparse")
		}
		expect(t, whole(t, "percentage-of-samples", "!=gap", nil), "sparse", 800.0/12.0,
			"percentage-of-samples !=gap on sparse")
		expect(t, whole(t, "percentage-of-samples", ">2", nil), "sparse", 75,
			"percentage-of-samples >2 on sparse")
		for _, points := range []string{"1", "2", "3", "6", "12"} {
			n, _ := strconv.Atoi(points)
			want := make([]expectedColumnPoint, n)
			step := int64(12 / n)
			for i := range want {
				want[i] = wantNumberAt(fixture.T0+int64(i+1)*step, 0)
			}
			result := whole(t, "percentage-of-samples", "==gap", map[string]string{
				"points": points, "options": "flip", "scope_dimensions": "bool",
			})
			if !assertExactColumn(t, result.cols, "bool", want, 0) {
				t.Errorf("points=%s: fully collected dimension did not return %d numeric zero rows", points, n)
			}
		}
	})

	t.Run("percentage-of-time", func(t *testing.T) {
		trackContract(t, "CASE-023/percentage-of-time")
		expect(t, whole(t, "percentage-of-time", "==0", nil), "bool", 50, "percentage-of-time ==0 on bool")
		expect(t, whole(t, "percentage-of-time", "==gap", nil), "sparse", 400.0/12.0,
			"percentage-of-time ==gap on sparse")
	})

	t.Run("number-of-times", func(t *testing.T) {
		trackContract(t, "CASE-023/number-of-times")
		expect(t, whole(t, "number-of-times", "==0", nil), "bool", 6, "number-of-times ==0 on bool")
		expect(t, whole(t, "number-of-times", "<previous", nil), "counter", 2,
			"number-of-times <previous on counter")
		expect(t, whole(t, "number-of-times", "<last", nil), "counter", 2,
			"number-of-times <last on counter")
		expect(t, whole(t, "number-of-times", "==previous", nil), "counter", 0,
			"number-of-times ==previous on counter")
		expect(t, whole(t, "number-of-times", "==gap", nil), "sparse", 4,
			"number-of-times ==gap on sparse")
		for _, points := range []string{"1", "2", "3", "6", "12"} {
			n, _ := strconv.Atoi(points)
			want := make([]expectedColumnPoint, n)
			step := int64(12 / n)
			for i := range want {
				want[i] = wantNumberAt(fixture.T0+int64(i+1)*step, 0)
			}
			result := whole(t, "number-of-times", "==gap", map[string]string{
				"points": points, "options": "flip", "scope_dimensions": "bool",
			})
			if !assertExactColumn(t, result.cols, "bool", want, 0) {
				t.Errorf("points=%s: fully collected dimension did not return %d numeric zero rows", points, n)
			}
		}
	})

	t.Run("number-of-flaps", func(t *testing.T) {
		trackContract(t, "CASE-023/number-of-flaps")
		expect(t, whole(t, "number-of-flaps", "==0", nil), "bool", 3, "number-of-flaps ==0 on bool")
		expect(t, whole(t, "number-of-flaps", "==1", nil), "bool", 2, "number-of-flaps ==1 on bool")
		expect(t, whole(t, "number-of-flaps", "<previous", nil), "counter", 2,
			"number-of-flaps <previous on counter")
		expect(t, whole(t, "number-of-flaps", "==gap", nil), "sparse", 1,
			"number-of-flaps ==gap on sparse")
	})

	t.Run("gap-slot-width", func(t *testing.T) {
		trackContract(t, "CASE-023/gap-slot-width")
		result := whole(t, "percentage-of-time", "==gap", map[string]string{"points": "6", "options": "flip"})
		want := make([]expectedColumnPoint, 0, 6)
		for i, value := range []float64{0, 0, 100, 100, 0, 0} {
			want = append(want, wantNumberAt(fixture.T0+int64((i+1)*2), value))
		}
		if !assertExactColumn(t, result.cols, "sparse", want, 1e-6) {
			t.Error("gap duration did not follow the two-second source slots")
		}
	})

	t.Run("units", func(t *testing.T) {
		trackContract(t, "CASE-023/fleet-grouping-units")
		for _, tc := range []struct{ group, units string }{
			{"percentage-of-samples", "%"},
			{"percentage-of-time", "%"},
			{"number-of-flaps", "flaps"},
			{"number-of-times", "events"},
			{"countif", "%"},
		} {
			got := viewUnits(whole(t, tc.group, ">0", nil).doc)
			if got != tc.units {
				t.Errorf("units for %s = %q, want %q", tc.group, got, tc.units)
			}
		}
	})
}
