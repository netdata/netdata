// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-019 — the v1 JSON-family formatters (json, jsonp, csvjsonarray,
// datatable) write dimension names RAW between quotes with no JSON
// escaping (json.c header loop: buffer_strcat of the name), so a name
// containing a double-quote produces INVALID JSON. Reachable with plain
// dimension names and — more commonly — through group_by=label, which
// promotes label VALUES to result names. The v2/v3 json2 path escapes
// properly (buffer_json), which is why the ladder's other layers never
// tripped it. Same family as #23115, which fixed a different emission
// site.
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
		Dimensions: []fixture.Dimension{{ID: `dim"quote`}},
	}
	for i := 1; i <= 5; i++ {
		ch.Dimensions[0].Points = append(ch.Dimensions[0].Points, fixture.Point{
			T: fixture.T0 + int64(i), Collected: strconv.Itoa(i), Flags: stream.FlagNotAnomalous,
		})
	}
	pushLiveBurst(t, "c019", guid(91), ch)
	if _, err := td.WaitRetention("c019", ch.Context, fixture.T0+1, fixture.T0+5, 15*time.Second); err != nil {
		t.Fatal(err)
	}

	invalid := 0
	for _, format := range []string{"json", "jsonp", "csvjsonarray", "datatable"} {
		params := map[string][]string{
			"chart":   {chart},
			"after":   {strconv.FormatInt(fixture.T0, 10)},
			"before":  {strconv.FormatInt(fixture.T0+5, 10)},
			"points":  {"5"},
			"group":   {"average"},
			"options": {"seconds"},
			"format":  {format},
		}
		body, err := td.DataV1Raw("c019", params)
		if err != nil {
			t.Fatal(err)
		}
		payload := body
		if format == "jsonp" {
			payload = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(body), "callback("), ");")
		}
		var v any
		if err := json.Unmarshal([]byte(payload), &v); err != nil {
			t.Logf("%s: invalid JSON with a double-quote in the dimension name: %v", format, err)
			invalid++
		}
	}

	expectAgentStatus(t, "CASE-019/v1-json-name-escaping", invalid == 0)
}
