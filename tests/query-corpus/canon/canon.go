// SPDX-License-Identifier: GPL-3.0-or-later

// Package canon decodes json2 query results into typed per-dimension columns
// for corpus assertions.
package canon

import (
	"fmt"
	"math"
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
// and Hidden come from aggregatable (raw) responses. Count is nonnil when
// declared by the point schema. Hidden is nonnil only for a finite percentage
// denominator accumulator; an absent field or null cell leaves it nil.
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
	width                         int
}

func decodePointSchema(result map[string]any) (pointSchema, error) {
	ps := pointSchema{value: -1, arp: -1, pa: -1, count: -1, hidden: -1}
	schema, ok := result["point"].(map[string]any)
	if !ok {
		return ps, fmt.Errorf("canon: result.point missing or not an object")
	}

	semanticIndices := make(map[string]int, len(schema))
	for name, idxAny := range schema {
		switch name {
		case "value", "arp", "anomaly_rate", "pa", "point_annotations", "count", "hidden":
		default:
			return ps, fmt.Errorf("canon: result.point has unknown field %q", name)
		}

		idx, err := schemaIndex(idxAny, "result.point."+name)
		if err != nil {
			return ps, err
		}

		semantic := name
		switch name {
		case "value":
		case "arp", "anomaly_rate":
			semantic = "arp"
		case "pa", "point_annotations":
			semantic = "pa"
		}
		if previous, seen := semanticIndices[semantic]; seen {
			if previous != idx {
				return ps, fmt.Errorf(
					"canon: result.point aliases for %s disagree: %d and %d",
					semantic, previous, idx)
			}
			continue
		}
		semanticIndices[semantic] = idx
	}

	for _, required := range []string{"value", "arp", "pa"} {
		if _, ok := semanticIndices[required]; !ok {
			return ps, fmt.Errorf("canon: result.point has no %s index", required)
		}
	}

	indexOwners := make(map[int]string, len(semanticIndices))
	for semantic, idx := range semanticIndices {
		if owner, duplicate := indexOwners[idx]; duplicate {
			return ps, fmt.Errorf(
				"canon: result.point index %d is shared by %s and %s",
				idx, owner, semantic)
		}
		indexOwners[idx] = semantic
	}
	for idx := 0; idx < len(indexOwners); idx++ {
		if _, ok := indexOwners[idx]; !ok {
			return ps, fmt.Errorf("canon: result.point has no field at index %d", idx)
		}
	}

	ps.value = semanticIndices["value"]
	ps.arp = semanticIndices["arp"]
	ps.pa = semanticIndices["pa"]
	if idx, ok := semanticIndices["count"]; ok {
		ps.count = idx
	}
	if idx, ok := semanticIndices["hidden"]; ok {
		ps.hidden = idx
	}
	if ps.hidden >= 0 && ps.count < 0 {
		return ps, fmt.Errorf("canon: result.point has hidden without count")
	}
	ps.width = len(indexOwners)
	return ps, nil
}

func finiteNumber(value any, path string) (float64, error) {
	number, ok := value.(float64)
	if !ok || math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, fmt.Errorf("canon: %s is not a finite number: %v", path, value)
	}
	return number, nil
}

func integer(value any, path string) (int64, error) {
	number, err := finiteNumber(value, path)
	if err != nil {
		return 0, err
	}
	if math.Trunc(number) != number || number < -math.Exp2(63) || number >= math.Exp2(63) {
		return 0, fmt.Errorf("canon: %s is not an int64: %v", path, value)
	}
	return int64(number), nil
}

func nonnegativeInt64(value any, path string) (int64, error) {
	number, err := integer(value, path)
	if err != nil {
		return 0, err
	}
	if number < 0 {
		return 0, fmt.Errorf("canon: %s is negative: %v", path, value)
	}
	return number, nil
}

func schemaIndex(value any, path string) (int, error) {
	number, err := nonnegativeInt64(value, path)
	if err != nil {
		return 0, err
	}
	if uint64(number) > uint64(^uint(0)>>1) {
		return 0, fmt.Errorf("canon: %s exceeds the platform index range: %v", path, value)
	}
	return int(number), nil
}

// EmptyResult recognizes the exact labels/data subshape for a no-match
// json2 result. Other result metadata is allowed.
func EmptyResult(doc map[string]any) bool {
	result, ok := doc["result"].(map[string]any)
	if !ok {
		return false
	}
	labels, ok := result["labels"].([]any)
	if !ok || len(labels) != 1 || labels[0] != "time" {
		return false
	}
	data, ok := result["data"].([]any)
	return ok && len(data) == 0
}

