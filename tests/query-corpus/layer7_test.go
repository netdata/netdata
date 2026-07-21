// SPDX-License-Identifier: GPL-3.0-or-later

// Layer 7 — formatters: every classic datasource format over one small
// formatter-hostile fixture (fractional and negative values, a gap run,
// a dimension name carrying a comma), asserted at the byte level for the
// text formats and structurally for the verbose ones.
//
// Pinned regressions: csvjsonarray must be VALID JSON with numeric
// timestamps (#23115 label quotes, #23117 unquoted datetimes — contract:
// timestamps always numeric). Dimension names carrying double-quotes are
// CASE-019 (case019_test.go): the v1 JSON-family formatters emit them
// unescaped — invalid JSON, red until fixed.
package corpus

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

const (
	l7Chart = "fixture.l7"
	l7Rows  = 6
)

// l7Fixture: dim "plain" = i + 0.5 (fractional, SN- and print-exact);
// dim "comma,dim" = -i with a gap at rows 3-4 (null cells; the raw comma
// pins the CURRENT csv contract: header cells are not RFC4180-quoted, so
// a separator inside a name makes the header ambiguous — documented
// behavior, cosmetic). Names with double-quotes are CASE-019 (red): the
// v1 JSON-family formatters emit them unescaped, producing invalid JSON.
func l7Fixture() fixture.Chart {
	quoted := `comma,dim`
	ch := fixture.Chart{
		ID: l7Chart, Title: "formatters", Units: "units", Family: "fixture",
		Context: l7Chart, UpdateEvery: 1,
		Dimensions: []fixture.Dimension{{ID: "plain"}, {ID: quoted}},
	}
	for i := 1; i <= l7Rows; i++ {
		ch.Dimensions[0].Points = append(ch.Dimensions[0].Points, fixture.Point{
			T: fixture.T0 + int64(i), Collected: fmt.Sprintf("%d.5", i), Flags: stream.FlagNotAnomalous,
		})
		p := fixture.Point{T: fixture.T0 + int64(i), Collected: strconv.Itoa(-i), Flags: stream.FlagNotAnomalous}
		if i == 3 || i == 4 {
			p.Flags = stream.FlagEmpty
		}
		ch.Dimensions[1].Points = append(ch.Dimensions[1].Points, p)
	}
	return ch
}

// l7Params builds the deterministic v1 query: absolute window, numeric
// times (options=seconds), natural point granularity.
func l7Params(extraOptions string) map[string][]string {
	options := "seconds"
	if extraOptions != "" {
		options += "|" + extraOptions
	}
	return map[string][]string{
		"chart":   {l7Chart},
		"after":   {strconv.FormatInt(fixture.T0, 10)},
		"before":  {strconv.FormatInt(fixture.T0+l7Rows, 10)},
		"points":  {strconv.Itoa(l7Rows)},
		"group":   {"average"},
		"options": {options},
	}
}

