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
	pushLayer5(t)
	members := l5Members()

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
				t.Run(mode+"/"+key+"/"+agg, func(t *testing.T) {
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
					cols, err := canon.Columns(doc)
					if err != nil {
						t.Fatal(err)
					}

					if len(cols) != len(groups) {
						t.Fatalf("got %d groups %v, want %d %v", len(cols), keys2(cols), len(groups), keys2(groups))
					}
					for gname, group := range groups {
						col, ok := cols[gname]
						if !ok {
							t.Errorf("group %q missing (have %v)", gname, keys2(cols))
							continue
						}
						if len(col) != l5Rows {
							t.Errorf("group %q: %d rows, want %d", gname, len(col), l5Rows)
							continue
						}
						for _, pt := range col {
							i := int(pt.T - fixture.T0)
							wantV, wantAR, wantGBC, wantPartial, wantEmpty := l5Expected(agg, group, i, raw)
							switch {
							case wantEmpty && pt.Value != nil:
								t.Errorf("%q row %d: value %v, want null", gname, i, *pt.Value)
							case !wantEmpty && pt.Value == nil:
								t.Errorf("%q row %d: null, want %v", gname, i, wantV)
							case !wantEmpty && !tierValueMatch(*pt.Value, wantV, 0):
								t.Errorf("%q row %d: value %v, want %v", gname, i, *pt.Value, wantV)
							}
							if !tierValueMatch(pt.ARP, wantAR, 0) {
								t.Errorf("%q row %d: arp %v, want %v", gname, i, pt.ARP, wantAR)
							}
							gotPartial := pt.PA&canon.AnnotationPartial != 0
							if gotPartial != wantPartial {
								t.Errorf("%q row %d: partial %v, want %v (pa %d)", gname, i, gotPartial, wantPartial, pt.PA)
							}
							if raw {
								switch {
								case pt.Count == nil:
									t.Errorf("%q row %d: raw response carries no count", gname, i)
								case !wantEmpty && *pt.Count != int64(wantGBC):
									t.Errorf("%q row %d: count %d, want %d", gname, i, *pt.Count, wantGBC)
								}
							} else if pt.Count != nil {
								t.Errorf("%q row %d: non-raw response carries a count (%d)", gname, i, *pt.Count)
							}
						}
					}
				})
			}
		}
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
	members := l5Members()
	if _, err := td.WaitRetention("l5-a", l5Context, fixture.T0+1, fixture.T0+l5Rows, 15*time.Second); err != nil {
		t.Skip("layer-5 palette not available (TestLayer5GroupByMatrix failed?)")
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

			t.Run(mode+"/"+key, func(t *testing.T) {
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
				cols, err := canon.Columns(doc)
				if err != nil {
					t.Fatal(err)
				}
				if len(cols) != len(selGroups) {
					t.Fatalf("got %d groups %v, want %d %v", len(cols), keys2(cols), len(selGroups), keys2(selGroups))
				}

				for gname, sel := range selGroups {
					col, ok := cols[gname]
					if !ok {
						t.Errorf("group %q missing (have %v)", gname, keys2(cols))
						continue
					}
					for _, pt := range col {
						i := int(pt.T - fixture.T0)
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
									t.Errorf("%q row %d: value %v, want null", gname, i, *pt.Value)
								}
							case pt.Value == nil:
								t.Errorf("%q row %d: raw value null, want selected sum %v", gname, i, n)
							case !tierValueMatch(*pt.Value, n, 0):
								t.Errorf("%q row %d: raw value %v, want selected sum %v", gname, i, *pt.Value, n)
							}
							if hCount > 0 {
								if pt.Hidden == nil || !tierValueMatch(*pt.Hidden, h, 0) {
									t.Errorf("%q row %d: raw hidden %v, want %v", gname, i, pt.Hidden, h)
								}
							}
							continue
						}
						// percentage-of-instance converts EVEN IN RAW MODE
						// (no hidden on the wire): per-instance groups never
						// span agents, so the cloud merge is a passthrough
						// and early conversion is safe — pinned contract
						if raw && key == "percentage-of-instance" && pt.Hidden != nil {
							t.Errorf("%q row %d: raw percentage-of-instance carries hidden %v, expected none", gname, i, *pt.Hidden)
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
								t.Errorf("%q row %d: value %v, want null", gname, i, *pt.Value)
							}
							continue
						}
						if pt.Value == nil || !tierValueMatch(*pt.Value, want, 1e-9) {
							t.Errorf("%q row %d: value %v, want %v (n=%v h=%v)", gname, i, pt.Value, want, n, h)
						}
					}
				}
			})
		}
	}
}

