// SPDX-License-Identifier: GPL-3.0-or-later

// S6 — the weights endpoints (/api/v1/weights, /api/v2/weights;
// /api/v1/metric_correlations shares the machinery), never touched by
// the corpus before. Methods (weights.c):
//   - value: the window average per metric (natural points);
//   - anomaly-rate: the anomaly bit as the value; since #23212 the
//     METHOD implies the option on every path, equivalent to the
//     explicit options=anomaly-bit flag (the dashboards' "Anomaly
//     Rate" selector on volume/ks2);
//   - volume: highlight-vs-baseline relative change times the fraction
//     of highlight time above/below the baseline average; metrics with
//     EQUAL averages are skipped entirely;
//   - ks2: two-sample Kolmogorov-Smirnov over the CONSECUTIVE DIFFS
//     (x100000 integer quantization) of the two windows; the corpus
//     pins the EXACT endpoints — identical diff distributions weigh 0,
//     fully one-sided diff distributions with n*d^2>=18 weigh 1
//     (KSfbar's special cases return exact 0/1) — and defers the
//     ~550-line KSfbar numeric port (intermediate values unpinned).
//
// Weight normalization (spread_results_evenly): applied UNLESS
// options=raw, method=value, or the MCP format — deterministic
// including ties (unique sorted values; weight = 1 - countLE/unique),
// ported below as spreadEvenly.
//
// Contracts pinned along the way:
//   - the weights window is after-INCLUSIVE: [T0+120, T0+240] serves
//     121 points, unlike /data's (after, before] (rulings batch);
//   - per-metric weights depend on the rrdcontext retention stamp,
//     which lags chart creation by ~1-2s — weightsSettle waits for it;
//   - the default options are NOT_ALIGNED|NULL2ZERO|NONZERO: with no
//     options= given, ZERO-WEIGHT results are dropped; any explicit
//     options= keeps them.
package corpus

