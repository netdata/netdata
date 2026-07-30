// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-022 — GREEN since #23257 (the feature PR): the `latest`
// time-grouping landed and this case is its end-to-end regression
// guard. Before it, the unknown name silently fell back to average.
// The contract under pin:
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

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

func TestCase022TimeGroupLatest(t *testing.T) {
	trackContract(t, "CASE-022/time-group-latest")

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

	const slowChart = "fixture.c022slow"
	slowBase := int64(fixture.T0) - int64(fixture.T0)%10
	slow := fixture.Series(slowChart, slowChart, slowBase, 12, 10,
		func(int) string { return strconv.Itoa(big) },
		func(i int) string {
			if i == 12 {
				return stream.FlagAnomalous
			}
			return stream.FlagNotAnomalous
		})
	pushLiveBurst(t, "c022-slow", guid(296), slow)
	if _, err := td.WaitRetention("c022-slow", slow.Context, slow.FirstT(), slow.LastT(), 15*time.Second); err != nil {
		t.Fatal(err)
	}

	getFor := func(host, context string, extra map[string]string) map[string]any {
		t.Helper()
		params := map[string][]string{
			"scope_contexts": {context},
			"time_group":     {"latest"},
			"format":         {"json2"},
			"group_by":       {"dimension"},
		}
		for k, v := range extra {
			params[k] = []string{v}
		}
		resp, err := td.HostJSON(host, "api/v3/data", params)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	get := func(extra map[string]string) map[string]any {
		t.Helper()
		return getFor("c022", chart, extra)
	}

	ok := true
	check := func(cond bool, what string, args ...any) {
		t.Helper()
		if !cond {
			t.Logf("latest contract not met: "+what, args...)
			ok = false
		}
	}

	decode := func(resp map[string]any, dimensions ...string) map[string][]canon.Pt {
		t.Helper()
		cols, err := canon.Columns(resp)
		if err != nil {
			t.Fatal(err)
		}
		if !assertExactColumnSet(t, cols, dimensions) {
			ok = false
		}
		return cols
	}
	exact := func(cols map[string][]canon.Pt, dimension string, want []expectedColumnPoint, tolerance float64) {
		t.Helper()
		if !assertExactColumn(t, cols, dimension, want, tolerance) {
			ok = false
		}
	}

	// ------------------------------------------------------------------
	// the name is accepted and echoed
	resp := get(map[string]string{
		"after":   strconv.FormatInt(fixture.T0, 10),
		"before":  strconv.FormatInt(fixture.T0+8, 10),
		"points":  "4",
		"options": "debug", // the request echo is emitted only with debug
	})
	request, requestOK := resp["request"].(map[string]any)
	aggregations, aggregationsOK := request["aggregations"].(map[string]any)
	timeAggregation, timeOK := aggregations["time"].(map[string]any)
	echo, echoOK := timeAggregation["time_group"].(string)
	check(requestOK && aggregationsOK && timeOK && echoOK && echo == "latest",
		"time_group echo is %v, want latest", timeAggregation["time_group"])

	// per-bucket semantics: buckets of 2 keep the LAST value of each pair
	cols := decode(resp, "plain", "big", "neg")
	exact(cols, "plain", []expectedColumnPoint{
		wantNumberAt(fixture.T0+2, 2),
		wantNumberAt(fixture.T0+4, 4),
		wantNumberAt(fixture.T0+6, 6),
		wantNumberAt(fixture.T0+8, 8),
	}, 0)
	check(assertExactView(t, resp, fixture.T0, fixture.T0+8, 2),
		"four-bucket response view is not the exact requested grid")

	// identity sweep: one bucket per sample - buckets covering the gap
	// samples stay EMPTY (null), every other bucket is its own sample
	respID := get(map[string]string{
		"after":  strconv.FormatInt(fixture.T0, 10),
		"before": strconv.FormatInt(fixture.T0+12, 10),
		"points": "12",
	})
	colsID := decode(respID, "plain", "big", "neg")
	plainWant := make([]expectedColumnPoint, 0, 12)
	bigWant := make([]expectedColumnPoint, 0, 12)
	negWant := make([]expectedColumnPoint, 0, 12)
	for i := 1; i <= 12; i++ {
		ts := fixture.T0 + int64(i)
		switch {
		case i <= 8:
			plainWant = append(plainWant, wantNumberAt(ts, float64(i)))
		case i <= 10:
			plainWant = append(plainWant, wantEmptyAt(ts))
		default:
			plainWant = append(plainWant, wantNumberAt(ts, float64(i-2)))
		}
		bigWant = append(bigWant, wantNumberAt(ts, fixture.SNRoundTrip(float64(big))))
		negWant = append(negWant, wantNumberAt(ts, -5))
	}
	exact(colsID, "plain", plainWant, 0)
	exact(colsID, "big", bigWant, 0)
	exact(colsID, "neg", negWant, 0)
	check(assertExactView(t, respID, fixture.T0, fixture.T0+12, 1),
		"identity response view is not the exact requested grid")

	// ------------------------------------------------------------------
	// the hot edge: points=1 before=0 anchors at the newest sample and
	// serves it from the collector cache - zero storage reads, the RAW
	// un-quantized value, anomaly rate 0
	respFast := get(map[string]string{
		"after":  strconv.FormatInt(fixture.T0, 10),
		"before": "0",
		"points": "1",
	})
	check(assertTierPresence(t, respFast, []bool{false, false, false}),
		"fast path read storage points")
	viewFast, viewFastOK := respFast["view"].(map[string]any)
	check(viewFastOK && viewFast["before"] == float64(fixture.T0+12),
		"hot-edge window.before = %v, want %d (the newest sample)", viewFast["before"], fixture.T0+12)
	colsFast := decode(respFast, "plain", "big", "neg")
	exact(colsFast, "big", []expectedColumnPoint{
		wantNumberAt(fixture.T0+12, float64(big)),
	}, 0)
	exact(colsFast, "plain", []expectedColumnPoint{
		wantNumberWithARPAt(fixture.T0+12, 10, 0),
	}, 0)
	exact(colsFast, "neg", []expectedColumnPoint{
		wantNumberAt(fixture.T0+12, -5),
	}, 0)

	// options=absolute keeps the fast path AND erases the sign, exactly
	// like the storage path does at fetch
	respAbs := get(map[string]string{
		"after":   strconv.FormatInt(fixture.T0, 10),
		"before":  "0",
		"points":  "1",
		"options": "absolute",
	})
	check(assertTierPresence(t, respAbs, []bool{false, false, false}),
		"absolute fast path read storage points")
	colsAbs := decode(respAbs, "plain", "big", "neg")
	exact(colsAbs, "plain", []expectedColumnPoint{wantNumberAt(fixture.T0+12, 10)}, 0)
	exact(colsAbs, "big", []expectedColumnPoint{wantNumberAt(fixture.T0+12, float64(big))}, 0)
	exact(colsAbs, "neg", []expectedColumnPoint{wantNumberAt(fixture.T0+12, 5)}, 0)

	// a relative before near now resolves to the hot edge the same way
	// (the rule compares the RESOLVED before against now - update_every)
	respRel := get(map[string]string{
		"after":  strconv.FormatInt(fixture.T0, 10),
		"before": "-1",
		"points": "1",
	})
	check(assertTierPresence(t, respRel, []bool{false, false, false}),
		"relative-before fast path read storage points")
	colsRel := decode(respRel, "plain", "big", "neg")
	exact(colsRel, "plain", []expectedColumnPoint{wantNumberAt(fixture.T0+12, 10)}, 0)
	exact(colsRel, "big", []expectedColumnPoint{wantNumberAt(fixture.T0+12, float64(big))}, 0)
	exact(colsRel, "neg", []expectedColumnPoint{wantNumberAt(fixture.T0+12, -5)}, 0)

	// The same hot-edge fast path with update_every=10 exercises a natural
	// query granularity greater than one. Its only row ends at the newest
	// sample. An internal-check build also requires the fast executor to
	// address that prefilled row, not the prior query-granularity boundary.
	respNaturalFast := getFor("c022-slow", slowChart, map[string]string{
		"after":   strconv.FormatInt(slowBase, 10),
		"before":  "0",
		"points":  "1",
		"options": "natural-points",
	})
	check(assertTierPresence(t, respNaturalFast, []bool{false, false, false}),
		"update_every=10 natural fast path read storage points")
	viewNaturalFast, viewNaturalFastOK := respNaturalFast["view"].(map[string]any)
	check(viewNaturalFastOK && viewNaturalFast["before"] == float64(slow.LastT()),
		"update_every=10 hot-edge window.before = %v, want %d",
		viewNaturalFast["before"], slow.LastT())
	colsNaturalFast := decode(respNaturalFast, slow.Dimensions[0].ID)
	exact(colsNaturalFast, slow.Dimensions[0].ID, []expectedColumnPoint{
		wantNumberWithMetadataAt(slow.LastT(), float64(big), 0, 0),
	}, 0)

	// the storage path (selected-tier disables the fast path) returns the
	// SN-quantized value and the engine-generic bucket anomaly rate
	respSlow := get(map[string]string{
		"after":   strconv.FormatInt(fixture.T0, 10),
		"before":  strconv.FormatInt(fixture.T0+12, 10),
		"points":  "1",
		"options": "selected-tier",
		"tier":    "0",
	})
	check(assertSelectedTier(t, respSlow, 0), "storage path did not select only tier 0")
	colsSlow := decode(respSlow, "plain", "big", "neg")
	// Without unaligned, the 12-second single bucket ends on the next
	// absolute 12-second boundary.
	slowEnd := int64((fixture.T0 + 12 + 11) / 12 * 12)
	exact(colsSlow, "big", []expectedColumnPoint{
		wantNumberAt(slowEnd, fixture.SNRoundTrip(float64(big))),
	}, 0)
	exact(colsSlow, "neg", []expectedColumnPoint{wantNumberAt(slowEnd, -5)}, 0)
	plainSlow := colsSlow["plain"]
	check(len(plainSlow) == 1 && plainSlow[0].T == slowEnd &&
		plainSlow[0].Value != nil && *plainSlow[0].Value == 10 &&
		plainSlow[0].ARP > 0 && plainSlow[0].PA&canon.AnnotationEmpty == 0,
		"slow plain = %v, want one numeric value 10 at %d with positive engine-generic anomaly rate",
		plainSlow, slowEnd)

	assertContract(t, "CASE-022/time-group-latest", ok)
}
