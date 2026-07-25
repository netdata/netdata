// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-023 — RED: the fleet time-aggregation contract, authored from the
// decided semantics BEFORE any engine code exists (the CASE-022 pattern).
//
// Four groupings answer the fleet questions the engine cannot express:
//   percentage-of-samples  share of samples matching  (canonical; `countif` alias)
//   percentage-of-time     share of TIME matching     (units "%")
//   number-of-flaps        false->true transitions    (units "flaps")
//   number-of-times        matching samples counted   (units "events")
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

	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

func TestCase023FleetTimeGroupings(t *testing.T) {
	const chart = "fixture.c023"

	// the three fixture series, 12 samples at T0+1 .. T0+12
	//
	// bool     a 0/1 availability signal
	// counter  a monotone counter that RESETS twice (t4, t9) — reboots
	// sparse   collected only outside t5..t8, which are gaps
	boolV := []int{1, 1, 0, 0, 1, 0, 0, 0, 1, 1, 1, 0}
	counterV := []int{10, 20, 30, 5, 15, 25, 35, 45, 5, 15, 25, 35}
	sparseV := []int{1, 2, 3, 4, -1, -1, -1, -1, 9, 10, 11, 12} // -1 marks a gap

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
	pushLiveBurst(t, "c023", guid(210), ch)
	if _, err := td.WaitRetention("c023", ch.Context, fixture.T0+1, fixture.T0+12, 15*time.Second); err != nil {
		t.Fatal(err)
	}

	ok := true
	check := func(cond bool, what string, args ...any) {
		t.Helper()
		if !cond {
			t.Logf("fleet grouping contract not met: "+what, args...)
			ok = false
		}
	}

	dig := func(m map[string]any, path ...string) any {
		var cur any = m
		for _, k := range path {
			mm, is := cur.(map[string]any)
			if !is {
				return nil
			}
			cur = mm[k]
		}
		return cur
	}
	dimIndex := func(resp map[string]any, name string) int {
		labels, _ := dig(resp, "result", "labels").([]any)
		for i, l := range labels {
			if l == name {
				return i - 1 // labels[0] is "time"
			}
		}
		return -1
	}
	rowVal := func(resp map[string]any, row, dim int) (float64, bool) {
		data, _ := dig(resp, "result", "data").([]any)
		if row >= len(data) {
			return 0, false
		}
		r, _ := data[row].([]any)
		if dim+1 >= len(r) {
			return 0, false
		}
		point, _ := r[dim+1].([]any)
		if len(point) < 1 {
			return 0, false
		}
		v, is := point[0].(float64)
		return v, is
	}

	// one whole-window bucket: after..before covers every sample, points=1,
	// so each assertion is a single number derived by hand below.
	//
	// `unaligned` is MANDATORY here: with points=1 the group is the whole
	// 12s window, and the aligned path rounds `before` to a multiple of the
	// group, which slides the window off the fixture (measured: it served
	// t4..t12, nine samples, so every hand-derived count would be wrong).
	whole := func(group, options string, extra map[string]string) map[string]any {
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
		return resp
	}
	// value of one dimension in the single whole-window bucket
	val := func(resp map[string]any, dim string) (float64, bool) {
		di := dimIndex(resp, dim)
		if di < 0 {
			return 0, false
		}
		return rowVal(resp, 0, di)
	}
	// json2 rounds values to seven decimals, so an exact ratio like 2/12
	// arrives as 16.6666667
	near := func(got, want float64) bool { return math.Abs(got-want) < 1e-6 }

	// ------------------------------------------------------------------
	// 1. the canonical name is echoed, and `countif` is an alias for it
	//
	// the echo carries the PARSED grouping, so an agent without the
	// feature answers "average" for every new name.
	for _, name := range []string{
		"percentage-of-samples", "percentage-of-time", "number-of-flaps", "number-of-times",
	} {
		resp := whole(name, ">0", map[string]string{"options": "debug"})
		echo := dig(resp, "request", "aggregations", "time", "time_group")
		check(echo == name, "echo for %s is %v, want %s", name, echo, name)
	}
	respAlias := whole("countif", ">0", map[string]string{"options": "debug"})
	echoAlias := dig(respAlias, "request", "aggregations", "time", "time_group")
	check(echoAlias == "percentage-of-samples",
		"countif echo is %v, want percentage-of-samples (alias resolves to the canonical name)", echoAlias)

	// ------------------------------------------------------------------
	// 2. bool: the four groupings over `==0`
	//
	// bool     = 1,1,0,0,1,0,0,0,1,1,1,0
	// (v==0)   = F,F,T,T,F,T,T,T,F,F,F,T
	// samples matching     : t3,t4,t6,t7,t8,t12            -> 6 of 12 = 50%
	// time matching        : 6 slots of 1s out of 12s      -> 50%
	// occurrences          : 6
	// false->true flips    : t2->t3, t5->t6, t11->t12      -> 3
	{
		r := whole("percentage-of-samples", "==0", nil)
		v, is := val(r, "bool")
		check(is && near(v, 50), "percentage-of-samples ==0 on bool = %v (num=%v), want 50", v, is)

		r = whole("percentage-of-time", "==0", nil)
		v, is = val(r, "bool")
		check(is && near(v, 50), "percentage-of-time ==0 on bool = %v (num=%v), want 50", v, is)

		r = whole("number-of-times", "==0", nil)
		v, is = val(r, "bool")
		check(is && near(v, 6), "number-of-times ==0 on bool = %v (num=%v), want 6", v, is)

		r = whole("number-of-flaps", "==0", nil)
		v, is = val(r, "bool")
		check(is && near(v, 3), "number-of-flaps ==0 on bool = %v (num=%v), want 3", v, is)
	}

	// the mirrored condition: (v==1) = T,T,F,F,T,F,F,F,T,T,T,F
	// false->true flips: t4->t5, t8->t9 -> 2. A series that STARTS true
	// contributes nothing until it drops and returns (D7).
	{
		r := whole("number-of-flaps", "==1", nil)
		v, is := val(r, "bool")
		check(is && near(v, 2), "number-of-flaps ==1 on bool = %v (num=%v), want 2 (leading true is not a flip)", v, is)
	}

	// ------------------------------------------------------------------
	// 3. counter + `previous`: reboot counting
	//
	// counter = 10,20,30,5,15,25,35,45,5,15,25,35
	// drops   =           t4 (5<30)        t9 (5<45)
	// the first sample has no predecessor and never matches (D10)
	// occurrences    : 2
	// false->true    : t4 and t9 are isolated true values -> 2
	// share of samples: 2 of 12 -> 16.666...%
	{
		r := whole("number-of-times", "<previous", nil)
		v, is := val(r, "counter")
		check(is && near(v, 2), "number-of-times <previous on counter = %v (num=%v), want 2 reboots", v, is)

		// `last` is an accepted synonym of `previous`
		r = whole("number-of-times", "<last", nil)
		v, is = val(r, "counter")
		check(is && near(v, 2), "number-of-times <last on counter = %v (num=%v), want 2 (synonym of previous)", v, is)

		r = whole("number-of-flaps", "<previous", nil)
		v, is = val(r, "counter")
		check(is && near(v, 2), "number-of-flaps <previous on counter = %v (num=%v), want 2", v, is)

		r = whole("percentage-of-samples", "<previous", nil)
		v, is = val(r, "counter")
		check(is && near(v, 200.0/12.0), "percentage-of-samples <previous on counter = %v (num=%v), want 2/12", v, is)

		// a monotone series never drops: bool rises and falls, counter is
		// the reboot probe — `==previous` finds a stuck metric, and this
		// fixture has none
		r = whole("number-of-times", "==previous", nil)
		v, is = val(r, "counter")
		check(is && near(v, 0), "number-of-times ==previous on counter = %v (num=%v), want 0 (never stuck)", v, is)
	}

	// ------------------------------------------------------------------
	// 4. sparse + gap tokens
	//
	// sparse = 1,2,3,4,gap,gap,gap,gap,9,10,11,12
	//
	// Naming a gap token pulls the gap slots INTO the denominator (D3):
	//   ==gap  -> 4 of 12 = 33.333%, 4 occurrences, 1 flip (t4->t5)
	//   !=gap  -> 8 of 12 = 66.666%
	// Not naming one keeps them invisible, exactly like countif today:
	//   >2     -> collected only: 3,4,9,10,11,12 of 8 = 75%
	{
		for _, tok := range []string{"gap", "nan", "null", "empty"} {
			r := whole("percentage-of-samples", "=="+tok, nil)
			v, is := val(r, "sparse")
			check(is && near(v, 400.0/12.0),
				"percentage-of-samples ==%s on sparse = %v (num=%v), want 4/12", tok, v, is)
		}

		r := whole("percentage-of-time", "==gap", nil)
		v, is := val(r, "sparse")
		check(is && near(v, 400.0/12.0), "percentage-of-time ==gap on sparse = %v (num=%v), want 4/12", v, is)

		r = whole("number-of-times", "==gap", nil)
		v, is = val(r, "sparse")
		check(is && near(v, 4), "number-of-times ==gap on sparse = %v (num=%v), want 4", v, is)

		r = whole("number-of-flaps", "==gap", nil)
		v, is = val(r, "sparse")
		check(is && near(v, 1), "number-of-flaps ==gap on sparse = %v (num=%v), want 1", v, is)

		r = whole("percentage-of-samples", "!=gap", nil)
		v, is = val(r, "sparse")
		check(is && near(v, 800.0/12.0), "percentage-of-samples !=gap on sparse = %v (num=%v), want 8/12", v, is)

		// no gap token named -> gaps invisible, denominator is the 8
		// collected samples
		r = whole("percentage-of-samples", ">2", nil)
		v, is = val(r, "sparse")
		check(is && near(v, 75), "percentage-of-samples >2 on sparse = %v (num=%v), want 75 (gaps invisible)", v, is)
	}

	// ------------------------------------------------------------------
	// 5. gap duration is the SLOT width, not the point's own span
	//
	// Six 2-second buckets over the window. Buckets are (T0,T0+2],
	// (T0+2,T0+4], ... and the gaps sit at t5..t8, so bucket 3 (t5,t6) and
	// bucket 4 (t7,t8) are wholly gap while the rest are wholly collected.
	// A gap point carries NO timestamps (QUERY_POINT_EMPTY), so its
	// contribution has to come from the slot, or these read 0.
	{
		params := map[string]string{"points": "6", "options": "flip"}
		r := whole("percentage-of-time", "==gap", params)
		si := dimIndex(r, "sparse")
		check(si >= 0, "sparse dimension missing from the 6-bucket sweep")
		if si >= 0 {
			want := []float64{0, 0, 100, 100, 0, 0} // oldest-first with flip
			for row, w := range want {
				v, is := rowVal(r, row, si)
				check(is && near(v, w), "6-bucket percentage-of-time ==gap row %d = %v (num=%v), want %v", row, v, is, w)
			}
		}
	}

	// ------------------------------------------------------------------
	// 6. units transform (D5)
	{
		for _, tc := range []struct{ group, units string }{
			{"percentage-of-samples", "%"},
			{"percentage-of-time", "%"},
			{"number-of-flaps", "flaps"},
			{"number-of-times", "events"},
			{"countif", "%"},
		} {
			r := whole(tc.group, ">0", nil)
			u := viewUnits(r)
			check(u == tc.units, "units for %s = %q, want %q", tc.group, u, tc.units)
		}
	}

	expectAgentStatus(t, "CASE-023/fleet-time-groupings", ok)
}

