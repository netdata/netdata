// SPDX-License-Identifier: GPL-3.0-or-later

// S6 — the weights endpoints (/api/v1/weights, /api/v2/weights;
// /api/v1/metric_correlations shares the machinery), never touched by
// the corpus before. Methods (weights.c):
//   - value: the window average per metric (natural points);
//   - anomaly-rate: the anomaly bit as the value. The WORKING contract
//     is the options=anomaly-bit flag (honored on every path — the
//     dashboards' "Anomaly Rate" selector is exactly this option on
//     volume/ks2); the BARE method is path-inconsistent (per-metric
//     and MCP force the bit, the multi-dimensional path does not) —
//     pinned green, RULING PENDING;
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
//   flat:  constant 50 (equal averages → volume skips it);
//   level: 10/11 alternating in baseline, constant 30 in highlight;
//   split: 100/101 alternating in baseline, +3 ramp in highlight;
//   anom:  constant 20, anomalous only in the highlight window.
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
//   flat2: constant 50 — identical (all-zero) diffs both windows → d=0
//          → weight exactly 0;
//   jump:  0/1 alternation in baseline (diffs ±1e5), then a -5 ramp in
//          the highlight (all consecutive diffs +5e5/+6e5, including
//          the window-boundary pair) — every highlight diff exceeds
//          every baseline diff → d=1 with n*d^2>=18 → weight exactly 1.
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

