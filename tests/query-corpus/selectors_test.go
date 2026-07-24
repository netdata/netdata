// SPDX-License-Identifier: GPL-3.0-or-later

// S8 slice 1 — the selector surface (query_target.c pattern matching):
// nodes/instances/dimensions/labels filters and their scope_
// counterparts, '!' negation, label key:value patterns, and the
// match-ids/match-names dimension matching modes — plus the silent
// fallbacks (unknown v1 format → json, unknown weights method → ks2)
// and extra cardinality_limit values.
//
// Selector cases run on the layer-5 palette (2 nodes x 2 instances x 3
// dims with team labels) and assert VALUES via the member-enumeration
// oracle, not just group presence. This file sorts after layer5/8 so
// the shared fixtures are already pushed.
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

// selGroups partitions the members that survive the filter under the
// given group_by key.
func selGroups(keep func(l5Member) bool, key string) map[string][]l5Member {
	out := map[string][]l5Member{}
	for _, m := range l5Members() {
		if !keep(m) {
			continue
		}
		id := l5GroupKey(key, m)
		out[id] = append(out[id], m)
	}
	return out
}

func TestSelectors(t *testing.T) {
	if _, err := td.WaitRetention("l5-a", l5Context, fixture.T0+1, fixture.T0+l5Rows, 15*time.Second); err != nil {
		t.Skip("layer-5 palette not available (TestLayer5GroupByMatrix failed?)")
	}

	cases := []struct {
		name     string
		set      func(p map[string][]string)
		key      string // group_by
		keep     func(l5Member) bool
	}{
		{"nodes-positive", func(p map[string][]string) { p["nodes"] = []string{"l5-a"} },
			"node", func(m l5Member) bool { return m.Host == "l5-a" }},
		{"nodes-negation", func(p map[string][]string) { p["nodes"] = []string{"!l5-a,*"} },
			"node", func(m l5Member) bool { return m.Host != "l5-a" }},
		{"instances-positive", func(p map[string][]string) { p["instances"] = []string{l5Context + "_one"} },
			"instance", func(m l5Member) bool { return strings.HasSuffix(m.Inst, "_one") }},
		{"scope-instances", func(p map[string][]string) { p["scope_instances"] = []string{l5Context + "_two"} },
			"instance", func(m l5Member) bool { return strings.HasSuffix(m.Inst, "_two") }},
		{"labels-key-value", func(p map[string][]string) { p["labels"] = []string{"team:alpha"} },
			"node", func(m l5Member) bool { return m.Team == "alpha" }},
		{"scope-labels", func(p map[string][]string) { p["scope_labels"] = []string{"team:gamma"} },
			"instance", func(m l5Member) bool { return m.Team == "gamma" }},
		{"dimensions-negation", func(p map[string][]string) { p["dimensions"] = []string{"!da,*"} },
			"dimension", func(m l5Member) bool { return m.Dim != "da" }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params := daemon.DataParams(l5Context, fixture.T0, fixture.T0+l5Rows, l5Rows)
			params.Set("group_by", tc.key)
			params.Set("aggregation", "sum")
			tc.set(params)
			assertGroups(t, params, selGroups(tc.keep, tc.key), "sum")
		})
	}
}

// TestSelectorsMatchModes: with dimension ids distinct from names, the
// default matches BOTH; match-ids restricts to ids, match-names to
// names (query_target.c:1287 — both default true when neither given).
func TestSelectorsMatchModes(t *testing.T) {
	const ctx = "fixture.selnamed"
	conn := connect(t, "sel-named", guid(166), stream.CapsLive)
	conn.DefineChart(stream.Chart{
		ID: ctx, Title: "named dims", Units: "units", Family: "fixture",
		Context: ctx, UpdateEvery: 1,
	})
	conn.DimensionNamed("d1", "alpha_name", "absolute", 1, 1)
	conn.DimensionNamed("d2", "beta_name", "absolute", 1, 1)
	for i := 1; i <= 10; i++ {
		conn.Begin2(ctx, 1, fixture.T0+int64(i))
		conn.Set2("d1", "10", stream.FlagNotAnomalous)
		conn.Set2("d2", "20", stream.FlagNotAnomalous)
		conn.End2()
	}
	if err := conn.Flush(); err != nil {
		t.Fatal(err)
	}
	if _, err := td.WaitRetention("sel-named", ctx, fixture.T0+1, fixture.T0+10, 15*time.Second); err != nil {
		t.Fatal(err)
	}

	query := func(selector, options string) map[string][]canon.Pt {
		t.Helper()
		params := daemon.DataParams(ctx, fixture.T0, fixture.T0+10, 10)
		params.Set("dimensions", selector)
		if options != "" {
			params.Set("options", "jsonwrap|"+options)
		}
		doc, err := td.DataV3("sel-named", params)
		if err != nil {
			t.Fatal(err)
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			// a no-match response carries a bare [time] labels row —
			// that IS the empty result
			if strings.Contains(err.Error(), "too short") {
				return map[string][]canon.Pt{}
			}
			t.Fatal(err)
		}
		return cols
	}

	sole := func(t *testing.T, cols map[string][]canon.Pt, wantValue float64) {
		t.Helper()
		if len(cols) != 1 {
			t.Fatalf("got %d dimensions %v, want 1", len(cols), keys2(cols))
		}
		for _, col := range cols {
			for _, pt := range col {
				if pt.Value == nil || *pt.Value != wantValue {
					t.Errorf("row t0%+d: %s, want %v", pt.T-fixture.T0, fmtPt(pt), wantValue)
				}
			}
		}
	}

	t.Run("default-matches-both", func(t *testing.T) {
		sole(t, query("alpha_name", ""), 10) // by name
		sole(t, query("d2", ""), 20)         // by id
	})
	t.Run("match-ids-only", func(t *testing.T) {
		sole(t, query("d1", "match-ids"), 10)
		if cols := query("alpha_name", "match-ids"); len(cols) != 0 {
			t.Errorf("match-ids matched a NAME: %v", keys2(cols))
		}
	})
	t.Run("match-names-only", func(t *testing.T) {
		sole(t, query("beta_name", "match-names"), 20)
		if cols := query("d2", "match-names"); len(cols) != 0 {
			t.Errorf("match-names matched an ID: %v", keys2(cols))
		}
	})
}

