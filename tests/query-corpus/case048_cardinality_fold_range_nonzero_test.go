// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"math"
	"slices"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

// TestCase048CardinalityFoldRangeNonzero: a cardinality fold creates new
// values, the per-row sums of the folded dimensions in the "remaining N
// dimensions" column. Everything the result reports about its values must
// describe the reduced result, not the dimensions before the fold:
//
//   - the overall min/max covers every value returned, the remaining column
//     included (and hidden columns too, as for unfolded results);
//   - options=nonzero hides the remaining column when every one of its row
//     sums is zero, as it does for every other column;
//   - options=nonzero never hides every column: when no exposed column of
//     the reduced result is nonzero the option is dropped.
//
// Every fixture collects one value per dimension per second over
// T0+1..T0+20 and is read back at one point per second, so each column's
// values and its contribution (|sum|) follow from the fixture.
func TestCase048CardinalityFoldRangeNonzero(t *testing.T) {
	const (
		contractRange    = "CASE-048/fold-view-range"
		contractNonzero  = "CASE-048/fold-remaining-nonzero"
		contractNotEmpty = "CASE-048/fold-nonzero-never-empty"
	)
	rangeScopes := []string{"v1-sum", "v2-sum", "v3-sum", "v1-percentage", "v3-shrinking", "v3-hidden-zero"}
	nonzeroScopes := []string{"zero-tail", "signed-cancel", "signed-cancel-raw"}
	notEmptyScopes := []string{"limit-1-cancel", "tie-cancel"}
	for _, c := range rangeScopes {
		registerContractComponent(t, contractRange, c)
	}
	for _, c := range nonzeroScopes {
		registerContractComponent(t, contractNonzero, c)
	}
	for _, c := range notEmptyScopes {
		registerContractComponent(t, contractNotEmpty, c)
	}

	const rows = 20

	// fixtures: dimension id -> value at row i (1-based)
	type series map[string]func(i int) int
	constant := func(v int) func(int) int { return func(int) int { return v } }
	alternating := func(v int) func(int) int {
		return func(i int) int {
			if i%2 == 1 {
				return v
			}
			return -v
		}
	}

	push := func(t *testing.T, name string, guidN int, s series) (host, context string) {
		t.Helper()
		host, context = "case048-"+name, "fixture.case048_"+name
		ch := fixture.Chart{
			ID: context, Title: "cardinality fold " + name, Units: "units", Family: "fixture",
			Context: context, UpdateEvery: 1,
		}
		ids := make([]string, 0, len(s))
		for id := range s {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			d := fixture.Dimension{ID: id}
			for i := 1; i <= rows; i++ {
				d.Points = append(d.Points, fixture.Point{T: fixture.T0 + int64(i), Collected: strconv.Itoa(s[id](i)), Flags: stream.FlagNotAnomalous})
			}
			ch.Dimensions = append(ch.Dimensions, d)
		}
		pushLiveBurst(t, host, guid(guidN), ch)
		if _, err := td.WaitRetention(host, context, fixture.T0+1, fixture.T0+rows, 15*time.Second); err != nil {
			t.Fatal(err)
		}
		return host, context
	}

	type result struct {
		ids      []string
		min, max float64
		options  []string // effective options (v2/v3 view.options, only with debug)
	}

	// v2/v3: json2, group by dimension (one column per dimension)
	queryV2 := func(t *testing.T, endpoint, host, context string, limit int, options string) result {
		t.Helper()
		params := daemon.DataParams(context, fixture.T0, fixture.T0+rows, rows)
		params.Set("options", "jsonwrap,"+options)
		params.Set("cardinality_limit", strconv.Itoa(limit))
		doc, err := td.HostJSON(host, endpoint, params)
		if err != nil {
			t.Fatal(err)
		}
		view, ok := doc["view"].(map[string]any)
		if !ok {
			t.Fatalf("view: not an object")
		}
		dims, ok := view["dimensions"].(map[string]any)
		if !ok {
			t.Fatalf("view.dimensions: not an object")
		}
		var res result
		res.ids = case048Strings(t, dims["ids"], "view.dimensions.ids")
		if opts, ok := view["options"]; ok {
			res.options = case048Strings(t, opts, "view.options")
		}
		res.min, res.max = case048Number(t, view["min"], "view.min"), case048Number(t, view["max"], "view.max")
		return res
	}

	// v1: classic json with jsonwrap
	queryV1 := func(t *testing.T, host, context string, limit int, options string) result {
		t.Helper()
		params := daemon.DataParams(context, fixture.T0, fixture.T0+rows, rows)
		for _, key := range []string{"scope_contexts", "time_group", "time_group_options", "group_by", "aggregation"} {
			params.Del(key)
		}
		params.Set("context", context)
		params.Set("group", "average")
		params.Set("format", "json")
		params.Set("options", "jsonwrap|seconds|unaligned|"+options)
		params.Set("cardinality_limit", strconv.Itoa(limit))
		doc, err := td.HostJSON(host, "api/v1/data", params)
		if err != nil {
			t.Fatal(err)
		}
		var res result
		res.ids = case048Strings(t, doc["dimension_ids"], "dimension_ids")
		res.min, res.max = case048Number(t, doc["min"], "min"), case048Number(t, doc["max"], "max")
		return res
	}

	assertRange := func(t *testing.T, got result, wantMin, wantMax float64) {
		t.Helper()
		const eps = 1e-9
		if math.Abs(got.min-wantMin) > eps*math.Max(1, math.Abs(wantMin)) ||
			math.Abs(got.max-wantMax) > eps*math.Max(1, math.Abs(wantMax)) {
			t.Errorf("range [%v, %v], want [%v, %v] (columns %q)", got.min, got.max, wantMin, wantMax, got.ids)
		}
	}
	assertIDs := func(t *testing.T, got result, want ...string) {
		t.Helper()
		g, w := slices.Clone(got.ids), slices.Clone(want)
		sort.Strings(g)
		sort.Strings(w)
		if !slices.Equal(g, w) {
			t.Errorf("columns %q, want %q", got.ids, want)
		}
	}
	// the effective options are reported (view.options) only with debug
	assertNonzeroDropped := func(t *testing.T, got result) {
		t.Helper()
		if got.options == nil {
			t.Fatalf("view.options missing: the query must ask for debug")
		}
		if slices.Contains(got.options, "nonzero") {
			t.Errorf("effective options %q still carry nonzero, but no exposed column is nonzero", got.options)
		}
	}

	// d1..d6 collect 1..6: at limit 2, d6 (6) is kept and d1..d5 fold into
	// a remaining column of 1+2+3+4+5 = 15 per row
	sumHost, sumContext := push(t, "sum", 13400, series{
		"d1": constant(1), "d2": constant(2), "d3": constant(3),
		"d4": constant(4), "d5": constant(5), "d6": constant(6),
	})
	for _, v := range []struct{ scope, endpoint string }{
		{"v2-sum", "api/v2/data"},
		{"v3-sum", "api/v3/data"},
	} {
		t.Run(v.scope, func(t *testing.T) {
			trackContractComponent(t, contractRange, v.scope)
			got := queryV2(t, v.endpoint, sumHost, sumContext, 2, "")
			assertIDs(t, got, "d6", "remaining 5 dimensions")
			assertRange(t, got, 6, 15)
		})
	}
	t.Run("v1-sum", func(t *testing.T) {
		trackContractComponent(t, contractRange, "v1-sum")
		got := queryV1(t, sumHost, sumContext, 2, "")
		assertIDs(t, got, "d6", "remaining 5 dimensions")
		assertRange(t, got, 6, 15)
	})
	t.Run("v1-percentage", func(t *testing.T) {
		trackContractComponent(t, contractRange, "v1-percentage")
		// each row totals 21: d6 is 600/21 %, the folded 1..5 are 1500/21 %
		got := queryV1(t, sumHost, sumContext, 2, "percentage")
		assertIDs(t, got, "d6", "remaining 5 dimensions")
		assertRange(t, got, 600.0/21, 1500.0/21)
	})

	// a=10, b=-9, c=8: contributions 10 > 9 > 8, a kept, b+c = -1 per row.
	// the range shrinks: -9 and 8 are no longer returned
	shrinkHost, shrinkContext := push(t, "shrink", 13401, series{
		"a": constant(10), "b": constant(-9), "c": constant(8),
	})
	t.Run("v3-shrinking", func(t *testing.T) {
		trackContractComponent(t, contractRange, "v3-shrinking")
		got := queryV2(t, "api/v3/data", shrinkHost, shrinkContext, 2, "")
		assertIDs(t, got, "a", "remaining 2 dimensions")
		assertRange(t, got, -1, 10)
	})

	// a=10, b=0, c=0: a kept, the remaining column is 0 at every row
	zeroHost, zeroContext := push(t, "zerotail", 13402, series{
		"a": constant(10), "b": constant(0), "c": constant(0),
	})
	t.Run("v3-hidden-zero", func(t *testing.T) {
		trackContractComponent(t, contractRange, "v3-hidden-zero")
		// nonzero hides the remaining column; its zeros still count, as
		// hidden columns do in unfolded results
		got := queryV2(t, "api/v3/data", zeroHost, zeroContext, 2, "nonzero")
		assertRange(t, got, 0, 10)
	})
	t.Run("zero-tail", func(t *testing.T) {
		trackContractComponent(t, contractNonzero, "zero-tail")
		assertIDs(t, queryV2(t, "api/v3/data", zeroHost, zeroContext, 2, "nonzero"), "a")
	})

	// a=10, b=3, c=-3: a kept, b+c = 0 at every row although neither is zero
	cancelHost, cancelContext := push(t, "cancel", 13403, series{
		"a": constant(10), "b": constant(3), "c": constant(-3),
	})
	t.Run("signed-cancel", func(t *testing.T) {
		trackContractComponent(t, contractNonzero, "signed-cancel")
		assertIDs(t, queryV2(t, "api/v3/data", cancelHost, cancelContext, 2, "nonzero"), "a")
	})
	t.Run("signed-cancel-raw", func(t *testing.T) {
		trackContractComponent(t, contractNonzero, "signed-cancel-raw")
		// raw returns every queried column, nonzero or not
		assertIDs(t, queryV2(t, "api/v3/data", cancelHost, cancelContext, 2, "raw,nonzero"), "a", "remaining 2 dimensions")
	})

	// a=3, b=-3 at limit 1: both fold into one column that is 0 at every row
	oneHost, oneContext := push(t, "limit1", 13404, series{
		"a": constant(3), "b": constant(-3),
	})
	t.Run("limit-1-cancel", func(t *testing.T) {
		trackContractComponent(t, contractNotEmpty, "limit-1-cancel")
		got := queryV2(t, "api/v3/data", oneHost, oneContext, 1, "nonzero,debug")
		assertIDs(t, got, "remaining 2 dimensions")
		assertNonzeroDropped(t, got)
	})

	// a is 0, b alternates +3/-3 and c -3/+3: every contribution is 0, the
	// id tie-break keeps a, and b+c = 0 at every row; before the fold b and c
	// are nonzero, so only the fold can tell that nothing nonzero is left
	tieHost, tieContext := push(t, "tie", 13405, series{
		"a": constant(0), "b": alternating(3), "c": alternating(-3),
	})
	t.Run("tie-cancel", func(t *testing.T) {
		trackContractComponent(t, contractNotEmpty, "tie-cancel")
		got := queryV2(t, "api/v3/data", tieHost, tieContext, 2, "nonzero,debug")
		assertIDs(t, got, "a", "remaining 2 dimensions")
		assertNonzeroDropped(t, got)
	})
}

func case048Strings(t *testing.T, v any, what string) []string {
	t.Helper()
	a, ok := v.([]any)
	if !ok {
		t.Fatalf("%s: not an array: %v", what, v)
	}
	out := make([]string, 0, len(a))
	for i, e := range a {
		s, ok := e.(string)
		if !ok {
			t.Fatalf("%s[%d] = %v, not a string", what, i, e)
		}
		out = append(out, s)
	}
	return out
}

func case048Number(t *testing.T, v any, what string) float64 {
	t.Helper()
	n, ok := v.(float64)
	if !ok {
		t.Fatalf("%s: not a number: %v", what, v)
	}
	return n
}
