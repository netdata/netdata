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
	"github.com/netdata/netdata/tests/query-corpus/stream"
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

type l6ChainResults struct {
	grouping, grid, values, pointAnomaly, viewAnomaly, partialEmpty, rawSchema bool
}

func l6RawCountMatches(count int64, empty bool, contributors int) bool {
	return empty || count == int64(contributors)
}

func TestL6RawCountSchemaGuard(t *testing.T) {
	if !l6RawCountMatches(0, true, 4) {
		t.Fatal("raw empty row rejected its schema count")
	}
	if l6RawCountMatches(3, false, 4) {
		t.Fatal("raw numeric row accepted the wrong contributor count")
	}
	if !l6RawCountMatches(4, false, 4) {
		t.Fatal("raw numeric row rejected its exact contributor count")
	}
}

func newL6ChainResults() l6ChainResults {
	return l6ChainResults{true, true, true, true, true, true, true}
}

func (r l6ChainResults) all() bool {
	return r.grouping && r.grid && r.values && r.pointAnomaly &&
		r.viewAnomaly && r.partialEmpty && r.rawSchema
}

// TestLayer6TwoPassMatrix covers chains with no average boundary. Their
// non-raw anomaly metadata can use raw-contributor weights without changing a
// value divisor; raw output retains its prior-pass group count.
func TestLayer6TwoPassMatrix(t *testing.T) {
	contractNames := []string{
		"L6/two-pass-grouping", "L6/two-pass-grid", "L6/two-pass-values",
		"L6/two-pass-point-anomaly", "L6/two-pass-view-anomaly",
		"L6/two-pass-partial-empty", "L6/two-pass-raw-schema",
	}
	for _, contract := range contractNames {
		registerContract(t, contract)
	}

	result := testLayer6TwoPassChains(t, []l6AggChain{
		{"sum", "sum"}, {"min", "min"}, {"max", "max"},
		{"extremes", "extremes"},
		{"sum", "min"}, {"max", "sum"}, {"min", "extremes"},
	})
	for contract, held := range map[string]bool{
		"L6/two-pass-grouping":      result.grouping,
		"L6/two-pass-grid":          result.grid,
		"L6/two-pass-values":        result.values,
		"L6/two-pass-point-anomaly": result.pointAnomaly,
		"L6/two-pass-view-anomaly":  result.viewAnomaly,
		"L6/two-pass-partial-empty": result.partialEmpty,
		"L6/two-pass-raw-schema":    result.rawSchema,
	} {
		assertContract(t, contract, held)
	}
}

