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
// mechanics: sums for average/sum, champions for min/max/extremes; the
// anomaly rate always accumulates raw (sum of member ARPs, no division
// before the final finalize). Returns the accumulator, the accumulated
// anomaly rate and the contribution count.
func l6Pass1(agg string, group []l5Member, i int) (acc, ar float64, gbc int) {
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
		if m.anomalous(i) {
			ar += 100
		}
		gbc++
	}
	return acc, ar, gbc
}

// l6Expected computes the CURRENT two-pass mechanics for a final group
// (the pass-1 groups it contains) at row i: pass-1 accumulators flow
// unconverted into the pass-2 aggregation; a final AVERAGE divides by
// the number of contributing pass-1 groups. The anomaly rate accumulates
// RAW through both passes and divides ONCE by the final contribution
// count (query-group-by-finalize.c:421) — so non-raw two-pass ARP is
// sum(member ARPs)/groups, INFLATED by members-per-group for every
// chain (the ar analog of the avg-of-sums family; pinned as current
// mechanics). In raw mode nothing divides: the value stays the pass-2
// accumulator, ar the total, and the point count is the number of
// contributing pass-1 GROUPS (not members) — the pin the cloud layers
// depend on.
func l6Expected(agg1, agg2 string, pass1Groups [][]l5Member, i int, raw bool) (val, ar float64, gbc int, partial, empty bool) {
	var acc, arTotal float64
	groups := 0
	expectedGroups := 0
	first := true
	for _, g := range pass1Groups {
		expectedGroups++
		a1, ar1, gbc1 := l6Pass1(agg1, g, i)
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
		arTotal += ar1
		groups++
	}
	if groups == 0 {
		return 0, 0, 0, false, true
	}
	if groups < expectedGroups {
		partial = true
	}
	if raw {
		return acc, arTotal, groups, partial, false
	}
	if agg2 == "average" {
		acc /= float64(groups)
	}
	return acc, arTotal / float64(groups), groups, partial, false
}

