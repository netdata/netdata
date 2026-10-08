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

	get := func(t *testing.T, extra map[string]string) map[string]any {
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

	decode := func(t *testing.T, resp map[string]any, dimensions ...string) (map[string][]canon.Pt, bool) {
		t.Helper()
		cols, err := canon.Columns(resp)
		if err != nil {
			t.Fatal(err)
		}
		return cols, assertExactColumnSet(t, cols, dimensions)
	}
	exactInteger := func(value any, want int64) bool {
		got, integer := queryInteger(value)
		return integer && got == want
	}

	t.Run("name-echo", func(t *testing.T) {
		const contract = "CASE-022/latest-name-echo"
		trackContract(t, contract)
		resp := get(t, map[string]string{
			"after":   strconv.FormatInt(fixture.T0, 10),
			"before":  strconv.FormatInt(fixture.T0+8, 10),
			"points":  "4",
			"options": "debug",
		})
		request, requestOK := resp["request"].(map[string]any)
		aggregations, aggregationsOK := request["aggregations"].(map[string]any)
		timeAggregation, timeOK := aggregations["time"].(map[string]any)
		echo, echoOK := timeAggregation["time_group"].(string)
		if !requestOK || !aggregationsOK || !timeOK || !echoOK || echo != "latest" {
			t.Errorf("BROKEN %s: time_group echo is %v, want latest", contract, timeAggregation["time_group"])
		}
	})

	t.Run("bucket-values", func(t *testing.T) {
		const contract = "CASE-022/latest-bucket-values"
		trackContract(t, contract)
		resp := get(t, map[string]string{
			"after": strconv.FormatInt(fixture.T0, 10), "before": strconv.FormatInt(fixture.T0+8, 10), "points": "4",
		})
		cols, ok := decode(t, resp, "plain", "big", "neg")
		ok = assertExactColumn(t, cols, "plain", []expectedColumnPoint{
			wantNumberAt(fixture.T0+2, 2), wantNumberAt(fixture.T0+4, 4),
			wantNumberAt(fixture.T0+6, 6), wantNumberAt(fixture.T0+8, 8),
		}, 0) && ok
		ok = assertExactView(t, resp, fixture.T0, fixture.T0+8, 2) && ok
		assertContract(t, contract, ok)
	})

	t.Run("empty-buckets", func(t *testing.T) {
		const contract = "CASE-022/latest-empty-buckets"
		trackContract(t, contract)
		resp := get(t, map[string]string{
			"after": strconv.FormatInt(fixture.T0, 10), "before": strconv.FormatInt(fixture.T0+12, 10), "points": "12",
		})
		cols, ok := decode(t, resp, "plain", "big", "neg")
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
		ok = assertExactColumn(t, cols, "plain", plainWant, 0) && ok
		ok = assertExactColumn(t, cols, "big", bigWant, 0) && ok
		ok = assertExactColumn(t, cols, "neg", negWant, 0) && ok
		ok = assertExactView(t, resp, fixture.T0, fixture.T0+12, 1) && ok
		assertContract(t, contract, ok)
	})

	fastAfter := int64(fixture.T0)
	fastBefore := int64(fixture.T0 + 20)
	t.Run("collector-cache", func(t *testing.T) {
		const contract = "CASE-022/latest-collector-cache"
		trackContract(t, contract)
		resp := get(t, map[string]string{
			"after": strconv.FormatInt(fastAfter, 10), "before": strconv.FormatInt(fastBefore, 10),
			"points": "1", "options": "unaligned",
		})
		ok := assertTierPresence(t, resp, []bool{false, false, false})
		ok = queryTimestampGridExact(t, resp, queryExpectedVirtualGrid(t, fastAfter, fastBefore, 1, false)) && ok
		cols, columnsOK := decode(t, resp, "plain", "big", "neg")
		ok = columnsOK && ok
		ok = assertExactColumn(t, cols, "big", []expectedColumnPoint{wantNumberAt(fastBefore, float64(big))}, 0) && ok
		ok = assertExactColumn(t, cols, "plain", []expectedColumnPoint{wantNumberWithARPAt(fastBefore, 10, 0)}, 0) && ok
		ok = assertExactColumn(t, cols, "neg", []expectedColumnPoint{wantNumberAt(fastBefore, -5)}, 0) && ok
		assertContract(t, contract, ok)
	})

	t.Run("absolute", func(t *testing.T) {
		const contract = "CASE-022/latest-absolute"
		trackContract(t, contract)
		resp := get(t, map[string]string{
			"after": strconv.FormatInt(fastAfter, 10), "before": strconv.FormatInt(fastBefore, 10),
			"points": "1", "options": "absolute|unaligned",
		})
		ok := assertTierPresence(t, resp, []bool{false, false, false})
		cols, columnsOK := decode(t, resp, "plain", "big", "neg")
		ok = columnsOK && ok
		ok = assertExactColumn(t, cols, "plain", []expectedColumnPoint{wantNumberAt(fastBefore, 10)}, 0) && ok
		ok = assertExactColumn(t, cols, "big", []expectedColumnPoint{wantNumberAt(fastBefore, float64(big))}, 0) && ok
		ok = assertExactColumn(t, cols, "neg", []expectedColumnPoint{wantNumberAt(fastBefore, 5)}, 0) && ok
		assertContract(t, contract, ok)
	})

	t.Run("before-zero-v3", func(t *testing.T) {
		const contract = "CASE-022/latest-before-zero-v3"
		trackContract(t, contract)
		resp := get(t, map[string]string{
			"after": strconv.FormatInt(fixture.T0, 10), "before": "0", "points": "1",
		})
		ok := assertTierPresence(t, resp, []bool{false, false, false})
		ok = queryTimestampGridExact(t, resp, queryExpectedGrid{
			after: fixture.T0, before: fixture.T0 + 12, updateEvery: 13, rows: 1,
		}) && ok
		cols, columnsOK := decode(t, resp, "plain", "big", "neg")
		ok = columnsOK && ok
		ok = assertExactColumn(t, cols, "big", []expectedColumnPoint{wantNumberAt(fixture.T0+12, float64(big))}, 0) && ok
		ok = assertExactColumn(t, cols, "plain", []expectedColumnPoint{wantNumberWithARPAt(fixture.T0+12, 10, 0)}, 0) && ok
		ok = assertExactColumn(t, cols, "neg", []expectedColumnPoint{wantNumberAt(fixture.T0+12, -5)}, 0) && ok
		assertContract(t, contract, ok)
	})

	t.Run("before-zero-v1", func(t *testing.T) {
		const contract = "CASE-022/latest-before-zero-v1"
		trackContract(t, contract)
		body, err := td.DataV1Raw("c022-database-end", map[string][]string{
			"context": {databaseEndChart}, "after": {strconv.FormatInt(fixture.T0+3, 10)},
			"before": {"0"}, "points": {"1"}, "group": {"latest"}, "format": {"json"},
			"options": {"jsonwrap|seconds|natural-points"},
		})
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		if err := json.Unmarshal([]byte(body), &doc); err != nil {
			t.Fatalf("parse v1 database-end response: %v (body %.300q)", err, body)
		}
		ok := exactInteger(doc["after"], fixture.T0+13) && exactInteger(doc["before"], fixture.T0+63) &&
			exactInteger(doc["view_update_every"], 60) && exactInteger(doc["points"], 1)
		if !ok {
			t.Logf("natural database-end view = after %v before %v update_every %v points %v, want %d/%d/60/1",
				doc["after"], doc["before"], doc["view_update_every"], doc["points"], fixture.T0+13, fixture.T0+63)
		}
		result, resultOK := doc["result"].(map[string]any)
		data, dataOK := result["data"].([]any)
		var row []any
		rowOK := false
		if dataOK && len(data) == 1 {
			row, rowOK = data[0].([]any)
		}
		rowHeld := resultOK && dataOK && len(data) == 1 && rowOK && len(row) >= 1 && exactInteger(row[0], fixture.T0+63)
		if !rowHeld {
			t.Logf("natural database-end data = %v, want one row timestamped %d", result["data"], fixture.T0+63)
		}
		assertContract(t, contract, ok && rowHeld)
	})

	t.Run("selected-tier-storage", func(t *testing.T) {
		const contract = "CASE-022/latest-selected-tier-storage"
		trackContract(t, contract)
		resp := get(t, map[string]string{
			"after": strconv.FormatInt(fixture.T0, 10), "before": strconv.FormatInt(fixture.T0+12, 10),
			"points": "1", "options": "selected-tier", "tier": "0",
		})
		ok := assertSelectedTier(t, resp, 0)
		cols, columnsOK := decode(t, resp, "plain", "big", "neg")
		ok = columnsOK && ok
		slowEnd := int64((fixture.T0 + 12 + 11) / 12 * 12)
		ok = assertExactColumn(t, cols, "big", []expectedColumnPoint{
			wantNumberAt(slowEnd, fixture.SNRoundTrip(float64(big))),
		}, 0) && ok
		ok = assertExactColumn(t, cols, "neg", []expectedColumnPoint{wantNumberAt(slowEnd, -5)}, 0) && ok
		plain := cols["plain"]
		plainOK := len(plain) == 1 && plain[0].T == slowEnd && plain[0].Value != nil && *plain[0].Value == 10 &&
			plain[0].ARP > 0 && plain[0].PA&canon.AnnotationEmpty == 0
		if !plainOK {
			t.Logf("slow plain = %v, want one numeric value 10 at %d with positive engine-generic anomaly rate", plain, slowEnd)
		}
		assertContract(t, contract, ok && plainOK)
	})
}