import (
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

const (
	wContext    = "fixture.weights"
	wKS2Context = "fixture.weightsks2"
	wRows       = 240
	wSplit      = 120 // baseline (T0, T0+120], highlight [T0+120, T0+240]
)

// main weights fixture:
//
//	flat:  constant 50 (equal averages → volume skips it);
//	level: 10/11 alternating in baseline, constant 30 in highlight;
//	split: 100/101 alternating in baseline, +3 ramp in highlight;
//	anom:  constant 20, anomalous only in the highlight window.
func weightsFixture() fixture.Chart {
	dims := []fixture.Dimension{{ID: "flat"}, {ID: "level"}, {ID: "split"}, {ID: "anom"}}
	val := func(id string, i int) string {
		switch id {
		case "flat":
			return "50"
		case "level":
			if i <= wSplit {
				if i%2 == 1 {
					return "10"
				}
				return "11"
			}
			return "30"
		case "split":
			if i <= wSplit {
				if i%2 == 1 {
					return "100"
				}
				return "101"
			}
			return strconv.Itoa(100 + 3*(i-wSplit-1))
		case "anom":
			return "20"
		}
		panic(id)
	}
	for d := range dims {
		for i := 1; i <= wRows; i++ {
			flags := stream.FlagNotAnomalous
			if dims[d].ID == "anom" && i > wSplit {
				flags = stream.FlagAnomalous
			}
			dims[d].Points = append(dims[d].Points, fixture.Point{
				T: fixture.T0 + int64(i), Collected: val(dims[d].ID, i), Flags: flags,
			})
		}
	}
	return fixture.Chart{
		ID: wContext, Title: "weights", Units: "units", Family: "fixture",
		Context: wContext, UpdateEvery: 1,
		Dimensions: dims,
	}
}

// ks2 endpoints fixture:
//
//	flat2: constant 50 — identical (all-zero) diffs both windows → d=0
//	       → weight exactly 0;
//	jump:  0/1 alternation in baseline (diffs ±1e5), then a -5 ramp in
//	       the highlight (all consecutive diffs +5e5/+6e5, including
//	       the window-boundary pair) — every highlight diff exceeds
//	       every baseline diff → d=1 with n*d^2>=18 → weight exactly 1.
func weightsKS2Fixture() fixture.Chart {
	dims := []fixture.Dimension{{ID: "flat2"}, {ID: "jump"}}
	val := func(id string, i int) string {
		if id == "flat2" {
			return "50"
		}
		if i <= wSplit {
			return strconv.Itoa(i % 2)
		}
		return strconv.Itoa(-5 * (i - wSplit))
	}
	for d := range dims {
		for i := 1; i <= wRows; i++ {
			dims[d].Points = append(dims[d].Points, fixture.Point{
				T: fixture.T0 + int64(i), Collected: val(dims[d].ID, i), Flags: stream.FlagNotAnomalous,
			})
		}
	}
	return fixture.Chart{
		ID: wKS2Context, Title: "weights ks2", Units: "units", Family: "fixture",
		Context: wKS2Context, UpdateEvery: 1,
		Dimensions: dims,
	}
}

// weightsSettle pushes ch once and waits for BOTH the retention barrier
// and the rrdcontext retention stamp (first_time_t != 0): the stamp
// lags chart creation by ~1-2s and the per-metric weights gate skips
// unstamped contexts entirely.
func weightsSettle(t *testing.T, host, machineGUID string, ch fixture.Chart) {
	t.Helper()
	if _, err := td.WaitRetention(host, ch.Context, ch.FirstT(), ch.LastT(), 2*time.Second); err != nil {
		pushLiveBurst(t, host, machineGUID, ch)
		if _, err := td.WaitRetention(host, ch.Context, ch.FirstT(), ch.LastT(), 15*time.Second); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		doc, err := td.HostJSON(host, "api/v1/contexts", url.Values{})
		if err == nil {
			if cs, ok := doc["contexts"].(map[string]any); ok {
				if c, ok := cs[ch.Context].(map[string]any); ok {
					if ft, _ := c["first_time_t"].(float64); ft != 0 {
						return
					}
				}
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("rrdcontext retention stamp for %s never arrived", ch.Context)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// spreadEvenly is the Go port of spread_results_evenly (weights.c): the
// registered values collapse to unique sorted slots and each result's
// weight becomes 1 - (slots <= value)/uniqueCount — deterministic
// including ties.
func spreadEvenly(values map[string]float64) map[string]float64 {
	uniq := map[float64]bool{}
	for _, v := range values {
		uniq[v] = true
	}
	slots := make([]float64, 0, len(uniq))
	for v := range uniq {
		slots = append(slots, v)
	}
	sort.Float64s(slots)
	out := make(map[string]float64, len(values))
	for k, v := range values {
		le := 0
		for _, s := range slots {
			if s <= v {
				le++
			}
		}
		out[k] = 1.0 - float64(le)/float64(len(slots))
	}
	return out
}

// weightsV1Params builds a /api/v1/weights request over the fixture
// windows against a single host's context tree.
func weightsV1Params(method, context, options string, baseline bool) url.Values {
	p := url.Values{}
	if method != "" {
		p.Set("method", method)
	}
	if context != "" {
		p.Set("context", context)
	}
	if options != "" {
		p.Set("options", options)
	}
	p.Set("after", strconv.FormatInt(fixture.T0+wSplit, 10))
	p.Set("before", strconv.FormatInt(fixture.T0+wRows, 10))
	if baseline {
		p.Set("baseline_after", strconv.FormatInt(fixture.T0, 10))
		p.Set("baseline_before", strconv.FormatInt(fixture.T0+wSplit, 10))
	}
	return p
}

// decodeV1ContextsWeights walks the CONTEXTS format down to
// {dimension: weight} while rejecting every malformed object or cell.
func decodeV1ContextsWeights(doc map[string]any, context string) (map[string]float64, error) {
	contexts, ok := doc["contexts"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("contexts is missing or not an object: %v", doc["contexts"])
	}
	contextAny, exists := contexts[context]
	if !exists {
		return nil, fmt.Errorf("context %q is missing", context)
	}
	ctx, ok := contextAny.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("context %q is not an object: %v", context, contextAny)
	}
	charts, ok := ctx["charts"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("context %q charts is missing or not an object: %v", context, ctx["charts"])
	}

	out := map[string]float64{}
	for chartID, chartAny := range charts {
		chart, ok := chartAny.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("chart %q is not an object: %v", chartID, chartAny)
		}
		dimensions, ok := chart["dimensions"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf(
				"chart %q dimensions is missing or not an object: %v",
				chartID, chart["dimensions"])
		}
		for id, value := range dimensions {
			if id == "" {
				return nil, fmt.Errorf("chart %q has an empty dimension id", chartID)
			}
			weight, ok := queryFiniteNumber(value)
			if !ok {
				return nil, fmt.Errorf(
					"chart %q dimension %q weight is not finite: %v",
					chartID, id, value)
			}
			if _, duplicate := out[id]; duplicate {
				return nil, fmt.Errorf("dimension %q appears in more than one chart", id)
			}
			out[id] = weight
		}
	}
	return out, nil
}

func v1ContextsWeights(t *testing.T, doc map[string]any, context string) map[string]float64 {
	t.Helper()
	out, err := decodeV1ContextsWeights(doc, context)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestDecodeV1ContextsWeightsRejectsMalformedDimensions(t *testing.T) {
	build := func() map[string]any {
		return map[string]any{"contexts": map[string]any{
			wContext: map[string]any{"charts": map[string]any{
				wContext: map[string]any{"dimensions": map[string]any{
					"flat": float64(0),
				}},
			}},
		}}
	}
	got, err := decodeV1ContextsWeights(build(), wContext)
	if err != nil || len(got) != 1 || got["flat"] != 0 {
		t.Fatalf("valid v1 contexts weights rejected: got=%v err=%v", got, err)
	}
	for name, mutate := range map[string]func(map[string]any){
		"malformed-context": func(doc map[string]any) {
			doc["contexts"].(map[string]any)[wContext] = nil
		},
		"malformed-chart": func(doc map[string]any) {
			ctx := doc["contexts"].(map[string]any)[wContext].(map[string]any)
			ctx["charts"].(map[string]any)[wContext] = nil
		},
		"extra-malformed-dimension": func(doc map[string]any) {
			ctx := doc["contexts"].(map[string]any)[wContext].(map[string]any)
			chart := ctx["charts"].(map[string]any)[wContext].(map[string]any)
			chart["dimensions"].(map[string]any)["junk"] = "not-a-number"
		},
		"nonfinite-dimension": func(doc map[string]any) {
			ctx := doc["contexts"].(map[string]any)[wContext].(map[string]any)
			chart := ctx["charts"].(map[string]any)[wContext].(map[string]any)
			chart["dimensions"].(map[string]any)["flat"] = math.Inf(1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			doc := build()
			mutate(doc)
			if _, err := decodeV1ContextsWeights(doc, wContext); err == nil {
				t.Errorf("accepted %s v1 contexts mutation", name)
			}
		})
	}
}

// weightsHighlightAverages: the after-INCLUSIVE 121-point highlight
// window averages of the main fixture.
func weightsHighlightAverages() map[string]float64 {
	return map[string]float64{
		"flat":  50,
		"level": 3611.0 / 121,  // 11 + 120x30
		"split": 33521.0 / 121, // 101 + sum(100..457 step 3)
		"anom":  20,
	}
}

func TestWeightsExpectedIDsExactlyOnce(t *testing.T) {
	want := map[string]float64{"flat": 0, "level": 0, "split": 0, "anom": 0}
	if err := weightsExpectedIDsExactlyOnce(
		[]string{"flat", "level", "split", "anom"}, want); err != nil {
		t.Fatalf("valid id sequence rejected: %v", err)
	}
	for name, ids := range map[string][]string{
		"duplicate-masks-missing": {"flat", "flat", "split", "anom"},
		"unexpected":              {"flat", "level", "split", "other"},
		"short":                   {"flat", "level", "split"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := weightsExpectedIDsExactlyOnce(ids, want); err == nil {
				t.Errorf("accepted %s id sequence %v", name, ids)
			}
		})
	}
}

func TestWeightsTimeframeStatsRequireFiniteNumbers(t *testing.T) {
	for name, value := range map[string]any{
		"string": "0",
		"null":   nil,
		"nan":    math.NaN(),
		"inf":    math.Inf(1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, ok := queryFiniteNumber(value); ok {
				t.Errorf("accepted malformed timeframe statistic %v", value)
			}
		})
	}
	if got, ok := queryFiniteNumber(float64(0)); !ok || got != 0 {
		t.Fatalf("finite zero = %v,%v", got, ok)
	}
}

func weightsExpectedIDsExactlyOnce(ids []string, want map[string]float64) error {
	counts := make(map[string]int, len(want))
	for _, id := range ids {
		if _, expected := want[id]; !expected {
			return fmt.Errorf("unexpected dimension %q", id)
		}
		counts[id]++
		if counts[id] > 1 {
			return fmt.Errorf("dimension %q appears %d times", id, counts[id])
		}
	}
	for id := range want {
		if counts[id] != 1 {
			return fmt.Errorf("dimension %q appears %d times, want exactly once", id, counts[id])
		}
	}
	return nil
}

type weightsMultiNodeRow struct {
	rowType   int64
	indices   [4]*int64 // node, context, instance, dimension
	weight    float64
	timeframe [6]float64
}

func weightsRequiredIndex(value any, path string) (int64, error) {
	index, ok := queryInteger(value)
	if !ok || index < 0 {
		return 0, fmt.Errorf("%s is not a nonnegative integer: %v", path, value)
	}
	return index, nil
}

func weightsNullableIndex(value any, path string) (*int64, error) {
	if value == nil {
		return nil, nil
	}
	index, err := weightsRequiredIndex(value, path)
	if err != nil {
		return nil, err
	}
	return &index, nil
}

func decodeWeightsIndexDictionary(
	dictionaries map[string]any,
	name, indexKey string,
) (map[int64]map[string]any, error) {
	entries, ok := dictionaries[name].([]any)
	if !ok {
		return nil, fmt.Errorf(
			"dictionaries.%s is missing or not an array: %v",
			name, dictionaries[name])
	}
	out := make(map[int64]map[string]any, len(entries))
	for i, entryAny := range entries {
		entry, ok := entryAny.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(
				"dictionaries.%s[%d] is not an object: %v", name, i, entryAny)
		}
		index, err := weightsRequiredIndex(
			entry[indexKey], fmt.Sprintf("dictionaries.%s[%d].%s", name, i, indexKey))
		if err != nil {
			return nil, err
		}
		if _, duplicate := out[index]; duplicate {
			return nil, fmt.Errorf("dictionaries.%s repeats index %d", name, index)
		}
		out[index] = entry
	}
	return out, nil
}

type weightsMultiNodeDictionaries struct {
	nodes      map[int64]map[string]any
	contexts   map[int64]map[string]any
	instances  map[int64]map[string]any
	dimensions map[int64]string
}

func decodeWeightsMultiNodeDictionaries(doc map[string]any) (weightsMultiNodeDictionaries, error) {
	dictionaries, ok := doc["dictionaries"].(map[string]any)
	if !ok {
		return weightsMultiNodeDictionaries{}, fmt.Errorf(
			"dictionaries is missing or not an object: %v", doc["dictionaries"])
	}
	nodes, err := decodeWeightsIndexDictionary(dictionaries, "nodes", "ni")
	if err != nil {
		return weightsMultiNodeDictionaries{}, err
	}
	contexts, err := decodeWeightsIndexDictionary(dictionaries, "contexts", "ci")
	if err != nil {
		return weightsMultiNodeDictionaries{}, err
	}
	instances, err := decodeWeightsIndexDictionary(dictionaries, "instances", "ii")
	if err != nil {
		return weightsMultiNodeDictionaries{}, err
	}
	dimensionEntries, err := decodeWeightsIndexDictionary(dictionaries, "dimensions", "di")
	if err != nil {
		return weightsMultiNodeDictionaries{}, err
	}
	dimensions := make(map[int64]string, len(dimensionEntries))
	seenIDs := make(map[string]struct{}, len(dimensionEntries))
	for index, dimension := range dimensionEntries {
		id, ok := dimension["id"].(string)
		if !ok || id == "" {
			return weightsMultiNodeDictionaries{}, fmt.Errorf(
				"dictionaries.dimensions index %d id is not a nonempty string: %v",
				index, dimension["id"])
		}
		if _, duplicate := seenIDs[id]; duplicate {
			return weightsMultiNodeDictionaries{}, fmt.Errorf(
				"dictionaries.dimensions repeats id %q", id)
		}
		dimensions[index] = id
		seenIDs[id] = struct{}{}
	}
	return weightsMultiNodeDictionaries{
		nodes: nodes, contexts: contexts, instances: instances, dimensions: dimensions,
	}, nil
}

func decodeWeightsMultiNodeRows(doc map[string]any) ([]weightsMultiNodeRow, error) {
	rows, ok := doc["result"].([]any)
	if !ok {
		return nil, fmt.Errorf("result is missing or not an array: %v", doc["result"])
	}
	out := make([]weightsMultiNodeRow, 0, len(rows))
	for rowIndex, rowAny := range rows {
		row, ok := rowAny.([]any)
		if !ok || len(row) != 7 {
			return nil, fmt.Errorf(
				"result[%d] is not an exact seven-cell row: %v", rowIndex, rowAny)
		}
		rowType, ok := queryInteger(row[0])
		if !ok || rowType < 0 || rowType > 3 {
			return nil, fmt.Errorf(
				"result[%d].row_type is not an integer in [0,3]: %v", rowIndex, row[0])
		}

		decoded := weightsMultiNodeRow{rowType: rowType}
		for i, name := range []string{"node", "context", "instance", "dimension"} {
			index, err := weightsNullableIndex(
				row[i+1], fmt.Sprintf("result[%d].%s_index", rowIndex, name))
			if err != nil {
				return nil, err
			}
			decoded.indices[i] = index
		}
		required := 4 - int(rowType)
		for i, index := range decoded.indices {
			wantPresent := i < required
			if (index != nil) != wantPresent {
				return nil, fmt.Errorf(
					"result[%d] row type %d %s index presence is %v, want %v",
					rowIndex, rowType,
					[]string{"node", "context", "instance", "dimension"}[i],
					index != nil, wantPresent)
			}
		}

		weight, ok := queryFiniteNumber(row[5])
		if !ok {
			return nil, fmt.Errorf("result[%d].weight is not finite: %v", rowIndex, row[5])
		}
		decoded.weight = weight

		timeframe, ok := row[6].([]any)
		if !ok || len(timeframe) != 6 {
			return nil, fmt.Errorf(
				"result[%d].timeframe is not an exact six-cell array: %v",
				rowIndex, row[6])
		}
		for i, value := range timeframe {
			number, ok := queryFiniteNumber(value)
			if !ok {
				return nil, fmt.Errorf(
					"result[%d].timeframe[%d] is not finite: %v", rowIndex, i, value)
			}
			if i >= 4 {
				integer, integerOK := queryInteger(value)
				if !integerOK || integer < 0 {
					return nil, fmt.Errorf(
						"result[%d].timeframe[%d] is not a nonnegative integer: %v",
						rowIndex, i, value)
				}
			}
			decoded.timeframe[i] = number
		}
		out = append(out, decoded)
	}
	return out, nil
}

func weightsValueMultiNodeExact(
	doc map[string]any,
	want map[string]float64,
	wantTF map[string][6]float64,
	rollup float64,
) error {
	dictionaries, err := decodeWeightsMultiNodeDictionaries(doc)
	if err != nil {
		return err
	}
	dimensionByIndex := dictionaries.dimensions
	dictionaryIDs := make([]string, 0, len(dimensionByIndex))
	for _, id := range dimensionByIndex {
		dictionaryIDs = append(dictionaryIDs, id)
	}
	if err := weightsExpectedIDsExactlyOnce(dictionaryIDs, want); err != nil {
		return fmt.Errorf("dimension dictionary identity: %w", err)
	}

	rows, err := decodeWeightsMultiNodeRows(doc)
	if err != nil {
		return err
	}
	indexDictionaries := []map[int64]map[string]any{
		dictionaries.nodes, dictionaries.contexts, dictionaries.instances,
	}
	for rowIndex, row := range rows {
		for indexPosition, dictionary := range indexDictionaries {
			index := row.indices[indexPosition]
			if index == nil {
				continue
			}
			if _, exists := dictionary[*index]; !exists {
				return fmt.Errorf(
					"result[%d] references unknown %s index %d",
					rowIndex,
					[]string{"node", "context", "instance"}[indexPosition],
					*index)
			}
		}
	}
	var dimensionIDs, rollupTypes []string
	var hierarchy [3]int64
	haveHierarchy := false
	for rowIndex, row := range rows {
		if row.rowType != 0 {
			continue
		}
		di := *row.indices[3]
		id, exists := dimensionByIndex[di]
		if !exists {
			return fmt.Errorf("result[%d] references unknown dimension index %d", rowIndex, di)
		}
		dimensionIDs = append(dimensionIDs, id)
		if !tierValueMatch(row.weight, want[id], 1e-9) {
			return fmt.Errorf(
				"%s weight is %v, want %v (after-inclusive 121-point window)",
				id, row.weight, want[id])
		}
		for i, wantValue := range wantTF[id] {
			if !tierValueMatch(row.timeframe[i], wantValue, 1e-9) {
				return fmt.Errorf(
					"%s timeframe[%d] is %v, want %v",
					id, i, row.timeframe[i], wantValue)
			}
		}
		gotHierarchy := [3]int64{*row.indices[0], *row.indices[1], *row.indices[2]}
		if !haveHierarchy {
			hierarchy = gotHierarchy
			haveHierarchy = true
		} else if gotHierarchy != hierarchy {
			return fmt.Errorf(
				"%s hierarchy is %v, want the fixture hierarchy %v",
				id, gotHierarchy, hierarchy)
		}
	}
	if err := weightsExpectedIDsExactlyOnce(dimensionIDs, want); err != nil {
		return fmt.Errorf("dimension-row identity: %w", err)
	}
	if !haveHierarchy {
		return fmt.Errorf("result has no dimension row")
	}

	rollupNames := map[int64]string{1: "instance", 2: "context", 3: "node"}
	wantRollups := map[string]float64{
		"instance": rollup, "context": rollup, "node": rollup,
	}
	for rowIndex, row := range rows {
		if row.rowType == 0 {
			continue
		}
		name := rollupNames[row.rowType]
		rollupTypes = append(rollupTypes, name)
		if !tierValueMatch(row.weight, rollup, 1e-9) {
			return fmt.Errorf(
				"result[%d] %s rollup weight is %v, want %v",
				rowIndex, name, row.weight, rollup)
		}
		required := 4 - int(row.rowType)
		for i := 0; i < required; i++ {
			if *row.indices[i] != hierarchy[i] {
				return fmt.Errorf(
					"result[%d] %s %s index is %d, want %d",
					rowIndex, name,
					[]string{"node", "context", "instance"}[i],
					*row.indices[i], hierarchy[i])
			}
		}
	}
	if err := weightsExpectedIDsExactlyOnce(rollupTypes, wantRollups); err != nil {
		return fmt.Errorf("rollup-row identity: %w", err)
	}
	return nil
}

func TestWeightsValueMultiNodeGuards(t *testing.T) {
	want := map[string]float64{"flat": 0, "level": 1, "split": 2, "anom": 3}
	wantTF := map[string][6]float64{
		"flat": {1, 1, 1, 1, 1, 0}, "level": {2, 2, 2, 2, 1, 0},
		"split": {3, 3, 3, 3, 1, 0}, "anom": {4, 4, 4, 4, 1, 1},
	}
	const rollup = 1.5
	build := func() map[string]any {
		dimensions := make([]any, 0, len(want))
		rows := make([]any, 0, len(want)+3)
		ids := []string{"flat", "level", "split", "anom"}
		for index, id := range ids {
			dimensions = append(dimensions, map[string]any{
				"di": float64(index), "id": id,
			})
			tf := wantTF[id]
			rows = append(rows, []any{
				float64(0), float64(0), float64(0), float64(0), float64(index),
				want[id],
				[]any{tf[0], tf[1], tf[2], tf[3], tf[4], tf[5]},
			})
		}
		for rowType := 1; rowType <= 3; rowType++ {
			indices := []any{float64(0), float64(0), float64(0), nil}
			for i := 4 - rowType; i < len(indices); i++ {
				indices[i] = nil
			}
			rows = append(rows, []any{
				float64(rowType), indices[0], indices[1], indices[2], indices[3],
				float64(rollup),
				[]any{float64(0), float64(0), float64(0), float64(0), float64(0), float64(0)},
			})
		}
		return map[string]any{
			"dictionaries": map[string]any{
				"nodes":      []any{map[string]any{"ni": float64(0)}},
				"contexts":   []any{map[string]any{"ci": float64(0)}},
				"instances":  []any{map[string]any{"ii": float64(0)}},
				"dimensions": dimensions,
			},
			"result": rows,
		}
	}
	if err := weightsValueMultiNodeExact(build(), want, wantTF, rollup); err != nil {
		t.Fatalf("valid MULTINODE control rejected: %v", err)
	}

	mutations := map[string]func(map[string]any){
		"missing-node-rollup": func(doc map[string]any) {
			rows := doc["result"].([]any)
			doc["result"] = rows[:len(rows)-1]
		},
		"duplicate-context-rollup": func(doc map[string]any) {
			rows := doc["result"].([]any)
			contextRow := rows[len(rows)-2].([]any)
			duplicate := append([]any(nil), contextRow...)
			doc["result"] = append(rows, duplicate)
		},
		"unexpected-row-type": func(doc map[string]any) {
			doc["result"].([]any)[0].([]any)[0] = float64(4)
		},
		"string-row-type": func(doc map[string]any) {
			doc["result"].([]any)[0].([]any)[0] = "dimension"
		},
		"malformed-dictionary-index": func(doc map[string]any) {
			dictionaries := doc["dictionaries"].(map[string]any)
			dictionaries["dimensions"].([]any)[0].(map[string]any)["di"] = "bad"
		},
		"fractional-row-index": func(doc map[string]any) {
			doc["result"].([]any)[0].([]any)[4] = float64(0.5)
		},
		"unknown-hierarchy-index": func(doc map[string]any) {
			for _, rowAny := range doc["result"].([]any) {
				row := rowAny.([]any)
				if row[1] != nil {
					row[1] = float64(999)
				}
			}
		},
		"trailing-row-field": func(doc map[string]any) {
			row := doc["result"].([]any)[0].([]any)
			doc["result"].([]any)[0] = append(row, "extra")
		},
		"wrong-null-layout": func(doc map[string]any) {
			rows := doc["result"].([]any)
			rows[len(rows)-3].([]any)[4] = float64(0)
		},
		"nonfinite-weight": func(doc map[string]any) {
			doc["result"].([]any)[0].([]any)[5] = math.Inf(1)
		},
		"nonfinite-timeframe": func(doc map[string]any) {
			row := doc["result"].([]any)[0].([]any)
			row[6].([]any)[0] = math.NaN()
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			doc := build()
			mutate(doc)
			if err := weightsValueMultiNodeExact(doc, want, wantTF, rollup); err == nil {
				t.Errorf("accepted %s MULTINODE mutation", name)
			}
		})
	}
}

