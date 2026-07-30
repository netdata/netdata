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
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

func TestC019FormatShapeGuards(t *testing.T) {
	labels := []any{"time", `dim"quote`, `dim'apos`}
	arrayRows := make([]any, 5)
	objectRows := make([]any, 5)
	tableRows := make([]any, 5)
	for i := range arrayRows {
		arrayRows[i] = []any{float64(i), float64(i), float64(i)}
		objectRows[i] = map[string]any{
			"time": float64(i), `dim"quote`: float64(i), `dim'apos`: float64(i),
		}
		tableRows[i] = map[string]any{"c": []any{
			map[string]any{"v": float64(i)},
			map[string]any{"v": ""},
			map[string]any{"v": ""},
			map[string]any{"v": float64(i)},
			map[string]any{"v": float64(i)},
		}}
	}
	valid := map[string]any{
		"json":         map[string]any{"labels": labels, "data": arrayRows},
		"csvjsonarray": append([]any{labels}, arrayRows...),
		"datatable": map[string]any{
			"cols": []any{
				map[string]any{"label": "time"},
				map[string]any{"label": "", "p": map[string]any{"role": "annotation"}},
				map[string]any{"label": "", "p": map[string]any{"role": "annotationText"}},
				map[string]any{"label": `dim"quote`},
				map[string]any{"label": `dim'apos`},
			},
			"rows": tableRows,
		},
		"objectrows": map[string]any{"data": objectRows},
	}
	for shape, value := range valid {
		if err := c019ValidateShape(shape, value); err != nil {
			t.Errorf("%s valid control rejected: %v", shape, err)
		}
	}
	for _, shape := range []string{"json", "datatable", "objectrows"} {
		if err := c019ValidateShape(shape, map[string]any{}); err == nil {
			t.Errorf("%s accepted an empty object", shape)
		}
	}
	if err := c019ValidateShape("csvjsonarray", []any{[]any{}}); err == nil {
		t.Error("csvjsonarray accepted an empty header with no rows")
	}
	for name, payload := range map[string]string{
		"plain-json":       `{"labels":[],"data":[]}`,
		"missing-suffix":   `callback({"labels":[],"data":[]}`,
		"missing-prefix":   `{"labels":[],"data":[]});`,
		"wrong-callback":   `other({"labels":[],"data":[]});`,
		"trailing-content": `callback({"labels":[],"data":[]});junk`,
	} {
		t.Run("jsonp-envelope/"+name, func(t *testing.T) {
			if _, err := c019UnwrapJSONP(payload); err == nil {
				t.Errorf("JSONP envelope accepted %q", payload)
			}
		})
	}
	if payload, err := c019UnwrapJSONP(` callback({"labels":[],"data":[]}); `); err != nil ||
		payload != `{"labels":[],"data":[]}` {
		t.Fatalf("valid JSONP envelope = %q, %v", payload, err)
	}
}

func c019UnwrapJSONP(payload string) (string, error) {
	const prefix, suffix = "callback(", ");"
	payload = strings.TrimSpace(payload)
	if !strings.HasPrefix(payload, prefix) || !strings.HasSuffix(payload, suffix) {
		return "", fmt.Errorf("invalid JSONP callback envelope")
	}
	return payload[len(prefix) : len(payload)-len(suffix)], nil
}

