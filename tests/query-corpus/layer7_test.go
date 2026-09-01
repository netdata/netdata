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

func l7ExpectedWireRow(wireIndex int) (int64, float64, any) {
	i := l7Rows - wireIndex
	timestamp := fixture.T0 + int64(i)
	plain := float64(i) + 0.5
	if i == 3 || i == 4 {
		return timestamp, plain, nil
	}
	return timestamp, plain, float64(-i)
}

func l7CellExact(got, want any) bool {
	if want == nil {
		return got == nil
	}
	gotNumber, gotOK := queryFiniteNumber(got)
	wantNumber, wantOK := want.(float64)
	return gotOK && wantOK && gotNumber == wantNumber
}

func l7ExpectedJSDate(timestamp int64) string {
	// The v1 datatable formatter uses localtime_r(), so use the process-local
	// timezone inherited by both the test and its daemon.
	tm := time.Unix(timestamp, 0).Local()
	return fmt.Sprintf(
		"Date(%04d,%02d,%02d,%02d,%02d,%02d)",
		tm.Year(), int(tm.Month())-1, tm.Day(), tm.Hour(), tm.Minute(), tm.Second())
}

func l7JSONRowsExact(doc map[string]any) error {
	labels, ok := doc["labels"].([]any)
	if !ok || len(labels) != 3 ||
		labels[0] != "time" || labels[1] != "plain" || labels[2] != "comma,dim" {
		return fmt.Errorf("labels are %v, want [time plain comma,dim]", doc["labels"])
	}
	rows, ok := doc["data"].([]any)
	if !ok || len(rows) != l7Rows {
		return fmt.Errorf("data is %v, want exactly %d rows", doc["data"], l7Rows)
	}
	for wireIndex, rowAny := range rows {
		row, ok := rowAny.([]any)
		if !ok || len(row) != 3 {
			return fmt.Errorf("data[%d] is not a three-cell row: %v", wireIndex, rowAny)
		}
		wantT, wantPlain, wantComma := l7ExpectedWireRow(wireIndex)
		gotT, timestampOK := queryInteger(row[0])
		if !timestampOK || gotT != wantT ||
			!l7CellExact(row[1], wantPlain) || !l7CellExact(row[2], wantComma) {
			return fmt.Errorf(
				"data[%d] is %v, want [%d %v %v]",
				wireIndex, row, wantT, wantPlain, wantComma)
		}
	}
	return nil
}

func l7ObjectRowsExact(doc map[string]any) error {
	rows, ok := doc["data"].([]any)
	if !ok || len(rows) != l7Rows {
		return fmt.Errorf("data is %v, want exactly %d object rows", doc["data"], l7Rows)
	}
	for wireIndex, rowAny := range rows {
		row, ok := rowAny.(map[string]any)
		if !ok || len(row) != 3 {
			return fmt.Errorf("data[%d] is not an exact three-field object: %v", wireIndex, rowAny)
		}
		wantT, wantPlain, wantComma := l7ExpectedWireRow(wireIndex)
		timeValue, timePresent := row["time"]
		plainValue, plainPresent := row["plain"]
		commaValue, commaPresent := row["comma,dim"]
		gotT, timestampOK := queryInteger(timeValue)
		if !timePresent || !plainPresent || !commaPresent ||
			!timestampOK || gotT != wantT ||
			!l7CellExact(plainValue, wantPlain) ||
			!l7CellExact(commaValue, wantComma) {
			return fmt.Errorf(
				"data[%d] is %v, want time=%d plain=%v comma,dim=%v",
				wireIndex, row, wantT, wantPlain, wantComma)
		}
	}
	return nil
}