// TestLayer6TwoPassLiveEdgePartialRow makes the final stored row incomplete
// inside one pass-1 group. The established near-live tolerance trims that
// unstable suffix while preserving every preceding complete row.
func TestLayer6TwoPassLiveEdgePartialRow(t *testing.T) {
	const (
		context = "fixture.l6edge"
		ue      = int64(300)
	)
	boundary := time.Now().Unix() / ue * ue
	point := func(at int64, value string) fixture.Point {
		return fixture.Point{T: at, Collected: value, Flags: stream.FlagNotAnomalous}
	}
	aPoints := make([]fixture.Point, 0, 5)
	bPoints := make([]fixture.Point, 0, 4)
	for at := boundary - 4*ue; at <= boundary; at += ue {
		aPoints = append(aPoints, point(at, "1"))
		if at < boundary {
			bPoints = append(bPoints, point(at, "10"))
		}
	}
	ch := fixture.Chart{
		ID: context, Title: "multipass live edge", Units: "units", Family: "fixture",
		Context: context, UpdateEvery: int(ue),
		Dimensions: []fixture.Dimension{
			{ID: "a", Points: aPoints},
			{ID: "b", Points: bPoints},
		},
	}
	pushLiveBurst(t, "l6-edge", guid(336), ch)
	if _, err := td.WaitRetention("l6-edge", context, boundary-4*ue, boundary, 15*time.Second); err != nil {
		t.Fatal(err)
	}

	after, before := boundary-6*ue, boundary
	params := daemon.DataParams(context, after, before, 6)
	params.Set("group_by[0]", "instance")
	params.Set("aggregation[0]", "sum")
	params.Set("group_by[1]", "selected")
	params.Set("aggregation[1]", "sum")
	params.Set("options", "jsonwrap|virtual-points|unaligned")
	doc, err := td.DataV3("l6-edge", params)
	if err != nil {
		t.Fatal(err)
	}

	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}
	col := cols["selected"]
	wantTimes := []int64{boundary - 5*ue, boundary - 4*ue, boundary - 3*ue, boundary - 2*ue, boundary - ue}

	t.Run("trimming", func(t *testing.T) {
		trackContract(t, "L6/two-pass-live-edge-trimming")
		ok := assertViewFields(t, doc, after+1, before, ue)
		ok = assertExactColumnSet(t, cols, []string{"selected"}) && ok
		if len(col) != len(wantTimes) {
			t.Logf("live-edge result has %d rows, want %d after trimming", len(col), len(wantTimes))
			ok = false
		}
		for i, pt := range col {
			if i >= len(wantTimes) {
				break
			}
			if pt.T != wantTimes[i] {
				t.Logf("live-edge row %d timestamp %d, want %d", i, pt.T, wantTimes[i])
				ok = false
			}
		}
		if !ok {
			t.Errorf("the incomplete near-live suffix was not trimmed at the contributor decline")
		}
	})

	t.Run("values", func(t *testing.T) {
		trackContract(t, "L6/two-pass-live-edge-values")
		number := func(value float64) *float64 { return &value }
		want := []*float64{nil, number(11), number(11), number(11), number(11)}
		if len(col) != len(want) {
			t.Fatalf("live-edge result has %d rows, want %d surviving rows", len(col), len(want))
		}
		for i, pt := range col {
			if pt.T != wantTimes[i] {
				t.Errorf("live-edge row %d timestamp %d, want %d before checking its value", i, pt.T, wantTimes[i])
				continue
			}
			switch {
			case want[i] == nil && pt.Value != nil:
				t.Errorf("live-edge row %d value %v, want null", i, *pt.Value)
			case want[i] != nil && (pt.Value == nil || !tierValueMatch(*pt.Value, *want[i], 0)):
				t.Errorf("live-edge row %d value %v, want %v", i, pt.Value, *want[i])
			}
		}
	})

	t.Run("annotations", func(t *testing.T) {
		trackContract(t, "L6/two-pass-live-edge-annotations")
		want := []int64{canon.AnnotationEmpty, 0, 0, 0, 0}
		if len(col) != len(want) {
			t.Fatalf("live-edge result has %d rows, want %d surviving rows", len(col), len(want))
		}
		for i, pt := range col {
			if pt.T != wantTimes[i] {
				t.Errorf("live-edge row %d timestamp %d, want %d before checking its annotation", i, pt.T, wantTimes[i])
				continue
			}
			if pt.PA != want[i] {
				t.Errorf("live-edge row %d annotation %d, want exactly %d", i, pt.PA, want[i])
			}
		}
	})
}

// An average at pass 2 needs two denominators: contributing pass-1 groups for
// the value, and raw metric contributors for anomaly metadata. Keep this held
// contract separate so the surgical no-average fix cannot corrupt its value.
func TestLayer6TwoPassAverageBoundary(t *testing.T) {
	trackContractComponent(t, "L6/two-pass-average-boundary", "sum-to-average")

	if !testLayer6TwoPassChains(t, []l6AggChain{{"sum", "average"}}).all() {
		t.Error("two-pass sum-to-average contract failed")
	}
}

