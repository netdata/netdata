// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-022 — feature-pending red: the agent has no `latest` time-grouping
// (an unknown time_group silently falls back to average), so the target
// contract fails on stock. The contract under pin:
//   - time_group=latest is accepted and echoed back;
//   - each output bucket keeps the LAST collected value inside it;
//     buckets without any collected sample stay EMPTY (gaps visible);
//   - points=1 with before=0 anchors the window at the newest stored
//     sample (the now-1 clamp would race the end-stamped collector tick)
//     and serves it from the collector cache WITHOUT touching storage:
//     zero db reads, the RAW un-quantized double (2^24+1 stays 2^24+1),
//     and anomaly rate 0 by design — while the storage path returns the
//     SN-quantized value and the engine-generic bucket anomaly rate
//     (pinned via options=selected-tier, which disables the fast path).
package corpus

import (
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

func TestCase022TimeGroupLatest(t *testing.T) {
	const chart = "fixture.c022"
	const big = 16777217 // 2^24+1: NOT representable in storage_number

	ch := fixture.Chart{
		ID: chart, Title: "latest", Units: "units", Family: "fixture",
		Context: chart, UpdateEvery: 1,
		Dimensions: []fixture.Dimension{{ID: "plain"}, {ID: "big"}, {ID: "neg"}},
	}
	for i := 1; i <= 12; i++ {
		var p fixture.Point
		switch {
		case i <= 8:
			p = fixture.Point{T: fixture.T0 + int64(i), Collected: strconv.Itoa(i), Flags: stream.FlagNotAnomalous}
		case i <= 10: // the gap bucket
			p = fixture.Point{T: fixture.T0 + int64(i), Flags: stream.FlagEmpty}
		case i == 11:
			p = fixture.Point{T: fixture.T0 + int64(i), Collected: "9", Flags: stream.FlagNotAnomalous}
		default: // the newest sample is anomalous
			p = fixture.Point{T: fixture.T0 + int64(i), Collected: "10", Flags: stream.FlagAnomalous}
		}
		ch.Dimensions[0].Points = append(ch.Dimensions[0].Points, p)
		ch.Dimensions[1].Points = append(ch.Dimensions[1].Points, fixture.Point{
			T: fixture.T0 + int64(i), Collected: strconv.Itoa(big), Flags: stream.FlagNotAnomalous,
		})
		ch.Dimensions[2].Points = append(ch.Dimensions[2].Points, fixture.Point{
			T: fixture.T0 + int64(i), Collected: "-5", Flags: stream.FlagNotAnomalous,
		})
	}
	pushLiveBurst(t, "c022", guid(200), ch)
	if _, err := td.WaitRetention("c022", ch.Context, fixture.T0+1, fixture.T0+12, 15*time.Second); err != nil {
		t.Fatal(err)
	}

	get := func(extra map[string]string) map[string]any {
		t.Helper()
		params := map[string][]string{
			"scope_contexts": {chart},
			"time_group":     {"latest"},
			"format":         {"json2"},
			"group_by":       {"dimension"},
		}
		for k, v := range extra {
			params[k] = []string{v}
		}
		resp, err := td.HostJSON("c022", "api/v3/data", params)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	ok := true
	check := func(cond bool, what string, args ...any) {
		t.Helper()
		if !cond {
			t.Logf("latest contract not met: "+what, args...)
			ok = false
		}
	}

	dig := func(m map[string]any, path ...string) any {
		var cur any = m
		for _, k := range path {
			mm, is := cur.(map[string]any)
			if !is {
				return nil
			}
			cur = mm[k]
		}
		return cur
	}

	// row value of a dimension: data rows are [t, [v, ar, pa], ...];
	// dimension order follows the labels array
	rowVal := func(resp map[string]any, row, dim int) (float64, bool) {
		data, _ := dig(resp, "result", "data").([]any)
		if row >= len(data) {
			return 0, false
		}
		r, _ := data[row].([]any)
		if dim+1 >= len(r) {
			return 0, false
		}
		point, _ := r[dim+1].([]any)
		if len(point) < 1 {
			return 0, false
		}
		v, is := point[0].(float64)
		return v, is
	}
	rowAr := func(resp map[string]any, row, dim int) float64 {
		data, _ := dig(resp, "result", "data").([]any)
		r, _ := data[row].([]any)
		point, _ := r[dim+1].([]any)
		if len(point) < 2 {
			return -1
		}
		ar, _ := point[1].(float64)
		return ar
	}
	dimIndex := func(resp map[string]any, name string) int {
		labels, _ := dig(resp, "result", "labels").([]any)
		for i, l := range labels {
			if l == name {
				return i - 1 // labels[0] is "time"
			}
		}
		return -1
	}

	// ------------------------------------------------------------------
	// the name is accepted and echoed
	resp := get(map[string]string{
		"after":   strconv.FormatInt(fixture.T0, 10),
		"before":  strconv.FormatInt(fixture.T0+8, 10),
		"points":  "4",
		"options": "debug", // the request echo is emitted only with debug
	})
	echo := dig(resp, "request", "aggregations", "time", "time_group")
	check(echo == "latest", "time_group echo is %v, want latest", echo)

	// per-bucket semantics: buckets of 2 keep the LAST value of each pair
	pi := dimIndex(resp, "plain")
	check(pi >= 0, "plain dimension missing")
	if pi >= 0 {
		want := []float64{8, 6, 4, 2} // newest first (default order)
		for row, w := range want {
			v, is := rowVal(resp, row, pi)
			check(is && v == w, "bucket row %d of plain = %v (num=%v), want %v", row, v, is, w)
		}
	}

	// identity sweep: one bucket per sample - buckets covering the gap
	// samples stay EMPTY (null), every other bucket is its own sample
	respID := get(map[string]string{
		"after":  strconv.FormatInt(fixture.T0, 10),
		"before": strconv.FormatInt(fixture.T0+12, 10),
		"points": "12",
	})
	if pi := dimIndex(respID, "plain"); pi >= 0 {
		wantByT := map[int64]float64{ // absent keys must be null
			fixture.T0 + 1: 1, fixture.T0 + 2: 2, fixture.T0 + 3: 3, fixture.T0 + 4: 4,
			fixture.T0 + 5: 5, fixture.T0 + 6: 6, fixture.T0 + 7: 7, fixture.T0 + 8: 8,
			fixture.T0 + 11: 9, fixture.T0 + 12: 10,
		}
		data, _ := dig(respID, "result", "data").([]any)
		check(len(data) == 12, "identity sweep rows = %d, want 12", len(data))
		for row := range data {
			r, _ := data[row].([]any)
			ts, _ := r[0].(float64)
			v, is := rowVal(respID, row, pi)
			if w, has := wantByT[int64(ts)]; has {
				check(is && v == w, "identity row t=%d of plain = %v (num=%v), want %v", int64(ts), v, is, w)
			} else {
				check(!is, "gap row t=%d of plain = %v, want null", int64(ts), v)
			}
		}
	}

	// ------------------------------------------------------------------
	// the hot edge: points=1 before=0 anchors at the newest sample and
	// serves it from the collector cache - zero storage reads, the RAW
	// un-quantized value, anomaly rate 0
	respFast := get(map[string]string{
		"after":  strconv.FormatInt(fixture.T0, 10),
		"before": "0",
		"points": "1",
	})
	tier0points, _ := dig(respFast, "db", "per_tier").([]any)
	check(len(tier0points) > 0, "no db.per_tier in the response")
	if len(tier0points) > 0 {
		p0, _ := tier0points[0].(map[string]any)
		check(p0["points"] == float64(0), "fast path read %v tier0 points, want 0", p0["points"])
	}
	check(dig(respFast, "view", "before") == float64(fixture.T0+12),
		"hot-edge window.before = %v, want %d (the newest sample)", dig(respFast, "view", "before"), fixture.T0+12)
	if bi := dimIndex(respFast, "big"); bi >= 0 {
		v, is := rowVal(respFast, 0, bi)
		check(is && v == float64(big), "fast big = %v, want the RAW %d", v, big)
	}
	if pi := dimIndex(respFast, "plain"); pi >= 0 {
		v, is := rowVal(respFast, 0, pi)
		check(is && v == 10, "fast plain = %v, want 10", v)
		check(rowAr(respFast, 0, pi) == 0, "fast plain ar = %v, want 0 by design", rowAr(respFast, 0, pi))
	}
	if ni := dimIndex(respFast, "neg"); ni >= 0 {
		v, is := rowVal(respFast, 0, ni)
		check(is && v == -5, "fast neg = %v, want -5 (sign preserved without absolute)", v)
	}

	// options=absolute keeps the fast path AND erases the sign, exactly
	// like the storage path does at fetch
	respAbs := get(map[string]string{
		"after":   strconv.FormatInt(fixture.T0, 10),
		"before":  "0",
		"points":  "1",
		"options": "absolute",
	})
	absTier, _ := dig(respAbs, "db", "per_tier").([]any)
	if len(absTier) > 0 {
		p0, _ := absTier[0].(map[string]any)
		check(p0["points"] == float64(0), "absolute fast path read %v tier0 points, want 0", p0["points"])
	}
	if ni := dimIndex(respAbs, "neg"); ni >= 0 {
		v, is := rowVal(respAbs, 0, ni)
		check(is && v == 5, "absolute fast neg = %v, want 5", v)
	}

	// a relative before near now resolves to the hot edge the same way
	// (the rule compares the RESOLVED before against now - update_every)
	respRel := get(map[string]string{
		"after":  strconv.FormatInt(fixture.T0, 10),
		"before": "-1",
		"points": "1",
	})
	relTier, _ := dig(respRel, "db", "per_tier").([]any)
	if len(relTier) > 0 {
		p0, _ := relTier[0].(map[string]any)
		check(p0["points"] == float64(0), "relative-before fast path read %v tier0 points, want 0", p0["points"])
	}
	if bi := dimIndex(respRel, "big"); bi >= 0 {
		v, is := rowVal(respRel, 0, bi)
		check(is && v == float64(big), "relative-before fast big = %v, want the RAW %d", v, big)
	}

	// the storage path (selected-tier disables the fast path) returns the
	// SN-quantized value and the engine-generic bucket anomaly rate
	respSlow := get(map[string]string{
		"after":   strconv.FormatInt(fixture.T0, 10),
		"before":  strconv.FormatInt(fixture.T0+12, 10),
		"points":  "1",
		"options": "selected-tier",
		"tier":    "0",
	})
	if bi := dimIndex(respSlow, "big"); bi >= 0 {
		v, is := rowVal(respSlow, 0, bi)
		want := fixture.SNRoundTrip(float64(big))
		check(is && v == want, "slow big = %v, want the quantized %v", v, want)
	}
	if pi := dimIndex(respSlow, "plain"); pi >= 0 {
		check(rowAr(respSlow, 0, pi) > 0, "slow plain ar = %v, want > 0 (engine-generic)", rowAr(respSlow, 0, pi))
	}

	expectAgentStatus(t, "CASE-022/time-group-latest", ok)
}
