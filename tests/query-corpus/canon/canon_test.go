// SPDX-License-Identifier: GPL-3.0-or-later

package canon

import (
	"math"
	"strings"
	"testing"
)

func testDocument(point map[string]any, labels []any, data []any) map[string]any {
	return map[string]any{
		"result": map[string]any{
			"labels": labels,
			"point":  point,
			"data":   data,
		},
	}
}

func TestColumnsValidSchemas(t *testing.T) {
	t.Run("short keys and sorted rows", func(t *testing.T) {
		doc := testDocument(
			map[string]any{"value": 0.0, "arp": 1.0, "pa": 2.0},
			[]any{"time", "load"},
			[]any{
				[]any{1700000002.0, []any{2.0, 25.0, 0.0}},
				[]any{1700000001.0, []any{nil, 0.0, float64(AnnotationEmpty)}},
			},
		)

		cols, err := Columns(doc)
		if err != nil {
			t.Fatal(err)
		}
		got := cols["load"]
		if len(got) != 2 || got[0].T != 1700000001 || got[1].T != 1700000002 {
			t.Fatalf("sorted timestamps = %+v, want 1700000001,1700000002", got)
		}
		if got[0].Value != nil || got[0].PA != AnnotationEmpty {
			t.Fatalf("empty point = %+v, want null with EMPTY", got[0])
		}
		if got[1].Value == nil || *got[1].Value != 2 || got[1].ARP != 25 {
			t.Fatalf("numeric point = %+v, want value=2 ARP=25", got[1])
		}
	})

	t.Run("long keys and raw metadata", func(t *testing.T) {
		doc := testDocument(
			map[string]any{
				"value":             0.0,
				"anomaly_rate":      1.0,
				"point_annotations": 2.0,
				"count":             3.0,
				"hidden":            4.0,
			},
			[]any{"time", "load"},
			[]any{
				[]any{1700000001000.0, []any{3.0, 50.0, 4.0, 2.0, 7.5}},
			},
		)

		cols, err := Columns(doc)
		if err != nil {
			t.Fatal(err)
		}
		got := cols["load"][0]
		if got.T != 1700000001 || got.Count == nil || *got.Count != 2 ||
			got.Hidden == nil || *got.Hidden != 7.5 {
			t.Fatalf("decoded raw point = %+v", got)
		}
	})

	t.Run("nullable raw hidden", func(t *testing.T) {
		doc := testDocument(
			map[string]any{
				"value":  0.0,
				"arp":    1.0,
				"pa":     2.0,
				"count":  3.0,
				"hidden": 4.0,
			},
			[]any{"time", "load"},
			[]any{
				[]any{1700000001.0, []any{3.0, 0.0, 0.0, 1.0, nil}},
			},
		)
		result := doc["result"].(map[string]any)
		point := result["point"].(map[string]any)
		if _, declared := point["hidden"]; !declared {
			t.Fatal("nullable raw fixture does not declare result.point.hidden")
		}

		cols, err := Columns(doc)
		if err != nil {
			t.Fatal(err)
		}
		got := cols["load"][0]
		if got.Count == nil || *got.Count != 1 || got.Hidden != nil {
			t.Fatalf("decoded raw point = %+v, want count 1 and null hidden", got)
		}
	})
}

