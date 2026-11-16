// SPDX-License-Identifier: GPL-3.0-or-later

// The jsonwrap-v2 summary reports, per queried node, the time its metrics
// took to query (summary.nodes[].st.ms). Two hosts carry a context of their
// own, so the query has exactly two nodes and one of them is queried last.
// Wall-clock dependent, so the check is an envelope: present, positive, and
// within the request's total time.
package corpus

import (
	"math"
	"testing"

	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
)

func TestSummaryNodeQueryDuration(t *testing.T) {
	trackContract(t, "L5/summary-node-duration")

	ch := abFixture()
	ch.ID = "fixture.nodems"
	ch.Context = "fixture.nodems"
	hosts := map[string]string{"node-ms-a": guid(13101), "node-ms-b": guid(13102)}
	for host, machineGUID := range hosts {
		pushLiveBurst(t, host, machineGUID, ch)
	}
	// a node joins the query target once rrdcontext publishes its retention,
	// which lags the storage retention
	for host, machineGUID := range hosts {
		weightsSettle(t, host, machineGUID, ch)
	}
	doc, err := td.DataV3All(daemon.DataParams(ch.Context, fixture.T0, fixture.T0+60, 60))
	if err != nil {
		t.Fatal(err)
	}

	timings, ok := doc["timings"].(map[string]any)
	if !ok {
		t.Fatalf("response has no timings object")
	}
	total, ok := timings["total_ms"].(float64)
	if !ok || math.IsNaN(total) || math.IsInf(total, 0) || total <= 0 {
		t.Fatalf("timings.total_ms is missing or not a positive number: %v", timings["total_ms"])
	}

	summary, ok := doc["summary"].(map[string]any)
	if !ok {
		t.Fatalf("response has no summary object")
	}
	nodes, ok := summary["nodes"].([]any)
	if !ok || len(nodes) != len(hosts) {
		t.Fatalf("summary.nodes = %v, want exactly %d nodes", summary["nodes"], len(hosts))
	}

	sum := 0.0
	for i, n := range nodes {
		node, ok := n.(map[string]any)
		if !ok {
			t.Errorf("summary.nodes[%d] is not an object: %v", i, n)
			continue
		}
		st, ok := node["st"].(map[string]any)
		if !ok {
			t.Errorf("summary.nodes[%d] (%v) has no st object: %v", i, node["nm"], node["st"])
			continue
		}
		ms, ok := st["ms"].(float64)
		if !ok || math.IsNaN(ms) || math.IsInf(ms, 0) || ms <= 0 {
			t.Errorf("summary.nodes[%d] (%v) st.ms = %v, want a positive number", i, node["nm"], st["ms"])
			continue
		}
		sum += ms
	}
	if sum > total {
		t.Errorf("summary.nodes[].st.ms add up to %v, more than timings.total_ms %v", sum, total)
	}
}