func TestLayer7Formatters(t *testing.T) {
	ch := l7Fixture()
	pushLiveBurst(t, "l7-fmt", guid(90), ch)
	if _, err := td.WaitRetention("l7-fmt", ch.Context, fixture.T0+1, fixture.T0+l7Rows, 15*time.Second); err != nil {
		t.Fatal(err)
	}

	// expected per-row cells, oldest first: t, plain, quoted
	type row struct {
		t      int64
		plain  string
		quoted string // gap cells print the literal "null" (pinned contract)
	}
	rows := make([]row, 0, l7Rows)
	for i := 1; i <= l7Rows; i++ {
		r := row{t: fixture.T0 + int64(i), plain: fmt.Sprintf("%d.5", i), quoted: strconv.Itoa(-i)}
		if i == 3 || i == 4 {
			r.quoted = "null"
		}
		rows = append(rows, r)
	}

	get := func(t *testing.T, format, extraOptions string) string {
		t.Helper()
		params := l7Params(extraOptions)
		params["format"] = []string{format}
		body, err := td.DataV1Raw("l7-fmt", params)
		if err != nil {
			t.Fatal(err)
		}
		return body
	}

	t.Run("csv", func(t *testing.T) {
		// v1 rows come NEWEST FIRST by default; options=flip ascends
		var b strings.Builder
		b.WriteString("time,plain,comma,dim\r\n")
		for i := len(rows) - 1; i >= 0; i-- {
			r := rows[i]
			fmt.Fprintf(&b, "%d,%s,%s\r\n", r.t, r.plain, r.quoted)
		}
		got := get(t, "csv", "")
		if got != b.String() {
			t.Errorf("csv mismatch:\ngot:\n%s\nwant:\n%s", got, b.String())
		}
	})

	t.Run("csv-flip-ascending", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("time,plain,comma,dim\r\n")
		for _, r := range rows {
			fmt.Fprintf(&b, "%d,%s,%s\r\n", r.t, r.plain, r.quoted)
		}
		got := get(t, "csv", "flip")
		if got != b.String() {
			t.Errorf("csv natural mismatch:\ngot:\n%s\nwant:\n%s", got, b.String())
		}
	})

	t.Run("tsv", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("time\tplain\tcomma,dim\r\n")
		for i := len(rows) - 1; i >= 0; i-- {
			r := rows[i]
			fmt.Fprintf(&b, "%d\t%s\t%s\r\n", r.t, r.plain, r.quoted)
		}
		got := get(t, "tsv", "")
		if got != b.String() {
			t.Errorf("tsv mismatch:\ngot:\n%s\nwant:\n%s", got, b.String())
		}
	})

	// the single-series formats reduce each row to the SUM of its
	// dimensions (rrdr2value default): plain + quoted = 0.5 everywhere,
	// except the gap rows where only `plain` contributes; newest first
	ssvWant := []string{"0.5", "0.5", "4.5", "3.5", "0.5", "0.5"}

	t.Run("ssv", func(t *testing.T) {
		got := get(t, "ssv", "")
		cells := strings.Split(strings.TrimSpace(got), " ")
		if len(cells) != l7Rows {
			t.Fatalf("ssv: %d cells, want %d (%q)", len(cells), l7Rows, got)
		}
		for i, c := range cells {
			if c != ssvWant[i] {
				t.Errorf("ssv cell %d: %q, want %q", i, c, ssvWant[i])
			}
		}
	})

	t.Run("ssvcomma", func(t *testing.T) {
		got := get(t, "ssvcomma", "")
		cells := strings.Split(strings.TrimSpace(got), ",")
		if len(cells) != l7Rows {
			t.Fatalf("ssvcomma: %d cells, want %d (%q)", len(cells), l7Rows, got)
		}
		for i, c := range cells {
			if c != ssvWant[i] {
				t.Errorf("ssvcomma cell %d: %q, want %q", i, c, ssvWant[i])
			}
		}
	})

	t.Run("csvjsonarray", func(t *testing.T) {
		got := get(t, "csvjsonarray", "")

		// #23115/#23117: the payload must be VALID JSON…
		var arr []any
		if err := json.Unmarshal([]byte(got), &arr); err != nil {
			t.Fatalf("csvjsonarray is not valid JSON (%v):\n%s", err, got)
		}
		if len(arr) != l7Rows+1 {
			t.Fatalf("csvjsonarray: %d rows, want header + %d", len(arr), l7Rows)
		}
		header, ok := arr[0].([]any)
		if !ok || len(header) != 3 {
			t.Fatalf("csvjsonarray: malformed header %v", arr[0])
		}
		if header[1] != "plain" || header[2] != `comma,dim` {
			t.Errorf("csvjsonarray header names %v — quote escaping broken?", header)
		}
		// …and every timestamp must be NUMERIC (contract from #23117)
		for ri, rowAny := range arr[1:] {
			cells, ok := rowAny.([]any)
			if !ok || len(cells) != 3 {
				t.Fatalf("csvjsonarray row %d malformed: %v", ri, rowAny)
			}
			if _, ok := cells[0].(float64); !ok {
				t.Errorf("csvjsonarray row %d: non-numeric timestamp %v (%T)", ri, cells[0], cells[0])
			}
		}
	})

	t.Run("markdown", func(t *testing.T) {
		got := get(t, "markdown", "")
		lines := strings.Split(strings.TrimSpace(got), "\n")
		// header + separator + data rows
		if len(lines) != 2+l7Rows {
			t.Fatalf("markdown: %d lines, want %d:\n%s", len(lines), 2+l7Rows, got)
		}
		if !strings.HasPrefix(lines[0], "time|") || !strings.HasPrefix(lines[1], ":---") {
			t.Errorf("markdown header shape unexpected:\n%s\n%s", lines[0], lines[1])
		}
	})

	t.Run("html", func(t *testing.T) {
		got := get(t, "html", "")
		if !strings.Contains(got, "<table") || strings.Count(got, "<tr") != l7Rows+1 {
			t.Errorf("html: expected a table with %d rows, got %d <tr:\n%.400s", l7Rows+1, strings.Count(got, "<tr"), got)
		}
	})

	t.Run("array", func(t *testing.T) {
		got := get(t, "array", "")
		var arr []float64
		if err := json.Unmarshal([]byte(got), &arr); err != nil {
			t.Fatalf("array is not valid JSON (%v): %q", err, got)
		}
		if len(arr) != l7Rows {
			t.Fatalf("array: %d cells, want %d", len(arr), l7Rows)
		}
		// same single-series reduction as ssv, newest first
		for i, v := range arr {
			want, err := strconv.ParseFloat(ssvWant[i], 64)
			if err != nil {
				t.Fatal(err)
			}
			if v != want {
				t.Errorf("array cell %d: %v, want %v", i, v, want)
			}
		}
	})

	t.Run("json", func(t *testing.T) {
		// without jsonwrap the v1 json format is UNWRAPPED: labels/data
		// live at the top level
		got := get(t, "json", "")
		var doc map[string]any
		if err := json.Unmarshal([]byte(got), &doc); err != nil {
			t.Fatalf("json format is not valid JSON (%v)", err)
		}
		data, _ := doc["data"].([]any)
		if len(data) != l7Rows {
			t.Errorf("json: %d data rows, want %d", len(data), l7Rows)
		}
		labels, _ := doc["labels"].([]any)
		if len(labels) != 3 || labels[1] != "plain" || labels[2] != `comma,dim` {
			t.Errorf("json labels %v — name escaping broken?", labels)
		}
	})

	t.Run("datatable", func(t *testing.T) {
		got := get(t, "datatable", "")
		var doc map[string]any
		if err := json.Unmarshal([]byte(got), &doc); err != nil {
			t.Fatalf("datatable is not valid JSON (%v):\n%.300s", err, got)
		}
		rowsAny, _ := doc["rows"].([]any)
		if len(rowsAny) != l7Rows {
			t.Errorf("datatable: %d rows, want %d", len(rowsAny), l7Rows)
		}
	})

	t.Run("jsonp", func(t *testing.T) {
		params := l7Params("")
		params["format"] = []string{"jsonp"}
		params["callback"] = []string{"corpusCb"}
		got, err := td.DataV1Raw("l7-fmt", params)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(got, "corpusCb(") || !strings.HasSuffix(strings.TrimSpace(got), ");") {
			t.Errorf("jsonp envelope unexpected: %.120q…", got)
		}
		inner := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(got), "corpusCb("), ");")
		var doc map[string]any
		if err := json.Unmarshal([]byte(inner), &doc); err != nil {
			t.Errorf("jsonp payload is not valid JSON (%v)", err)
		}
	})
}