func c019ValidateShape(shape string, value any) error {
	wantLabels := []string{"time", `dim"quote`, `dim'apos`}
	validateLabels := func(raw any, path string) error {
		labels, ok := raw.([]any)
		if !ok || len(labels) != len(wantLabels) {
			return fmt.Errorf("%s is %v, want exactly %v", path, raw, wantLabels)
		}
		for i, want := range wantLabels {
			if labels[i] != want {
				return fmt.Errorf("%s[%d] is %v, want %q", path, i, labels[i], want)
			}
		}
		return nil
	}
	validateArrayRows := func(raw any, path string) error {
		rows, ok := raw.([]any)
		if !ok || len(rows) != 5 {
			return fmt.Errorf("%s is %v, want exactly 5 rows", path, raw)
		}
		for i, rowAny := range rows {
			row, ok := rowAny.([]any)
			if !ok || len(row) != len(wantLabels) {
				return fmt.Errorf("%s[%d] is %v, want exactly 3 cells", path, i, rowAny)
			}
			for j, cell := range row {
				number, ok := cell.(float64)
				if !ok || math.IsNaN(number) || math.IsInf(number, 0) {
					return fmt.Errorf("%s[%d][%d] is not finite numeric: %v", path, i, j, cell)
				}
			}
		}
		return nil
	}

	switch shape {
	case "json":
		doc, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("json payload is not an object: %v", value)
		}
		if err := validateLabels(doc["labels"], "labels"); err != nil {
			return err
		}
		return validateArrayRows(doc["data"], "data")

	case "csvjsonarray":
		rows, ok := value.([]any)
		if !ok || len(rows) != 6 {
			return fmt.Errorf("csvjsonarray is %v, want header plus 5 rows", value)
		}
		if err := validateLabels(rows[0], "header"); err != nil {
			return err
		}
		return validateArrayRows(rows[1:], "rows")

	case "datatable":
		doc, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("datatable payload is not an object: %v", value)
		}
		cols, ok := doc["cols"].([]any)
		if !ok || len(cols) != 5 {
			return fmt.Errorf("datatable cols is %v, want time, two annotation columns, and two dimensions", doc["cols"])
		}
		for _, expected := range []struct {
			index int
			label string
		}{
			{0, wantLabels[0]},
			{3, wantLabels[1]},
			{4, wantLabels[2]},
		} {
			i, want := expected.index, expected.label
			column, ok := cols[i].(map[string]any)
			if !ok || column["label"] != want {
				return fmt.Errorf("datatable cols[%d] is %v, want label %q", i, cols[i], want)
			}
		}
		for i, role := range []string{"annotation", "annotationText"} {
			column, ok := cols[i+1].(map[string]any)
			properties, propertiesOK := column["p"].(map[string]any)
			if !ok || column["label"] != "" || !propertiesOK || properties["role"] != role {
				return fmt.Errorf("datatable cols[%d] is %v, want empty-label %s role", i+1, cols[i+1], role)
			}
		}
		rows, ok := doc["rows"].([]any)
		if !ok || len(rows) != 5 {
			return fmt.Errorf("datatable rows is %v, want exactly 5 rows", doc["rows"])
		}
		for i, rowAny := range rows {
			row, ok := rowAny.(map[string]any)
			if !ok {
				return fmt.Errorf("datatable rows[%d] is not an object: %v", i, rowAny)
			}
			cells, ok := row["c"].([]any)
			if !ok || len(cells) != len(cols) {
				return fmt.Errorf("datatable rows[%d].c is %v, want exactly %d cells", i, row["c"], len(cols))
			}
			for j, cellAny := range cells {
				cell, ok := cellAny.(map[string]any)
				if !ok {
					return fmt.Errorf("datatable rows[%d].c[%d] is not an object: %v", i, j, cellAny)
				}
				if _, hasValue := cell["v"]; !hasValue {
					return fmt.Errorf("datatable rows[%d].c[%d] has no value", i, j)
				}
			}
		}
		return nil

	case "objectrows":
		doc, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("objectrows payload is not an object: %v", value)
		}
		rows, ok := doc["data"].([]any)
		if !ok || len(rows) != 5 {
			return fmt.Errorf("objectrows data is %v, want exactly 5 rows", doc["data"])
		}
		for i, rowAny := range rows {
			row, ok := rowAny.(map[string]any)
			if !ok || len(row) != len(wantLabels) {
				return fmt.Errorf("objectrows data[%d] is %v, want exactly the 3 expected keys", i, rowAny)
			}
			for _, key := range wantLabels {
				number, ok := row[key].(float64)
				if !ok || math.IsNaN(number) || math.IsInf(number, 0) {
					return fmt.Errorf("objectrows data[%d][%q] is not finite numeric: %v", i, key, row[key])
				}
			}
		}
		return nil
	}
	return fmt.Errorf("unknown CASE-019 shape %q", shape)
}

func TestCase019JsonNameEscaping(t *testing.T) {
	trackContract(t, "CASE-019/v1-json-name-escaping")

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
	mustValidate := func(format, shape, options, payload string) {
		var v any
		if err := json.Unmarshal([]byte(payload), &v); err != nil {
			t.Logf("%s (options=%s): invalid JSON with a quote in a dimension name: %v", format, options, err)
			invalid++
			return
		}
		if err := c019ValidateShape(shape, v); err != nil {
			t.Logf("%s (options=%s): malformed formatter payload: %v", format, options, err)
			invalid++
		}
	}

	// the JSON shapes must parse — the double-quote name is the breaker
	for _, format := range []string{"json", "jsonp", "csvjsonarray", "datatable"} {
		payload := c019(format, "seconds")
		if format == "jsonp" {
			var err error
			payload, err = c019UnwrapJSONP(payload)
			if err != nil {
				t.Logf("%s (options=seconds): invalid JSONP envelope: %v", format, err)
				invalid++
				continue
			}
		}
		shape := format
		if format == "jsonp" {
			shape = "json"
		}
		mustValidate(format, shape, "seconds", payload)
	}

	// objectrows repeats the names as every row's object keys
	mustValidate("json", "objectrows", "seconds,objectrows", c019("json", "seconds,objectrows"))

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

	assertContract(t, "CASE-019/v1-json-name-escaping", invalid == 0)
}
