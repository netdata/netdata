// SPDX-License-Identifier: GPL-3.0-or-later

// Layer 6 — two-pass (hierarchical) group-by: pass 2 consumes pass 1's
// groups. The mechanics (query-group-by-finalize.c): pass-1 accumulations
// flow into pass 2 UNCONVERTED — only percentage converts per pass; the
// AVERAGE division happens once, at the END, with the FINAL pass's
// contribution counts (= number of pass-1 groups).
//
// Consequences, pinned here:
//   - sum→sum, min→min, max→max, extremes→extremes and sum→avg are
//     mechanically correct (green);
//   - any chain with AVERAGE at pass 1 is the KNOWN avg-of-sums bug
//     family (bug-list item 3; SOW-20260701-query-rollup-hierarchical-
//     correctness, in planning): pass 2 sees group SUMS where group
//     averages were meant — CASE-018 pins it red;
//   - [instance, percentage] → [selected, avg] returns the MEAN of the
//     per-instance percentages (pct converts per pass, then avg over
//     groups) — pinned as current behavior; whether the contract should
//     be the pooled percentage is the rollup SOW's ruling to make.
package corpus

import (
	"math"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
)

// l6Pass1 accumulates one pass-1 group for row i per the add_metric
// mechanics: sums for average/sum, champions for min/max/extremes.
// Returns the accumulator, the contribution count and whether any member
// contributed.
func l6Pass1(agg string, group []l5Member, i int) (acc float64, gbc int) {
	first := true
	for _, m := range group {
		if m.gap(i) {
			continue
		}
		v := m.value(i)
		switch agg {
		case "average", "sum":
			acc += v
		case "min":
			if first || v < acc {
				acc = v
			}
		case "max":
			if first || v > acc {
				acc = v
			}
		case "extremes":
			if first || math.Abs(v) > math.Abs(acc) {
				acc = v
			}
		}
		first = false
		gbc++
	}
	return acc, gbc
}

// l6Expected computes the CURRENT two-pass mechanics for a final group
// (the pass-1 groups it contains) at row i: pass-1 accumulators flow
// unconverted into the pass-2 aggregation; a final AVERAGE divides by the
// number of contributing pass-1 groups.
func l6Expected(agg1, agg2 string, pass1Groups [][]l5Member, i int) (val float64, partial, empty bool) {
	var acc float64
	groups := 0
	expectedGroups := 0
	first := true
	for _, g := range pass1Groups {
		expectedGroups++
		a1, gbc1 := l6Pass1(agg1, g, i)
		if gbc1 == 0 {
			continue
		}
		if gbc1 < len(g) {
			partial = true
		}
		switch agg2 {
		case "average", "sum":
			acc += a1
		case "min":
			if first || a1 < acc {
				acc = a1
			}
		case "max":
			if first || a1 > acc {
				acc = a1
			}
		case "extremes":
			if first || math.Abs(a1) > math.Abs(acc) {
				acc = a1
			}
		}
		first = false
		groups++
	}
	if groups == 0 {
		return 0, false, true
	}
	if groups < expectedGroups {
		partial = true
	}
	if agg2 == "average" {
		acc /= float64(groups)
	}
	return acc, partial, false
}

// l6Groups builds the two-pass structure: final groups (by key2) of
// pass-1 groups (by key1).
func l6Groups(key1, key2 string, members []l5Member) map[string][][]l5Member {
	pass1 := map[string][]l5Member{}
	for _, m := range members {
		k := l5GroupKey(key1, m)
		pass1[k] = append(pass1[k], m)
	}
	out := map[string][][]l5Member{}
	for _, g := range pass1 {
		k2 := l5GroupKey(key2, g[0])
		out[k2] = append(out[k2], g)
	}
	return out
}