func TestWeightsValueMultiNode(t *testing.T) {
	trackContractComponent(t, "W/value", "multi-node")

	weightsSettle(t, "weights-h", guid(160), weightsFixture())

	p := url.Values{}
	p.Set("scope_contexts", wContext)
	p.Set("method", "value")
	p.Set("after", strconv.FormatInt(fixture.T0+wSplit, 10))
	p.Set("before", strconv.FormatInt(fixture.T0+wRows, 10))
	doc, err := td.HostJSON("weights-h", "api/v2/weights", p)
	if err != nil {
		t.Fatal(err)
	}

	want := weightsHighlightAverages()
	wantTF := map[string][6]float64{
		"flat":  {50, 50, 50, 6050, 121, 0},
		"level": {11, 3611.0 / 121, 30, 3611, 121, 0},
		"split": {100, 33521.0 / 121, 457, 33521, 121, 0},
		"anom":  {20, 20, 20, 2420, 121, 120},
	}
	rollup := (50 + 3611.0/121 + 33521.0/121 + 20) / 4

	// MULTINODE rows are exactly
	// [row_type, ni, ci, ii, di, weight, [min,avg,max,sum,count,anomaly_count]].
	// The one-node fixture must expose four dimensions plus one rollup at each
	// instance/context/node level.
	if err := weightsValueMultiNodeExact(doc, want, wantTF, rollup); err != nil {
		t.Error(err)
	}
}

