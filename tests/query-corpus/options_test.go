// SPDX-License-Identifier: GPL-3.0-or-later

// S8 slice 2 — the options and formats long tail, pinned over the
// layer-7 formatter fixture (dims: plain=i.5, "comma,dim"=-i with a
// gap at rows 3-4, newest first):
//   - timestamp renderings: seconds (baseline), ms, rfc3339;
//   - v1 json shapes: objectrows, all-dimensions;
//   - the google-viz envelope (google_json / tqx) and format aliases
//     (tsv-excel == tsv, datasource);
//   - csv label-quotes;
//   - the per-row reductions min2max/min/max/average on the
//     single-series formats (default: sum);
//   - v2 shape switches: minimal-stats, long-json-keys,
//     group-by-labels.
package corpus

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
)

func optsL7(t *testing.T) {
	t.Helper()
	if _, err := td.WaitRetention("l7-fmt", l7Chart, fixture.T0+1, fixture.T0+6, 15*time.Second); err != nil {
		t.Skip("layer-7 fixture not available")
	}
}

func optsGet(t *testing.T, format, options string, extra map[string]string) string {
	t.Helper()
	params := l7Params(options)
	params["format"] = []string{format}
	for k, v := range extra {
		params[k] = []string{v}
	}
	body, err := td.DataV1Raw("l7-fmt", params)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func optionsStringArrayExact(value any, want []string, path string) error {
	values, ok := value.([]any)
	if !ok {
		return fmt.Errorf("%s is not an array: %v", path, value)
	}
	if len(values) != len(want) {
		return fmt.Errorf("%s has %d values, want %d: %v", path, len(values), len(want), values)
	}
	for i, wantValue := range want {
		got, ok := values[i].(string)
		if !ok || got != wantValue {
			return fmt.Errorf("%s[%d] is %v, want %q", path, i, values[i], wantValue)
		}
	}
	return nil
}

func optionsStringPairSetExact(value any, want map[[2]string]struct{}, path string) error {
	values, ok := value.([]any)
	if !ok {
		return fmt.Errorf("%s is not an array: %v", path, value)
	}
	if len(values) != len(want) {
		return fmt.Errorf("%s has %d pairs, want %d: %v", path, len(values), len(want), values)
	}
	seen := make(map[[2]string]struct{}, len(values))
	for i, value := range values {
		row, ok := value.([]any)
		if !ok || len(row) != 2 {
			return fmt.Errorf("%s[%d] is not a two-cell pair: %v", path, i, value)
		}
		left, leftOK := row[0].(string)
		right, rightOK := row[1].(string)
		if !leftOK || !rightOK || left == "" || right == "" {
			return fmt.Errorf("%s[%d] is not a nonempty string pair: %v", path, i, row)
		}
		pair := [2]string{left, right}
		if _, expected := want[pair]; !expected {
			return fmt.Errorf("%s[%d] has unexpected pair %q/%q", path, i, left, right)
		}
		if _, duplicate := seen[pair]; duplicate {
			return fmt.Errorf("%s repeats pair %q/%q", path, left, right)
		}
		seen[pair] = struct{}{}
	}
	return nil
}

func optionsL7AllDimensionsExact(doc map[string]any) error {
	if err := optionsStringArrayExact(
		doc["dimension_names"], []string{"plain"}, "dimension_names"); err != nil {
		return err
	}
	if err := optionsStringPairSetExact(doc["full_dimension_list"], map[[2]string]struct{}{
		{"plain", "plain"}:         {},
		{"comma,dim", "comma,dim"}: {},
	}, "full_dimension_list"); err != nil {
		return err
	}
	if err := optionsStringPairSetExact(doc["full_chart_list"], map[[2]string]struct{}{
		{l7Chart, l7Chart}: {},
	}, "full_chart_list"); err != nil {
		return err
	}
	return optionsStringPairSetExact(doc["full_chart_labels"], map[[2]string]struct{}{
		{"_collect_plugin", "fixture-pusher"}: {},
		{"_collect_module", "corpus"}:         {},
	}, "full_chart_labels")
}

func TestOptionsAllDimensionsMetadataGuards(t *testing.T) {
	build := func() map[string]any {
		return map[string]any{
			"dimension_names": []any{"plain"},
			"full_dimension_list": []any{
				[]any{"plain", "plain"},
				[]any{"comma,dim", "comma,dim"},
			},
			"full_chart_list": []any{
				[]any{l7Chart, l7Chart},
			},
			"full_chart_labels": []any{
				[]any{"_collect_plugin", "fixture-pusher"},
				[]any{"_collect_module", "corpus"},
			},
		}
	}
	if err := optionsL7AllDimensionsExact(build()); err != nil {
		t.Fatalf("valid all-dimensions metadata rejected: %v", err)
	}

	mutations := map[string]func(map[string]any){
		"wrong-selection": func(doc map[string]any) {
			doc["dimension_names"] = []any{"comma,dim"}
		},
		"duplicate-dimension": func(doc map[string]any) {
			doc["full_dimension_list"] = []any{
				[]any{"plain", "plain"},
				[]any{"plain", "plain"},
			}
		},
		"null-chart-list": func(doc map[string]any) {
			doc["full_chart_list"] = nil
		},
		"missing-label-list": func(doc map[string]any) {
			delete(doc, "full_chart_labels")
		},
		"malformed-label-pair": func(doc map[string]any) {
			doc["full_chart_labels"].([]any)[0] = []any{"_collect_plugin", nil}
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			doc := build()
			mutate(doc)
			if err := optionsL7AllDimensionsExact(doc); err == nil {
				t.Errorf("accepted %s all-dimensions metadata mutation", name)
			}
		})
	}
}