// TestCase023CountifBareNumber pins the shared parser's bare-number fix
// (bug-list ruling (d), resolved 2026-07-25 — "fix it for all of them").
//
// tg_countif_create advances one character past the operator switch even
// when NO operator matched (countif.h:78), so a bare number loses its
// first digit: options "5" targets 0, not 5. Health has never had this
// bug — health_config.c only advances on a matched operator, and
// health-config-unittest.c:96 asserts countif(0.5) parses as "=0.5".
// The shared parser aligns the API to health.
//
// Fixture reuse: the `bool` series of TestCase023FleetTimeGroupings has
// six zeros and six ones, so the two readings are 50 (target 0, buggy)
// and 0 (target 5, correct).
func TestCase023CountifBareNumber(t *testing.T) {
	const chart = "fixture.c023"

	resp, err := td.HostJSON("c023", "api/v3/data", map[string][]string{
		"scope_contexts":     {chart},
		"time_group":         {"countif"},
		"time_group_options": {"5"},
		"format":             {"json2"},
		"group_by":           {"dimension"},
		"after":              {strconv.FormatInt(fixture.T0, 10)},
		"before":             {strconv.FormatInt(fixture.T0+12, 10)},
		"points":             {"1"},
		"options":            {"unaligned"}, // see the note in whole()
	})
	if err != nil {
		t.Skip("c023 fixture not available (TestCase023FleetTimeGroupings failed?)")
	}

	labels, _ := resp["result"].(map[string]any)
	if labels == nil {
		t.Skip("c023 fixture not available (TestCase023FleetTimeGroupings failed?)")
	}
	lab, _ := labels["labels"].([]any)
	di := -1
	for i, l := range lab {
		if l == "bool" {
			di = i - 1
		}
	}
	if di < 0 {
		t.Skip("c023 fixture not available (TestCase023FleetTimeGroupings failed?)")
	}
	data, _ := labels["data"].([]any)
	if len(data) == 0 {
		t.Skip("c023 fixture not available (TestCase023FleetTimeGroupings failed?)")
	}
	row, _ := data[0].([]any)
	point, _ := row[di+1].([]any)
	v, _ := point[0].(float64)

	t.Logf("countif with a bare '5' on a 0/1 series reads %v (50 = the swallowed digit, 0 = parsed as =5)", v)
	expectAgentStatus(t, "CASE-023/countif-bare-number", math.Abs(v-0) < 1e-9)
}
