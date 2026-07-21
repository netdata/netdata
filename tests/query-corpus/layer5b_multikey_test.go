// SPDX-License-Identifier: GPL-3.0-or-later

// Layer 5b — multi-key group-by, the parser collapse rules and the
// aggregation-name fallbacks (query-group-by.c, query-group-by-init.c):
//
//   - group_by accepts multiple keys (OR-combined bitmask); groups are
//     the distinct attribute TUPLES; ids join the attributes with ","
//     in the FIXED engine order dimension, instance, label(s), node,
//     context, units (make_dimension_id) REGARDLESS of the order in
//     the request — group_by=node,dimension == group_by=dimension,node;
//   - when node is in the mask, the instance part uses the BARE
//     instance id (the @node suffix would duplicate the node column);
//   - selected combined with any other key collapses to selected alone
//     (group_by_parse); percentage-of-instance combined with any other
//     key collapses to percentage-of-instance alone;
//   - aggregation "avg" is an alias of "average", and an UNKNOWN
//     aggregation name silently falls back to average
//     (group_by_aggregate_function_parse) — pinned, like the
//     time-group fallback.
//
// Raw-mode multi-key behavior is layer 6 material (two-pass raw); this
// file pins the non-raw contract on the layer-5 palette.
package corpus

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
)

// l5MultiKeyID builds the engine's composite group id for member m:
// attributes join with "," in the fixed engine order; instance uses its
// bare id when node is in the mask.
func l5MultiKeyID(keys []string, m l5Member) string {
	has := map[string]bool{}
	for _, k := range keys {
		has[k] = true
	}
	var parts []string
	if has["dimension"] {
		parts = append(parts, m.Dim)
	}
	if has["instance"] {
		if has["node"] {
			parts = append(parts, m.Inst)
		} else {
			parts = append(parts, m.Inst+"@"+m.GUID)
		}
	}
	if has["label"] {
		parts = append(parts, m.Team)
	}
	if has["node"] {
		parts = append(parts, m.GUID)
	}
	if has["context"] {
		parts = append(parts, l5Context)
	}
	if has["units"] {
		parts = append(parts, "units")
	}
	return strings.Join(parts, ",")
}

// l5MultiKeyGroups partitions the palette by composite id.
func l5MultiKeyGroups(keys []string, members []l5Member) map[string][]l5Member {
	out := map[string][]l5Member{}
	for _, m := range members {
		id := l5MultiKeyID(keys, m)
		out[id] = append(out[id], m)
	}
	return out
}

// multiKeyParams builds a V3All query for the palette with the given
// group_by request string and aggregation.
func multiKeyParams(request, agg string) url.Values {
	params := daemon.DataParams(l5Context, fixture.T0, fixture.T0+l5Rows, l5Rows)
	params.Set("group_by", request)
	params.Set("aggregation", agg)
	if strings.Contains(request, "label") {
		params.Set("group_by_label", "team")
	}
	return params
}

// assertGroups fetches the query and asserts every composite group's
// points (value, anomaly rate, PARTIAL) against the member-enumeration
// oracle.
func assertGroups(t *testing.T, params url.Values, groups map[string][]l5Member, agg string) {
	t.Helper()
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
	for id, group := range groups {
		col, ok := cols[id]
		if !ok {
			t.Errorf("group %q missing (have %v)", id, keys2(cols))
			continue
		}
		if len(col) != l5Rows {
			t.Errorf("%q: got %d rows, want %d", id, len(col), l5Rows)
			continue
		}
		for _, pt := range col {
			i := int(pt.T - fixture.T0)
			want, wantAR, _, wantPartial, wantEmpty := l5Expected(agg, group, i, false)
			switch {
			case wantEmpty && pt.Value != nil:
				t.Errorf("%q row %d: value %v, want null", id, i, *pt.Value)
			case !wantEmpty && pt.Value == nil:
				t.Errorf("%q row %d: null, want %v", id, i, want)
			case !wantEmpty && !tierValueMatch(*pt.Value, want, 1e-9):
				t.Errorf("%q row %d: value %v, want %v", id, i, *pt.Value, want)
			}
			if !wantEmpty && !tierValueMatch(pt.ARP, wantAR, 1e-9) {
				t.Errorf("%q row %d: arp %v, want %v", id, i, pt.ARP, wantAR)
			}
			gotPartial := pt.PA&canon.AnnotationPartial != 0
			if gotPartial != wantPartial {
				t.Errorf("%q row %d: partial %v, want %v (pa %d)", id, i, gotPartial, wantPartial, pt.PA)
			}
		}
	}
}

