// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"math"
	"testing"

	"github.com/netdata/netdata/tests/query-corpus/canon"
)

const queryCorpusStorageTiers = 3

// expectedColumnPoint is the exact fixture-derived shape of one json2
// dimension point. A nil Value means the row must be null and carry EMPTY.
type expectedColumnPoint struct {
	T     int64
	Value *float64
}

func wantNumberAt(t int64, value float64) expectedColumnPoint {
	return expectedColumnPoint{T: t, Value: &value}
}

func wantEmptyAt(t int64) expectedColumnPoint {
	return expectedColumnPoint{T: t}
}

// assertExactColumn rejects every vacuous-success path: a missing column,
// dropped or duplicate row, timestamp drift, null in place of a number, or
// a number in place of an explicitly expected empty row.
func assertExactColumn(
	t *testing.T,
	cols map[string][]canon.Pt,
	dimension string,
	want []expectedColumnPoint,
	tolerance float64,
) bool {
	t.Helper()

	got, has := cols[dimension]
	if !has {
		t.Logf("dimension %q missing (have %v)", dimension, keys(cols))
		return false
	}

	ok := true
	const maxDetailedFailures = 10
	reported, suppressed := 0, 0
	report := func(format string, args ...any) {
		if reported < maxDetailedFailures {
			t.Logf(format, args...)
			reported++
		} else {
			suppressed++
		}
	}
	if len(got) != len(want) {
		report("dimension %q has %d rows, want exactly %d", dimension, len(got), len(want))
		ok = false
	}

	seen := make(map[int64]int, len(got))
	for i, pt := range got {
		seen[pt.T]++
		if seen[pt.T] > 1 {
			report("dimension %q repeats timestamp %d", dimension, pt.T)
			ok = false
		}

		if i >= len(want) {
			continue
		}
		exp := want[i]
		if pt.T != exp.T {
			report("dimension %q row %d ends at %d, want %d", dimension, i, pt.T, exp.T)
			ok = false
		}

		if exp.Value == nil {
			if pt.Value != nil {
				report("dimension %q row %d at %d is %v, want null", dimension, i, pt.T, *pt.Value)
				ok = false
			}
			if pt.PA&canon.AnnotationEmpty == 0 {
				report("dimension %q row %d at %d is expected empty but PA=%d lacks EMPTY",
					dimension, i, pt.T, pt.PA)
				ok = false
			}
			continue
		}

		if pt.Value == nil {
			report("dimension %q row %d at %d is null, want %v", dimension, i, pt.T, *exp.Value)
			ok = false
			continue
		}
		if pt.PA&canon.AnnotationEmpty != 0 {
			report("dimension %q row %d at %d has numeric value %v but PA=%d includes EMPTY",
				dimension, i, pt.T, *pt.Value, pt.PA)
			ok = false
		}
		if math.IsNaN(*pt.Value) || math.IsInf(*pt.Value, 0) ||
			math.Abs(*pt.Value-*exp.Value) > tolerance {
			report("dimension %q row %d at %d is %v, want %v ± %v",
				dimension, i, pt.T, *pt.Value, *exp.Value, tolerance)
			ok = false
		}
	}
	if suppressed > 0 {
		t.Logf("dimension %q has %d additional exact-row failures not shown", dimension, suppressed)
	}

	return ok
}

func assertOnlyColumn(t *testing.T, cols map[string][]canon.Pt, dimension string) bool {
	t.Helper()

	if len(cols) != 1 {
		t.Logf("result has %d columns, want exactly %q: %v", len(cols), dimension, keys(cols))
		return false
	}
	if _, has := cols[dimension]; !has {
		t.Logf("result has column %v, want exactly %q", keys(cols), dimension)
		return false
	}
	return true
}

// assertExactView checks an unaligned multi-row json2 view. The request's
// lower bound is exclusive, while view.after is the first selected second.
func assertExactView(t *testing.T, doc map[string]any, after, before, updateEvery int64) bool {
	t.Helper()

	view, ok := doc["view"].(map[string]any)
	if !ok {
		t.Logf("response has no view object")
		return false
	}
	gotAfter, afterOK := view["after"].(float64)
	gotBefore, beforeOK := view["before"].(float64)
	gotEvery, everyOK := view["update_every"].(float64)
	viewAfter := after + 1
	if !afterOK || !beforeOK || !everyOK ||
		gotAfter != float64(viewAfter) || gotBefore != float64(before) ||
		gotEvery != float64(updateEvery) {
		t.Logf("view after=%v before=%v update_every=%v, want %d/%d/%d",
			view["after"], view["before"], view["update_every"],
			viewAfter, before, updateEvery)
		return false
	}
	return true
}

