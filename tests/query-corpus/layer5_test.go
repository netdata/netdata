// SPDX-License-Identifier: GPL-3.0-or-later

// Layer 5 — level-1 group-by (non-raw contract): every group_by key with
// every aggregation over a multi-node palette, against a generic Go oracle
// that enumerates group members from the fixture definition.
//
// Oracle contracts (query-group-by-finalize.c):
//   - EMPTY member points contribute nothing; average/sum accumulate SUMS,
//     min/max compare plainly, extremes champions by |abs|;
//   - non-raw AVERAGE divides by the contribution count (gbc) at finalize;
//     anomaly rates divide by gbc for every aggregation;
//   - a point receiving fewer contributions than the group's member count
//     is stamped PARTIAL (the gap member's rows);
//   - group ids comma-join the selected axes (dimension name, instance id,
//     label value, node, context, units; "selected" literal).
package corpus

import (
	"math"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

const (
	l5Context = "fixture.l5"
	l5Rows    = 60
)

func l5ExactGrid(col []canon.Pt) bool {
	if len(col) != l5Rows {
		return false
	}
	for row, pt := range col {
		if pt.T != int64(fixture.T0+row+1) {
			return false
		}
	}
	return true
}

func TestL5ExactGridGuard(t *testing.T) {
	control := make([]canon.Pt, l5Rows)
	for row := range control {
		control[row].T = int64(fixture.T0 + row + 1)
	}
	if !l5ExactGrid(control) {
		t.Fatal("L5 exact-grid guard rejected the valid control")
	}

	duplicate := append([]canon.Pt(nil), control...)
	duplicate[7].T = duplicate[0].T
	if l5ExactGrid(duplicate) {
		t.Fatal("L5 exact-grid guard accepted a duplicate timestamp and missing expected row")
	}
	if l5ExactGrid(control[:len(control)-1]) {
		t.Fatal("L5 exact-grid guard accepted a dropped row")
	}
}

// l5Member is one metric of the palette: 2 nodes × 2 instances × 3 dims.
type l5Member struct {
	Host, GUID string
	Inst       string // chart/instance id
	Team       string // instance label value
	Dim        string
	Base       int
	GapLo      int // 1-based sample range with EMPTY flags (0 = none)
	GapHi      int
	AnomLo     int // 1-based sample range flagged anomalous (0 = none)
	AnomHi     int
}

// l5Members enumerates the palette: values are base + i%7, one member
// carries a gap run (drops its group's gbc → PARTIAL), another an anomaly
// run (drives fractional group anomaly rates).
func l5Members() []l5Member {
	var out []l5Member
	hosts := []struct {
		name, guid string
		teams      [2]string
	}{
		{"l5-a", guid(81), [2]string{"alpha", "alpha"}},
		{"l5-b", guid(82), [2]string{"beta", "gamma"}},
	}
	insts := []string{l5Context + "_one", l5Context + "_two"}
	dims := []string{"da", "db", "dc"}
	for hi, h := range hosts {
		for ii, inst := range insts {
			for di, dim := range dims {
				m := l5Member{
					Host: h.name, GUID: h.guid, Inst: inst, Team: h.teams[ii], Dim: dim,
					Base: 1000*hi + 100*ii + 10*di,
				}
				if h.name == "l5-a" && ii == 0 && dim == "da" {
					m.GapLo, m.GapHi = 21, 30
				}
				if h.name == "l5-b" && ii == 1 && dim == "dc" {
					m.AnomLo, m.AnomHi = 41, 50
				}
				out = append(out, m)
			}
		}
	}
	return out
}

// value/gap/anomalous of member m at 1-based sample i.
func (m l5Member) gap(i int) bool       { return m.GapLo > 0 && i >= m.GapLo && i <= m.GapHi }
func (m l5Member) anomalous(i int) bool { return m.AnomLo > 0 && i >= m.AnomLo && i <= m.AnomHi }
func (m l5Member) value(i int) float64  { return float64(m.Base + i%7) }

// pushLayer5 pushes the palette: one connection per host, two charts per
// host (same context) with the team label, three dims each.
func pushLayer5(t *testing.T) {
	t.Helper()
	members := l5Members()
	type instKey struct{ host, inst string }
	byInst := map[instKey][]l5Member{}
	for _, m := range members {
		k := instKey{m.Host, m.Inst}
		byInst[k] = append(byInst[k], m)
	}
	conns := map[string]*stream.Conn{}
	for _, m := range members {
		if conns[m.Host] == nil {
			conns[m.Host] = connect(t, m.Host, m.GUID, stream.CapsLive)
		}
	}
	// deterministic instance order per host
	keys := make([]instKey, 0, len(byInst))
	for k := range byInst {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(a, b int) bool {
		if keys[a].host != keys[b].host {
			return keys[a].host < keys[b].host
		}
		return keys[a].inst < keys[b].inst
	})
	for _, k := range keys {
		ms := byInst[k]
		conn := conns[k.host]
		ch := fixture.Chart{
			ID: k.inst, Title: "l5", Units: "units", Family: "fixture",
			Context: l5Context, UpdateEvery: 1,
			Labels: [][2]string{{"team", ms[0].Team}},
		}
		for _, m := range ms {
			d := fixture.Dimension{ID: m.Dim}
			for i := 1; i <= l5Rows; i++ {
				p := fixture.Point{T: fixture.T0 + int64(i), Collected: strconv.Itoa(int(m.value(i))), Flags: stream.FlagNotAnomalous}
				if m.gap(i) {
					p.Flags = stream.FlagEmpty
				} else if m.anomalous(i) {
					p.Flags = stream.FlagAnomalous
				}
				d.Points = append(d.Points, p)
			}
			ch.Dimensions = append(ch.Dimensions, d)
		}
		ch.Define(conn)
		ch.PushLive(conn)
		if err := conn.Flush(); err != nil {
			t.Fatal(err)
		}
	}
	for _, host := range []string{"l5-a", "l5-b"} {
		if _, err := td.WaitRetention(host, l5Context, fixture.T0+1, fixture.T0+l5Rows, 15*time.Second); err != nil {
			t.Fatalf("%s: %v", host, err)
		}
	}

	// Host-local retention can settle before the all-node context index sees
	// both children. Wait on the routing surface the group-by assertions use.
	wantNodes := map[string]bool{guid(81): true, guid(82): true}
	deadline := time.Now().Add(15 * time.Second)
	var seenNodes map[string]bool
	for {
		params := daemon.DataParams(l5Context, fixture.T0, fixture.T0+l5Rows, l5Rows)
		params.Set("group_by", "node")
		doc, err := td.DataV3All(params)
		seenNodes = map[string]bool{}
		if err == nil {
			if columns, err := canon.Columns(doc); err == nil {
				for node := range columns {
					seenNodes[node] = true
				}
			}
		}

		allSeen := true
		for machineGUID := range wantNodes {
			allSeen = allSeen && seenNodes[machineGUID]
		}
		if allSeen {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("all-node context index did not include both layer-5 children: have %v, want %v", seenNodes, wantNodes)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// l5GroupKey returns the group column name for a member under a group_by
// key (query-group-by-init.c naming: names comma-join the selected axes).
func l5GroupKey(groupBy string, m l5Member) string {
	switch groupBy {
	case "selected":
		return "selected"
	case "dimension":
		return m.Dim
	case "instance", "percentage-of-instance":
		return m.Inst + "@" + m.GUID
	case "node":
		// node groups are keyed by machine GUID (query-group-by-init.c
		// uses rrdhost->machine_guid for both the id and the name)
		return m.GUID
	case "label":
		return m.Team
	case "context":
		return l5Context
	case "units":
		return "units"
	}
	panic("unknown group_by " + groupBy)
}

// l5Expected computes the group-by oracle for one group's row i (1-based):
// aggregated value, anomaly rate, contribution count, partial flag. In raw
// (aggregatable) mode the finalize conversions are skipped: AVERAGE keeps
// the accumulated SUM (the cloud divides after merging) and the anomaly
// rate stays the accumulated member total (not the mean).
func l5Expected(agg string, group []l5Member, i int, raw bool) (val, arp float64, gbc int, partial, empty bool) {
	var sum, minV, maxV, ext, ar float64
	count := 0
	for _, m := range group {
		if m.gap(i) {
			continue
		}
		v := m.value(i)
		if count == 0 {
			minV, maxV, ext = v, v, v
		} else {
			minV = math.Min(minV, v)
			maxV = math.Max(maxV, v)
			if math.Abs(v) > math.Abs(ext) {
				ext = v
			}
		}
		sum += v
		if m.anomalous(i) {
			ar += 100
		}
		count++
	}
	if count == 0 {
		return 0, 0, 0, false, true
	}
	switch agg {
	case "avg", "average":
		if raw {
			val = sum
		} else {
			val = sum / float64(count)
		}
	case "sum":
		val = sum
	case "min":
		val = minV
	case "max":
		val = maxV
	case "extremes":
		val = ext
	}
	if !raw {
		ar /= float64(count)
	}
	return val, ar, count, count < len(group), false
}

// TestLayer5GroupByMatrix drives every single group_by key with every
// non-percentage aggregation over the multi-node palette, in BOTH
// contracts: non-raw (finalize converts) and raw (the cloud-facing
// aggregatable mode — sums stay undivided, anomaly rates accumulated,
// per-point contribution counts on the wire).
func TestLayer5GroupByMatrix(t *testing.T) {
	contracts := map[string]bool{
		"L5/group-by-grid":             true,
		"L5/group-by-naming":           true,
		"L5/group-by-values":           true,
		"L5/group-by-anomaly-metadata": true,
		"L5/group-by-partial-empty":    true,
		"L5/group-by-raw-schema":       true,
	}
	for contract := range contracts {
		registerContract(t, contract)
	}

	pushLayer5(t)
	members := l5Members()
	fail := func(contract, format string, args ...any) {
		t.Helper()
		t.Logf(format, args...)
		contracts[contract] = false
	}

	keys := []string{"selected", "dimension", "instance", "node", "label", "context", "units"}
	aggs := []string{"average", "min", "max", "sum", "extremes"}

	for _, raw := range []bool{false, true} {
		mode := "non-raw"
		if raw {
			mode = "raw"
		}
		for _, key := range keys {
			groups := map[string][]l5Member{}
			for _, m := range members {
				k := l5GroupKey(key, m)
				groups[k] = append(groups[k], m)
			}

			for _, agg := range aggs {
				label := mode + "/" + key + "/" + agg
				params := daemon.DataParams(l5Context, fixture.T0, fixture.T0+l5Rows, l5Rows)
				params.Set("group_by", key)
				if key == "label" {
					params.Set("group_by_label", "team")
				}
				params.Set("aggregation", agg)
				if raw {
					params.Set("options", "jsonwrap|raw")
				}
				doc, err := td.DataV3All(params)
				if err != nil {
					t.Fatal(err)
				}
				if err := queryPointSchemaField(doc, "hidden", false); err != nil {
					fail("L5/group-by-raw-schema", "%s: %v", label, err)
				}
				cols, err := canon.Columns(doc)
				if err != nil {
					t.Fatal(err)
				}

				if len(cols) != len(groups) {
					fail("L5/group-by-naming", "%s: got %d groups %v, want %d %v",
						label, len(cols), keys2(cols), len(groups), keys2(groups))
				}
				for gname, group := range groups {
					col, present := cols[gname]
					if !present {
						fail("L5/group-by-naming", "%s: group %q missing (have %v)", label, gname, keys2(cols))
						contracts["L5/group-by-grid"] = false
						contracts["L5/group-by-values"] = false
						contracts["L5/group-by-anomaly-metadata"] = false
						contracts["L5/group-by-partial-empty"] = false
						contracts["L5/group-by-raw-schema"] = false
						continue
					}
					if !l5ExactGrid(col) {
						fail("L5/group-by-grid", "%s: group %q does not contain the exact unique grid t0+1 through t0+%d",
							label, gname, l5Rows)
						contracts["L5/group-by-values"] = false
						contracts["L5/group-by-anomaly-metadata"] = false
						contracts["L5/group-by-partial-empty"] = false
						contracts["L5/group-by-raw-schema"] = false
						continue
					}
					for row, pt := range col {
						i := row + 1
						wantV, wantAR, wantGBC, wantPartial, wantEmpty := l5Expected(agg, group, i, raw)
						switch {
						case wantEmpty && pt.Value != nil:
							fail("L5/group-by-partial-empty", "%s: %q row %d value %v, want null", label, gname, i, *pt.Value)
						case !wantEmpty && pt.Value == nil:
							fail("L5/group-by-partial-empty", "%s: %q row %d is null, want numeric", label, gname, i)
							fail("L5/group-by-values", "%s: %q row %d is null, want %v", label, gname, i, wantV)
						case !wantEmpty && !tierValueMatch(*pt.Value, wantV, 0):
							fail("L5/group-by-values", "%s: %q row %d value %v, want %v", label, gname, i, *pt.Value, wantV)
						}
						if !tierValueMatch(pt.ARP, wantAR, 0) {
							fail("L5/group-by-anomaly-metadata", "%s: %q row %d arp %v, want %v", label, gname, i, pt.ARP, wantAR)
						}
						gotPartial := pt.PA&canon.AnnotationPartial != 0
						if gotPartial != wantPartial {
							fail("L5/group-by-partial-empty", "%s: %q row %d partial %v, want %v (pa %d)",
								label, gname, i, gotPartial, wantPartial, pt.PA)
						}
						if raw {
							switch {
							case pt.Count == nil:
								fail("L5/group-by-raw-schema", "%s: %q row %d raw response carries no count", label, gname, i)
							case !wantEmpty && *pt.Count != int64(wantGBC):
								fail("L5/group-by-raw-schema", "%s: %q row %d count %d, want %d", label, gname, i, *pt.Count, wantGBC)
							}
						} else if pt.Count != nil {
							fail("L5/group-by-raw-schema", "%s: %q row %d non-raw response carries a count (%d)",
								label, gname, i, *pt.Count)
						}
					}
				}
			}
		}
	}

	for _, contract := range []string{
		"L5/group-by-grid", "L5/group-by-naming", "L5/group-by-values",
		"L5/group-by-anomaly-metadata", "L5/group-by-partial-empty", "L5/group-by-raw-schema",
	} {
		assertContract(t, contract, contracts[contract])
	}
}

// TestLayer5Percentage pins aggregation=percentage with a dimensions=da
// selector: the selected members are the numerator, the unselected (db,
// dc) become the hidden denominator routed to the SAME group key.
//
//   - non-raw: value = n*100/(n+h) per row (n NaN → 0, h NaN → 100,
//     total 0 → 0) — query-group-by-finalize.c
//     rrdr2rrdr_group_by_calculate_percentage_of_group;
//   - raw: the conversion is DEFERRED for the cloud — value stays the
//     selected SUM, the hidden accumulator rides the wire per point;
//   - group_by=dimension is DEGENERATE by construction: hidden dims group
//     separately and are filtered, so the selected column reads flat 100%.
func TestLayer5Percentage(t *testing.T) {
	contracts := map[string]bool{
		"L5/percentage-grid":               true,
		"L5/percentage-nonraw":             true,
		"L5/percentage-raw-hidden":         true,
		"L5/percentage-group-by-dimension": true,
		"L5/percentage-of-instance":        true,
	}
	for contract := range contracts {
		registerContract(t, contract)
	}

	members := l5Members()
	if _, err := td.WaitRetention("l5-a", l5Context, fixture.T0+1, fixture.T0+l5Rows, 15*time.Second); err != nil {
		t.Skip("layer-5 palette not available (TestLayer5GroupByMatrix failed?)")
	}
	fail := func(contract, format string, args ...any) {
		t.Helper()
		t.Logf(format, args...)
		contracts[contract] = false
	}

	// percentage-of-instance is the exclusive single-key shorthand for the
	// same machinery (query-group-by.c drops all other groupings for it) —
	// it must behave exactly like instance + aggregation=percentage
	keys := []string{"selected", "node", "instance", "dimension", "percentage-of-instance"}
	for _, raw := range []bool{false, true} {
		mode := "non-raw"
		if raw {
			mode = "raw"
		}
		for _, key := range keys {
			// selected (numerator) groups and their hidden complements
			selGroups := map[string][]l5Member{}
			hidGroups := map[string][]l5Member{}
			for _, m := range members {
				k := l5GroupKey(key, m)
				if m.Dim == "da" {
					selGroups[k] = append(selGroups[k], m)
				} else if key != "dimension" {
					// under group_by=dimension the hidden dims form their
					// own (filtered) groups — nothing maps to "da"
					hidGroups[k] = append(hidGroups[k], m)
				}
			}

			contract := "L5/percentage-nonraw"
			switch {
			case key == "dimension":
				contract = "L5/percentage-group-by-dimension"
			case key == "percentage-of-instance":
				contract = "L5/percentage-of-instance"
			case raw:
				contract = "L5/percentage-raw-hidden"
			}
			label := mode + "/" + key
			params := daemon.DataParams(l5Context, fixture.T0, fixture.T0+l5Rows, l5Rows)
			params.Set("group_by", key)
			params.Set("dimensions", "da")
			params.Set("aggregation", "percentage")
			if raw {
				params.Set("options", "jsonwrap|raw")
			}
			doc, err := td.DataV3All(params)
			if err != nil {
				t.Fatal(err)
			}
			wantHiddenSchema := raw && key != "percentage-of-instance"
			if err := queryPointSchemaField(doc, "hidden", wantHiddenSchema); err != nil {
				fail(contract, "%s: %v", label, err)
			}
			cols, err := canon.Columns(doc)
			if err != nil {
				t.Fatal(err)
			}
			if len(cols) != len(selGroups) {
				fail(contract, "%s: got %d groups %v, want %d %v",
					label, len(cols), keys2(cols), len(selGroups), keys2(selGroups))
				fail("L5/percentage-grid", "%s: group set does not match the expected result grid", label)
			}

			for gname, sel := range selGroups {
				col, present := cols[gname]
				if !present {
					fail(contract, "%s: group %q missing (have %v)", label, gname, keys2(cols))
					fail("L5/percentage-grid", "%s: group %q is missing from the result grid", label, gname)
					continue
				}
				if !l5ExactGrid(col) {
					fail("L5/percentage-grid", "%s: %q does not contain the exact unique grid t0+1 through t0+%d",
						label, gname, l5Rows)
					contracts[contract] = false
					continue
				}
				for row, pt := range col {
					i := row + 1
					var n, h float64
					nCount, hCount := 0, 0
					for _, m := range sel {
						if !m.gap(i) {
							n += m.value(i)
							nCount++
						}
					}
					for _, m := range hidGroups[gname] {
						if !m.gap(i) {
							h += m.value(i)
							hCount++
						}
					}

					if raw && key != "percentage-of-instance" {
						// deferred: value = selected sum, hidden on the wire
						switch {
						case nCount == 0:
							if pt.Value != nil {
								fail(contract, "%s: %q row %d value %v, want null", label, gname, i, *pt.Value)
							}
						case pt.Value == nil:
							fail(contract, "%s: %q row %d raw value null, want selected sum %v", label, gname, i, n)
						case !tierValueMatch(*pt.Value, n, 0):
							fail(contract, "%s: %q row %d raw value %v, want selected sum %v", label, gname, i, *pt.Value, n)
						}
						if hCount > 0 {
							if pt.Hidden == nil || !tierValueMatch(*pt.Hidden, h, 0) {
								fail(contract, "%s: %q row %d raw hidden %v, want %v", label, gname, i, pt.Hidden, h)
							}
						} else if pt.Hidden != nil {
							fail(contract, "%s: %q row %d raw hidden %v, want null without a hidden contributor",
								label, gname, i, *pt.Hidden)
						}
						continue
					}
					// percentage-of-instance converts EVEN IN RAW MODE
					// (no hidden on the wire): per-instance groups never
					// span agents, so the cloud merge is a passthrough
					// and early conversion is safe — pinned contract
					if raw && key == "percentage-of-instance" && pt.Hidden != nil {
						fail(contract, "%s: %q row %d raw percentage-of-instance carries hidden %v, expected none",
							label, gname, i, *pt.Hidden)
					}

					var want float64
					switch {
					case nCount == 0:
						want = 0.0
					case hCount == 0:
						want = 100.0
					case n+h != 0:
						want = n * 100.0 / (n + h)
					}
					if nCount == 0 {
						// no selected contributions: the point stays EMPTY
						if pt.Value != nil {
							fail(contract, "%s: %q row %d value %v, want null", label, gname, i, *pt.Value)
						}
						continue
					}
					if pt.Value == nil || !tierValueMatch(*pt.Value, want, 1e-9) {
						fail(contract, "%s: %q row %d value %v, want %v (n=%v h=%v)", label, gname, i, pt.Value, want, n, h)
					}
				}
			}
		}
	}

	for _, contract := range []string{
		"L5/percentage-grid", "L5/percentage-nonraw", "L5/percentage-raw-hidden",
		"L5/percentage-group-by-dimension", "L5/percentage-of-instance",
	} {
		assertContract(t, contract, contracts[contract])
	}
}

func TestStrictDimensionStatsGuards(t *testing.T) {
	valid := func() map[string]any {
		return map[string]any{"view": map[string]any{"dimensions": map[string]any{
			"ids": []any{"a", "b"},
			"sts": map[string]any{
				"sum": []any{float64(1), float64(2)},
				"cnt": []any{float64(3), float64(4)},
			},
		}}}
	}
	if stats, ok := strictDimensionStats(t, valid(), "view", []string{"a", "b"}, []string{"sum", "cnt"}); !ok ||
		stats["a"]["sum"] != 1 || stats["b"]["cnt"] != 4 {
		t.Fatal("strict dimension statistics rejected the valid control")
	}

	mutations := map[string]func(map[string]any){
		"missing-required": func(doc map[string]any) {
			sts := doc["view"].(map[string]any)["dimensions"].(map[string]any)["sts"].(map[string]any)
			delete(sts, "cnt")
		},
		"short-array": func(doc map[string]any) {
			sts := doc["view"].(map[string]any)["dimensions"].(map[string]any)["sts"].(map[string]any)
			sts["sum"] = []any{float64(1)}
		},
		"duplicate-id": func(doc map[string]any) {
			dims := doc["view"].(map[string]any)["dimensions"].(map[string]any)
			dims["ids"] = []any{"a", "a"}
		},
		"non-numeric": func(doc map[string]any) {
			sts := doc["view"].(map[string]any)["dimensions"].(map[string]any)["sts"].(map[string]any)
			sts["cnt"] = []any{float64(3), "four"}
		},
		"non-finite": func(doc map[string]any) {
			sts := doc["view"].(map[string]any)["dimensions"].(map[string]any)["sts"].(map[string]any)
			sts["cnt"] = []any{float64(3), math.Inf(1)}
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			doc := valid()
			mutate(doc)
			if _, ok := strictDimensionStats(t, doc, "view", []string{"a", "b"}, []string{"sum", "cnt"}); ok {
				t.Errorf("strict dimension statistics accepted the %s mutation", name)
			}
		})
	}
}

// strictDimensionStats validates one jsonwrap-v2 dimensions statistics
// section and returns every advertised array keyed by its unique id.
func strictDimensionStats(
	t *testing.T,
	doc map[string]any,
	section string,
	wantIDs, requiredFields []string,
) (map[string]map[string]float64, bool) {
	t.Helper()

	parent, ok := doc[section].(map[string]any)
	if !ok {
		t.Logf("response has no %s object", section)
		return nil, false
	}
	dimensions, ok := parent["dimensions"].(map[string]any)
	if !ok {
		t.Logf("%s has no dimensions object", section)
		return nil, false
	}
	idsAny, ok := dimensions["ids"].([]any)
	if !ok {
		t.Logf("%s.dimensions.ids is missing or malformed: %v", section, dimensions["ids"])
		return nil, false
	}
	sts, ok := dimensions["sts"].(map[string]any)
	if !ok {
		t.Logf("%s.dimensions.sts is missing or malformed: %v", section, dimensions["sts"])
		return nil, false
	}

	valid := true
	ids := make([]string, len(idsAny))
	seenIDs := make(map[string]struct{}, len(idsAny))
	for i, idAny := range idsAny {
		id, isString := idAny.(string)
		if !isString || id == "" {
			t.Logf("%s.dimensions.ids[%d] is not a nonempty string: %v", section, i, idAny)
			valid = false
			continue
		}
		if _, duplicate := seenIDs[id]; duplicate {
			t.Logf("%s.dimensions.ids repeats %q", section, id)
			valid = false
		}
		seenIDs[id] = struct{}{}
		ids[i] = id
	}

	wanted := make(map[string]struct{}, len(wantIDs))
	for _, id := range wantIDs {
		if id == "" {
			t.Fatal("strict dimension statistics expected a nonempty id")
		}
		if _, duplicate := wanted[id]; duplicate {
			t.Fatalf("strict dimension statistics expected duplicate id %q", id)
		}
		wanted[id] = struct{}{}
	}
	if len(seenIDs) != len(wanted) {
		t.Logf("%s.dimensions.ids has %v, want exactly %v", section, ids, wantIDs)
		valid = false
	}
	for id := range wanted {
		if _, has := seenIDs[id]; !has {
			t.Logf("%s.dimensions.ids is missing %q", section, id)
			valid = false
		}
	}
	for id := range seenIDs {
		if _, expected := wanted[id]; !expected {
			t.Logf("%s.dimensions.ids has unexpected id %q", section, id)
			valid = false
		}
	}

	required := make(map[string]struct{}, len(requiredFields))
	for _, field := range requiredFields {
		if field == "" {
			t.Fatal("strict dimension statistics requires a nonempty field")
		}
		if _, duplicate := required[field]; duplicate {
			t.Fatalf("strict dimension statistics repeats required field %q", field)
		}
		required[field] = struct{}{}
		if _, has := sts[field]; !has {
			t.Logf("%s.dimensions.sts is missing required field %q", section, field)
			valid = false
		}
	}

	out := make(map[string]map[string]float64, len(ids))
	for _, id := range ids {
		if id != "" {
			out[id] = make(map[string]float64, len(sts))
		}
	}
	for field, valuesAny := range sts {
		values, isArray := valuesAny.([]any)
		if !isArray || len(values) != len(ids) {
			t.Logf("%s.dimensions.sts.%s is %v, want exactly %d values",
				section, field, valuesAny, len(ids))
			valid = false
			continue
		}
		for i, valueAny := range values {
			value, isNumber := valueAny.(float64)
			if !isNumber || math.IsNaN(value) || math.IsInf(value, 0) {
				t.Logf("%s.dimensions.sts.%s[%d] is not finite numeric: %v",
					section, field, i, valueAny)
				valid = false
				continue
			}
			if ids[i] != "" {
				out[ids[i]][field] = value
			}
		}
	}
	return out, valid
}

// TestLayer5Statistics pins the per-group view statistics (the D-B / SUM-sts
// question): for every aggregation EXCEPT average the sts pair averages
// over the view ROWS (mean plotted value, consistent with the row-extreme
// min/max); AVERAGE keeps the (pre-division sum, contribution) pair — a
// correct weighted mean. In raw mode the (sum, count) pair rides the wire
// untouched for the cloud.
func TestLayer5Statistics(t *testing.T) {
	contracts := map[string]bool{
		"L5/statistics-weighted-average": true,
		"L5/statistics-row-aggregations": true,
		"L5/statistics-raw-sum-count":    true,
	}
	for contract := range contracts {
		registerContract(t, contract)
	}

	members := l5Members()
	if _, err := td.WaitRetention("l5-a", l5Context, fixture.T0+1, fixture.T0+l5Rows, 15*time.Second); err != nil {
		t.Skip("layer-5 palette not available (TestLayer5GroupByMatrix failed?)")
	}
	fail := func(contract, format string, args ...any) {
		t.Helper()
		t.Logf(format, args...)
		contracts[contract] = false
	}

	groups := map[string][]l5Member{}
	for _, m := range members {
		groups[m.Dim] = append(groups[m.Dim], m)
	}

	for _, raw := range []bool{false, true} {
		mode := "non-raw"
		if raw {
			mode = "raw"
		}
		for _, agg := range []string{"average", "min", "max", "sum", "extremes"} {
			contract := "L5/statistics-row-aggregations"
			if raw {
				contract = "L5/statistics-raw-sum-count"
			} else if agg == "average" {
				contract = "L5/statistics-weighted-average"
			}
			label := mode + "/" + agg
			params := daemon.DataParams(l5Context, fixture.T0, fixture.T0+l5Rows, l5Rows)
			params.Set("group_by", "dimension")
			params.Set("aggregation", agg)
			if raw {
				params.Set("options", "jsonwrap|raw")
			}
			doc, err := td.DataV3All(params)
			if err != nil {
				t.Fatal(err)
			}
			var sts, extremaSTS map[string]map[string]float64
			switch {
			case raw:
				var statsOK bool
				sts, statsOK = strictDimensionStats(t, doc, "view", keys2(groups), []string{"sum", "cnt"})
				if !statsOK {
					fail(contract, "%s: raw view dimension statistics are malformed", label)
				}
			case agg == "average":
				var avgOK, extremaOK bool
				sts, avgOK = strictDimensionStats(t, doc, "view", keys2(groups), []string{"avg"})
				extremaSTS, extremaOK = strictDimensionStats(t, doc, "view", keys2(groups), []string{"min", "max"})
				if !avgOK {
					fail("L5/statistics-weighted-average", "%s: weighted-average statistics are malformed", label)
				}
				if !extremaOK {
					fail("L5/statistics-row-aggregations", "%s: row-extrema statistics are malformed", label)
				}
			default:
				var statsOK bool
				sts, statsOK = strictDimensionStats(t, doc, "view", keys2(groups), []string{"avg", "min", "max"})
				extremaSTS = sts
				if !statsOK {
					fail(contract, "%s: view dimension statistics are malformed", label)
				}
			}

			for gname, group := range groups {
				got, gotPresent := sts[gname]
				if !gotPresent {
					fail(contract, "%s: group %q missing from view sts (have %v)", label, gname, keys2(sts))
				}
				extremaGot, extremaPresent := extremaSTS[gname]
				if !raw && !extremaPresent {
					fail("L5/statistics-row-aggregations", "%s: group %q missing from row-extrema sts (have %v)",
						label, gname, keys2(extremaSTS))
				}

				// derive the expected sts from the per-row oracle;
				// the pre-division sum and the row extremes feed only
				// the non-raw assertions
				var rowSum, preDivSum, minV, maxV float64
				rows, contributions := 0, 0
				for i := 1; i <= l5Rows; i++ {
					v, _, gbc, _, empty := l5Expected(agg, group, i, raw)
					if empty {
						continue
					}
					rowSum += v
					if !raw {
						sumV, _, _, _, _ := l5Expected("sum", group, i, false)
						preDivSum += sumV
						if rows == 0 {
							minV, maxV = v, v
						} else {
							minV = math.Min(minV, v)
							maxV = math.Max(maxV, v)
						}
					}
					rows++
					contributions += gbc
				}

				if raw {
					if !gotPresent {
						continue
					}
					// raw keeps the accumulated (sum, count) pair —
					// for min/max/extremes the rows carry the
					// champion values, so their sum is the row sum too
					if !tierValueMatch(got["sum"], rowSum, 1e-9) {
						fail(contract, "%s: %q raw sts sum %v, want %v", label, gname, got["sum"], rowSum)
					}
					wantCnt := contributions
					if cnt := got["cnt"]; int(cnt) != wantCnt || cnt != float64(wantCnt) {
						fail(contract, "%s: %q raw sts count %v, want %d", label, gname, cnt, wantCnt)
					}
					continue
				}

				var wantAvg float64
				if agg == "average" {
					wantAvg = preDivSum / float64(contributions)
				} else {
					wantAvg = rowSum / float64(rows)
				}
				if gotPresent && !tierValueMatch(got["avg"], wantAvg, 1e-9) {
					fail(contract, "%s: %q sts avg %v, want %v", label, gname, got["avg"], wantAvg)
				}
				if extremaPresent && !tierValueMatch(extremaGot["min"], minV, 1e-9) {
					fail("L5/statistics-row-aggregations", "%s: %q sts min %v, want %v", label, gname, extremaGot["min"], minV)
				}
				if extremaPresent && !tierValueMatch(extremaGot["max"], maxV, 1e-9) {
					fail("L5/statistics-row-aggregations", "%s: %q sts max %v, want %v", label, gname, extremaGot["max"], maxV)
				}
			}
		}
	}

	for _, contract := range []string{
		"L5/statistics-weighted-average", "L5/statistics-row-aggregations", "L5/statistics-raw-sum-count",
	} {
		assertContract(t, contract, contracts[contract])
	}
}

func keys2[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