func testLayer6TwoPassChains(t *testing.T, aggCombos []l6AggChain) l6ChainResults {
	t.Helper()
	result := newL6ChainResults()

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
				label := mode + "/" + kc.key1 + "-" + ac.agg1 + "/" + kc.key2 + "-" + ac.agg2
				passed := t.Run(label, func(t *testing.T) {
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
						t.Logf("%s: %v", label, err)
						result.rawSchema = false
					}
					cols, err := canon.Columns(doc)
					if err != nil {
						t.Fatal(err)
					}
					if len(cols) != len(groups) {
						t.Logf("%s: got %d final groups %v, want %d %v", label, len(cols), keys2(cols), len(groups), keys2(groups))
						result.grouping = false
					}
					var viewStats map[string]map[string]float64
					assertViewARP := !raw && ac.agg1 != "average" && ac.agg2 != "average"
					if assertViewARP {
						var statsOK bool
						viewStats, statsOK = strictDimensionStats(
							t, doc, "view", keys2(groups), []string{"arp"})
						if !statsOK {
							t.Logf("%s: view dimension anomaly statistics are malformed", label)
							result.viewAnomaly = false
						}
					}
					for gname, pass1Groups := range groups {
						col, ok := cols[gname]
						if !ok {
							t.Logf("%s: final group %q missing (have %v)", label, gname, keys2(cols))
							result.grouping = false
							result.grid, result.values = false, false
							result.pointAnomaly, result.viewAnomaly = false, false
							result.partialEmpty, result.rawSchema = false, false
							continue
						}
						if len(col) != l5Rows {
							t.Logf("%s: %q got %d rows, want %d", label, gname, len(col), l5Rows)
							result.grid, result.values = false, false
							result.pointAnomaly, result.viewAnomaly = false, false
							result.partialEmpty = false
							result.rawSchema = false
						}
						viewARPTotal := 0.0
						viewARPRows := 0
						for rowIndex, pt := range col {
							if rowIndex >= l5Rows {
								break
							}
							i := rowIndex + 1
							wantT := fixture.T0 + int64(i)
							want, wantAR, wantGbc, wantPartial, wantEmpty := l6Expected(ac.agg1, ac.agg2, pass1Groups, i, raw)
							if pt.T != wantT {
								t.Logf("%s: %q row %d timestamp %d, want %d", label, gname, i, pt.T, wantT)
								result.grid = false
								result.values, result.pointAnomaly = false, false
								result.viewAnomaly, result.partialEmpty = false, false
								result.rawSchema = false
							}
							switch {
							case wantEmpty && pt.Value != nil:
								t.Logf("%s: %q row %d value %v, want null", label, gname, i, *pt.Value)
								result.partialEmpty = false
							case !wantEmpty && pt.Value == nil:
								t.Logf("%s: %q row %d null, want %v", label, gname, i, want)
								result.values, result.partialEmpty = false, false
							case !wantEmpty && !tierValueMatch(*pt.Value, want, 1e-9):
								t.Logf("%s: %q row %d value %v, want %v", label, gname, i, *pt.Value, want)
								result.values = false
							}
							if !tierValueMatch(pt.ARP, wantAR, 1e-9) {
								t.Logf("%s: %q row %d arp %v, want %v", label, gname, i, pt.ARP, wantAR)
								result.pointAnomaly = false
							}
							if assertViewARP && !wantEmpty {
								viewARPTotal += wantAR
								viewARPRows++
							}
							if raw {
								if pt.Count == nil {
									t.Logf("%s: %q row %d raw point has no count", label, gname, i)
									result.rawSchema = false
								} else if !l6RawCountMatches(*pt.Count, wantEmpty, wantGbc) {
									t.Logf("%s: %q row %d count %d, want %d prior-pass groups",
										label, gname, i, *pt.Count, wantGbc)
									result.rawSchema = false
								}
							} else if pt.Count != nil {
								t.Logf("%s: %q row %d count %d is present, want absent", label, gname, i, *pt.Count)
								result.rawSchema = false
							}
							wantPA := int64(0)
							if wantEmpty {
								wantPA = canon.AnnotationEmpty
							} else if wantPartial {
								wantPA = canon.AnnotationPartial
							}
							if pt.PA != wantPA {
								t.Logf("%s: %q row %d pa %d, want exactly %d", label, gname, i, pt.PA, wantPA)
								result.partialEmpty = false
							}
						}
						if assertViewARP {
							// Class B — dview truncates sum(row ARP)*10 into anomaly_count, then
							// jsonwrap-v2 divides by the row count and the 1000x dview multiplier:
							// netdata/netdata @ 89a2855db958400528ebd996e8869564c9c20862,
							// src/web/api/queries/query-group-by-finalize.c:380-460;
							// src/web/api/queries/rrdr.h:51;
							// src/libnetdata/storage-point.h:120-121;
							// src/web/api/formatters/jsonwrap-v2.c:102-104,176-182.
							got, ok := viewStats[gname]["arp"]
							if !ok {
								t.Logf("%s: %q view sts arp is missing (have %v)", label, gname, viewStats[gname])
								result.viewAnomaly = false
							} else if viewARPRows == 0 {
								t.Logf("%s: %q fixture oracle found no rows for view sts arp", label, gname)
								result.viewAnomaly = false
							} else if want := math.Floor(viewARPTotal*10) / float64(viewARPRows*10); !tierValueMatch(got, want, 1e-9) {
								t.Logf("%s: %q view sts arp %v, want mean row anomaly rate %v", label, gname, got, want)
								result.viewAnomaly = false
							}
						}
					}
				})
				if !passed {
					// A fatal child could not verify any shared contract.
					result = l6ChainResults{}
				}
			}
		}
	}
	return result
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
	trackContractComponent(t, "L6/two-pass-percentage", "sum-to-percentage")

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
						if raw {
							if pt.Count == nil {
								t.Errorf("%q row %d: count is absent, want %d visible prior-pass groups",
									fk, i, visibleGroups)
							} else if !l6RawCountMatches(*pt.Count, empty, visibleGroups) {
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

func TestLayer6TwoPassHeldBoundaryPreservation(t *testing.T) {
	const (
		context = "fixture.l6held"
		host    = "l6-held"
	)
	point := func(value, flags string) []fixture.Point {
		// Two rows keep this control independent of the separately broken
		// points=1 query-window contract.
		return []fixture.Point{
			{T: fixture.T0 + 1, Collected: value, Flags: flags},
			{T: fixture.T0 + 2, Collected: value, Flags: flags},
		}
	}
	ch := fixture.Chart{
		ID: context, Title: "held multipass boundaries", Units: "units", Family: "fixture",
		Context: context, UpdateEvery: 1,
		Dimensions: []fixture.Dimension{
			{ID: "avg_a", Points: point("0", stream.FlagAnomalous)},
			{ID: "avg_b", Points: point("0", stream.FlagNotAnomalous)},
			{ID: "pct_a", Points: point("1", stream.FlagAnomalous)},
			{ID: "pct_b", Points: point("1", stream.FlagNotAnomalous)},
		},
	}
	pushed := false
	requireFixture := func(t *testing.T) {
		t.Helper()
		if !pushed {
			pushLiveBurst(t, host, guid(337), ch)
			if _, err := td.WaitRetention(
				host, context, fixture.T0+1, fixture.T0+2, 15*time.Second); err != nil {
				t.Fatal(err)
			}
			pushed = true
		}
	}
	assert := func(t *testing.T, group1, aggregation1, scope, selected string, value, arp float64) {
		t.Helper()
		requireFixture(t)

		params := daemon.DataParams(context, fixture.T0, fixture.T0+2, 2)
		params.Set("group_by[0]", group1)
		params.Set("aggregation[0]", aggregation1)
		params.Set("group_by[1]", "selected")
		params.Set("aggregation[1]", "sum")
		if scope != "" {
			params.Set("scope_dimensions", scope)
		}
		if selected != "" {
			params.Set("dimensions", selected)
		}
		doc, err := td.DataV3(host, params)
		if err != nil {
			t.Fatal(err)
		}
		if err := queryPointSchemaField(doc, "hidden", false); err != nil {
			t.Error(err)
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}
		if !assertOnlyColumn(t, cols, "selected") ||
			!assertColumnExactGrid(t, cols, "selected", fixture.T0, fixture.T0+2, 1) {
			t.FailNow()
		}
		for row, pt := range cols["selected"] {
			var gotValue any
			if pt.Value != nil {
				gotValue = *pt.Value
			}
			if pt.Value == nil || !tierValueMatch(*pt.Value, value, 0) {
				t.Errorf("row %d: value %v, want %v", row+1, gotValue, value)
			}
			if !tierValueMatch(pt.ARP, arp, 0) {
				t.Errorf("row %d: arp %v, want held-boundary stability value %v", row+1, pt.ARP, arp)
			}
			if pt.PA != 0 {
				t.Errorf("row %d: pa %d, want exactly 0", row+1, pt.PA)
			}
			if pt.Count != nil || pt.Hidden != nil {
				t.Errorf("row %d: non-raw count/hidden = %v/%v, want absent", row+1, pt.Count, pt.Hidden)
			}
		}
	}

	// Class C stability controls, not correctness rulings for average or
	// percentage composition. Zero-valued average inputs and a single instance
	// make the values invariant across the held redesign forks; only released
	// anomaly-divisor behavior and the absence of spurious PARTIAL on complete
	// rows are pinned:
	// netdata/netdata @ 89a2855db958400528ebd996e8869564c9c20862,
	// src/web/api/queries/query-group-by-init.c:258-326,492-523;
	// src/web/api/queries/query-group-by-finalize.c:10-117,158-186,284-421.
	t.Run("average-to-sum-held", func(t *testing.T) {
		trackContract(t, "L6/two-pass-average-held-boundary")
		assert(t, "instance", "average", "avg*", "", 0, 100)
	})
	t.Run("percentage-to-sum-held", func(t *testing.T) {
		trackContract(t, "L6/two-pass-percentage-held-boundary")
		assert(t, "instance", "percentage", "", "pct*", 100, 100)
	})
	t.Run("percentage-of-instance-to-sum-held", func(t *testing.T) {
		trackContract(t, "L6/two-pass-percentage-of-instance-held-boundary")
		assert(t, "percentage-of-instance", "sum", "", "pct*", 100, 100)
	})
}