func TestWeightsPerMetricAnomalyRate(t *testing.T) {
	weightsSettle(t, "weights-h", guid(160), weightsFixture())

	t.Run("values", func(t *testing.T) {
		trackContract(t, "W/anomaly-rate-per-metric-values")

		// the per-metric path (v1 host route, NO context selector) applies
		// the anomaly bit: raw weights are the true window anomaly rates
		doc, err := td.HostJSON("weights-h", "api/v1/weights", weightsV1Params("anomaly-rate", "", "raw", false))
		if err != nil {
			t.Fatal(err)
		}
		got := v1ContextsWeights(t, doc, wContext)
		want := map[string]float64{"flat": 0, "level": 0, "split": 0, "anom": 12000.0 / 121}
		if len(got) != len(want) {
			t.Fatalf("got %d dims %v, want %d", len(got), got, len(want))
		}
		for id, w := range want {
			if g, ok := got[id]; !ok || !tierValueMatch(g, w, 1e-9) {
				t.Errorf("%s: weight %v, want true anomaly rate %v", id, got[id], w)
			}
		}
	})

	t.Run("nonzero-default", func(t *testing.T) {
		trackContract(t, "W/anomaly-rate-per-metric-nonzero-default")

		// the NONZERO default: with no options= given, zero-weight results
		// are dropped — only the anomalous dimension survives
		doc, err := td.HostJSON("weights-h", "api/v1/weights", weightsV1Params("anomaly-rate", "", "", false))
		if err != nil {
			t.Fatal(err)
		}
		got := v1ContextsWeights(t, doc, wContext)
		if len(got) != 1 {
			t.Errorf("default options kept %d dims %v, want only the anomalous one", len(got), got)
		}
		if _, ok := got["anom"]; !ok {
			t.Errorf("anom missing from default-options result %v", got)
		}
	})
}