// l6Groups builds the two-pass structure: final groups (by key2) of
// pass-1 groups. The engine merges every later pass's keys into the
// earlier passes (query-group-by-init.c:263-302), so pass 1 partitions
// by the UNION of key1 and key2 — every pass-1 group maps into exactly
// one final group by construction.
func l6Groups(key1, key2 string, members []l5Member) map[string][][]l5Member {
	pass1 := map[string][]l5Member{}
	for _, m := range members {
		k := l5GroupKey(key1, m) + "\x00" + l5GroupKey(key2, m)
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
		// cross-key chains: pass 1 partitions by the UNION of both keys
		{"dimension", "node"},
		{"dimension", "instance"},
		{"dimension", "label"},
		{"label", "node"},
		{"instance", "label"},
		{"instance", "units"},
	}
	aggCombos := []struct{ agg1, agg2 string }{
		{"sum", "sum"}, {"min", "min"}, {"max", "max"},
		{"extremes", "extremes"}, {"sum", "average"},
		// mixed chains: pass 2 consumes pass-1 accumulators as-is
		{"sum", "min"}, {"max", "sum"}, {"min", "extremes"},
	}

	for _, mode := range []string{"non-raw", "raw"} {
		raw := mode == "raw"
		for _, kc := range keyCombos {
			groups := l6Groups(kc.key1, kc.key2, members)
			for _, ac := range aggCombos {
				t.Run(mode+"/"+kc.key1+"-"+ac.agg1+"/"+kc.key2+"-"+ac.agg2, func(t *testing.T) {
					params := daemon.DataParams(l5Context, fixture.T0, fixture.T0+l5Rows, l5Rows)
					params.Set("group_by[0]", kc.key1)
					params.Set("aggregation[0]", ac.agg1)
					params.Set("group_by[1]", kc.key2)
					params.Set("aggregation[1]", ac.agg2)
					if kc.key1 == "label" {
						params.Set("group_by_label[0]", "team")
					}
					if kc.key2 == "label" {
						params.Set("group_by_label[1]", "team")
					}
					if raw {
						params.Set("options", "jsonwrap|raw")
					}
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
						if len(col) != l5Rows {
							t.Errorf("%q: got %d rows, want %d", gname, len(col), l5Rows)
							continue
						}
						for _, pt := range col {
							i := int(pt.T - fixture.T0)
							want, wantAR, wantGbc, wantPartial, wantEmpty := l6Expected(ac.agg1, ac.agg2, pass1Groups, i, raw)
							switch {
							case wantEmpty && pt.Value != nil:
								t.Errorf("%q row %d: value %v, want null", gname, i, *pt.Value)
							case !wantEmpty && pt.Value == nil:
								t.Errorf("%q row %d: null, want %v", gname, i, want)
							case !wantEmpty && !tierValueMatch(*pt.Value, want, 1e-9):
								t.Errorf("%q row %d: value %v, want %v", gname, i, *pt.Value, want)
							}
							if !wantEmpty && !tierValueMatch(pt.ARP, wantAR, 1e-9) {
								t.Errorf("%q row %d: arp %v, want %v", gname, i, pt.ARP, wantAR)
							}
							if raw && !wantEmpty {
								if pt.Count == nil {
									t.Errorf("%q row %d: raw point has no count", gname, i)
								} else if *pt.Count != int64(wantGbc) {
									t.Errorf("%q row %d: count %d, want %d pass-1 groups", gname, i, *pt.Count, wantGbc)
								}
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
			sum, _, gbc := l6Pass1("average", g, i)
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

// TestLayer6TwoPassPercentage pins percentage as the PASS-2 aggregation.
// The percentage pass is the FIRST pass with a percentage aggregation
// (query-group-by-init.c percentage_of_group_pass), so pass 1 runs in
// SHADOW hidden mode: dimensions excluded by the `dimensions` selector
// accumulate in per-group shadow buckets, kept apart from the visible
// sums, and fold into the DENOMINATOR (vh) of their normal group at the
// percentage pass. A shadow bucket that is itself incomplete (a gapped
// hidden member) taints the final point PARTIAL through the hgbc top
// bit. Non-raw converts v*100/(v+h); raw converts NOTHING — the value
// stays the visible accumulator, the hidden accumulator rides the wire,
// and the point count is the number of visible pass-1 groups.
func TestLayer6TwoPassPercentage(t *testing.T) {
	members := l5Members()
	if _, err := td.WaitRetention("l5-a", l5Context, fixture.T0+1, fixture.T0+l5Rows, 15*time.Second); err != nil {
		t.Skip("layer-5 palette not available (TestLayer5GroupByMatrix failed?)")
	}

	// select dc: the anomaly-run member stays visible (drives ARP), the
	// gap member (da) lands on the hidden side (drives the hgbc taint)
	const sel = "dc"

	chains := []struct{ key1, key2 string }{
		{"instance", "node"},
		{"dimension", "selected"},
	}

	for _, mode := range []string{"non-raw", "raw"} {
		raw := mode == "raw"
		for _, kc := range chains {
			t.Run(mode+"/"+kc.key1+"-sum/"+kc.key2+"-percentage", func(t *testing.T) {
				// bucket the palette: per final group, the visible and
				// shadow pass-1 buckets (partitioned by the union key)
				type buckets struct{ vis, hid [][]l5Member }
				finals := map[string]*buckets{}
				addTo := func(m l5Member, hidden bool) {
					fk := l5GroupKey(kc.key2, m)
					b := finals[fk]
					if b == nil {
						b = &buckets{}
						finals[fk] = b
					}
					uk := l5GroupKey(kc.key1, m) + "\x00" + fk
					list := &b.vis
					if hidden {
						list = &b.hid
					}
					placed := false
					for gi := range *list {
						if l5GroupKey(kc.key1, (*list)[gi][0])+"\x00"+l5GroupKey(kc.key2, (*list)[gi][0]) == uk {
							(*list)[gi] = append((*list)[gi], m)
							placed = true
							break
						}
					}
					if !placed {
						*list = append(*list, []l5Member{m})
					}
				}
				for _, m := range members {
					addTo(m, m.Dim != sel)
				}

				params := daemon.DataParams(l5Context, fixture.T0, fixture.T0+l5Rows, l5Rows)
				params.Set("dimensions", sel)
				params.Set("group_by[0]", kc.key1)
				params.Set("aggregation[0]", "sum")
				params.Set("group_by[1]", kc.key2)
				params.Set("aggregation[1]", "percentage")
				if raw {
					params.Set("options", "jsonwrap|raw")
				}
				doc, err := td.DataV3All(params)
				if err != nil {
					t.Fatal(err)
				}
				cols, err := canon.Columns(doc)
				if err != nil {
					t.Fatal(err)
				}
				if len(cols) != len(finals) {
					t.Fatalf("got %d groups %v, want %d %v", len(cols), keys2(cols), len(finals), keys2(finals))
				}

				for fk, b := range finals {
					col, ok := cols[fk]
					if !ok {
						t.Errorf("group %q missing (have %v)", fk, keys2(cols))
						continue
					}
					if len(col) != l5Rows {
						t.Errorf("%q: got %d rows, want %d", fk, len(col), l5Rows)
						continue
					}
					for _, pt := range col {
						i := int(pt.T - fixture.T0)

						var v, h, arTot float64
						gbc := 0
						partial := false
						for _, g := range b.vis {
							sum, ar1, gbc1 := l6Pass1("sum", g, i)
							if gbc1 == 0 {
								continue
							}
							if gbc1 < len(g) {
								partial = true
							}
							v += sum
							arTot += ar1
							gbc++
						}
						// a visible pass-1 group contributing NOTHING on a
						// row (all members gapped) shorts the engine's gbc
						// against its expected count → PARTIAL, mirroring
						// the hidden-side check below
						if gbc > 0 && gbc < len(b.vis) {
							partial = true
						}
						hidContrib := 0
						for _, g := range b.hid {
							sum, _, gbc1 := l6Pass1("sum", g, i)
							if gbc1 == 0 {
								continue
							}
							if gbc1 < len(g) {
								partial = true
							}
							h += sum
							hidContrib++
						}
						if hidContrib < len(b.hid) {
							partial = true
						}

						if gbc == 0 {
							if pt.Value != nil {
								t.Errorf("%q row %d: value %v, want null", fk, i, *pt.Value)
							}
							continue
						}

						want := v
						wantAR := arTot
						if !raw {
							want = v * 100 / (v + h)
							wantAR = arTot / float64(gbc)
						}
						switch {
						case pt.Value == nil:
							t.Errorf("%q row %d: null, want %v", fk, i, want)
						case !tierValueMatch(*pt.Value, want, 1e-9):
							t.Errorf("%q row %d: value %v, want %v", fk, i, *pt.Value, want)
						}
						if !tierValueMatch(pt.ARP, wantAR, 1e-9) {
							t.Errorf("%q row %d: arp %v, want %v", fk, i, pt.ARP, wantAR)
						}
						if raw {
							if pt.Count == nil || *pt.Count != int64(gbc) {
								t.Errorf("%q row %d: count %v, want %d visible pass-1 groups", fk, i, pt.Count, gbc)
							}
							if pt.Hidden == nil || !tierValueMatch(*pt.Hidden, h, 1e-9) {
								t.Errorf("%q row %d: hidden %v, want %v", fk, i, pt.Hidden, h)
							}
						}
						gotPartial := pt.PA&canon.AnnotationPartial != 0
						if gotPartial != partial {
							t.Errorf("%q row %d: partial %v, want %v (pa %d)", fk, i, gotPartial, partial, pt.PA)
						}
					}
				}
			})
		}
	}
}