func TestOptionsTimestamps(t *testing.T) {
	trackContractComponent(t, "API/options-long-tail", "timestamps")

	optsL7(t)

	t.Run("ms", func(t *testing.T) {
		got := optsGet(t, "csv", "ms", nil)
		wantFirst := strconv.FormatInt((fixture.T0+6)*1000, 10) + ","
		lines := strings.Split(got, "\r\n")
		if len(lines) < 2 {
			t.Fatalf("options=ms has no data row: %q", got)
		}
		if !strings.HasPrefix(lines[1], wantFirst) {
			t.Errorf("options=ms first row %q, want prefix %q", lines[1], wantFirst)
		}
	})

	t.Run("rfc3339", func(t *testing.T) {
		// the v1 formatters ignore rfc3339 when seconds is set — the
		// baseline epoch rendering wins (pinned); the RFC3339 rendering
		// belongs to the v2 surface
		got := optsGet(t, "csv", "rfc3339", nil)
		lines := strings.Split(got, "\r\n")
		if len(lines) < 2 {
			t.Fatalf("no data rows: %q", got)
		}
		cell := strings.SplitN(lines[1], ",", 2)[0]
		if cell != strconv.FormatInt(fixture.T0+6, 10) {
			// if this ever changes, the option started winning — re-pin
			ts, err := time.Parse(time.RFC3339, cell)
			if err != nil || ts.Unix() != fixture.T0+6 {
				t.Errorf("rfc3339+seconds first cell %q — neither the pinned epoch nor a valid RFC3339 of t0+6", cell)
			}
		}
	})
}

func TestOptionsV1JSONShapes(t *testing.T) {
	trackContractComponent(t, "API/options-long-tail", "v1-json-shapes")

	optsL7(t)

	t.Run("objectrows", func(t *testing.T) {
		got := optsGet(t, "json", "objectrows", nil)
		var doc map[string]any
		if err := json.Unmarshal([]byte(got), &doc); err != nil {
			t.Fatalf("objectrows not valid JSON: %v", err)
		}
		if err := l7ObjectRowsExact(doc); err != nil {
			t.Errorf("objectrows structure/value mismatch: %v", err)
		}
	})

	t.Run("all-dimensions", func(t *testing.T) {
		// the queried selection stays as-is (dimension_names = the
		// selected dim); all-dimensions ADDS the full_dimension_list /
		// full_chart_list / full_chart_labels metadata blocks
		got := optsGet(t, "json", "jsonwrap|all-dimensions", map[string]string{"dimension": "plain"})
		var doc map[string]any
		if err := json.Unmarshal([]byte(got), &doc); err != nil {
			t.Fatalf("not valid JSON: %v", err)
		}
		if err := optionsL7AllDimensionsExact(doc); err != nil {
			t.Errorf("all-dimensions metadata: %v", err)
		}
	})
}