// TestFallbackPins: unknown names never error — the v1 format falls
// back to json, the weights method to ks2.
func TestFallbackPins(t *testing.T) {
	if _, err := td.WaitRetention("l7-fmt", l7Chart, fixture.T0+1, fixture.T0+6, 15*time.Second); err != nil {
		t.Skip("layer-7 fixture not available")
	}

	t.Run("unknown-format-is-json", func(t *testing.T) {
		p1 := l7Params("")
		p1["format"] = []string{"no-such-format"}
		p2 := l7Params("")
		p2["format"] = []string{"json"}
		a, err := td.DataV1Raw("l7-fmt", p1)
		if err != nil {
			t.Fatal(err)
		}
		b, err := td.DataV1Raw("l7-fmt", p2)
		if err != nil {
			t.Fatal(err)
		}
		if a != b {
			t.Errorf("unknown format did not fall back to json:\n%.200s\nvs\n%.200s", a, b)
		}
	})

	t.Run("unknown-weights-method-is-ks2", func(t *testing.T) {
		weightsSettle(t, "weights-ks2", guid(163), weightsKS2Fixture())
		p := weightsV1Params("no-such-method", wKS2Context, "null2zero", true)
		doc, err := td.HostJSON("weights-ks2", "api/v1/weights", p)
		if err != nil {
			t.Fatal(err)
		}
		if m, _ := doc["method"].(string); m != "ks2" {
			t.Errorf("unknown method resolved to %q, want the ks2 default", m)
		}
	})
}

// TestCardinalityLimitSweep: more limit values over the 6-dim fixture —
// limit 2 keeps the top dimension and folds five; limits at or above
// the dimension count fold nothing.
func TestCardinalityLimitSweep(t *testing.T) {
	const context = "fixture.l8card"
	if _, err := td.WaitRetention("l8-card", context, fixture.T0+1, fixture.T0+20, 15*time.Second); err != nil {
		t.Skip("cardinality fixture not available (TestLayer8CardinalityLimit failed?)")
	}

	query := func(limit int) map[string][]canon.Pt {
		t.Helper()
		params := daemon.DataParams(context, fixture.T0, fixture.T0+20, 20)
		params.Set("cardinality_limit", strconv.Itoa(limit))
		doc, err := td.DataV3("l8-card", params)
		if err != nil {
			t.Fatal(err)
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}
		return cols
	}

	t.Run("limit-2", func(t *testing.T) {
		cols := query(2)
		if len(cols) != 2 {
			t.Fatalf("got %d columns %v, want top-1 + the fold", len(cols), keys2(cols))
		}
		top, ok := cols["d6"]
		if !ok {
			t.Fatalf("d6 (largest |sum|) missing: %v", keys2(cols))
		}
		for _, pt := range top {
			if pt.Value == nil || *pt.Value != 6 {
				t.Errorf("d6 row: %s, want 6", fmtPt(pt))
			}
		}
		fold, ok := cols["remaining 5 dimensions"]
		if !ok {
			t.Fatalf("fold column missing: %v", keys2(cols))
		}
		for _, pt := range fold {
			if pt.Value == nil || *pt.Value != 1+2+3+4+5 {
				t.Errorf("fold row: %s, want 15", fmtPt(pt))
			}
		}
	})

	for _, limit := range []int{6, 7} {
		t.Run("limit-"+strconv.Itoa(limit), func(t *testing.T) {
			cols := query(limit)
			if len(cols) != 6 {
				t.Errorf("limit %d folded a 6-dim chart: %d columns %v", limit, len(cols), keys2(cols))
			}
			for id, col := range cols {
				if strings.HasPrefix(id, "remaining") {
					t.Errorf("unexpected fold column %q at limit %d", id, limit)
				}
				_ = col
			}
		})
	}
}