func l7DatatableExact(doc map[string]any) error {
	cols, ok := doc["cols"].([]any)
	if !ok || len(cols) != 5 {
		return fmt.Errorf("cols is %v, want five columns", doc["cols"])
	}
	wantColumns := []struct {
		label string
		kind  string
		role  string
	}{
		{label: "time", kind: "datetime"},
		{kind: "string", role: "annotation"},
		{kind: "string", role: "annotationText"},
		{label: "plain", kind: "number"},
		{label: "comma,dim", kind: "number"},
	}
	for i, want := range wantColumns {
		column, ok := cols[i].(map[string]any)
		if !ok || column["label"] != want.label || column["type"] != want.kind {
			return fmt.Errorf("cols[%d] is %v, want label=%q type=%q", i, cols[i], want.label, want.kind)
		}
		if want.role != "" {
			properties, ok := column["p"].(map[string]any)
			if !ok || properties["role"] != want.role {
				return fmt.Errorf("cols[%d].p is %v, want role=%q", i, column["p"], want.role)
			}
		}
	}

	rows, ok := doc["rows"].([]any)
	if !ok || len(rows) != l7Rows {
		return fmt.Errorf("rows is %v, want exactly %d rows", doc["rows"], l7Rows)
	}
	for wireIndex, rowAny := range rows {
		row, ok := rowAny.(map[string]any)
		if !ok {
			return fmt.Errorf("rows[%d] is not an object: %v", wireIndex, rowAny)
		}
		cells, ok := row["c"].([]any)
		if !ok || len(cells) != 5 {
			return fmt.Errorf("rows[%d].c is %v, want five cells", wireIndex, row["c"])
		}
		values := make([]any, len(cells))
		for i, cellAny := range cells {
			cell, ok := cellAny.(map[string]any)
			if !ok {
				return fmt.Errorf("rows[%d].c[%d] is not an object: %v", wireIndex, i, cellAny)
			}
			value, present := cell["v"]
			if !present {
				return fmt.Errorf("rows[%d].c[%d] has no v field: %v", wireIndex, i, cell)
			}
			values[i] = value
		}
		wantT, wantPlain, wantComma := l7ExpectedWireRow(wireIndex)
		wantDate := l7ExpectedJSDate(wantT)
		if values[0] != wantDate || values[1] != nil || values[2] != nil ||
			!l7CellExact(values[3], wantPlain) || !l7CellExact(values[4], wantComma) {
			return fmt.Errorf(
				"rows[%d] values are %v, want [%q nil nil %v %v]",
				wireIndex, values, wantDate, wantPlain, wantComma)
		}
	}
	return nil
}