func TestOptionsGoogleViz(t *testing.T) {
	trackContractComponent(t, "API/options-long-tail", "google-viz")

	optsL7(t)

	got := optsGet(t, "datatable", "", map[string]string{"tqx": "version:0.6;reqId:7;out:json"})
	if !strings.Contains(got, "google.visualization.Query.setResponse") {
		t.Errorf("tqx datatable is not a gviz envelope:\n%.200s", got)
	}
	if !strings.Contains(got, "reqId:'7'") && !strings.Contains(got, `"reqId":"7"`) {
		t.Errorf("gviz envelope does not echo reqId 7:\n%.300s", got)
	}
}

func TestOptionsFormatAliases(t *testing.T) {
	trackContractComponent(t, "API/options-long-tail", "format-aliases")

	optsL7(t)

	if a, b := optsGet(t, "tsv-excel", "", nil), optsGet(t, "tsv", "", nil); a != b {
		t.Errorf("tsv-excel differs from tsv:\n%.200s\nvs\n%.200s", a, b)
	}
}

func TestOptionsLabelQuotes(t *testing.T) {
	trackContractComponent(t, "API/options-long-tail", "label-quotes")

	optsL7(t)

	got := optsGet(t, "csv", "label-quotes", nil)
	if !strings.HasPrefix(got, `"time","plain","comma,dim"`) {
		t.Errorf("label-quotes header not quoted:\n%.120s", got)
	}
}

func TestOptionsReductions(t *testing.T) {
	trackContract(t, "API/row-reductions")

	optsL7(t)

	// per-row reductions on the single-series ssv format; rows newest
	// first, the gap rows carry only `plain`
	cases := map[string][]string{
		// max - min: (i.5) - (-i); single-value rows reduce to 0
		"min2max": {"12.5", "10.5", "0", "0", "4.5", "2.5"},
		// min: -i, or the sole value on gap rows
		"min": {"-6", "-5", "4.5", "3.5", "-2", "-1"},
		// max: always plain
		"max": {"6.5", "5.5", "4.5", "3.5", "2.5", "1.5"},
		// average: (i.5 - i)/2 = 0.25; gaps: the sole value
		"average": {"0.25", "0.25", "4.5", "3.5", "0.25", "0.25"},
	}
	for opt, want := range cases {
		t.Run(opt, func(t *testing.T) {
			got := optsGet(t, "ssv", opt, nil)
			cells := strings.Split(strings.TrimSpace(got), " ")
			if len(cells) != len(want) {
				t.Fatalf("%s: %d cells (%q), want %d", opt, len(cells), got, len(want))
			}
			for i, c := range cells {
				if c != want[i] {
					t.Errorf("%s cell %d: %q, want %q", opt, i, c, want[i])
				}
			}
		})
	}
}

func TestOptionsV2Shapes(t *testing.T) {
	trackContractComponent(t, "API/options-long-tail", "v2-shapes")

	if _, err := td.WaitRetention("l5-a", l5Context, fixture.T0+1, fixture.T0+l5Rows, 15*time.Second); err != nil {
		t.Skip("layer-5 palette not available")
	}

	base := func() map[string][]string {
		return daemon.DataParams(l5Context, fixture.T0, fixture.T0+l5Rows, 10)
	}

	t.Run("minimal-stats", func(t *testing.T) {
		p := base()
		p["options"] = []string{"jsonwrap|minimal"}
		doc, err := td.DataV3All(p)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := doc["totals"]; ok {
			t.Errorf("minimal-stats response still carries totals")
		}
	})

	t.Run("long-json-keys", func(t *testing.T) {
		p := base()
		p["options"] = []string{"jsonwrap|long-keys"}
		doc, err := td.DataV3All(p)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(doc["view"])
		if !strings.Contains(string(b), "anomaly_rate_percent") {
			t.Errorf("long-json-keys view block lacks the long arp key:\n%.300s", b)
		}
	})

	t.Run("group-by-labels", func(t *testing.T) {
		p := base()
		p["options"] = []string{"jsonwrap|group-by-labels"}
		p["group_by"] = []string{"label"}
		p["group_by_label"] = []string{"team"}
		doc, err := td.DataV3All(p)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(doc["view"])
		if !strings.Contains(string(b), "alpha") || !strings.Contains(string(b), "gamma") {
			t.Errorf("group-by-labels view lacks the label values:\n%.300s", b)
		}
	})
}
