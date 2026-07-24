// SPDX-License-Identifier: GPL-3.0-or-later

// Package canon canonicalizes query responses for corpus assertions:
// volatile fields (timings, agent identities, versions) are stripped, and
// json2 result payloads are decoded into typed per-dimension columns.
package canon

import (
	"fmt"
	"maps"
	"slices"
)

// Point annotation bits, mirroring RRDR_VALUE_* in src/web/api/queries/rrdr.h.
const (
	AnnotationEmpty   = 1 << 0
	AnnotationReset   = 1 << 1
	AnnotationPartial = 1 << 2
)

// Pt is one decoded json2 point of one dimension: T in seconds, Value nil
// for nulls, ARP the anomaly rate percent, PA the annotation bits. Count
// and Hidden are present only in aggregatable (raw) responses, where the
// point schema adds them (Count = group contributions, Hidden = the
// percentage denominator accumulator).
type Pt struct {
	T      int64
	Value  *float64
	ARP    float64
	PA     int64
	Count  *int64
	Hidden *float64
}

// pointSchema resolves the result.point name→index map; json2 emits it in
// short (arp, pa) or long (anomaly_rate, point_annotations) key forms.
type pointSchema struct {
	value, arp, pa, count, hidden int
}

func decodePointSchema(result map[string]any) pointSchema {
	ps := pointSchema{value: 0, arp: 1, pa: 2, count: -1, hidden: -1}
	schema, ok := result["point"].(map[string]any)
	if !ok {
		return ps
	}
	for name, idxAny := range schema {
		idx, ok := idxAny.(float64)
		if !ok {
			continue
		}
		switch name {
		case "value":
			ps.value = int(idx)
		case "arp", "anomaly_rate":
			ps.arp = int(idx)
		case "pa", "point_annotations":
			ps.pa = int(idx)
		case "count":
			ps.count = int(idx)
		case "hidden":
			ps.hidden = int(idx)
		}
	}
	return ps
}

// Columns decodes a json2 document's result payload into per-dimension
// point columns keyed by the dimension label, sorted by time. json2 rows are
// [time_ms, [value, arp, pa], ...] with one triplet per labeled dimension.
func Columns(doc map[string]any) (map[string][]Pt, error) {
	result, ok := doc["result"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("canon: no result object in document")
	}
	labelsAny, ok := result["labels"].([]any)
	if !ok || len(labelsAny) < 2 {
		return nil, fmt.Errorf("canon: result.labels missing or too short: %v", result["labels"])
	}
	labels := make([]string, 0, len(labelsAny)-1)
	for _, l := range labelsAny[1:] {
		s, ok := l.(string)
		if !ok {
			return nil, fmt.Errorf("canon: non-string label %v", l)
		}
		labels = append(labels, s)
	}

	rowsAny, ok := result["data"].([]any)
	if !ok {
		return nil, fmt.Errorf("canon: result.data missing")
	}
	ps := decodePointSchema(result)

	cols := make(map[string][]Pt, len(labels))
	for _, rowAny := range rowsAny {
		row, ok := rowAny.([]any)
		if !ok || len(row) != len(labels)+1 {
			return nil, fmt.Errorf("canon: malformed row %v (want time + %d points)", rowAny, len(labels))
		}
		tRaw, ok := row[0].(float64)
		if !ok {
			return nil, fmt.Errorf("canon: non-numeric row time %v", row[0])
		}
		// v2/v3 emit seconds; v1-era options emit milliseconds — normalize
		// by magnitude (the fixed 2023 epoch makes the ranges unambiguous)
		tsec := int64(tRaw)
		if tsec > 1_000_000_000_000 {
			tsec /= 1000
		}
		for i, lbl := range labels {
			point, ok := row[1+i].([]any)
			if !ok || len(point) < 3 {
				return nil, fmt.Errorf("canon: malformed point %v for %s", row[1+i], lbl)
			}
			pt := Pt{T: tsec}
			if v, ok := point[ps.value].(float64); ok {
				pt.Value = &v
			}
			if arp, ok := point[ps.arp].(float64); ok {
				pt.ARP = arp
			}
			if pa, ok := point[ps.pa].(float64); ok {
				pt.PA = int64(pa)
			}
			if ps.count >= 0 && ps.count < len(point) {
				if c, ok := point[ps.count].(float64); ok {
					n := int64(c)
					pt.Count = &n
				}
			}
			if ps.hidden >= 0 && ps.hidden < len(point) {
				if h, ok := point[ps.hidden].(float64); ok {
					pt.Hidden = &h
				}
			}
			cols[lbl] = append(cols[lbl], pt)
		}
	}

	for lbl := range cols {
		slices.SortFunc(cols[lbl], func(a, b Pt) int {
			return int(a.T - b.T)
		})
	}
	return cols, nil
}

// volatileTopLevel are document keys that vary between runs and carry no
// contract: they are dropped from canonical documents.
var volatileTopLevel = []string{"timings", "agents", "versions"}

// Canonicalize returns a shallow-copied document with volatile fields
// removed. The strip list grows as corpus layers pin more of the payload.
func Canonicalize(doc map[string]any) map[string]any {
	out := maps.Clone(doc)
	for _, k := range volatileTopLevel {
		delete(out, k)
	}
	return out
}