// TestWeightsMultiDimAnomalyRate: the method IMPLIES the anomaly bit
// on every path since #23212 — the bare method and the explicit
// options=anomaly-bit (what the dashboards send with volume/ks2) are
// equivalent, and both return true anomaly rates through the
// multi-dimensional path (was: the bare method ranked by plain value
// averages there, while the per-metric and MCP paths forced the bit).
func TestWeightsMultiDimAnomalyRate(t *testing.T) {
	trackContract(t, "W/anomaly-rate-multidim")

	weightsSettle(t, "weights-h", guid(160), weightsFixture())

	rates := map[string]float64{"flat": 0, "level": 0, "split": 0, "anom": 12000.0 / 121}
	averages := weightsHighlightAverages()

	for _, options := range []string{"raw|anomaly-bit", "raw"} {
		doc, err := td.HostJSON("weights-h", "api/v1/weights", weightsV1Params("anomaly-rate", wContext, options, false))
		if err != nil {
			t.Fatal(err)
		}
		got := v1ContextsWeights(t, doc, wContext)
		if len(got) != len(rates) {
			t.Fatalf("options=%q got %d dims %v, want %d", options, len(got), got, len(rates))
		}
		for id, w := range rates {
			g, ok := got[id]
			if !ok || !tierValueMatch(g, w, 1e-9) {
				if ok && tierValueMatch(g, averages[id], 1e-9) {
					t.Errorf("options=%q %s: weight %v is the VALUE AVERAGE — the #23212 regression returned", options, id, g)
				} else {
					t.Errorf("options=%q %s: weight %v, want the true rate %v", options, id, got[id], w)
				}
			}
		}
	}
}

