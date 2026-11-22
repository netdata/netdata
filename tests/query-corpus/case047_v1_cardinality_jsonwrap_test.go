// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"math"
	"strconv"
	"testing"

	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

// TestCase047V1CardinalityJsonwrap: the v1 jsonwrap per-dimension arrays
// chart_ids, chart_labels[key] and latest_values are parallel to
// dimension_ids. After a cardinality fold every kept column carries its own
// metric's chart, label and last collected value, and the "remaining N
// dimensions" column, which has no single metric behind it, carries null.
// Without a limit each column carries its own metric (control).
//
// Each scope runs on its own daemon: on an engine with the over-read the
// folded query aborts an AddressSanitizer build, and that must not take the
// shared corpus daemon or the other scopes down with it.
func TestCase047V1CardinalityJsonwrap(t *testing.T) {
	const contract = "CASE-047/v1-jsonwrap-arrays-after-fold"
	for _, component := range []string{"no-limit", "limit-3", "limit-1"} {
		registerContractComponent(t, contract, component)
	}

	const host, context, labelKey = "case047-v1card", "fixture.case047_v1card", "case047_side"

	// two charts of one context, constant collected values: a metric's
	// contribution is its value x rows and its last collected value is its
	// value, so the fold order and every expected entry follow from here.
	// a3 ranks between b3 and b2, so the two kept columns of limit-3 come
	// from different charts and carry different chart ids and labels.
	type metric struct {
		chart, side string
		value       int
	}
	metrics := map[string]metric{
		"a1": {"case047.left", "left", 1},
		"a2": {"case047.left", "left", 2},
		"a3": {"case047.left", "left", 25},
		"b1": {"case047.right", "right", 10},
		"b2": {"case047.right", "right", 20},
		"b3": {"case047.right", "right", 30},
	}
	const rows = 20

	charts := make([]fixture.Chart, 0, 2)
	for _, side := range []struct{ chart, side string }{{"case047.left", "left"}, {"case047.right", "right"}} {
		ch := fixture.Chart{
			ID: side.chart, Title: "v1 cardinality", Units: "units", Family: "fixture",
			Context: context, UpdateEvery: 1,
			Labels: [][2]string{{labelKey, side.side}},
		}
		for _, id := range []string{"a1", "a2", "a3", "b1", "b2", "b3"} {
			m := metrics[id]
			if m.chart != side.chart {
				continue
			}
			d := fixture.Dimension{ID: id}
			for i := 1; i <= rows; i++ {
				d.Points = append(d.Points, fixture.Point{T: fixture.T0 + int64(i), Collected: strconv.Itoa(m.value), Flags: stream.FlagNotAnomalous})
			}
			ch.Dimensions = append(ch.Dimensions, d)
		}
		charts = append(charts, ch)
	}

	// start a daemon for one scope, push both charts on one connection
	// (kept open: latest_values is the last value the collector stored) and
	// settle each chart, since one chart alone satisfies context retention
	start := func(t *testing.T) *daemon.Daemon {
		t.Helper()
		dd := startDedicatedStorageDaemon(t, daemon.Options{})
		conn, err := stream.Connect(dd.Addr, dd.StreamKey, stream.HostInfo{
			Hostname: host, MachineGUID: guid(13300),
		}, stream.CapsLive)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		for _, ch := range charts {
			ch.Define(conn)
			ch.PushLive(conn)
		}
		if err := conn.Flush(); err != nil {
			t.Fatal(err)
		}
		waitChartsRetentionOn(t, dd, host, charts...)
		return dd
	}

	type column struct {
		chart, side any // string or nil
		latest      any // float64 or nil
	}
	query := func(t *testing.T, dd *daemon.Daemon, limit int) map[string]column {
		t.Helper()
		params := daemon.DataParams(context, fixture.T0, fixture.T0+rows, rows)
		for _, key := range []string{"scope_contexts", "time_group", "time_group_options", "group_by", "aggregation"} {
			params.Del(key)
		}
		params.Set("context", context)
		params.Set("group", "average")
		params.Set("format", "json")
		params.Set("options", "jsonwrap|seconds|unaligned")
		params.Set("chart_label_key", labelKey)
		if limit > 0 {
			params.Set("cardinality_limit", strconv.Itoa(limit))
		}
		doc, err := dd.HostJSON(host, "api/v1/data", params)
		if err != nil {
			t.Fatal(err)
		}

		arr := func(v any, what string) []any {
			a, ok := v.([]any)
			if !ok {
				t.Fatalf("%s: not an array: %v", what, v)
			}
			return a
		}
		ids := arr(doc["dimension_ids"], "dimension_ids")
		charts := arr(doc["chart_ids"], "chart_ids")
		latest := arr(doc["latest_values"], "latest_values")
		labels, ok := doc["chart_labels"].(map[string]any)
		if !ok {
			t.Fatalf("chart_labels: not an object: %v", doc["chart_labels"])
		}
		sides := arr(labels[labelKey], "chart_labels."+labelKey)
		if len(charts) != len(ids) || len(latest) != len(ids) || len(sides) != len(ids) {
			t.Fatalf("arrays not parallel: dimension_ids %d, chart_ids %d, chart_labels %d, latest_values %d",
				len(ids), len(charts), len(sides), len(latest))
		}

		out := map[string]column{}
		for i, v := range ids {
			id, ok := v.(string)
			if !ok {
				t.Fatalf("dimension_ids[%d] = %v, not a string", i, v)
			}
			if _, dup := out[id]; dup {
				t.Fatalf("dimension %s returned twice", id)
			}
			out[id] = column{chart: charts[i], side: sides[i], latest: latest[i]}
		}
		return out
	}

	own := func(id string) column {
		m := metrics[id]
		return column{chart: m.chart, side: m.side, latest: float64(m.value)}
	}
	assert := func(t *testing.T, got, want map[string]column) {
		t.Helper()
		if len(got) != len(want) {
			t.Errorf("got %d columns %v, want %d %v", len(got), got, len(want), want)
		}
		for id, w := range want {
			g, ok := got[id]
			if !ok {
				t.Errorf("column %s missing", id)
				continue
			}
			if g.chart != w.chart || g.side != w.side || !sameLatest(g.latest, w.latest) {
				t.Errorf("column %s: chart=%v side=%v latest=%v, want chart=%v side=%v latest=%v",
					id, g.chart, g.side, g.latest, w.chart, w.side, w.latest)
			}
		}
	}

	t.Run("no-limit", func(t *testing.T) {
		trackContractComponent(t, contract, "no-limit")
		want := map[string]column{}
		for id := range metrics {
			want[id] = own(id)
		}
		assert(t, query(t, start(t), 0), want)
	})

	t.Run("limit-3", func(t *testing.T) {
		trackContractComponent(t, contract, "limit-3")
		// contributions b3 > a3 > b2 > b1 > a2 > a1: b3 and a3 kept, four folded
		assert(t, query(t, start(t), 3), map[string]column{
			"b3":                     own("b3"),
			"a3":                     own("a3"),
			"remaining 4 dimensions": {},
		})
	})

	t.Run("limit-1", func(t *testing.T) {
		trackContractComponent(t, contract, "limit-1")
		// nothing kept: one column holding all six metrics
		assert(t, query(t, start(t), 1), map[string]column{
			"remaining 6 dimensions": {},
		})
	})
}

func sameLatest(got, want any) bool {
	if want == nil {
		return got == nil
	}
	g, ok := got.(float64)
	w, _ := want.(float64)
	return ok && math.Abs(g-w) < 1e-9
}
