// SPDX-License-Identifier: GPL-3.0-or-later

// Layer 6 — two-pass (hierarchical) group-by: pass 2 consumes finalized
// pass-1 groups. Pass-2 average divides by contributing pass-1 groups;
// finalized anomaly metadata remains weighted by the raw metric contributors
// beneath those groups. Raw output retains the old Agent-Cloud merge contract:
// its single count field counts prior-pass groups because it can also be the
// value divisor for a Cloud-rewritten average.
//
// Consequences, pinned here:
//   - non-raw chains without an average boundary are the surgical
//     contributor-weight contract;
//   - sum→average is held separately because its value and metadata need
//     different denominators;
//   - CASE-018 distinguishes the correct mean of finalized pass-1 averages
//     from the current avg-of-sums defect;
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

// l6Expected computes the two-pass value and metadata contract for a final group
// (the pass-1 groups it contains) at row i: pass-1 accumulators flow
// unconverted into the pass-2 aggregation; a final AVERAGE divides by
// the number of contributing pass-1 groups. ARP instead divides the sum
// of raw member anomaly rates by the number of raw metric contributors.
// Raw mode keeps that numerator but exposes the prior-pass group count required
// by the existing Agent-Cloud merge contract.
func l6Expected(agg1, agg2 string, pass1Groups [][]l5Member, i int, raw bool) (val, ar float64, gbc int, partial, empty bool) {
	var acc, arTotal float64
	groups := 0
	contributors := 0
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
		contributors += gbc1
	}
	if contributors == 0 {
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
	return acc, arTotal / float64(contributors), contributors, partial, false
}

func TestL6ContributorWeightedMetadataOracle(t *testing.T) {
	groups := [][]l5Member{
		{{Base: 1}},
		{
			{Base: 2, AnomLo: 1, AnomHi: 1},
			{Base: 3},
			{Base: 4},
		},
	}
	for _, raw := range []bool{false, true} {
		_, arp, count, _, empty := l6Expected("sum", "sum", groups, 1, raw)
		wantARP := 25.0
		wantCount := 4
		if raw {
			wantARP = 100
			wantCount = 2
		}
		if empty || arp != wantARP || count != wantCount {
			t.Errorf("raw=%v: arp=%v count=%d empty=%v, want %v/%d/false",
				raw, arp, count, empty, wantARP, wantCount)
		}
	}
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

type l6AggChain struct{ agg1, agg2 string }

// TestLayer6TwoPassMatrix covers chains with no average boundary. Their
// non-raw anomaly metadata can use raw-contributor weights without changing a
// value divisor; raw output retains its prior-pass group count.
func TestLayer6TwoPassMatrix(t *testing.T) {
	trackContract(t, "L6/two-pass-matrix")

	testLayer6TwoPassChains(t, []l6AggChain{
		{"sum", "sum"}, {"min", "min"}, {"max", "max"},
		{"extremes", "extremes"},
		{"sum", "min"}, {"max", "sum"}, {"min", "extremes"},
	})
}

// An average at pass 2 needs two denominators: contributing pass-1 groups for
// the value, and raw metric contributors for anomaly metadata. Keep this held
// contract separate so the surgical no-average fix cannot corrupt its value.
func TestLayer6TwoPassAverageBoundary(t *testing.T) {
	trackContract(t, "L6/two-pass-average-boundary")

	testLayer6TwoPassChains(t, []l6AggChain{{"sum", "average"}})
}

func testLayer6TwoPassChains(t *testing.T, aggCombos []l6AggChain) {
	t.Helper()

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
					if err := queryPointSchemaField(doc, "hidden", false); err != nil {
						t.Errorf("%s/%s-%s/%s-%s: %v",
							mode, kc.key1, ac.agg1, kc.key2, ac.agg2, err)
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
						for rowIndex, pt := range col {
							i := rowIndex + 1
							wantT := fixture.T0 + int64(i)
							want, wantAR, wantGbc, wantPartial, wantEmpty := l6Expected(ac.agg1, ac.agg2, pass1Groups, i, raw)
							if pt.T != wantT {
								t.Errorf("%q row %d: timestamp %d, want %d", gname, i, pt.T, wantT)
							}
							switch {
							case wantEmpty && pt.Value != nil:
								t.Errorf("%q row %d: value %v, want null", gname, i, *pt.Value)
							case !wantEmpty && pt.Value == nil:
								t.Errorf("%q row %d: null, want %v", gname, i, want)
							case !wantEmpty && !tierValueMatch(*pt.Value, want, 1e-9):
								t.Errorf("%q row %d: value %v, want %v", gname, i, *pt.Value, want)
							}
							if !tierValueMatch(pt.ARP, wantAR, 1e-9) {
								t.Errorf("%q row %d: arp %v, want %v", gname, i, pt.ARP, wantAR)
							}
							if raw && !wantEmpty {
								if pt.Count == nil {
									t.Errorf("%q row %d: raw point has no count", gname, i)
								} else if *pt.Count != int64(wantGbc) {
									t.Errorf("%q row %d: count %d, want %d prior-pass groups",
										gname, i, *pt.Count, wantGbc)
								}
							} else if pt.Count != nil {
								t.Errorf("%q row %d: count %d is present, want absent", gname, i, *pt.Count)
							}
							wantPA := int64(0)
							if wantEmpty {
								wantPA = canon.AnnotationEmpty
							} else if wantPartial {
								wantPA = canon.AnnotationPartial
							}
							if pt.PA != wantPA {
								t.Errorf("%q row %d: pa %d, want exactly %d", gname, i, pt.PA, wantPA)
							}
						}
					}
				})
			}
		}
	}
}