// v1ContextsWeights walks the CONTEXTS format down to {dimension: weight}.
func v1ContextsWeights(t *testing.T, doc map[string]any, context string) map[string]float64 {
	t.Helper()
	out := map[string]float64{}
	contexts, _ := doc["contexts"].(map[string]any)
	ctx, _ := contexts[context].(map[string]any)
	charts, _ := ctx["charts"].(map[string]any)
	for _, chAny := range charts {
		chm, _ := chAny.(map[string]any)
		dims, _ := chm["dimensions"].(map[string]any)
		for id, w := range dims {
			if f, ok := w.(float64); ok {
				out[id] = f
			}
		}
	}
	return out
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

func TestWeightsValueMultiNode(t *testing.T) {
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

	// MULTINODE rows: [row_type, ni, ci, ii, di, weight, [min,avg,max,sum,count,anomaly_count]]
	dictAny, _ := doc["dictionaries"].(map[string]any)
	dimsAny, _ := dictAny["dimensions"].([]any)
	dimID := map[int]string{}
	for _, dAny := range dimsAny {
		dm, _ := dAny.(map[string]any)
		di, _ := dm["di"].(float64)
		id, _ := dm["id"].(string)
		dimID[int(di)] = id
	}
	rows, _ := doc["result"].([]any)

	want := weightsHighlightAverages()
	wantTF := map[string][6]float64{
		"flat":  {50, 50, 50, 6050, 121, 0},
		"level": {11, 3611.0 / 121, 30, 3611, 121, 0},
		"split": {100, 33521.0 / 121, 457, 33521, 121, 0},
		"anom":  {20, 20, 20, 2420, 121, 120},
	}
	rollup := (50 + 3611.0/121 + 33521.0/121 + 20) / 4

	seen := 0
	for _, rowAny := range rows {
		row, _ := rowAny.([]any)
		if len(row) < 7 {
			t.Fatalf("malformed result row %v", rowAny)
		}
		rowType, _ := row[0].(float64)
		weight, _ := row[5].(float64)
		if rowType != 0 {
			// instance/context/node rollups carry the mean of their dims
			if !tierValueMatch(weight, rollup, 1e-9) {
				t.Errorf("rollup row type %v: weight %v, want %v", rowType, weight, rollup)
			}
			continue
		}
		di, _ := row[4].(float64)
		id := dimID[int(di)]
		w, ok := want[id]
		if !ok {
			t.Errorf("unexpected dimension %q", id)
			continue
		}
		seen++
		if !tierValueMatch(weight, w, 1e-9) {
			t.Errorf("%s: weight %v, want %v (after-inclusive 121-point window)", id, weight, w)
		}
		tf, _ := row[6].([]any)
		if len(tf) != 6 {
			t.Errorf("%s: timeframe %v, want 6 stats", id, tf)
			continue
		}
		for j, wantV := range wantTF[id] {
			got, _ := tf[j].(float64)
			if !tierValueMatch(got, wantV, 1e-9) {
				t.Errorf("%s: timeframe[%d] = %v, want %v", id, j, got, wantV)
			}
		}
	}
	if seen != len(want) {
		t.Errorf("saw %d dimension rows, want %d", seen, len(want))
	}
}

func TestWeightsPerMetricAnomalyRate(t *testing.T) {
	weightsSettle(t, "weights-h", guid(160), weightsFixture())

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

	// the NONZERO default: with no options= given, zero-weight results
	// are dropped — only the anomalous dimension survives
	doc, err = td.HostJSON("weights-h", "api/v1/weights", weightsV1Params("anomaly-rate", "", "", false))
	if err != nil {
		t.Fatal(err)
	}
	got = v1ContextsWeights(t, doc, wContext)
	if len(got) != 1 {
		t.Errorf("default options kept %d dims %v, want only the anomalous one", len(got), got)
	}
	if _, ok := got["anom"]; !ok {
		t.Errorf("anom missing from default-options result %v", got)
	}
}

// TestWeightsMultiDimAnomalyRate pins BOTH halves of the multi-dim
// anomaly contract (RULING PENDING on the bare-method half):
//   - options=anomaly-bit is the WORKING contract: the multi-dim path
//     honors it and returns true anomaly rates — this is what the
//     dashboards send (their "Anomaly Rate" selector is the option on
//     volume/ks2, never method=anomaly-rate);
//   - the BARE method (no option) is path-INCONSISTENT: the per-metric
//     and MCP paths force the anomaly bit, the multi-dimensional path
//     does not — a bare /api/v1/weights with a context selector (its
//     default method IS anomaly-rate) ranks by plain value averages.
//     Pinned as current behavior; if ruled a bug, forcing the bit
//     flips this half and the pin demands its update.
func TestWeightsMultiDimAnomalyRate(t *testing.T) {
	weightsSettle(t, "weights-h", guid(160), weightsFixture())

	averages := weightsHighlightAverages()
	rates := map[string]float64{"flat": 0, "level": 0, "split": 0, "anom": 12000.0 / 121}

	// the working contract: explicit anomaly-bit → true rates
	doc, err := td.HostJSON("weights-h", "api/v1/weights", weightsV1Params("anomaly-rate", wContext, "raw|anomaly-bit", false))
	if err != nil {
		t.Fatal(err)
	}
	got := v1ContextsWeights(t, doc, wContext)
	for id, w := range rates {
		if g, ok := got[id]; !ok || !tierValueMatch(g, w, 1e-9) {
			t.Errorf("with anomaly-bit, %s: weight %v, want the true rate %v", id, got[id], w)
		}
	}

	// the bare-method inconsistency: no option → value averages
	doc, err = td.HostJSON("weights-h", "api/v1/weights", weightsV1Params("anomaly-rate", wContext, "raw", false))
	if err != nil {
		t.Fatal(err)
	}
	got = v1ContextsWeights(t, doc, wContext)
	for id, avg := range averages {
		g, ok := got[id]
		if !ok {
			continue
		}
		if tierValueMatch(g, rates[id], 1e-9) && !tierValueMatch(g, avg, 1e-9) {
			t.Errorf("bare method now returns anomaly rates on the multi-dim path (%s = %v) — the inconsistency was fixed: update this pin and the manifest", id, g)
		} else if !tierValueMatch(g, avg, 1e-9) {
			t.Errorf("bare method %s: weight %v matches neither the pinned average %v nor the rate — investigate", id, g, avg)
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
	if len(got) != len(want) {
		t.Fatalf("got %d dims %v, want %d (equal-averages metrics must be skipped)", len(got), got, len(want))
	}
	for id, w := range want {
		if g, ok := got[id]; !ok || !tierValueMatch(g, w, 1e-9) {
			t.Errorf("%s: weight %v, want %v", id, got[id], w)
		}
	}
}

func TestWeightsKS2(t *testing.T) {
	weightsSettle(t, "weights-ks2", guid(163), weightsKS2Fixture())

	// raw: the exact endpoints without normalization
	doc, err := td.HostJSON("weights-ks2", "api/v1/weights", weightsV1Params("ks2", wKS2Context, "raw", true))
	if err != nil {
		t.Fatal(err)
	}
	got := v1ContextsWeights(t, doc, wKS2Context)
	want := map[string]float64{"flat2": 0, "jump": 1}
	if len(got) != len(want) {
		t.Fatalf("got %d dims %v, want %d", len(got), got, len(want))
	}
	for id, w := range want {
		if g, ok := got[id]; !ok || g != w {
			t.Errorf("%s: weight %v, want exactly %v (KSfbar special case)", id, got[id], w)
		}
	}

	// spread: the same endpoints through spread_results_evenly
	doc, err = td.HostJSON("weights-ks2", "api/v1/weights", weightsV1Params("ks2", wKS2Context, "null2zero", true))
	if err != nil {
		t.Fatal(err)
	}
	got = v1ContextsWeights(t, doc, wKS2Context)
	for id, w := range spreadEvenly(want) {
		if g, ok := got[id]; !ok || !tierValueMatch(g, w, 1e-9) {
			t.Errorf("%s: spread weight %v, want %v", id, got[id], w)
		}
	}
}

func TestWeightsValueNeverSpreads(t *testing.T) {
	weightsSettle(t, "weights-h", guid(160), weightsFixture())

	// method=value skips spreading even on v1 — raw averages come back
	doc, err := td.HostJSON("weights-h", "api/v1/weights", weightsV1Params("value", wContext, "null2zero", false))
	if err != nil {
		t.Fatal(err)
	}
	got := v1ContextsWeights(t, doc, wContext)
	for id, w := range weightsHighlightAverages() {
		if g, ok := got[id]; !ok || !tierValueMatch(g, w, 1e-9) {
			t.Errorf("%s: weight %v, want the raw average %v (value method never spreads)", id, got[id], w)
		}
	}
}
