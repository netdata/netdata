// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-022 — GREEN since #23257 (the feature PR): the `latest`
// time-grouping landed and this case is its end-to-end regression
// guard. Before it, the unknown name silently fell back to average.
// The contract under pin:
//   - time_group=latest is accepted and echoed back;
//   - each output bucket keeps the LAST collected value inside it;
//     buckets without any collected sample stay EMPTY (gaps visible);
//   - points=1 over an explicit window containing the newest stored sample
//     serves it from the collector cache WITHOUT touching storage while
//     keeping the public timestamp grid derived only from the request:
//     zero db reads, the RAW un-quantized double (2^24+1 stays 2^24+1),
//     and anomaly rate 0 by design — while the storage path returns the
//     SN-quantized value and the engine-generic bucket anomaly rate
//     (pinned via options=selected-tier, which disables the fast path);
//   - before=0 retains its established, explicit database-end meaning.
package corpus

import (
	"encoding/json"
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

	// A natural-points before=0 query can expose an off-grid database end.
	// Keep this fixture separate from the v2/v3 matrix: those APIs request
	// virtual points by default, while v1 can exercise the established natural
	// alert-shaped geometry directly.
	const databaseEndChart = "fixture.c022_database_end"
	databaseEnd := fixture.Chart{
		ID: databaseEndChart, Title: "latest database end", Units: "units", Family: "fixture",
		Context: databaseEndChart, UpdateEvery: 10,
		Dimensions: []fixture.Dimension{{ID: "value", Algorithm: "absolute"}},
	}
	for offset := int64(3); offset <= 63; offset += 10 {
		databaseEnd.Dimensions[0].Points = append(databaseEnd.Dimensions[0].Points, fixture.Point{
			T: fixture.T0 + offset, Collected: strconv.FormatInt(offset, 10), Flags: stream.FlagNotAnomalous,
		})
	}
	pushLiveBurst(t, "c022-database-end", guid(345), databaseEnd)
	if _, err := td.WaitRetention(
		"c022-database-end", databaseEnd.Context, fixture.T0+3, fixture.T0+63, 15*time.Second); err != nil {
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
	// One output point whose requested window contains the newest sample is
	// served from the collector cache: zero storage reads, the RAW
	// un-quantized value, anomaly rate 0. Its row stays on the requested
	// grid rather than moving to the source sample timestamp.
	fastAfter := int64(fixture.T0)
	fastBefore := int64(fixture.T0 + 20)
	fastGrid := queryExpectedVirtualGrid(t, fastAfter, fastBefore, 1, false)
	respFast := get(map[string]string{
		"after":   strconv.FormatInt(fastAfter, 10),
		"before":  strconv.FormatInt(fastBefore, 10),
		"points":  "1",
		"options": "unaligned",
	})
	check(assertTierPresence(t, respFast, []bool{false, false, false}),
		"fast path read storage points")
	check(queryTimestampGridExact(t, respFast, fastGrid),
		"collector-cache response changed the requested timestamp grid")
	colsFast := decode(respFast, "plain", "big", "neg")
	exact(colsFast, "big", []expectedColumnPoint{
		wantNumberAt(fastBefore, float64(big)),
	}, 0)
	exact(colsFast, "plain", []expectedColumnPoint{
		wantNumberWithARPAt(fastBefore, 10, 0),
	}, 0)
	exact(colsFast, "neg", []expectedColumnPoint{
		wantNumberAt(fastBefore, -5),
	}, 0)

	// options=absolute keeps the fast path AND erases the sign, exactly
	// like the storage path does at fetch
	respAbs := get(map[string]string{
		"after":   strconv.FormatInt(fastAfter, 10),
		"before":  strconv.FormatInt(fastBefore, 10),
		"points":  "1",
		"options": "absolute|unaligned",
	})
	check(assertTierPresence(t, respAbs, []bool{false, false, false}),
		"absolute fast path read storage points")
	colsAbs := decode(respAbs, "plain", "big", "neg")
	exact(colsAbs, "plain", []expectedColumnPoint{wantNumberAt(fastBefore, 10)}, 0)
	exact(colsAbs, "big", []expectedColumnPoint{wantNumberAt(fastBefore, float64(big))}, 0)
	exact(colsAbs, "neg", []expectedColumnPoint{wantNumberAt(fastBefore, 5)}, 0)

	// before=0 is the API's explicit database-end sentinel, not an absolute
	// timestamp. Preserve that existing contract for alert-style queries while
	// keeping explicit and relative windows independent of stored timestamps.
	respDatabaseEnd := get(map[string]string{
		"after":  strconv.FormatInt(fixture.T0, 10),
		"before": "0",
		"points": "1",
	})
	check(assertTierPresence(t, respDatabaseEnd, []bool{false, false, false}),
		"database-end sentinel fast path read storage points")
	check(queryTimestampGridExact(t, respDatabaseEnd, queryExpectedGrid{
		after: fixture.T0, before: fixture.T0 + 12, updateEvery: 13, rows: 1,
	}), "database-end sentinel changed its existing newest-sample grid")
	colsDatabaseEnd := decode(respDatabaseEnd, "plain", "big", "neg")
	exact(colsDatabaseEnd, "big", []expectedColumnPoint{
		wantNumberAt(fixture.T0+12, float64(big)),
	}, 0)
	exact(colsDatabaseEnd, "plain", []expectedColumnPoint{
		wantNumberWithARPAt(fixture.T0+12, 10, 0),
	}, 0)
	exact(colsDatabaseEnd, "neg", []expectedColumnPoint{
		wantNumberAt(fixture.T0+12, -5),
	}, 0)

	// The established sentinel restores the exact newest stored timestamp after
	// natural update-every rounding. Merely skipping the final alignment would
	// return T0+60 here and silently move alert-style results off T0+63.
	v1Body, err := td.DataV1Raw("c022-database-end", map[string][]string{
		"context": {databaseEndChart},
		"after":   {strconv.FormatInt(fixture.T0+3, 10)},
		"before":  {"0"},
		"points":  {"1"},
		"group":   {"latest"},
		"format":  {"json"},
		"options": {"jsonwrap|seconds|natural-points"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var v1 map[string]any
	if err := json.Unmarshal([]byte(v1Body), &v1); err != nil {
		t.Fatalf("parse v1 database-end response: %v (body %.300q)", err, v1Body)
	}
	exactInteger := func(value any, want int64) bool {
		got, integer := queryInteger(value)
		return integer && got == want
	}
	check(exactInteger(v1["after"], fixture.T0+13) &&
		exactInteger(v1["before"], fixture.T0+63) &&
		exactInteger(v1["view_update_every"], 60) &&
		exactInteger(v1["points"], 1),
		"natural database-end view = after %v before %v update_every %v points %v, want %d/%d/60/1",
		v1["after"], v1["before"], v1["view_update_every"], v1["points"], fixture.T0+13, fixture.T0+63)
	v1Result, v1ResultOK := v1["result"].(map[string]any)
	v1Data, v1DataOK := v1Result["data"].([]any)
	var v1Row []any
	v1RowOK := false
	if v1DataOK && len(v1Data) == 1 {
		v1Row, v1RowOK = v1Data[0].([]any)
	}
	check(v1ResultOK && v1DataOK && len(v1Data) == 1 && v1RowOK && len(v1Row) >= 1 &&
		exactInteger(v1Row[0], fixture.T0+63),
		"natural database-end data = %v, want one row timestamped %d", v1Result["data"], fixture.T0+63)

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