func queryObject(t *testing.T, parent map[string]any, key, path string) map[string]any {
	t.Helper()
	value, ok := parent[key].(map[string]any)
	if !ok {
		t.Fatalf("%s is missing or not an object: %v", path, parent[key])
	}
	return value
}

func queryStrictOneUnit(t *testing.T, value any, path string) string {
	t.Helper()

	switch units := value.(type) {
	case string:
		if units == "" {
			t.Fatalf("%s is empty", path)
		}
		return units
	case []any:
		if len(units) != 1 {
			t.Fatalf("%s has %d values, want exactly one: %v", path, len(units), units)
		}
		unit, ok := units[0].(string)
		if !ok || unit == "" {
			t.Fatalf("%s[0] is empty or not a string: %v", path, units[0])
		}
		return unit
	default:
		t.Fatalf("%s is missing or has invalid type %T: %v", path, value, value)
		return ""
	}
}

func queryStrictDimensionUnit(t *testing.T, section map[string]any, path string) string {
	t.Helper()
	dimensions := queryObject(t, section, "dimensions", path+".dimensions")
	return queryStrictOneUnit(t, dimensions["units"], path+".dimensions.units")
}

// assertColumnShapeAndTotal rejects missing, empty, duplicate or time-shifted
// rows, then checks the exact sum of their numeric answers. Use this only when
// row placement is genuinely not part of the contract.
func assertColumnShapeAndTotal(
	t *testing.T,
	cols map[string][]canon.Pt,
	dimension string,
	after, before, rowSpan int64,
	wantTotal float64,
) bool {
	t.Helper()

	col, has := cols[dimension]
	if !has {
		t.Logf("dimension %q missing", dimension)
		return false
	}

	points := int((before - after) / rowSpan)
	ok := true
	const maxDetailedFailures = 10
	reported, suppressed := 0, 0
	report := func(format string, args ...any) {
		if reported < maxDetailedFailures {
			t.Logf(format, args...)
			reported++
		} else {
			suppressed++
		}
	}
	if len(col) != points {
		report("dimension %q has %d rows, want exactly %d", dimension, len(col), points)
		ok = false
	}

	total := 0.0
	seen := make(map[int64]bool, len(col))
	for i, point := range col {
		if seen[point.T] {
			report("dimension %q repeats timestamp %d", dimension, point.T)
			ok = false
		}
		seen[point.T] = true

		if i < points {
			wantT := after + int64(i+1)*rowSpan
			if point.T != wantT {
				report("dimension %q row %d ends at %d, want %d", dimension, i, point.T, wantT)
				ok = false
			}
		}
		if point.Value == nil {
			report("dimension %q row %d at %d is null", dimension, i, point.T)
			ok = false
			continue
		}
		if point.PA&canon.AnnotationEmpty != 0 {
			report("dimension %q row %d at %d is numeric but marked EMPTY", dimension, i, point.T)
			ok = false
		}
		if math.IsNaN(*point.Value) || math.IsInf(*point.Value, 0) {
			report("dimension %q row %d at %d is %v", dimension, i, point.T, *point.Value)
			ok = false
			continue
		}
		total += *point.Value
	}

	if total != wantTotal {
		t.Logf("dimension %q totals %.12g, want exactly %.12g", dimension, total, wantTotal)
		ok = false
	}
	if suppressed > 0 {
		t.Logf("dimension %q has %d additional row-shape failures not shown", dimension, suppressed)
	}
	return ok
}

// assertEventColumnShapeAndTotal validates an event-count series without
// inventing where an event inside a rolled-up storage window occurred. Every
// row must be a whole count within the fixture-derived maximum; the total pins
// loss and duplication across the query.
func assertEventColumnShapeAndTotal(
	t *testing.T,
	cols map[string][]canon.Pt,
	dimension string,
	after, before, rowSpan int64,
	maxEventsPerRow int64,
	wantTotal float64,
) bool {
	t.Helper()

	if maxEventsPerRow < 0 {
		t.Fatalf("event-count maximum cannot be negative: %d", maxEventsPerRow)
	}
	if !assertColumnShapeAndTotal(t, cols, dimension, after, before, rowSpan, wantTotal) {
		return false
	}

	ok := true
	for i, point := range cols[dimension] {
		if point.Value == nil {
			continue
		}
		if *point.Value < 0 || *point.Value > float64(maxEventsPerRow) ||
			math.Trunc(*point.Value) != *point.Value {
			t.Logf("dimension %q row %d at %d is %v, want a whole event count from 0 through %d",
				dimension, i, point.T, *point.Value, maxEventsPerRow)
			ok = false
		}
	}
	return ok
}