func TestWeightsVolume(t *testing.T) {
	weightsSettle(t, "weights-h", guid(160), weightsFixture())

	doc, err := td.HostJSON("weights-h", "api/v1/weights", weightsV1Params("volume", wContext, "raw", true))
	if err != nil {
		t.Fatal(err)
	}
	got := v1ContextsWeights(t, doc, wContext)

	// flat and anom have EQUAL baseline/highlight averages → skipped
	// entirely; level and split weigh (hl-bl)/bl x fraction-of-time
	// above the baseline average (split's first highlight row, 100, is
	// below its 100.5 baseline → 120/121)
	levelHL, splitHL := 3611.0/121, 33521.0/121
	want := map[string]float64{
		"level": (levelHL - 10.5) / 10.5 * (121.0 * 100 / 121 / 100),
		"split": (splitHL - 100.5) / 100.5 * (120.0 * 100 / 121 / 100),
	}
	t.Run("equal-baseline-skip", func(t *testing.T) {
		trackContract(t, "W/volume-equal-baseline-skip")
		for _, id := range []string{"flat", "anom"} {
			if _, found := got[id]; found {
				t.Errorf("equal-baseline metric %q was not skipped: %v", id, got)
			}
		}
	})

	t.Run("formula", func(t *testing.T) {
		trackContract(t, "W/volume-formula")
		for id, w := range want {
			if g, ok := got[id]; !ok || !tierValueMatch(g, w, 1e-9) {
				t.Errorf("%s: weight %v, want %v", id, got[id], w)
			}
		}
	})
}

