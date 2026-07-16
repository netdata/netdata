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
	for _, m := range members {
		if _, err := td.WaitRetention(m.Host, l5Context, fixture.T0+1, fixture.T0+l5Rows, 15*time.Second); err != nil {
			t.Fatalf("%s: %v", m.Host, err)
		}
		break // context retention settles per host; check both below
	}
	if _, err := td.WaitRetention("l5-b", l5Context, fixture.T0+1, fixture.T0+l5Rows, 15*time.Second); err != nil {
		t.Fatal(err)
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
	case "instance":
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

// l5Expected computes the non-raw oracle for one group's row i (1-based):
// aggregated value, anomaly rate, partial flag, contribution count.
func l5Expected(agg string, group []l5Member, i int) (val, arp float64, partial, empty bool) {
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
		return 0, 0, false, true
	}
	switch agg {
	case "avg", "average":
		val = sum / float64(count)
	case "sum":
		val = sum
	case "min":
		val = minV
	case "max":
		val = maxV
	case "extremes":
		val = ext
	}
	return val, ar / float64(count), count < len(group), false
}

// TestLayer5GroupByMatrix drives every single group_by key with every
// non-percentage aggregation over the multi-node palette.
func TestLayer5GroupByMatrix(t *testing.T) {
	pushLayer5(t)
	members := l5Members()

	keys := []string{"selected", "dimension", "instance", "node", "label", "context", "units"}
	aggs := []string{"average", "min", "max", "sum", "extremes"}

	for _, key := range keys {
		groups := map[string][]l5Member{}
		for _, m := range members {
			k := l5GroupKey(key, m)
			groups[k] = append(groups[k], m)
		}

		for _, agg := range aggs {
			t.Run(key+"/"+agg, func(t *testing.T) {
				params := daemon.DataParams(l5Context, fixture.T0, fixture.T0+l5Rows, l5Rows)
				params.Set("group_by", key)
				if key == "label" {
					params.Set("group_by_label", "team")
				}
				params.Set("aggregation", agg)
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
						wantV, wantAR, wantPartial, wantEmpty := l5Expected(agg, group, i)
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