func TestColumnsRejectsMalformedDocumentsWithoutPanicking(t *testing.T) {
	base := func() map[string]any {
		return testDocument(
			map[string]any{"value": 0.0, "arp": 1.0, "pa": 2.0},
			[]any{"time", "load"},
			[]any{[]any{1700000001.0, []any{1.0, 0.0, 0.0}}},
		)
	}

	tests := map[string]func() map[string]any{
		"missing result": func() map[string]any { return map[string]any{} },
		"missing labels": func() map[string]any {
			doc := base()
			delete(doc["result"].(map[string]any), "labels")
			return doc
		},
		"wrong time label": func() map[string]any {
			doc := base()
			doc["result"].(map[string]any)["labels"] = []any{"timestamp", "load"}
			return doc
		},
		"duplicate dimension label": func() map[string]any {
			doc := base()
			result := doc["result"].(map[string]any)
			result["labels"] = []any{"time", "load", "load"}
			result["data"] = []any{[]any{1700000001.0, []any{1.0, 0.0, 0.0}, []any{2.0, 0.0, 0.0}}}
			return doc
		},
		"missing point schema": func() map[string]any {
			doc := base()
			delete(doc["result"].(map[string]any), "point")
			return doc
		},
		"missing required schema field": func() map[string]any {
			doc := base()
			doc["result"].(map[string]any)["point"] = map[string]any{"value": 0.0, "arp": 1.0}
			return doc
		},
		"non-numeric schema index": func() map[string]any {
			doc := base()
			doc["result"].(map[string]any)["point"].(map[string]any)["value"] = "0"
			return doc
		},
		"non-finite schema index": func() map[string]any {
			doc := base()
			doc["result"].(map[string]any)["point"].(map[string]any)["value"] = math.Inf(1)
			return doc
		},
		"fractional schema index": func() map[string]any {
			doc := base()
			doc["result"].(map[string]any)["point"].(map[string]any)["value"] = 0.5
			return doc
		},
		"negative schema index": func() map[string]any {
			doc := base()
			doc["result"].(map[string]any)["point"].(map[string]any)["value"] = -1.0
			return doc
		},
		"duplicate schema index": func() map[string]any {
			doc := base()
			doc["result"].(map[string]any)["point"].(map[string]any)["arp"] = 0.0
			return doc
		},
		"conflicting aliases": func() map[string]any {
			doc := base()
			doc["result"].(map[string]any)["point"].(map[string]any)["anomaly_rate"] = 2.0
			return doc
		},
		"unknown schema field": func() map[string]any {
			doc := base()
			result := doc["result"].(map[string]any)
			result["point"].(map[string]any)["contributors"] = 3.0
			result["data"] = []any{[]any{1700000001.0, []any{1.0, 0.0, 0.0, 1.0}}}
			return doc
		},
		"schema index outside point": func() map[string]any {
			doc := base()
			doc["result"].(map[string]any)["point"].(map[string]any)["pa"] = 3.0
			return doc
		},
		"missing data": func() map[string]any {
			doc := base()
			delete(doc["result"].(map[string]any), "data")
			return doc
		},
		"wrong row width": func() map[string]any {
			doc := base()
			doc["result"].(map[string]any)["data"] = []any{[]any{1700000001.0}}
			return doc
		},
		"non-numeric timestamp": func() map[string]any {
			doc := base()
			doc["result"].(map[string]any)["data"] = []any{[]any{"1700000001", []any{1.0, 0.0, 0.0}}}
			return doc
		},
		"non-finite timestamp": func() map[string]any {
			doc := base()
			doc["result"].(map[string]any)["data"] = []any{[]any{math.NaN(), []any{1.0, 0.0, 0.0}}}
			return doc
		},
		"fractional timestamp": func() map[string]any {
			doc := base()
			doc["result"].(map[string]any)["data"] = []any{[]any{1700000001.5, []any{1.0, 0.0, 0.0}}}
			return doc
		},
		"wrong point width": func() map[string]any {
			doc := base()
			doc["result"].(map[string]any)["data"] = []any{[]any{1700000001.0, []any{1.0, 0.0}}}
			return doc
		},
		"wrong value type": func() map[string]any {
			doc := base()
			doc["result"].(map[string]any)["data"] = []any{[]any{1700000001.0, []any{"1", 0.0, 0.0}}}
			return doc
		},
		"non-finite value": func() map[string]any {
			doc := base()
			doc["result"].(map[string]any)["data"] = []any{[]any{1700000001.0, []any{math.Inf(-1), 0.0, 0.0}}}
			return doc
		},
		"wrong anomaly rate type": func() map[string]any {
			doc := base()
			doc["result"].(map[string]any)["data"] = []any{[]any{1700000001.0, []any{1.0, nil, 0.0}}}
			return doc
		},
		"non-finite anomaly rate": func() map[string]any {
			doc := base()
			doc["result"].(map[string]any)["data"] = []any{[]any{1700000001.0, []any{1.0, math.NaN(), 0.0}}}
			return doc
		},
		"fractional annotation": func() map[string]any {
			doc := base()
			doc["result"].(map[string]any)["data"] = []any{[]any{1700000001.0, []any{1.0, 0.0, 0.5}}}
			return doc
		},
		"negative annotation": func() map[string]any {
			doc := base()
			doc["result"].(map[string]any)["data"] = []any{[]any{1700000001.0, []any{1.0, 0.0, -1.0}}}
			return doc
		},
		"fractional count": func() map[string]any {
			doc := base()
			result := doc["result"].(map[string]any)
			result["point"].(map[string]any)["count"] = 3.0
			result["data"] = []any{[]any{1700000001.0, []any{1.0, 0.0, 0.0, 1.5}}}
			return doc
		},
		"non-finite hidden": func() map[string]any {
			doc := base()
			result := doc["result"].(map[string]any)
			result["point"].(map[string]any)["count"] = 3.0
			result["point"].(map[string]any)["hidden"] = 4.0
			result["data"] = []any{[]any{1700000001.0, []any{1.0, 0.0, 0.0, 1.0, math.Inf(1)}}}
			return doc
		},
		"wrong hidden type": func() map[string]any {
			doc := base()
			result := doc["result"].(map[string]any)
			result["point"].(map[string]any)["count"] = 3.0
			result["point"].(map[string]any)["hidden"] = 4.0
			result["data"] = []any{[]any{1700000001.0, []any{1.0, 0.0, 0.0, 1.0, "0"}}}
			return doc
		},
	}

	for name, build := range tests {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("Columns panicked: %v", recovered)
				}
			}()

			_, err := Columns(build())
			if err == nil {
				t.Fatal("Columns accepted malformed document")
			}
			if !strings.HasPrefix(err.Error(), "canon:") {
				t.Fatalf("error %q does not identify the decoder", err)
			}
		})
	}
}

func TestEmptyResultRequiresExactNoMatchSubshape(t *testing.T) {
	valid := map[string]any{"result": map[string]any{
		"labels": []any{"time"},
		"data":   []any{},
	}}
	if !EmptyResult(valid) {
		t.Fatal("exact empty result rejected")
	}

	for name, doc := range map[string]map[string]any{
		"missing-result": {},
		"extra-label": {"result": map[string]any{
			"labels": []any{"time", "value"},
			"data":   []any{},
		}},
		"wrong-label": {"result": map[string]any{
			"labels": []any{"timestamp"},
			"data":   []any{},
		}},
		"nonempty-data": {"result": map[string]any{
			"labels": []any{"time"},
			"data":   []any{[]any{float64(1)}},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			if EmptyResult(doc) {
				t.Errorf("accepted %s as the exact no-match subshape", name)
			}
		})
	}
}