func strictTierPoints(t *testing.T, doc map[string]any) (map[int]int64, bool) {
	t.Helper()

	db, ok := doc["db"].(map[string]any)
	if !ok {
		t.Logf("response has no db object")
		return nil, false
	}
	perTier, ok := db["per_tier"].([]any)
	if !ok || len(perTier) == 0 {
		t.Logf("db.per_tier is missing or empty: %v", db["per_tier"])
		return nil, false
	}

	points := make(map[int]int64, len(perTier))
	valid := true
	for i, raw := range perTier {
		entry, ok := raw.(map[string]any)
		if !ok {
			t.Logf("db.per_tier[%d] is malformed: %v", i, raw)
			valid = false
			continue
		}

		tierRaw, tierOK := entry["tier"].(float64)
		pointsRaw, pointsOK := entry["points"].(float64)
		tier := int(tierRaw)
		count := int64(pointsRaw)
		if !tierOK || tier < 0 || tierRaw != float64(tier) ||
			!pointsOK || pointsRaw < 0 || pointsRaw != float64(count) {
			t.Logf("db.per_tier[%d] has invalid tier/points: %v", i, entry)
			valid = false
			continue
		}
		if _, seen := points[tier]; seen {
			t.Logf("db.per_tier repeats tier %d", tier)
			valid = false
			continue
		}
		points[tier] = count
	}
	return points, valid
}

func assertTierPresence(t *testing.T, doc map[string]any, want []bool) bool {
	t.Helper()

	points, ok := strictTierPoints(t, doc)
	if len(points) != len(want) {
		t.Logf("db.per_tier has tiers %v, want exactly tiers 0..%d", points, len(want)-1)
		ok = false
	}
	for tier, expected := range want {
		count, has := points[tier]
		if !has {
			t.Logf("db.per_tier is missing tier %d", tier)
			ok = false
			continue
		}
		if present := count > 0; present != expected {
			t.Logf("tier%d contributed=%v, want %v (per_tier %v)", tier, present, expected, points)
			ok = false
		}
	}
	return ok
}

// assertSelectedTier proves that a forced-tier query was actually served by
// that tier. The selected tier must read points and every other tier must read
// none; malformed, missing, or duplicate per_tier entries fail.
func assertSelectedTier(t *testing.T, doc map[string]any, selected int) bool {
	t.Helper()

	points, valid := strictTierPoints(t, doc)
	if len(points) != queryCorpusStorageTiers {
		t.Logf("db.per_tier has tiers %v, want exactly tiers 0..%d",
			points, queryCorpusStorageTiers-1)
		valid = false
	}
	for tier := 0; tier < queryCorpusStorageTiers; tier++ {
		if _, has := points[tier]; !has {
			t.Logf("db.per_tier is missing configured tier %d", tier)
			valid = false
		}
	}
	selectedPoints, hasSelected := points[selected]
	if !hasSelected {
		t.Logf("forced tier %d is absent from db.per_tier", selected)
		valid = false
	}
	if selectedPoints <= 0 {
		t.Logf("forced tier %d read %d points", selected, selectedPoints)
		valid = false
	}
	for tier, count := range points {
		if tier != selected && count != 0 {
			t.Logf("forced tier %d also read %d points from tier %d", selected, count, tier)
			valid = false
		}
	}
	return valid
}