// Columns decodes a json2 document's result payload into per-dimension
// point columns keyed by the dimension label, sorted by time. Timestamps may
// be seconds or milliseconds and are normalized to seconds; point width and
// fields are declared by result.point.
func Columns(doc map[string]any) (map[string][]Pt, error) {
	result, ok := doc["result"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("canon: no result object in document")
	}
	labelsAny, ok := result["labels"].([]any)
	if !ok || len(labelsAny) < 2 {
		return nil, fmt.Errorf("canon: result.labels missing or too short: %v", result["labels"])
	}
	if labelsAny[0] != "time" {
		return nil, fmt.Errorf("canon: result.labels[0] is %v, want time", labelsAny[0])
	}
	labels := make([]string, 0, len(labelsAny)-1)
	labelSet := make(map[string]struct{}, len(labelsAny)-1)
	for i, l := range labelsAny[1:] {
		s, ok := l.(string)
		if !ok || s == "" {
			return nil, fmt.Errorf("canon: result.labels[%d] is not a nonempty string: %v", i+1, l)
		}
		if _, duplicate := labelSet[s]; duplicate {
			return nil, fmt.Errorf("canon: duplicate result label %q", s)
		}
		labelSet[s] = struct{}{}
		labels = append(labels, s)
	}

	rowsAny, ok := result["data"].([]any)
	if !ok {
		return nil, fmt.Errorf("canon: result.data missing")
	}
	ps, err := decodePointSchema(result)
	if err != nil {
		return nil, err
	}

	cols := make(map[string][]Pt, len(labels))
	for rowIndex, rowAny := range rowsAny {
		row, ok := rowAny.([]any)
		if !ok || len(row) != len(labels)+1 {
			return nil, fmt.Errorf("canon: malformed row %v (want time + %d points)", rowAny, len(labels))
		}
		tsec, err := integer(row[0], fmt.Sprintf("result.data[%d][0]", rowIndex))
		if err != nil {
			return nil, err
		}
		// v2/v3 emit seconds; v1-era options emit milliseconds — normalize
		// by magnitude (the fixed 2023 epoch makes the ranges unambiguous)
		if tsec > 1_000_000_000_000 || tsec < -1_000_000_000_000 {
			if tsec%1000 != 0 {
				return nil, fmt.Errorf(
					"canon: result.data[%d][0] millisecond timestamp is not second-aligned: %d",
					rowIndex, tsec)
			}
			tsec /= 1000
		}
		for i, lbl := range labels {
			point, ok := row[1+i].([]any)
			if !ok || len(point) != ps.width {
				return nil, fmt.Errorf(
					"canon: malformed point %v for %s (want exactly %d fields)",
					row[1+i], lbl, ps.width)
			}
			pt := Pt{T: tsec}
			if point[ps.value] != nil {
				v, err := finiteNumber(
					point[ps.value],
					fmt.Sprintf("result.data[%d][%d].value", rowIndex, i+1))
				if err != nil {
					return nil, err
				}
				pt.Value = &v
			}
			pt.ARP, err = finiteNumber(
				point[ps.arp],
				fmt.Sprintf("result.data[%d][%d].arp", rowIndex, i+1))
			if err != nil {
				return nil, err
			}
			pa, err := nonnegativeInt64(
				point[ps.pa],
				fmt.Sprintf("result.data[%d][%d].pa", rowIndex, i+1))
			if err != nil {
				return nil, err
			}
			pt.PA = pa
			if ps.count >= 0 {
				count, err := nonnegativeInt64(
					point[ps.count],
					fmt.Sprintf("result.data[%d][%d].count", rowIndex, i+1))
				if err != nil {
					return nil, err
				}
				pt.Count = &count
			}
			if ps.hidden >= 0 && point[ps.hidden] != nil {
				hidden, err := finiteNumber(
					point[ps.hidden],
					fmt.Sprintf("result.data[%d][%d].hidden", rowIndex, i+1))
				if err != nil {
					return nil, err
				}
				pt.Hidden = &hidden
			}
			cols[lbl] = append(cols[lbl], pt)
		}
	}

	for lbl := range cols {
		slices.SortFunc(cols[lbl], func(a, b Pt) int {
			switch {
			case a.T < b.T:
				return -1
			case a.T > b.T:
				return 1
			default:
				return 0
			}
		})
	}
	return cols, nil
}
