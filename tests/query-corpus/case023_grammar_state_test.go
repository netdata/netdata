// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

// TestCase023ExpressionGrammarAndState keeps parser validity separate from
// the grouping arithmetic. It also puts predecessor and flap transitions
// exactly on bucket boundaries, where resetting state during flush would
// erase them.
func TestCase023ExpressionGrammarAndState(t *testing.T) {
	trackContract(t, "CASE-023/expression-grammar-and-state")

	const (
		host  = "c023-grammar-state"
		chart = "fixture.c023grammarstate"
	)

	// state, for condition >0:
	//   F,F | T,T | F,F | T,T
	// The two false->true transitions cross bucket boundaries.
	state := []int{-1, 0, 1, 2, -1, 0, 1, 2}

	// counter has one real drop, 20 -> 5, separated by two gaps. The last
	// two gaps also make gap transitions independently observable:
	//   collected, collected | gap, gap | collected, collected | gap, gap
	counter := []int{10, 20, 0, 0, 5, 6, 0, 0}
	counterGap := []bool{false, false, true, true, false, false, true, true}

	ch := fixture.Chart{
		ID: chart, Title: "expression grammar and state", Units: "units", Family: "fixture",
		Context: chart, UpdateEvery: 1,
		Dimensions: []fixture.Dimension{{ID: "state"}, {ID: "counter"}},
	}
	for i := range state {
		ts := fixture.T0 + int64(i+1)
		ch.Dimensions[0].Points = append(ch.Dimensions[0].Points, fixture.Point{
			T: ts, Collected: strconv.Itoa(state[i]), Flags: stream.FlagNotAnomalous,
		})

		p := fixture.Point{T: ts, Flags: stream.FlagEmpty}
		if !counterGap[i] {
			p.Collected = strconv.Itoa(counter[i])
			p.Flags = stream.FlagNotAnomalous
		}
		ch.Dimensions[1].Points = append(ch.Dimensions[1].Points, p)
	}

	pushLiveBurst(t, host, guid(320), ch)
	if _, err := td.WaitRetention(host, ch.Context, ch.FirstT(), ch.LastT(), 15*time.Second); err != nil {
		t.Fatal(err)
	}

	query := func(t *testing.T, group, expression, dimension string, after, before, points int64, sendExpression bool) []canon.Pt {
		t.Helper()

		params := daemon.DataParams(ch.Context, after, before, points)
		params.Set("time_group", group)
		params.Set("dimensions", dimension)
		params.Set("options", "jsonwrap,unaligned")
		if sendExpression {
			params.Set("time_group_options", expression)
		}

		doc, err := td.DataV3(host, params)
		if err != nil {
			t.Fatalf("%s(%q) on %s: %v", group, expression, dimension, err)
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatalf("%s(%q) on %s: %v", group, expression, dimension, err)
		}
		if len(cols) != 1 {
			t.Fatalf("%s(%q) on %s: got columns %v, want exactly [%s]",
				group, expression, dimension, keys(cols), dimension)
		}
		col, ok := cols[dimension]
		if !ok {
			t.Fatalf("%s(%q): dimension %s is absent", group, expression, dimension)
		}
		return col
	}

	requireValues := func(t *testing.T, group, expression, dimension string, after, before int64, got []canon.Pt, want []float64) {
		t.Helper()

		span := before - after
		if span%int64(len(want)) != 0 {
			t.Fatalf("test bug: span %d does not divide %d rows", span, len(want))
		}
		step := span / int64(len(want))
		expected := make([]expectedColumnPoint, len(want))
		for i, value := range want {
			expected[i] = wantNumberAt(after+int64(i+1)*step, value)
		}
		if !assertExactColumn(t, map[string][]canon.Pt{dimension: got}, dimension, expected, 0) {
			t.Errorf("%s(%q) on %s did not match the exact fixture-derived rows", group, expression, dimension)
		}
	}

	whole := func(t *testing.T, group, expression, dimension string, sendExpression bool, want float64) {
		t.Helper()
		got := query(t, group, expression, dimension, fixture.T0, fixture.T0+8, 1, sendExpression)
		requireValues(t, group, expression, dimension, fixture.T0, fixture.T0+8, got, []float64{want})
	}

	// Every accepted operator spelling is independently discriminated on
	// the literal state series. No expected value comes from the engine.
	for _, tc := range []struct {
		expression string
		want       float64
	}{
		{"!1", 75}, {"!=1", 75}, {"!:1", 75}, {"<>1", 75},
		{">0", 50}, {">=1", 50}, {">:1", 50},
		{"<1", 50}, {"<=1", 75}, {"<:1", 75},
		{"=1", 25}, {"==1", 25}, {":1", 25},
		{"  <>  1  ", 75},
		{"1", 25}, // a bare operand means equality; its first digit is not swallowed
	} {
		tc := tc
		t.Run("operator/"+testName(tc.expression), func(t *testing.T) {
			whole(t, "percentage-of-samples", tc.expression, "state", true, tc.want)
		})
	}

	// Missing, empty and whitespace-only options retain the documented =0
	// default at every entry point. They are different from supplying an
	// operator with no value.
	for _, group := range []struct {
		name string
		want float64
	}{
		{name: "percentage-of-samples", want: 25},
		{name: "percentage-of-time", want: 25},
		{name: "number-of-flaps", want: 2},
		{name: "number-of-times", want: 2},
	} {
		group := group
		t.Run("default/"+group.name, func(t *testing.T) {
			whole(t, group.name, "", "state", false, group.want)
			whole(t, group.name, "", "state", true, group.want)
			whole(t, group.name, "   ", "state", true, group.want)
		})
	}

	for _, token := range []string{"gap", "nan", "null", "empty"} {
		token := token
		t.Run("gap-token/"+token, func(t *testing.T) {
			whole(t, "percentage-of-samples", "=="+token, "counter", true, 50)
		})
	}

	// One representative of each operand class runs through every grouping.
	// These values are derived directly from the eight fixture slots.
	for _, tc := range []struct {
		group    string
		numeric  float64
		gap      float64
		previous float64
	}{
		{"percentage-of-samples", 50, 50, 25},
		{"percentage-of-time", 50, 50, 12.5},
		{"number-of-flaps", 2, 2, 1},
		{"number-of-times", 4, 4, 1},
	} {
		tc := tc
		t.Run("groups/"+tc.group, func(t *testing.T) {
			whole(t, tc.group, ">0", "state", true, tc.numeric)
			whole(t, tc.group, "==gap", "counter", true, tc.gap)
			whole(t, tc.group, "<previous", "counter", true, tc.previous)
		})
	}
	t.Run("previous-last-alias", func(t *testing.T) {
		whole(t, "number-of-times", "<last", "counter", true, 1)
	})

	// A first sample has no predecessor and therefore cannot match.
	t.Run("previous-first-sample", func(t *testing.T) {
		got := query(t, "number-of-times", "<previous", "counter",
			fixture.T0, fixture.T0+1, 1, true)
		requireValues(t, "number-of-times", "<previous", "counter",
			fixture.T0, fixture.T0+1, got, []float64{0})
	})

	// The predecessor from t2 must survive both the t3/t4 gap and the
	// bucket flush, so the drop at t5 belongs to the second bucket.
	t.Run("previous-across-gap-and-flush", func(t *testing.T) {
		got := query(t, "number-of-times", "<previous", "counter",
			fixture.T0, fixture.T0+8, 2, true)
		requireValues(t, "number-of-times", "<previous", "counter",
			fixture.T0, fixture.T0+8, got, []float64{0, 1})
	})

	// Four 2-second buckets put both state transitions at the first sample
	// of a new bucket. Flush must reset only the per-bucket count, not the
	// boolean state or predecessor.
	t.Run("flap-state-across-flush", func(t *testing.T) {
		got := query(t, "number-of-flaps", ">0", "state",
			fixture.T0, fixture.T0+8, 4, true)
		requireValues(t, "number-of-flaps", ">0", "state",
			fixture.T0, fixture.T0+8, got, []float64{0, 1, 0, 1})
	})

	// Operators always require a complete operand. Only an absent or blank
	// whole expression defaults to =0.
	operatorOnly := []string{
		"!", "!=", "!:", ">", ">=", ">:", "<", "<=", "<:", "<>", "=", "==", ":",
		">   ",
	}
	for _, expression := range operatorOnly {
		expression := expression
		t.Run("reject/operator-only/"+testName(expression), func(t *testing.T) {
			requireExpressionRejected(t, host, ch.Context, "percentage-of-samples", expression)
		})
	}

	// Every grouping must honor parser failure; otherwise one caller can
	// silently turn the same malformed condition into ==0.
	for _, group := range []string{
		"percentage-of-samples", "percentage-of-time", "number-of-flaps", "number-of-times",
	} {
		group := group
		for _, expression := range []string{">", "abc"} {
			expression := expression
			t.Run("reject/"+group+"/"+testName(expression), func(t *testing.T) {
				requireExpressionRejected(t, host, ch.Context, group, expression)
			})
		}
	}

	for _, expression := range []string{">.", ">==5", ">0junk", "==previous-junk", "==gap and x"} {
		expression := expression
		t.Run("reject/malformed/"+testName(expression), func(t *testing.T) {
			requireExpressionRejected(t, host, ch.Context, "percentage-of-samples", expression)
		})
	}
}

func requireExpressionRejected(t *testing.T, host, context, group, expression string) {
	t.Helper()

	params := daemon.DataParams(context, fixture.T0, fixture.T0+8, 1)
	params.Set("time_group", group)
	params.Set("time_group_options", expression)
	params.Set("options", "jsonwrap,unaligned")

	if _, err := td.DataV3(host, params); err == nil {
		t.Errorf("%s(%q): malformed expression was accepted", group, expression)
	} else if !strings.Contains(err.Error(), "HTTP 400") {
		t.Errorf("%s(%q): malformed expression returned the wrong failure: %v", group, expression, err)
	}
}

func testName(s string) string {
	r := strings.NewReplacer(
		" ", "_",
		">", "gt",
		"<", "lt",
		"=", "eq",
		"!", "not",
		":", "colon",
	)
	name := strings.Trim(r.Replace(s), "_")
	if name == "" {
		return "empty"
	}
	return name
}