func TestQueryAssertionGuardsDetectMutations(t *testing.T) {
	number := 7.0
	want := []expectedColumnPoint{
		wantNumberAt(10, number),
		wantEmptyAt(20),
	}
	validColumn := []canon.Pt{
		{T: 10, Value: &number},
		{T: 20, PA: canon.AnnotationEmpty},
	}
	if !assertExactColumn(t, map[string][]canon.Pt{"value": validColumn}, "value", want, 0) {
		t.Fatal("exact-column guard rejected its valid control")
	}
	if !assertOnlyColumn(t, map[string][]canon.Pt{"value": validColumn}, "value") {
		t.Fatal("only-column guard rejected its valid control")
	}
	five := 5.0
	validEvents := map[string][]canon.Pt{
		"events": {{T: 300, Value: &five}, {T: 600, Value: &five}},
	}
	if !assertEventColumnShapeAndTotal(t, validEvents, "events", 0, 600, 300, 5, 10) {
		t.Fatal("event-count guard rejected its valid control")
	}
	validView := map[string]any{"view": map[string]any{
		"after":        float64(1),
		"before":       float64(20),
		"update_every": float64(10),
	}}
	if !assertExactView(t, validView, 0, 20, 10) {
		t.Fatal("exact-view guard rejected its valid control")
	}

	columnMutations := map[string]map[string][]canon.Pt{
		"missing-column": {},
		"missing-row": {
			"value": validColumn[:1],
		},
		"extra-row": {
			"value": append(append([]canon.Pt{}, validColumn...), canon.Pt{T: 30, Value: &number}),
		},
		"duplicate-timestamp": {
			"value": {{T: 10, Value: &number}, {T: 10, PA: canon.AnnotationEmpty}},
		},
		"shifted-timestamp": {
			"value": {{T: 11, Value: &number}, {T: 20, PA: canon.AnnotationEmpty}},
		},
		"null-for-number": {
			"value": {{T: 10, PA: canon.AnnotationEmpty}, {T: 20, PA: canon.AnnotationEmpty}},
		},
		"number-for-null": {
			"value": {{T: 10, Value: &number}, {T: 20, Value: &number}},
		},
	}
	nan := math.NaN()
	columnMutations["non-finite-number"] = map[string][]canon.Pt{
		"value": {{T: 10, Value: &nan}, {T: 20, PA: canon.AnnotationEmpty}},
	}
	for name, columns := range columnMutations {
		t.Run("column/"+name, func(t *testing.T) {
			if assertExactColumn(t, columns, "value", want, 0) {
				t.Errorf("exact-column guard accepted the %s mutation", name)
			}
		})
	}
	t.Run("column/extra-column", func(t *testing.T) {
		if assertOnlyColumn(t, map[string][]canon.Pt{
			"value": validColumn,
			"leak":  validColumn,
		}, "value") {
			t.Error("only-column guard accepted an extra result column")
		}
	})
	t.Run("column/balanced-event-duplication", func(t *testing.T) {
		six, four := 6.0, 4.0
		if assertEventColumnShapeAndTotal(t, map[string][]canon.Pt{
			"events": {{T: 300, Value: &six}, {T: 600, Value: &four}},
		}, "events", 0, 600, 300, 5, 10) {
			t.Error("event-count guard accepted balanced duplication and loss")
		}
	})
	for name, view := range map[string]map[string]any{
		"fractional-after": {
			"view": map[string]any{
				"after":        1.5,
				"before":       float64(20),
				"update_every": float64(10),
			},
		},
		"fractional-before": {
			"view": map[string]any{
				"after":        float64(1),
				"before":       20.5,
				"update_every": float64(10),
			},
		},
		"fractional-update-every": {
			"view": map[string]any{
				"after":        float64(1),
				"before":       float64(20),
				"update_every": 10.5,
			},
		},
	} {
		t.Run("view/"+name, func(t *testing.T) {
			if assertExactView(t, view, 0, 20, 10) {
				t.Errorf("exact-view guard accepted the %s mutation", name)
			}
		})
	}

	tierEntry := func(tier, points float64) map[string]any {
		return map[string]any{"tier": tier, "points": points}
	}
	tierDocument := func(entries ...any) map[string]any {
		return map[string]any{"db": map[string]any{"per_tier": entries}}
	}
	validTiers := tierDocument(
		tierEntry(0, 4),
		tierEntry(1, 0),
		tierEntry(2, 0),
	)
	if !assertSelectedTier(t, validTiers, 0) {
		t.Fatal("selected-tier guard rejected its valid control")
	}

	tierMutations := map[string]map[string]any{
		"missing-configured-tier": tierDocument(
			tierEntry(0, 4),
			tierEntry(1, 0),
		),
		"selected-tier-empty": tierDocument(
			tierEntry(0, 0),
			tierEntry(1, 0),
			tierEntry(2, 0),
		),
		"other-tier-contributed": tierDocument(
			tierEntry(0, 4),
			tierEntry(1, 1),
			tierEntry(2, 0),
		),
		"duplicate-tier": tierDocument(
			tierEntry(0, 4),
			tierEntry(0, 0),
			tierEntry(2, 0),
		),
		"malformed-tier": tierDocument(
			map[string]any{"tier": "0", "points": float64(4)},
			tierEntry(1, 0),
			tierEntry(2, 0),
		),
	}
	for name, document := range tierMutations {
		t.Run("tier/"+name, func(t *testing.T) {
			if assertSelectedTier(t, document, 0) {
				t.Errorf("selected-tier guard accepted the %s mutation", name)
			}
		})
	}
}