func TestL7StructuredResponseGuards(t *testing.T) {
	jsonControl := func() map[string]any {
		rows := make([]any, l7Rows)
		for wireIndex := range rows {
			timestamp, plain, comma := l7ExpectedWireRow(wireIndex)
			rows[wireIndex] = []any{float64(timestamp), plain, comma}
		}
		return map[string]any{
			"labels": []any{"time", "plain", "comma,dim"},
			"data":   rows,
		}
	}
	if err := l7JSONRowsExact(jsonControl()); err != nil {
		t.Fatalf("valid JSON rows rejected: %v", err)
	}
	for name, mutate := range map[string]func(map[string]any){
		"wrong-time-label": func(doc map[string]any) {
			doc["labels"].([]any)[0] = "timestamp"
		},
		"null-row": func(doc map[string]any) {
			doc["data"].([]any)[0] = nil
		},
		"wrong-value": func(doc map[string]any) {
			doc["data"].([]any)[0].([]any)[1] = float64(0)
		},
	} {
		t.Run("json-"+name, func(t *testing.T) {
			doc := jsonControl()
			mutate(doc)
			if err := l7JSONRowsExact(doc); err == nil {
				t.Errorf("accepted %s JSON mutation", name)
			}
		})
	}

	objectControl := func() map[string]any {
		rows := make([]any, l7Rows)
		for wireIndex := range rows {
			timestamp, plain, comma := l7ExpectedWireRow(wireIndex)
			rows[wireIndex] = map[string]any{
				"time": float64(timestamp), "plain": plain, "comma,dim": comma,
			}
		}
		return map[string]any{"data": rows}
	}
	if err := l7ObjectRowsExact(objectControl()); err != nil {
		t.Fatalf("valid object rows rejected: %v", err)
	}
	for name, mutate := range map[string]func(map[string]any){
		"null-time": func(doc map[string]any) {
			doc["data"].([]any)[0].(map[string]any)["time"] = nil
		},
		"null-row": func(doc map[string]any) {
			doc["data"].([]any)[1] = nil
		},
		"extra-field": func(doc map[string]any) {
			doc["data"].([]any)[0].(map[string]any)["extra"] = true
		},
		"missing-null-field-with-replacement": func(doc map[string]any) {
			row := doc["data"].([]any)[2].(map[string]any)
			delete(row, "comma,dim")
			row["unrelated"] = nil
		},
	} {
		t.Run("object-"+name, func(t *testing.T) {
			doc := objectControl()
			mutate(doc)
			if err := l7ObjectRowsExact(doc); err == nil {
				t.Errorf("accepted %s object-row mutation", name)
			}
		})
	}

	datatableControl := func() map[string]any {
		cols := []any{
			map[string]any{"label": "time", "type": "datetime"},
			map[string]any{"label": "", "type": "string", "p": map[string]any{"role": "annotation"}},
			map[string]any{"label": "", "type": "string", "p": map[string]any{"role": "annotationText"}},
			map[string]any{"label": "plain", "type": "number"},
			map[string]any{"label": "comma,dim", "type": "number"},
		}
		rows := make([]any, l7Rows)
		for wireIndex := range rows {
			timestamp, plain, comma := l7ExpectedWireRow(wireIndex)
			rows[wireIndex] = map[string]any{"c": []any{
				map[string]any{"v": l7ExpectedJSDate(timestamp)},
				map[string]any{"v": nil},
				map[string]any{"v": nil},
				map[string]any{"v": plain},
				map[string]any{"v": comma},
			}}
		}
		return map[string]any{"cols": cols, "rows": rows}
	}
	if err := l7DatatableExact(datatableControl()); err != nil {
		t.Fatalf("valid datatable rejected: %v", err)
	}
	for name, mutate := range map[string]func(map[string]any){
		"wrong-column": func(doc map[string]any) {
			doc["cols"].([]any)[0].(map[string]any)["label"] = "timestamp"
		},
		"null-row": func(doc map[string]any) {
			doc["rows"].([]any)[0] = nil
		},
		"missing-cell-value": func(doc map[string]any) {
			row := doc["rows"].([]any)[0].(map[string]any)
			delete(row["c"].([]any)[3].(map[string]any), "v")
		},
	} {
		t.Run("datatable-"+name, func(t *testing.T) {
			doc := datatableControl()
			mutate(doc)
			if err := l7DatatableExact(doc); err == nil {
				t.Errorf("accepted %s datatable mutation", name)
			}
		})
	}
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
		trackContract(t, "L7/format-csv")

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

		b.Reset()
		b.WriteString("time,plain,comma,dim\r\n")
		for _, r := range rows {
			fmt.Fprintf(&b, "%d,%s,%s\r\n", r.t, r.plain, r.quoted)
		}
		got = get(t, "csv", "flip")
		if got != b.String() {
			t.Errorf("csv natural mismatch:\ngot:\n%s\nwant:\n%s", got, b.String())
		}
	})

	t.Run("tsv", func(t *testing.T) {
		trackContract(t, "L7/format-tsv")

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
		trackContract(t, "L7/format-ssv")

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
		trackContract(t, "L7/format-ssvcomma")

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
		trackContract(t, "L7/format-csvjsonarray")

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
		trackContract(t, "L7/format-markdown")

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
		trackContract(t, "L7/format-html")

		got := get(t, "html", "")
		if !strings.Contains(got, "<table") || strings.Count(got, "<tr") != l7Rows+1 {
			t.Errorf("html: expected a table with %d rows, got %d <tr:\n%.400s", l7Rows+1, strings.Count(got, "<tr"), got)
		}
	})

	t.Run("array", func(t *testing.T) {
		trackContract(t, "L7/format-array")

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
		trackContract(t, "L7/format-json")

		// without jsonwrap the v1 json format is UNWRAPPED: labels/data
		// live at the top level
		got := get(t, "json", "")
		var doc map[string]any
		if err := json.Unmarshal([]byte(got), &doc); err != nil {
			t.Fatalf("json format is not valid JSON (%v)", err)
		}
		if err := l7JSONRowsExact(doc); err != nil {
			t.Errorf("json structure/value mismatch: %v", err)
		}
	})

	t.Run("datatable", func(t *testing.T) {
		trackContract(t, "L7/format-datatable")

		got := get(t, "datatable", "")
		var doc map[string]any
		if err := json.Unmarshal([]byte(got), &doc); err != nil {
			t.Fatalf("datatable is not valid JSON (%v):\n%.300s", err, got)
		}
		if err := l7DatatableExact(doc); err != nil {
			t.Errorf("datatable structure/value mismatch: %v", err)
		}
	})

	t.Run("jsonp", func(t *testing.T) {
		trackContract(t, "L7/format-jsonp")

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
		} else if err := l7JSONRowsExact(doc); err != nil {
			t.Errorf("jsonp payload structure/value mismatch: %v", err)
		}
	})
}