// viewSts decodes view.dimensions {ids, sts{min,max,avg,sum,cnt}} into
// per-group arrays keyed by id.
func viewSts(doc map[string]any) map[string]map[string]float64 {
	view, _ := doc["view"].(map[string]any)
	dims, _ := view["dimensions"].(map[string]any)
	idsAny, _ := dims["ids"].([]any)
	sts, _ := dims["sts"].(map[string]any)
	out := map[string]map[string]float64{}
	for idx, idAny := range idsAny {
		id, _ := idAny.(string)
		vals := map[string]float64{}
		for field, arrAny := range sts {
			arr, _ := arrAny.([]any)
			if idx < len(arr) {
				if v, ok := arr[idx].(float64); ok {
					vals[field] = v
				}
			}
		}
		out[id] = vals
	}
	return out
}

// TestLayer5Statistics pins the per-group view statistics (the D-B / SUM-sts
// question): for every aggregation EXCEPT average the sts pair averages
// over the view ROWS (mean plotted value, consistent with the row-extreme
// min/max); AVERAGE keeps the (pre-division sum, contribution) pair — a
// correct weighted mean. In raw mode the (sum, count) pair rides the wire
// untouched for the cloud.
func TestLayer5Statistics(t *testing.T) {
	members := l5Members()
	if _, err := td.WaitRetention("l5-a", l5Context, fixture.T0+1, fixture.T0+l5Rows, 15*time.Second); err != nil {
		t.Skip("layer-5 palette not available (TestLayer5GroupByMatrix failed?)")
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
			t.Run(mode+"/"+agg, func(t *testing.T) {
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
				sts := viewSts(doc)

				for gname, group := range groups {
					got, ok := sts[gname]
					if !ok {
						t.Errorf("group %q missing from view sts (have %v)", gname, keys2(sts))
						continue
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
						// raw keeps the accumulated (sum, count) pair —
						// for min/max/extremes the rows carry the
						// champion values, so their sum is the row sum too
						if !tierValueMatch(got["sum"], rowSum, 1e-9) {
							t.Errorf("%q: raw sts sum %v, want %v", gname, got["sum"], rowSum)
						}
						wantCnt := contributions
						if cnt, ok := got["cnt"]; ok && int(cnt) != wantCnt {
							t.Errorf("%q: raw sts count %v, want %d", gname, cnt, wantCnt)
						}
						continue
					}

					var wantAvg float64
					if agg == "average" {
						wantAvg = preDivSum / float64(contributions)
					} else {
						wantAvg = rowSum / float64(rows)
					}
					if !tierValueMatch(got["avg"], wantAvg, 1e-9) {
						t.Errorf("%q: sts avg %v, want %v (agg %s)", gname, got["avg"], wantAvg, agg)
					}
					if !tierValueMatch(got["min"], minV, 1e-9) {
						t.Errorf("%q: sts min %v, want %v", gname, got["min"], minV)
					}
					if !tierValueMatch(got["max"], maxV, 1e-9) {
						t.Errorf("%q: sts max %v, want %v", gname, got["max"], maxV)
					}
				}
			})
		}
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
