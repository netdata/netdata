// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
)

// TestCase046CardinalityDetailsAs: with options=details every grouped metric
// reports, as "as", the result dimension it is aggregated into. After a
// cardinality fold that is the reduced result: kept dimensions keep their own
// name and folded ones name the "remaining N dimensions" bucket. Without a
// limit, each metric names its own dimension (control).
func TestCase046CardinalityDetailsAs(t *testing.T) {
	const contract = "CASE-046/details-as-after-fold"
	for _, component := range []string{"no-limit", "limit-4"} {
		registerContractComponent(t, contract, component)
	}

	const host, context = "case046-card", "fixture.case046_card"
	ch := cardinalityChart(context, "cardinality details")
	pushLiveBurst(t, host, guid(13200), ch)
	if _, err := td.WaitRetention(host, context, fixture.T0+1, fixture.T0+20, 15*time.Second); err != nil {
		t.Fatal(err)
	}

	query := func(t *testing.T, limit int) map[string]string {
		t.Helper()
		params := daemon.DataParams(context, fixture.T0, fixture.T0+20, 20)
		params.Set("options", "jsonwrap,details")
		if limit > 0 {
			params.Set("cardinality_limit", strconv.Itoa(limit))
		}
		doc, err := td.DataV3(host, params)
		if err != nil {
			t.Fatal(err)
		}
		as, err := detailsGroupedAs(doc)
		if err != nil {
			t.Fatal(err)
		}
		return as
	}

	t.Run("no-limit", func(t *testing.T) {
		trackContractComponent(t, contract, "no-limit")
		want := map[string]string{}
		for k := 1; k <= cardinalityDims; k++ {
			want[fmt.Sprintf("d%d", k)] = fmt.Sprintf("d%d", k)
		}
		assertGroupedAs(t, query(t, 0), want)
	})

	t.Run("limit-4", func(t *testing.T) {
		trackContractComponent(t, contract, "limit-4")
		// contributions d6 > d5 > ... > d1: d6, d5, d4 kept; d1..d3 folded
		const folded = "remaining 3 dimensions"
		want := map[string]string{
			"d6": "d6", "d5": "d5", "d4": "d4",
			"d3": folded, "d2": folded, "d1": folded,
		}
		assertGroupedAs(t, query(t, 4), want)
	})
}

// detailsGroupedAs walks detailed.nodes.*.contexts.*.instances.*.dimensions.*
// and returns metric id -> its "as" value.
func detailsGroupedAs(doc map[string]any) (map[string]string, error) {
	obj := func(v any, what string) (map[string]any, error) {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s: not an object", what)
		}
		return m, nil
	}

	detailed, err := obj(doc["detailed"], "detailed")
	if err != nil {
		return nil, err
	}
	nodes, err := obj(detailed["nodes"], "detailed.nodes")
	if err != nil {
		return nil, err
	}

	out := map[string]string{}
	for nid, nv := range nodes {
		node, err := obj(nv, "node "+nid)
		if err != nil {
			return nil, err
		}
		contexts, err := obj(node["contexts"], "contexts of "+nid)
		if err != nil {
			return nil, err
		}
		for cid, cv := range contexts {
			ctx, err := obj(cv, "context "+cid)
			if err != nil {
				return nil, err
			}
			instances, err := obj(ctx["instances"], "instances of "+cid)
			if err != nil {
				return nil, err
			}
			for iid, iv := range instances {
				inst, err := obj(iv, "instance "+iid)
				if err != nil {
					return nil, err
				}
				dimensions, err := obj(inst["dimensions"], "dimensions of "+iid)
				if err != nil {
					return nil, err
				}
				for did, dv := range dimensions {
					dim, err := obj(dv, "dimension "+did)
					if err != nil {
						return nil, err
					}
					as, ok := dim["as"].(string)
					if !ok {
						return nil, fmt.Errorf("dimension %s: missing string \"as\"", did)
					}
					if _, dup := out[did]; dup {
						return nil, fmt.Errorf("dimension %s reported twice", did)
					}
					out[did] = as
				}
			}
		}
	}
	return out, nil
}

func assertGroupedAs(t *testing.T, got, want map[string]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("details reported %d metrics %q, want %d %q", len(got), got, len(want), want)
	}
	for id, w := range want {
		if g, ok := got[id]; !ok {
			t.Errorf("metric %s missing from details", id)
		} else if g != w {
			t.Errorf("metric %s: as=%q, want %q", id, g, w)
		}
	}
}
