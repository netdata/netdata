// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"fmt"
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
	ARP   *float64
	PA    *int64
}

func wantNumberAt(t int64, value float64) expectedColumnPoint {
	return expectedColumnPoint{T: t, Value: &value}
}

func wantNumberWithARPAt(t int64, value, arp float64) expectedColumnPoint {
	return expectedColumnPoint{T: t, Value: &value, ARP: &arp}
}

func wantNumberWithPAAt(t int64, value float64, pa int64) expectedColumnPoint {
	return expectedColumnPoint{T: t, Value: &value, PA: &pa}
}

func wantEmptyAt(t int64) expectedColumnPoint {
	return expectedColumnPoint{T: t}
}

func wantNumberWithMetadataAt(t int64, value, arp float64, pa int64) expectedColumnPoint {
	return expectedColumnPoint{T: t, Value: &value, ARP: &arp, PA: &pa}
}

func wantEmptyWithMetadataAt(t int64, arp float64, pa int64) expectedColumnPoint {
	return expectedColumnPoint{T: t, ARP: &arp, PA: &pa}
}

type exactColumnFields uint8

const (
	exactColumnValues exactColumnFields = 1 << iota
	exactColumnMetadata
	exactColumnAll = exactColumnValues | exactColumnMetadata
)

// assertExactColumnFields rejects every vacuous-success path while allowing
// independently actionable value and metadata contracts to be evaluated
// without one hiding the other.
func assertExactColumnFields(
	t *testing.T,
	cols map[string][]canon.Pt,
	dimension string,
	want []expectedColumnPoint,
	tolerance float64,
	fields exactColumnFields,
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
		if fields&exactColumnMetadata != 0 && exp.ARP != nil &&
			(math.IsNaN(pt.ARP) || math.IsInf(pt.ARP, 0) || pt.ARP != *exp.ARP) {
			report("dimension %q row %d at %d has ARP %v, want exactly %v",
				dimension, i, pt.T, pt.ARP, *exp.ARP)
			ok = false
		}
		if fields&exactColumnMetadata != 0 && exp.PA != nil && pt.PA != *exp.PA {
			report("dimension %q row %d at %d has PA %d, want exactly %d",
				dimension, i, pt.T, pt.PA, *exp.PA)
			ok = false
		}

		if fields&exactColumnValues == 0 {
			continue
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
		if pt.PA&canon.AnnotationEmpty != 0 &&
			(exp.PA == nil || *exp.PA&canon.AnnotationEmpty == 0) {
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

// assertExactColumn rejects every vacuous-success path and compares values
// plus every explicitly requested metadata field.
func assertExactColumn(
	t *testing.T,
	cols map[string][]canon.Pt,
	dimension string,
	want []expectedColumnPoint,
	tolerance float64,
) bool {
	t.Helper()
	return assertExactColumnFields(t, cols, dimension, want, tolerance, exactColumnAll)
}

func assertExactColumnValues(
	t *testing.T,
	cols map[string][]canon.Pt,
	dimension string,
	want []expectedColumnPoint,
	tolerance float64,
) bool {
	t.Helper()
	return assertExactColumnFields(t, cols, dimension, want, tolerance, exactColumnValues)
}

func assertExactColumnMetadata(
	t *testing.T,
	cols map[string][]canon.Pt,
	dimension string,
	want []expectedColumnPoint,
) bool {
	t.Helper()
	return assertExactColumnFields(t, cols, dimension, want, 0, exactColumnMetadata)
}

func assertOnlyColumn(t *testing.T, cols map[string][]canon.Pt, dimension string) bool {
	t.Helper()
	return assertExactColumnSet(t, cols, []string{dimension})
}

func assertExactColumnSet(t *testing.T, cols map[string][]canon.Pt, dimensions []string) bool {
	t.Helper()

	wanted := make(map[string]struct{}, len(dimensions))
	for _, dimension := range dimensions {
		if dimension == "" {
			t.Fatal("exact column set contains an empty dimension")
		}
		if _, duplicate := wanted[dimension]; duplicate {
			t.Fatalf("exact column set repeats %q", dimension)
		}
		wanted[dimension] = struct{}{}
	}

	ok := true
	if len(cols) != len(wanted) {
		t.Logf("result has %d columns, want exactly %d: got %v want %v",
			len(cols), len(wanted), keys(cols), dimensions)
		ok = false
	}
	for dimension := range wanted {
		if _, has := cols[dimension]; !has {
			t.Logf("result is missing column %q (have %v)", dimension, keys(cols))
			ok = false
		}
	}
	for dimension := range cols {
		if _, expected := wanted[dimension]; !expected {
			t.Logf("result has unexpected column %q", dimension)
			ok = false
		}
	}
	return ok
}

// assertExactView checks an unaligned multi-row json2 view. The request's
// lower bound is exclusive, while view.after is the first selected second.
func assertExactView(t *testing.T, doc map[string]any, after, before, updateEvery int64) bool {
	t.Helper()
	return assertViewFields(t, doc, after+1, before, updateEvery)
}

func assertViewFields(t *testing.T, doc map[string]any, after, before, updateEvery int64) bool {
	t.Helper()
	view, ok := doc["view"].(map[string]any)
	if !ok {
		t.Logf("response has no view object")
		return false
	}
	gotAfter, afterOK := view["after"].(float64)
	gotBefore, beforeOK := view["before"].(float64)
	gotEvery, everyOK := view["update_every"].(float64)
	if !afterOK || !beforeOK || !everyOK ||
		gotAfter != float64(after) || gotBefore != float64(before) ||
		gotEvery != float64(updateEvery) {
		t.Logf("view after=%v before=%v update_every=%v, want %d/%d/%d",
			view["after"], view["before"], view["update_every"],
			after, before, updateEvery)
		return false
	}
	return true
}

type queryExpectedGrid struct {
	after, before, updateEvery int64
	rows                       int
}

// queryExpectedVirtualGrid is an independent port of the explicit absolute,
// one-second-granularity query-window path. It pins the public time geometry;
// values and storage availability cannot influence this result.
//
// Source: netdata/netdata @ 89a2855db958400528ebd996e8869564c9c20862
// src/web/api/queries/query-window.c:214-269,297-302,333-364
// query_target_calculate_window()
func queryExpectedVirtualGrid(
	t *testing.T,
	after, before, requestedPoints int64,
	aligned bool,
) queryExpectedGrid {
	t.Helper()

	duration := before - after
	if duration <= 0 || requestedPoints <= 0 {
		t.Fatalf("unsupported virtual-grid fixture: after=%d before=%d points=%d",
			after, before, requestedPoints)
	}

	// At one-second query granularity both endpoint seconds participate in
	// layout arithmetic. Required coverage remains the requested duration.
	available := duration + 1
	points := requestedPoints
	if points > available {
		points = available
	}
	if points > 86400 {
		points = 86400
	}
	group := available / points
	if group == 0 {
		group = 1
	}
	if available%points > points/2 {
		group++
	}
	if points*group < duration {
		points = (available + group - 1) / group
	}

	viewBefore := before
	if aligned {
		if remainder := viewBefore % group; remainder != 0 {
			viewBefore += group - remainder
		}
	}
	viewAfter := viewBefore - ((points-1)*group + group - 1)
	return queryExpectedGrid{
		after:       viewAfter,
		before:      viewBefore,
		updateEvery: group,
		rows:        int(points),
	}
}

// queryTimestampGridExact validates the public view geometry and raw wire
// timestamps. It intentionally does not decode or compare values.
func queryTimestampGridExact(t *testing.T, doc map[string]any, want queryExpectedGrid) bool {
	t.Helper()

	ok := assertViewFields(t, doc, want.after, want.before, want.updateEvery)
	wantWire := make([]int64, want.rows)
	for i := range wantWire {
		wantWire[i] = want.before - int64(i)*want.updateEvery
	}
	if err := queryRawTimestampsExact(doc, wantWire); err != nil {
		t.Logf("timestamp grid: %v", err)
		ok = false
	}
	return ok
}

func queryObject(t *testing.T, parent map[string]any, key, path string) map[string]any {
	t.Helper()
	value, ok := parent[key].(map[string]any)
	if !ok {
		t.Fatalf("%s is missing or not an object: %v", path, parent[key])
	}
	return value
}

func queryFiniteNumber(value any) (float64, bool) {
	number, ok := value.(float64)
	return number, ok && !math.IsNaN(number) && !math.IsInf(number, 0)
}

func queryInteger(value any) (int64, bool) {
	number, ok := queryFiniteNumber(value)
	if !ok || math.Trunc(number) != number ||
		number < -math.Exp2(63) || number >= math.Exp2(63) {
		return 0, false
	}
	return int64(number), true
}

// queryPointSchemaField verifies response-wide json2 schema presence. A nil
// decoded cell cannot answer this because both an absent field and a declared
// null cell intentionally decode to nil.
func queryPointSchemaField(doc map[string]any, field string, wantPresent bool) error {
	result, ok := doc["result"].(map[string]any)
	if !ok {
		return fmt.Errorf("result is missing or not an object: %v", doc["result"])
	}
	point, ok := result["point"].(map[string]any)
	if !ok {
		return fmt.Errorf("result.point is missing or not an object: %v", result["point"])
	}
	_, present := point[field]
	if present != wantPresent {
		return fmt.Errorf("result.point field %q presence is %v, want %v", field, present, wantPresent)
	}
	return nil
}

// queryRawTimestampsExact validates result.data before canon.Columns sorts the
// decoded columns. The order of want is therefore the public wire order.
func queryRawTimestampsExact(doc map[string]any, want []int64) error {
	result, ok := doc["result"].(map[string]any)
	if !ok {
		return fmt.Errorf("result is missing or not an object: %v", doc["result"])
	}
	rows, ok := result["data"].([]any)
	if !ok {
		return fmt.Errorf("result.data is missing or not an array: %v", result["data"])
	}
	if len(rows) != len(want) {
		return fmt.Errorf("result.data has %d rows, want %d", len(rows), len(want))
	}
	for i, rowAny := range rows {
		row, ok := rowAny.([]any)
		if !ok || len(row) == 0 {
			return fmt.Errorf("result.data[%d] is not a nonempty row: %v", i, rowAny)
		}
		timestamp, ok := queryInteger(row[0])
		if !ok || timestamp != want[i] {
			return fmt.Errorf(
				"result.data[%d] timestamp is %v, want %d in wire order",
				i, row[0], want[i])
		}
	}
	return nil
}

func TestQueryStructuredResponseGuards(t *testing.T) {
	t.Run("point-schema-presence", func(t *testing.T) {
		withHidden := func() map[string]any {
			return map[string]any{"result": map[string]any{
				"point": map[string]any{
					"value": float64(0), "arp": float64(1), "pa": float64(2),
					"count": float64(3), "hidden": float64(4),
				},
				"data": []any{
					[]any{float64(1), []any{float64(2), float64(0), float64(0), float64(1), nil}},
				},
			}}
		}

		control := withHidden()
		if err := queryPointSchemaField(control, "hidden", true); err != nil {
			t.Fatalf("valid hidden schema rejected: %v", err)
		}

		removed := withHidden()
		result := removed["result"].(map[string]any)
		delete(result["point"].(map[string]any), "hidden")
		row := result["data"].([]any)[0].([]any)
		row[1] = row[1].([]any)[:4]
		if err := queryPointSchemaField(removed, "hidden", true); err == nil {
			t.Fatal("accepted coherent hidden schema-and-cell removal")
		}

		absent := withHidden()
		absentResult := absent["result"].(map[string]any)
		delete(absentResult["point"].(map[string]any), "hidden")
		absentRow := absentResult["data"].([]any)[0].([]any)
		absentRow[1] = absentRow[1].([]any)[:4]
		if err := queryPointSchemaField(absent, "hidden", false); err != nil {
			t.Fatalf("valid absent hidden schema rejected: %v", err)
		}
		absentResult["point"].(map[string]any)["hidden"] = float64(4)
		absentRow[1] = append(absentRow[1].([]any), nil)
		if err := queryPointSchemaField(absent, "hidden", false); err == nil {
			t.Fatal("accepted coherent hidden schema-and-null-cell addition")
		}
	})

	t.Run("raw-wire-order", func(t *testing.T) {
		build := func() map[string]any {
			return map[string]any{"result": map[string]any{"data": []any{
				[]any{float64(30), []any{float64(3)}},
				[]any{float64(20), []any{float64(2)}},
				[]any{float64(10), []any{float64(1)}},
			}}}
		}
		want := []int64{30, 20, 10}
		if err := queryRawTimestampsExact(build(), want); err != nil {
			t.Fatalf("valid raw wire order rejected: %v", err)
		}
		for name, mutate := range map[string]func([]any){
			"adjacent-swap": func(rows []any) { rows[0], rows[1] = rows[1], rows[0] },
			"reverse":       func(rows []any) { rows[0], rows[2] = rows[2], rows[0] },
		} {
			t.Run(name, func(t *testing.T) {
				doc := build()
				mutate(doc["result"].(map[string]any)["data"].([]any))
				if err := queryRawTimestampsExact(doc, want); err == nil {
					t.Errorf("accepted %s raw row mutation", name)
				}
			})
		}
	})
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
	return assertColumnShapeAndTotalRange(
		t, cols, dimension, after, before, rowSpan,
		columnTotalBounds{min: wantTotal, max: wantTotal})
}

type columnTotalBounds struct {
	min float64
	max float64
}

// assertColumnExactGrid validates only row identity and placement. Values may
// be null so a forced-tier control can prove its complete response grid even
// when part of the requested window predates that tier's retention.
func assertColumnExactGrid(
	t *testing.T,
	cols map[string][]canon.Pt,
	dimension string,
	after, before, rowSpan int64,
) bool {
	t.Helper()

	if rowSpan <= 0 || before < after || (before-after)%rowSpan != 0 {
		t.Fatalf("invalid exact grid (%d,%d] at row span %d", after, before, rowSpan)
	}
	if before == after {
		t.Logf("exact grid (%d,%d] has zero rows", after, before)
		return false
	}

	col, has := cols[dimension]
	if !has {
		t.Logf("dimension %q missing", dimension)
		return false
	}

	points := int((before - after) / rowSpan)
	ok := true
	if len(col) != points {
		t.Logf("dimension %q has %d rows, want exactly %d", dimension, len(col), points)
		ok = false
	}

	seen := make(map[int64]bool, len(col))
	for i, point := range col {
		if seen[point.T] {
			t.Logf("dimension %q repeats timestamp %d", dimension, point.T)
			ok = false
		}
		seen[point.T] = true

		if i < points {
			wantT := after + int64(i+1)*rowSpan
			if point.T != wantT {
				t.Logf("dimension %q row %d ends at %d, want %d", dimension, i, point.T, wantT)
				ok = false
			}
		}
	}
	return ok
}

// assertColumnShapeAndTotalRange is the bounded-approximation counterpart of
// assertColumnShapeAndTotal. Row shape remains exact; only the total may vary
// within the explicitly documented inclusive range.
func assertColumnShapeAndTotalRange(
	t *testing.T,
	cols map[string][]canon.Pt,
	dimension string,
	after, before, rowSpan int64,
	bounds columnTotalBounds,
) bool {
	t.Helper()

	if bounds.min > bounds.max {
		t.Fatalf("invalid total range %.12g through %.12g", bounds.min, bounds.max)
	}
	if !assertColumnExactGrid(t, cols, dimension, after, before, rowSpan) {
		return false
	}

	col, has := cols[dimension]
	if !has {
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
	total := 0.0
	for i, point := range col {
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

	if total < bounds.min || total > bounds.max {
		if bounds.min == bounds.max {
			t.Logf("dimension %q totals %.12g, want exactly %.12g", dimension, total, bounds.min)
		} else {
			t.Logf("dimension %q totals %.12g, want %.12g through %.12g",
				dimension, total, bounds.min, bounds.max)
		}
		ok = false
	}
	if suppressed > 0 {
		t.Logf("dimension %q has %d additional row-shape failures not shown", dimension, suppressed)
	}
	return ok
}

type eventColumnContract struct {
	after           int64
	before          int64
	rowSpan         int64
	maxEventsPerRow int64
	total           columnTotalBounds
}

// assertEventColumnContract validates an event-count series without inventing
// where an event inside a rolled-up storage window occurred. Every row must be
// a whole count within the fixture-derived maximum; the total stays within the
// contract's explicit inclusive bound.
func assertEventColumnContract(
	t *testing.T,
	cols map[string][]canon.Pt,
	dimension string,
	contract eventColumnContract,
) bool {
	t.Helper()

	if contract.maxEventsPerRow < 0 {
		t.Fatalf("event-count maximum cannot be negative: %d", contract.maxEventsPerRow)
	}
	if !assertColumnShapeAndTotalRange(
		t, cols, dimension, contract.after, contract.before, contract.rowSpan, contract.total) {
		return false
	}

	ok := true
	for i, point := range cols[dimension] {
		if point.Value == nil {
			continue
		}
		if *point.Value < 0 || *point.Value > float64(contract.maxEventsPerRow) ||
			math.Trunc(*point.Value) != *point.Value {
			t.Logf("dimension %q row %d at %d is %v, want a whole event count from 0 through %d",
				dimension, i, point.T, *point.Value, contract.maxEventsPerRow)
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
	if !assertExactColumnSet(t, map[string][]canon.Pt{
		"value": validColumn,
		"other": validColumn,
	}, []string{"value", "other"}) {
		t.Fatal("exact-column-set guard rejected its valid control")
	}
	metadataWant := []expectedColumnPoint{
		wantNumberWithMetadataAt(10, number, 25, canon.AnnotationPartial),
		wantEmptyWithMetadataAt(20, 0, canon.AnnotationEmpty),
	}
	metadataColumn := []canon.Pt{
		{T: 10, Value: &number, ARP: 25, PA: canon.AnnotationPartial},
		{T: 20, PA: canon.AnnotationEmpty},
	}
	if !assertExactColumn(t, map[string][]canon.Pt{"value": metadataColumn}, "value", metadataWant, 0) {
		t.Fatal("exact metadata guard rejected its valid control")
	}
	wrongMetadata := []canon.Pt{
		{T: 10, Value: &number, ARP: 50},
		{T: 20, PA: canon.AnnotationEmpty},
	}
	if !assertExactColumnValues(t, map[string][]canon.Pt{"value": wrongMetadata}, "value", metadataWant, 0) {
		t.Fatal("value-only guard rejected correct values with independently wrong metadata")
	}
	if assertExactColumnMetadata(t, map[string][]canon.Pt{"value": wrongMetadata}, "value", metadataWant) {
		t.Fatal("metadata-only guard accepted wrong metadata")
	}
	wrongValue := 8.0
	wrongValues := []canon.Pt{
		{T: 10, Value: &wrongValue, ARP: 25, PA: canon.AnnotationPartial},
		{T: 20, PA: canon.AnnotationEmpty},
	}
	if assertExactColumnValues(t, map[string][]canon.Pt{"value": wrongValues}, "value", metadataWant, 0) {
		t.Fatal("value-only guard accepted a wrong value")
	}
	if !assertExactColumnMetadata(t, map[string][]canon.Pt{"value": wrongValues}, "value", metadataWant) {
		t.Fatal("metadata-only guard rejected correct metadata with an independently wrong value")
	}
	numericEmpty := map[string][]canon.Pt{
		"value": {{T: 10, Value: &number, PA: canon.AnnotationEmpty}},
	}
	if !assertExactColumn(t, numericEmpty, "value", []expectedColumnPoint{
		wantNumberWithPAAt(10, number, canon.AnnotationEmpty),
	}, 0) {
		t.Fatal("exact metadata guard rejected an explicitly expected null2zero annotation")
	}
	if assertColumnShapeAndTotal(
		t, map[string][]canon.Pt{"value": {}}, "value", 0, 0, 1, 0,
	) {
		t.Error("shape-and-total guard accepted a zero-row request")
	}
	five := 5.0
	validEvents := map[string][]canon.Pt{
		"events": {{T: 300, Value: &five}, {T: 600, Value: &five}},
	}
	exactEvents := eventColumnContract{
		after: 0, before: 600, rowSpan: 300, maxEventsPerRow: 5,
		total: columnTotalBounds{min: 10, max: 10},
	}
	if !assertEventColumnContract(t, validEvents, "events", exactEvents) {
		t.Fatal("event-count guard rejected its valid control")
	}
	boundedEvents := eventColumnContract{
		after: 0, before: 600, rowSpan: 300, maxEventsPerRow: 5,
		total: columnTotalBounds{min: 10, max: 11},
	}
	if !assertEventColumnContract(t, validEvents, "events", boundedEvents) {
		t.Fatal("bounded event-count guard rejected its lower-bound control")
	}
	six := 6.0
	boundedUpper := map[string][]canon.Pt{
		"events": {{T: 300, Value: &six}, {T: 600, Value: &five}},
	}
	boundedEvents.maxEventsPerRow = 6
	if !assertEventColumnContract(t, boundedUpper, "events", boundedEvents) {
		t.Fatal("bounded event-count guard rejected its upper-bound control")
	}
	validView := map[string]any{"view": map[string]any{
		"after":        float64(1),
		"before":       float64(20),
		"update_every": float64(10),
	}}
	if !assertExactView(t, validView, 0, 20, 10) {
		t.Fatal("exact-view guard rejected its valid control")
	}
	if !assertViewFields(t, validView, 1, 20, 10) {
		t.Fatal("explicit-view guard rejected its valid control")
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
	for name, column := range map[string][]canon.Pt{
		"wrong-arp": {
			{T: 10, Value: &number, ARP: 50, PA: canon.AnnotationPartial},
			{T: 20, PA: canon.AnnotationEmpty},
		},
		"wrong-numeric-pa": {
			{T: 10, Value: &number, ARP: 25, PA: 0},
			{T: 20, PA: canon.AnnotationEmpty},
		},
		"extra-empty-pa": {
			{T: 10, Value: &number, ARP: 25, PA: canon.AnnotationPartial},
			{T: 20, PA: canon.AnnotationEmpty | canon.AnnotationReset},
		},
	} {
		t.Run("column/metadata-"+name, func(t *testing.T) {
			if assertExactColumn(
				t, map[string][]canon.Pt{"value": column}, "value", metadataWant, 0) {
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
	t.Run("column/set-missing-column", func(t *testing.T) {
		if assertExactColumnSet(t, map[string][]canon.Pt{
			"value": validColumn,
		}, []string{"value", "other"}) {
			t.Error("exact-column-set guard accepted a missing result column")
		}
	})
	t.Run("column/set-extra-column", func(t *testing.T) {
		if assertExactColumnSet(t, map[string][]canon.Pt{
			"value": validColumn,
			"other": validColumn,
			"leak":  validColumn,
		}, []string{"value", "other"}) {
			t.Error("exact-column-set guard accepted an extra result column")
		}
	})
	t.Run("column/balanced-event-duplication", func(t *testing.T) {
		four := 4.0
		if assertEventColumnContract(t, map[string][]canon.Pt{
			"events": {{T: 300, Value: &six}, {T: 600, Value: &four}},
		}, "events", exactEvents) {
			t.Error("event-count guard accepted balanced duplication and loss")
		}
	})
	t.Run("column/event-total-below-bound", func(t *testing.T) {
		four := 4.0
		if assertEventColumnContract(t, map[string][]canon.Pt{
			"events": {{T: 300, Value: &five}, {T: 600, Value: &four}},
		}, "events", boundedEvents) {
			t.Error("bounded event-count guard accepted a total below its lower bound")
		}
	})
	t.Run("column/event-total-above-bound", func(t *testing.T) {
		seven := 7.0
		aboveBound := boundedEvents
		aboveBound.maxEventsPerRow = 7
		if assertEventColumnContract(t, map[string][]canon.Pt{
			"events": {{T: 300, Value: &seven}, {T: 600, Value: &five}},
		}, "events", aboveBound) {
			t.Error("bounded event-count guard accepted a total above its upper bound")
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
		"missing-db": {},
		"missing-per-tier": {
			"db": map[string]any{},
		},
		"empty-per-tier": tierDocument(),
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
