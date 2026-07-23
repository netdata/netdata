// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-019 — FIXED by #23216. The v1 JSON-family formatters (json,
// jsonp, csvjsonarray, datatable) wrote dimension names RAW between
// quotes with no JSON escaping (json.c header loop), so a name
// containing a double-quote produced INVALID JSON. Reachable with plain
// dimension names and — more commonly — through group_by=label, which
// promotes label VALUES to result names. The v2/v3 json2 path always
// escaped properly (buffer_json), which is why the ladder's other layers
// never tripped it. Same family as #23115, which fixed a different
// emission site.
//
// The case also pins the two sibling emission sites the fix PR's review
// surfaced: the options=objectrows row keys (repeated the raw name as
// every row's object key) and the google visualization flavor
// (datatable + google_json), whose labels are single-quoted JavaScript
// strings — there the apostrophe was the breaking character, and a JSON
// escaper alone would not cover it (a double quote needs no escape
// between single quotes, and must stay raw).
package corpus

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

func TestCase019JsonNameEscaping(t *testing.T) {
	const chart = "fixture.c019"
	ch := fixture.Chart{
		ID: chart, Title: "escaping", Units: "units", Family: "fixture",
		Context: chart, UpdateEvery: 1,
		Dimensions: []fixture.Dimension{{ID: `dim"quote`}, {ID: `dim'apos`}},
	}
	for d := range ch.Dimensions {
		for i := 1; i <= 5; i++ {
			ch.Dimensions[d].Points = append(ch.Dimensions[d].Points, fixture.Point{
				T: fixture.T0 + int64(i), Collected: strconv.Itoa(i), Flags: stream.FlagNotAnomalous,
			})
		}
	}
	pushLiveBurst(t, "c019", guid(91), ch)
	if _, err := td.WaitRetention("c019", ch.Context, fixture.T0+1, fixture.T0+5, 15*time.Second); err != nil {
		t.Fatal(err)
	}

	c019 := func(format, options string) string {
		t.Helper()
		body, err := td.DataV1Raw("c019", map[string][]string{
			"chart":   {chart},
			"after":   {strconv.FormatInt(fixture.T0, 10)},
			"before":  {strconv.FormatInt(fixture.T0+5, 10)},
			"points":  {"5"},
			"group":   {"average"},
			"options": {options},
			"format":  {format},
		})
		if err != nil {
			t.Fatal(err)
		}
		return body
	}

	invalid := 0
	mustParse := func(format, options, payload string) {
		var v any
		if err := json.Unmarshal([]byte(payload), &v); err != nil {
			t.Logf("%s (options=%s): invalid JSON with a quote in a dimension name: %v", format, options, err)
			invalid++
		}
	}

	// the JSON shapes must parse — the double-quote name is the breaker
	for _, format := range []string{"json", "jsonp", "csvjsonarray", "datatable"} {
		payload := c019(format, "seconds")
		if format == "jsonp" {
			payload = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(payload), "callback("), ");")
		}
		mustParse(format, "seconds", payload)
	}

	// objectrows repeats the names as every row's object keys
	mustParse("json", "seconds,objectrows", c019("json", "seconds,objectrows"))

	// the google flavor emits single-quoted JavaScript labels: the
	// apostrophe name must ship escaped, the double-quote name raw
	// (a double-quote needs no escape between single quotes)
	gviz := c019("datatable", "seconds,google_json")
	if !strings.Contains(gviz, `dim\'apos`) {
		t.Logf("google datatable: apostrophe in a dimension name not escaped:\n%.300s", gviz)
		invalid++
	}
	if !strings.Contains(gviz, `dim"quote`) {
		t.Logf("google datatable: double-quote name over-escaped:\n%.300s", gviz)
		invalid++
	}

	expectAgentStatus(t, "CASE-019/v1-json-name-escaping", invalid == 0)
}