// assertSameResponse pins response identity: both queries must return
// identical columns pointwise (value, arp, pa).
func assertSameResponse(t *testing.T, a, b url.Values) {
	t.Helper()
	docA, err := td.DataV3All(a)
	if err != nil {
		t.Fatal(err)
	}
	docB, err := td.DataV3All(b)
	if err != nil {
		t.Fatal(err)
	}
	colsA, err := canon.Columns(docA)
	if err != nil {
		t.Fatal(err)
	}
	colsB, err := canon.Columns(docB)
	if err != nil {
		t.Fatal(err)
	}
	if len(colsA) != len(colsB) {
		t.Fatalf("column sets differ: %v vs %v", keys2(colsA), keys2(colsB))
	}
	for id, colA := range colsA {
		colB, ok := colsB[id]
		if !ok {
			t.Errorf("column %q missing from second response (have %v)", id, keys2(colsB))
			continue
		}
		if len(colA) != len(colB) {
			t.Errorf("%q: %d vs %d rows", id, len(colA), len(colB))
			continue
		}
		for i := range colA {
			pa, pb := colA[i], colB[i]
			same := pa.T == pb.T && pa.ARP == pb.ARP && pa.PA == pb.PA &&
				(pa.Value == nil) == (pb.Value == nil) &&
				(pa.Value == nil || *pa.Value == *pb.Value)
			if !same {
				t.Errorf("%q row %d differs: %s vs %s", id, i, fmtPt(pa), fmtPt(pb))
			}
		}
	}
}

func TestLayer5MultiKeyGroupBy(t *testing.T) {
	members := l5Members()
	if _, err := td.WaitRetention("l5-a", l5Context, fixture.T0+1, fixture.T0+l5Rows, 15*time.Second); err != nil {
		t.Skip("layer-5 palette not available (TestLayer5GroupByMatrix failed?)")
	}

	// composite tuples against the member-enumeration oracle
	cases := []struct {
		request string   // as sent
		keys    []string // canonical key set for the oracle
		agg     string
	}{
		{"dimension,node", []string{"dimension", "node"}, "sum"},
		{"instance,node", []string{"instance", "node"}, "sum"}, // bare instance id
		{"label,node", []string{"label", "node"}, "average"},
		{"dimension,instance", []string{"dimension", "instance"}, "max"},
		{"dimension,units", []string{"dimension", "units"}, "sum"},
		{"node,context", []string{"node", "context"}, "sum"},
		// request order must not matter: same oracle as dimension,node
		{"node,dimension", []string{"dimension", "node"}, "sum"},
	}
	for _, tc := range cases {
		t.Run(tc.request+"-"+tc.agg, func(t *testing.T) {
			assertGroups(t, multiKeyParams(tc.request, tc.agg), l5MultiKeyGroups(tc.keys, members), tc.agg)
		})
	}

	// selected absorbs every other key
	t.Run("selected-collapse", func(t *testing.T) {
		assertSameResponse(t, multiKeyParams("selected,node", "sum"), multiKeyParams("selected", "sum"))
	})

	// percentage-of-instance is exclusive
	t.Run("percentage-of-instance-collapse", func(t *testing.T) {
		a := multiKeyParams("percentage-of-instance,node", "percentage")
		b := multiKeyParams("percentage-of-instance", "percentage")
		a.Set("dimensions", "da")
		b.Set("dimensions", "da")
		assertSameResponse(t, a, b)
	})

	// avg is an alias; unknown aggregation names silently parse to average
	t.Run("aggregation-avg-alias", func(t *testing.T) {
		assertSameResponse(t, multiKeyParams("instance", "avg"), multiKeyParams("instance", "average"))
	})
	t.Run("aggregation-unknown-fallback", func(t *testing.T) {
		assertSameResponse(t, multiKeyParams("instance", "no-such-aggregation"), multiKeyParams("instance", "average"))
	})
}
