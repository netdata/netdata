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

func TestOptionsTimestamps(t *testing.T) {
	optsL7(t)

	t.Run("ms", func(t *testing.T) {
		got := optsGet(t, "csv", "ms", nil)
		wantFirst := strconv.FormatInt((fixture.T0+6)*1000, 10) + ","
		lines := strings.Split(got, "\r\n")
		if len(lines) < 2 || !strings.HasPrefix(lines[1], wantFirst) {
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
	optsL7(t)

	t.Run("objectrows", func(t *testing.T) {
		got := optsGet(t, "json", "objectrows", nil)
		var doc map[string]any
		if err := json.Unmarshal([]byte(got), &doc); err != nil {
			t.Fatalf("objectrows not valid JSON: %v", err)
		}
		data, _ := doc["data"].([]any)
		if len(data) != l7Rows {
			t.Fatalf("objectrows: %d rows, want %d", len(data), l7Rows)
		}
		row, ok := data[0].(map[string]any)
		if !ok {
			t.Fatalf("objectrows row is not an object: %v", data[0])
		}
		if _, ok := row["time"]; !ok {
			t.Errorf("objectrows row lacks a time field: %v", row)
		}
		if v, ok := row["plain"].(float64); !ok || v != 6.5 {
			t.Errorf("objectrows newest plain = %v, want 6.5", row["plain"])
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
		if names, _ := doc["dimension_names"].([]any); len(names) != 1 {
			t.Errorf("queried dimension_names %v, want just the selection", names)
		}
		full, _ := doc["full_dimension_list"].([]any)
		if len(full) != 2 {
			t.Errorf("full_dimension_list %v, want both dims", full)
		}
		if _, ok := doc["full_chart_list"]; !ok {
			t.Errorf("full_chart_list missing")
		}
	})
}

func TestOptionsGoogleViz(t *testing.T) {
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
	optsL7(t)

	if a, b := optsGet(t, "tsv-excel", "", nil), optsGet(t, "tsv", "", nil); a != b {
		t.Errorf("tsv-excel differs from tsv:\n%.200s\nvs\n%.200s", a, b)
	}
}

func TestOptionsLabelQuotes(t *testing.T) {
	optsL7(t)

	got := optsGet(t, "csv", "label-quotes", nil)
	if !strings.HasPrefix(got, `"time","plain","comma,dim"`) {
		t.Errorf("label-quotes header not quoted:\n%.120s", got)
	}
}

func TestOptionsReductions(t *testing.T) {
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