func TestWeightsKS2(t *testing.T) {
	weightsSettle(t, "weights-ks2", guid(163), weightsKS2Fixture())

	want := map[string]float64{"flat2": 0, "jump": 1}
	t.Run("raw-endpoints", func(t *testing.T) {
		trackContract(t, "W/ks2-raw-endpoints")

		// raw: the exact endpoints without normalization
		doc, err := td.HostJSON("weights-ks2", "api/v1/weights", weightsV1Params("ks2", wKS2Context, "raw", true))
		if err != nil {
			t.Fatal(err)
		}
		got := v1ContextsWeights(t, doc, wKS2Context)
		if len(got) != len(want) {
			t.Fatalf("got %d dims %v, want %d", len(got), got, len(want))
		}
		for id, w := range want {
			if g, ok := got[id]; !ok || g != w {
				t.Errorf("%s: weight %v, want exactly %v (KSfbar special case)", id, got[id], w)
			}
		}
	})

	t.Run("spread-normalization", func(t *testing.T) {
		trackContract(t, "W/ks2-spread-normalization")

		// spread: the same endpoints through spread_results_evenly
		doc, err := td.HostJSON("weights-ks2", "api/v1/weights", weightsV1Params("ks2", wKS2Context, "null2zero", true))
		if err != nil {
			t.Fatal(err)
		}
		got := v1ContextsWeights(t, doc, wKS2Context)
		spreadWant := spreadEvenly(want)
		if len(got) != len(spreadWant) {
			t.Fatalf("spread got %d dims %v, want %d", len(got), got, len(spreadWant))
		}
		for id, w := range spreadWant {
			if g, ok := got[id]; !ok || !tierValueMatch(g, w, 1e-9) {
				t.Errorf("%s: spread weight %v, want %v", id, got[id], w)
			}
		}
	})
}

func TestWeightsValueNeverSpreads(t *testing.T) {
	trackContractComponent(t, "W/value", "never-spreads")

	weightsSettle(t, "weights-h", guid(160), weightsFixture())

	// method=value skips spreading even on v1 — raw averages come back
	doc, err := td.HostJSON("weights-h", "api/v1/weights", weightsV1Params("value", wContext, "null2zero", false))
	if err != nil {
		t.Fatal(err)
	}
	got := v1ContextsWeights(t, doc, wContext)
	want := weightsHighlightAverages()
	if len(got) != len(want) {
		t.Fatalf("got %d dims %v, want %d", len(got), got, len(want))
	}
	for id, w := range want {
		if g, ok := got[id]; !ok || !tierValueMatch(g, w, 1e-9) {
			t.Errorf("%s: weight %v, want the raw average %v (value method never spreads)", id, got[id], w)
		}
	}
}