// TestCase018MultipassAverage discriminates the correct mean of finalized
// pass-1 group averages from the current avg-of-sums defect.
func TestCase018MultipassAverage(t *testing.T) {
	trackContract(t, "CASE-018/multipass-average")

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
	ok := assertOnlyColumn(t, cols, "selected")
	if !assertColumnExactGrid(t, cols, "selected", fixture.T0, fixture.T0+l5Rows, 1) {
		ok = false
	}
	col := cols["selected"]

	reproduced, classified := 0, 0
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
		if groupsSeen == 0 {
			t.Logf("row %d has no contributing fixture groups", i)
			ok = false
			continue
		}
		if pt.Value == nil {
			t.Logf("row %d is null, want a numeric avg-of-sums or mean-of-averages result", i)
			ok = false
			continue
		}
		broken := brokenAcc / float64(groupsSeen)
		meanOfAvgs /= float64(groupsSeen)

		switch {
		case tierValueMatch(*pt.Value, broken, 1e-9) && !tierValueMatch(broken, meanOfAvgs, 1e-9):
			reproduced++
			classified++
		case tierValueMatch(*pt.Value, meanOfAvgs, 1e-9):
			// the fix landed: the engine now averages group averages
			classified++
		default:
			t.Logf("row %d: value %v matches neither avg-of-sums %v nor mean-of-averages %v — new behavior, investigate",
				i, *pt.Value, broken, meanOfAvgs)
			ok = false
		}
	}
	if classified != l5Rows {
		t.Logf("classified %d/%d expected rows", classified, l5Rows)
		ok = false
	}

	t.Logf("avg-of-sums reproduced on %d/%d rows", reproduced, len(col))
	assertContract(t, "CASE-018/multipass-average", ok && reproduced == 0)
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
// and the point count remains the number of visible prior-pass groups.
func TestLayer6TwoPassPercentage(t *testing.T) {
	trackContract(t, "L6/two-pass-percentage")

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
				if err := queryPointSchemaField(doc, "hidden", raw); err != nil {
					t.Errorf("%s/%s/%s: %v", mode, kc.key1, kc.key2, err)
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
					for rowIndex, pt := range col {
						i := rowIndex + 1
						wantT := fixture.T0 + int64(i)
						if pt.T != wantT {
							t.Errorf("%q row %d: timestamp %d, want %d", fk, i, pt.T, wantT)
						}

						var v, h, arTot float64
						visibleGroups := 0
						visibleContributors := 0
						hiddenContributors := 0
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
							visibleGroups++
							visibleContributors += gbc1
						}
						// a visible pass-1 group contributing NOTHING on a
						// row (all members gapped) shorts the engine's gbc
						// against its expected count → PARTIAL, mirroring
						// the hidden-side check below
						if visibleGroups > 0 && visibleGroups < len(b.vis) {
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
							hiddenContributors += gbc1
						}
						if hidContrib < len(b.hid) {
							partial = true
						}

						empty := visibleContributors == 0
						want, wantAR := v, arTot
						if !raw && !empty {
							want = v * 100 / (v + h)
							wantAR = arTot / float64(visibleContributors)
						}
						switch {
						case empty && pt.Value != nil:
							t.Errorf("%q row %d: value %v, want null", fk, i, *pt.Value)
						case !empty && pt.Value == nil:
							t.Errorf("%q row %d: null, want %v", fk, i, want)
						case !empty && !tierValueMatch(*pt.Value, want, 1e-9):
							t.Errorf("%q row %d: value %v, want %v", fk, i, *pt.Value, want)
						}
						if !tierValueMatch(pt.ARP, wantAR, 1e-9) {
							t.Errorf("%q row %d: arp %v, want %v", fk, i, pt.ARP, wantAR)
						}
						if raw && !empty {
							if pt.Count == nil {
								t.Errorf("%q row %d: count is absent, want %d visible prior-pass groups",
									fk, i, visibleGroups)
							} else if *pt.Count != int64(visibleGroups) {
								t.Errorf("%q row %d: count %d, want %d visible prior-pass groups",
									fk, i, *pt.Count, visibleGroups)
							}
						} else if pt.Count != nil {
							t.Errorf("%q row %d: count %d is present, want absent", fk, i, *pt.Count)
						}
						if raw && hiddenContributors > 0 {
							if pt.Hidden == nil || !tierValueMatch(*pt.Hidden, h, 1e-9) {
								t.Errorf("%q row %d: hidden %v, want %v", fk, i, pt.Hidden, h)
							}
						} else if pt.Hidden != nil {
							t.Errorf("%q row %d: hidden %v is present, want absent", fk, i, *pt.Hidden)
						}
						wantPA := int64(0)
						if empty {
							wantPA = canon.AnnotationEmpty
						} else if partial {
							wantPA = canon.AnnotationPartial
						}
						if pt.PA != wantPA {
							t.Errorf("%q row %d: pa %d, want exactly %d", fk, i, pt.PA, wantPA)
						}
					}
				}
			})
		}
	}
}