// TestLayer6TwoPassMatrix pins the mechanically-correct two-pass combos
// green: chains whose pass-1 accumulator IS the group's final value (sum
// and the champions), so pass 2 consumes exactly what the semantics mean.
func TestLayer6TwoPassMatrix(t *testing.T) {
	members := l5Members()
	if _, err := td.WaitRetention("l5-a", l5Context, fixture.T0+1, fixture.T0+l5Rows, 15*time.Second); err != nil {
		t.Skip("layer-5 palette not available (TestLayer5GroupByMatrix failed?)")
	}

	keyCombos := []struct{ key1, key2 string }{
		{"instance", "selected"},
		{"instance", "node"},
		{"dimension", "selected"},
		{"node", "selected"},
	}
	aggCombos := []struct{ agg1, agg2 string }{
		{"sum", "sum"}, {"min", "min"}, {"max", "max"},
		{"extremes", "extremes"}, {"sum", "average"},
	}

	for _, kc := range keyCombos {
		groups := l6Groups(kc.key1, kc.key2, members)
		for _, ac := range aggCombos {
			t.Run(kc.key1+"-"+ac.agg1+"/"+kc.key2+"-"+ac.agg2, func(t *testing.T) {
				params := daemon.DataParams(l5Context, fixture.T0, fixture.T0+l5Rows, l5Rows)
				params.Set("group_by[0]", kc.key1)
				params.Set("aggregation[0]", ac.agg1)
				params.Set("group_by[1]", kc.key2)
				params.Set("aggregation[1]", ac.agg2)
				doc, err := td.DataV3All(params)
				if err != nil {
					t.Fatal(err)
				}
				cols, err := canon.Columns(doc)
				if err != nil {
					t.Fatal(err)
				}
				if len(cols) != len(groups) {
					t.Fatalf("got %d final groups %v, want %d %v", len(cols), keys2(cols), len(groups), keys2(groups))
				}
				for gname, pass1Groups := range groups {
					col, ok := cols[gname]
					if !ok {
						t.Errorf("final group %q missing (have %v)", gname, keys2(cols))
						continue
					}
					for _, pt := range col {
						i := int(pt.T - fixture.T0)
						want, wantPartial, wantEmpty := l6Expected(ac.agg1, ac.agg2, pass1Groups, i)
						switch {
						case wantEmpty && pt.Value != nil:
							t.Errorf("%q row %d: value %v, want null", gname, i, *pt.Value)
						case !wantEmpty && pt.Value == nil:
							t.Errorf("%q row %d: null, want %v", gname, i, want)
						case !wantEmpty && !tierValueMatch(*pt.Value, want, 1e-9):
							t.Errorf("%q row %d: value %v, want %v", gname, i, *pt.Value, want)
						}
						gotPartial := pt.PA&canon.AnnotationPartial != 0
						if gotPartial != wantPartial {
							t.Errorf("%q row %d: partial %v, want %v (pa %d)", gname, i, gotPartial, wantPartial, pt.PA)
						}
					}
				}
			})
		}
	}
}

// TestCase018MultipassAverage pins the KNOWN avg-of-sums bug: with
// AVERAGE at pass 1, pass 2 consumes the groups' SUMS (the per-group
// division never happens), so [dimension,avg]→[selected,avg] returns
// sum-of-all divided by the GROUP count — inflated by roughly the members
// per group versus the mean of the group averages.
func TestCase018MultipassAverage(t *testing.T) {
	members := l5Members()
	if _, err := td.WaitRetention("l5-a", l5Context, fixture.T0+1, fixture.T0+l5Rows, 15*time.Second); err != nil {
		t.Skip("layer-5 palette not available (TestLayer5GroupByMatrix failed?)")
	}

	groups := l6Groups("dimension", "selected", members)["selected"]

	params := daemon.DataParams(l5Context, fixture.T0, fixture.T0+l5Rows, l5Rows)
	params.Set("group_by[0]", "dimension")
	params.Set("aggregation[0]", "average")
	params.Set("group_by[1]", "selected")
	params.Set("aggregation[1]", "average")
	doc, err := td.DataV3All(params)
	if err != nil {
		t.Fatal(err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}
	col := cols["selected"]
	if len(col) == 0 {
		t.Fatal("no selected column")
	}

	reproduced := 0
	for _, pt := range col {
		i := int(pt.T - fixture.T0)

		// what the engine mechanically produces: (Σ group sums) / groups
		var brokenAcc float64
		groupsSeen := 0
		// what a mean of the group AVERAGES would be
		var meanOfAvgs float64
		for _, g := range groups {
			sum, gbc := l6Pass1("average", g, i)
			if gbc == 0 {
				continue
			}
			brokenAcc += sum
			meanOfAvgs += sum / float64(gbc)
			groupsSeen++
		}
		if groupsSeen == 0 || pt.Value == nil {
			continue
		}
		broken := brokenAcc / float64(groupsSeen)
		meanOfAvgs /= float64(groupsSeen)

		switch {
		case tierValueMatch(*pt.Value, broken, 1e-9) && !tierValueMatch(broken, meanOfAvgs, 1e-9):
			reproduced++
		case tierValueMatch(*pt.Value, meanOfAvgs, 1e-9):
			// the fix landed: the engine now averages group averages
		default:
			t.Fatalf("row %d: value %v matches neither avg-of-sums %v nor mean-of-averages %v — new behavior, investigate",
				i, *pt.Value, broken, meanOfAvgs)
		}
	}

	t.Logf("avg-of-sums reproduced on %d/%d rows", reproduced, len(col))
	expectAgentStatus(t, "CASE-018/multipass-average", reproduced == 0)
}
