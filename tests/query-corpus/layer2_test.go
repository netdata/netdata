// SPDX-License-Identifier: GPL-3.0-or-later

// Layer 2 — tier rollups: tier1/tier2 points are the exact min/max/sum/
// count/anomaly-count derivation of the pushed samples, per the researched
// ingestion contract (rrddim-collection.c):
//
//   - windows are wall-clock aligned to update_every × tier grouping (stock
//     grouping 60 per tier); the stored timestamp is the aligned window end;
//   - higher tiers aggregate the ORIGINAL collected doubles, not the tier0
//     storage_number-quantized values;
//   - tier pages store float32 sum/min/max (one cast at write) and exact
//     uint16 count/anomaly_count — and NO flags, so the RESET annotation is
//     lost at tier1+;
//   - gap samples contribute nothing; an all-gap window is stored as a
//     NAN/count-0 point; a whole-chart gap window is never stored at all.
//
// Settle rule: a completed window is written when the NEXT window completes
// (or earlier via the spread-write modulo), so every fixture pushes at least
// TWO full tier windows beyond the last asserted one.
package corpus

import (
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

// tier1Gran is the tier1 window granularity at update_every=1 with the stock
// grouping of 60 iterations; tier2Gran adds the second stock ×60.
const (
	tier1Gran = 60
	tier2Gran = 3600
)

// printTol is the absolute wire tolerance of json2 values: the daemon prints
// doubles with 7 fractional digits (print_netdata_double, buffer.h), so a
// parsed value may differ from the exact double by half an ulp of the 7th
// fractional digit.
const printTol = 5e-8

// tierValueMatch compares a queried tier value to the oracle: within the
// JSON print tolerance absolutely, or within the case tolerance relatively.
func tierValueMatch(got, want, relTol float64) bool {
	diff := got - want
	if diff < 0 {
		diff = -diff
	}
	if diff <= printTol {
		return true
	}
	return valuesMatch(got, want, relTol)
}

type tierWindowAssertion uint8

const (
	tierWindowAll tierWindowAssertion = iota
	tierWindowGrid
	tierWindowValue
	tierWindowAnomaly
	tierWindowAnnotation
)

// verifyTierWindows asserts every tier bucket of every dimension of ch in
// [firstEnd, lastEnd] (aligned window ends, inclusive) against the fixture
// tier oracle, through four forced-tier queries (sum, min, max, average) —
// values, anomaly rate, and annotations.
//
// Callers must pick firstEnd so that the query's `after` (= firstEnd -
// granularity) does NOT coincide with a stored non-empty tier point: a tier
// point ending exactly at `after` is absorbed into the first bucket —
// CASE-017 pins that bug; the green cases here start before their data.
func verifyTierWindows(t *testing.T, host string, ch fixture.Chart, tier int, granularity, firstEnd, lastEnd int64) {
	t.Helper()
	verifyTierWindowsWithReporter(t, tierWindowQuery{
		host: host, chart: ch, tier: tier,
		granularity: granularity, firstEnd: firstEnd, lastEnd: lastEnd,
	},
		func(_ tierWindowAssertion, format string, args ...any) {
			t.Errorf(format, args...)
		})
}

type tierWindowQuery struct {
	host        string
	chart       fixture.Chart
	tier        int
	granularity int64
	firstEnd    int64
	lastEnd     int64
}

func verifyTierWindowsWithReporter(
	t *testing.T,
	q tierWindowQuery,
	report func(tierWindowAssertion, string, ...any),
) {
	t.Helper()
	host, ch, tier := q.host, q.chart, q.tier
	granularity, firstEnd, lastEnd := q.granularity, q.firstEnd, q.lastEnd

	after := firstEnd - granularity
	points := (lastEnd - after) / granularity

	oracles := make(map[string]map[int64]fixture.TierPoint, len(ch.Dimensions))
	for _, d := range ch.Dimensions {
		oracles[d.ID] = d.TierWindows(granularity, int64(ch.UpdateEvery))
	}

	for _, tg := range []string{"sum", "min", "max", "average"} {
		doc, err := td.DataV3(host, daemon.DataParamsTier(ch.Context, tier, after, lastEnd, points, tg))
		if err != nil {
			report(tierWindowAll, "tier%d %s query: %v", tier, tg, err)
			continue
		}
		if !assertSelectedTier(t, doc, tier) {
			report(tierWindowAll, "tier%d %s query was not served only by the forced tier", tier, tg)
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			report(tierWindowAll, "tier%d %s decode: %v", tier, tg, err)
			continue
		}
		dimensions := make([]string, 0, len(ch.Dimensions))
		for _, dim := range ch.Dimensions {
			dimensions = append(dimensions, dim.ID)
		}
		if !assertExactColumnSet(t, cols, dimensions) {
			report(tierWindowAll, "tier%d %s returned the wrong dimension set", tier, tg)
		}

		for _, dim := range ch.Dimensions {
			col, ok := cols[dim.ID]
			if !ok {
				report(tierWindowAll, "tier%d %s: dimension %q missing from result (have %v)", tier, tg, dim.ID, keys(cols))
				continue
			}
			if len(col) != int(points) {
				report(tierWindowGrid, "tier%d %s dim %q: got %d buckets, want %d (view drifted from the tier grid?)",
					tier, tg, dim.ID, len(col), points)
			}
			for i, pt := range col {
				if i >= int(points) {
					break
				}
				wantEnd := firstEnd + int64(i)*granularity
				if pt.T != wantEnd {
					report(tierWindowGrid, "tier%d %s dim %q bucket %d: time t0%+d, want t0%+d",
						tier, tg, dim.ID, i, pt.T-fixture.T0, wantEnd-fixture.T0)
					continue
				}

				want, stored := oracles[dim.ID][pt.T]
				if !stored || want.Empty {
					// never-stored and stored-empty windows read identically:
					// null value, EMPTY annotation
					if pt.Value != nil {
						report(tierWindowValue, "tier%d %s dim %q t0%+d: value %v, want null (%s window)",
							tier, tg, dim.ID, pt.T-fixture.T0, *pt.Value, emptyKind(stored))
					}
					if pt.PA&canon.AnnotationEmpty == 0 {
						report(tierWindowAnnotation, "tier%d %s dim %q t0%+d: EMPTY annotation missing on %s window (pa %d)",
							tier, tg, dim.ID, pt.T-fixture.T0, emptyKind(stored), pt.PA)
					}
					if pt.ARP != 0 {
						report(tierWindowAnomaly, "tier%d %s dim %q t0%+d: empty anomaly rate %v, want 0",
							tier, tg, dim.ID, pt.T-fixture.T0, pt.ARP)
					}
					if pt.PA != canon.AnnotationEmpty {
						report(tierWindowAnnotation, "tier%d %s dim %q t0%+d: empty annotations %d, want exactly %d",
							tier, tg, dim.ID, pt.T-fixture.T0, pt.PA, canon.AnnotationEmpty)
					}
					continue
				}

				var exp float64
				tol := ch.ValueTolerance
				switch tg {
				case "sum":
					exp = want.Sum
				case "min":
					exp = want.Min
				case "max":
					exp = want.Max
				case "average":
					exp = want.Sum / float64(want.Count)
				}
				if pt.Value == nil {
					report(tierWindowValue, "tier%d %s dim %q t0%+d: null, want %v (count %d)",
						tier, tg, dim.ID, pt.T-fixture.T0, exp, want.Count)
				} else if !tierValueMatch(*pt.Value, exp, tol) {
					report(tierWindowValue, "tier%d %s dim %q t0%+d: value %v, want %v (count %d, tolerance %v)",
						tier, tg, dim.ID, pt.T-fixture.T0, *pt.Value, exp, want.Count, tol)
				}

				expARP := 100 * float64(want.AnomalyCount) / float64(want.Count)
				if !tierValueMatch(pt.ARP, expARP, 0) {
					report(tierWindowAnomaly, "tier%d %s dim %q t0%+d: anomaly rate %v, want %v (%d/%d)",
						tier, tg, dim.ID, pt.T-fixture.T0, pt.ARP, expARP, want.AnomalyCount, want.Count)
				}

				// Tier pages do not retain source flags, so RESET is absent.
				// A reduced source-slot count is nevertheless recoverable for
				// stable-cadence pages and must mark the numeric result PARTIAL.
				wantPA := int64(0)
				if want.GapCount > 0 {
					wantPA = canon.AnnotationPartial
				}
				if pt.PA != wantPA {
					report(tierWindowAnnotation, "tier%d %s dim %q t0%+d: annotations %d, want %d (count %d, gaps %d)",
						tier, tg, dim.ID, pt.T-fixture.T0, pt.PA, wantPA, want.Count, want.GapCount)
				}
			}
		}
	}
}

func emptyKind(stored bool) string {
	if stored {
		return "stored-empty"
	}
	return "never-stored"
}

// TestLayer2Tier1Palette drives the layer-1 edge-data palette through the
// tier1 rollup. T0 is deliberately unaligned to the tier grid (T0 % 60 = 20),
// so the first window is always partial: it ends at T0+40 covering samples
// T0+1..T0+40; full windows follow every 60s. Every fixture pushes two full
// windows beyond the last asserted end (the tier write-delay settle rule).
func TestLayer2Tier1Palette(t *testing.T) {
	const b1 = fixture.T0 + 40 // first aligned tier1 window end after T0
	contracts := map[string]bool{
		"L2/tier1-complete":       true,
		"L2/tier1-interior-gaps":  true,
		"L2/tier1-anomaly-rate":   true,
		"L2/tier1-reset-flags":    true,
		"L2/tier1-float32-fields": true,
	}
	for contract := range contracts {
		registerContract(t, contract)
	}

	cases := map[string]struct {
		contract          string
		hostname          string
		guid              string
		chart             fixture.Chart
		firstEnd, lastEnd int64
	}{
		// W1 partial (40 samples), W2..W5 full — plain identity arithmetic
		"complete": {
			contract: "L2/tier1-complete",
			hostname: "l2-complete", guid: guid(41),
			chart:    fixture.Series("fixture.l2complete", "fixture.l2complete", fixture.T0, 400, 1, modVal, notAnom),
			firstEnd: b1, lastEnd: b1 + 4*tier1Gran,
		},
		// gap run i=90..170: W2 partial (count 49), W3 all-gap (stored-empty
		// tier point), W4 partial (count 50)
		"interior-gaps": {
			contract: "L2/tier1-interior-gaps",
			hostname: "l2-gaps", guid: guid(42),
			chart: fixture.Series("fixture.l2gaps", "fixture.l2gaps", fixture.T0, 400, 1, modVal, func(i int) string {
				if i >= 90 && i <= 170 {
					return stream.FlagEmpty
				}
				return stream.FlagNotAnomalous
			}),
			firstEnd: b1, lastEnd: b1 + 4*tier1Gran,
		},
		// anomaly run i=50..75 inside W2: fractional anomaly rate 26/60
		"anomaly-rate": {
			contract: "L2/tier1-anomaly-rate",
			hostname: "l2-anom", guid: guid(44),
			chart: fixture.Series("fixture.l2anom", "fixture.l2anom", fixture.T0, 280, 1, modVal, func(i int) string {
				if i >= 50 && i <= 75 {
					return stream.FlagAnomalous
				}
				return stream.FlagNotAnomalous
			}),
			firstEnd: b1, lastEnd: b1 + 2*tier1Gran,
		},
		// resets: lone-R (reset+anomalous) at i=50, AR at i=110 — the RESET
		// annotation is asserted ABSENT on every tier bucket (pages store no
		// flags); the lone-R contributes 1/60 anomaly rate to W2
		"reset-lost": {
			contract: "L2/tier1-reset-flags",
			hostname: "l2-reset", guid: guid(45),
			chart: fixture.Series("fixture.l2reset", "fixture.l2reset", fixture.T0, 280, 1, modVal, func(i int) string {
				switch i {
				case 50:
					return stream.FlagReset
				case 110:
					return stream.FlagNotAnomalous + stream.FlagReset
				}
				return stream.FlagNotAnomalous
			}),
			firstEnd: b1, lastEnd: b1 + 2*tier1Gran,
		},
		// mixed-sign fractional values: the float32 write-rounding of
		// sum/min/max is visible and must match the oracle's single cast
		"fractional-f32": {
			contract: "L2/tier1-float32-fields",
			hostname: "l2-frac", guid: guid(46),
			chart: func() fixture.Chart {
				ch := fixture.Series("fixture.l2frac", "fixture.l2frac", fixture.T0, 280, 1, func(i int) string {
					return strconv.FormatFloat(float64(i%13-6)+float64(i%7)/10, 'f', 1, 64)
				}, notAnom)
				ch.ValueTolerance = 1e-9
				return ch
			}(),
			firstEnd: b1, lastEnd: b1 + 2*tier1Gran,
		},
	}
	routes := func(caseContract string, assertion tierWindowAssertion) []string {
		valueContract := caseContract
		if caseContract == "L2/tier1-anomaly-rate" || caseContract == "L2/tier1-reset-flags" {
			valueContract = "L2/tier1-complete"
		}
		annotationContract := "L2/tier1-reset-flags"
		if caseContract == "L2/tier1-interior-gaps" {
			annotationContract = caseContract
		}

		switch assertion {
		case tierWindowValue:
			return []string{valueContract}
		case tierWindowAnomaly:
			return []string{"L2/tier1-anomaly-rate"}
		case tierWindowAnnotation:
			return []string{annotationContract}
		default:
			seen := map[string]bool{}
			var out []string
			for _, contract := range []string{valueContract, "L2/tier1-anomaly-rate", annotationContract} {
				if !seen[contract] {
					seen[contract] = true
					out = append(out, contract)
				}
			}
			return out
		}
	}

	for name, tc := range cases {
		if !t.Run(name, func(t *testing.T) {
			pushLiveBurst(t, tc.hostname, tc.guid, tc.chart)
			if _, err := td.WaitRetention(tc.hostname, tc.chart.Context, tc.chart.FirstT(), tc.chart.LastT(), 15*time.Second); err != nil {
				t.Fatal(err)
			}
			verifyTierWindowsWithReporter(t, tierWindowQuery{
				host: tc.hostname, chart: tc.chart, tier: 1,
				granularity: tier1Gran, firstEnd: tc.firstEnd, lastEnd: tc.lastEnd,
			},
				func(assertion tierWindowAssertion, format string, args ...any) {
					t.Logf(format, args...)
					for _, contract := range routes(tc.contract, assertion) {
						contracts[contract] = false
					}
				})
		}) {
			for _, contract := range routes(tc.contract, tierWindowAll) {
				contracts[contract] = false
			}
		}
	}

	for _, contract := range []string{
		"L2/tier1-complete", "L2/tier1-interior-gaps", "L2/tier1-anomaly-rate",
		"L2/tier1-reset-flags", "L2/tier1-float32-fields",
	} {
		assertContract(t, contract, contracts[contract])
	}
}

// TestLayer2PartialWidePoint pins PARTIAL propagation when one partial
// higher-tier record is projected into several result rows. PARTIAL describes
// the source evidence behind each derived row, so it must not disappear after
// the record's first delivery.
func TestLayer2PartialWidePoint(t *testing.T) {
	registerContract(t, "L2/partial-wide-point")
	registerContract(t, "L2/partial-wide-point-values")

	const value = 7
	ch := fixture.Series("fixture.l2partialwide", "fixture.l2partialwide", fixture.T0, 280, 1,
		func(int) string { return strconv.Itoa(value) }, notAnom)

	pushLiveBurst(t, "l2-partial-wide", guid(51), ch)
	if _, err := td.WaitRetention("l2-partial-wide", ch.Context, ch.FirstT(), ch.LastT(), 15*time.Second); err != nil {
		t.Fatal(err)
	}

	const (
		firstEnd = int64(fixture.T0 + 40)
		rowSpan  = int64(10)
		rows     = int64(6)
	)
	after := firstEnd - tier1Gran
	doc, err := td.DataV3("l2-partial-wide",
		daemon.DataParamsTier(ch.Context, 1, after, firstEnd, rows, "average"))
	if err != nil {
		t.Fatal(err)
	}
	commonOK := true
	if !assertSelectedTier(t, doc, 1) || !assertExactView(t, doc, after, firstEnd, rowSpan) {
		commonOK = false
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !assertOnlyColumn(t, cols, ch.Dimensions[0].ID) {
		commonOK = false
	}
	for i, point := range cols[ch.Dimensions[0].ID] {
		if point.Value == nil || math.IsNaN(*point.Value) || math.IsInf(*point.Value, 0) {
			t.Logf("dimension %q row %d at %d is not numeric", ch.Dimensions[0].ID, i, point.T)
			commonOK = false
		}
	}
	want := make([]expectedColumnPoint, rows)
	for i := range want {
		want[i] = wantNumberWithPAAt(after+int64(i+1)*rowSpan, value, canon.AnnotationPartial)
	}
	t.Run("values", func(t *testing.T) {
		ok := commonOK && assertExactColumnValues(t, cols, ch.Dimensions[0].ID, want, 0)
		assertContract(t, "L2/partial-wide-point-values", ok)
	})
	t.Run("evidence", func(t *testing.T) {
		ok := commonOK && assertExactColumnMetadata(t, cols, ch.Dimensions[0].ID, want)
		assertContract(t, "L2/partial-wide-point", ok)
	})
}

// TestLayer2WholeChartAbsence pins the two flavors of a missing tier window:
// samples exist for W1..W2-part and again from W5, with NOTHING pushed in
// between — W3/W4 are never stored (vs the stored NAN/count-0 point of an
// all-gap window). Both must read back as null + EMPTY; the flanking partial
// windows carry the reduced counts.
func TestLayer2WholeChartAbsence(t *testing.T) {
	trackContract(t, "L2/whole-chart-absence")

	const b1 = fixture.T0 + 40
	points := make([]fixture.Point, 0, 240)
	for i := 1; i <= 400; i++ {
		if i > 80 && i < 241 {
			continue // whole-chart gap: these samples are never sent
		}
		points = append(points, fixture.Point{
			T: fixture.T0 + int64(i), Collected: strconv.Itoa(i % 10), Flags: stream.FlagNotAnomalous,
		})
	}
	ch := fixture.Chart{
		ID: "fixture.l2absence", Title: "Corpus series", Units: "units", Family: "fixture",
		Context: "fixture.l2absence", UpdateEvery: 1,
		Dimensions: []fixture.Dimension{{ID: "load", Points: points}},
	}

	pushLiveBurst(t, "l2-absence", guid(43), ch)
	if _, err := td.WaitRetention("l2-absence", ch.Context, ch.FirstT(), ch.LastT(), 15*time.Second); err != nil {
		t.Fatal(err)
	}
	// W1 (count 40), W2 partial (count 40: i 41..80), W3/W4 never stored,
	// W5 partial (count 40: i 241..280)
	verifyTierWindows(t, "l2-absence", ch, 1, tier1Gran, b1, b1+4*tier1Gran)
}

// TestLayer2SNvsOriginal is the sharp pin of the "tiers aggregate ORIGINAL
// values" contract: 16777217 (2^24+1) quantizes at tier0 to 16777220 (decimal
// mantissa step), while float32-of-original is 16777216. If the engine ever
// fed tier rollups from the quantized tier0 values, every tier1 field would
// read 16777220. The same fixture cross-checks the tier0 identity
// (SNRoundTrip oracle) so both contracts are asserted on the same data.
func TestLayer2SNvsOriginal(t *testing.T) {
	for _, contract := range []string{
		"L2/tier0-storage-number-quantization",
		"L2/tier-rollup-original-values",
	} {
		registerContract(t, contract)
	}

	const v = "16777217"
	ch := fixture.Series("fixture.l2snorig", "fixture.l2snorig", fixture.T0, 280, 1, func(_ int) string {
		return v
	}, notAnom)

	pushLiveBurst(t, "l2-snorig", guid(47), ch)

	t.Run("tier0-storage-number", func(t *testing.T) {
		trackContract(t, "L2/tier0-storage-number-quantization")
		settleAndVerify(t, "l2-snorig", ch)
	})

	t.Run("tier-rollup-original", func(t *testing.T) {
		trackContract(t, "L2/tier-rollup-original-values")
		if _, err := td.WaitRetention("l2-snorig", ch.Context, ch.FirstT(), ch.LastT(), 15*time.Second); err != nil {
			t.Fatal(err)
		}
		f, _ := strconv.ParseFloat(v, 64)
		if q := fixture.SNRoundTrip(f); q == float64(float32(f)) {
			t.Fatalf("fixture lost its discriminating power: SNRoundTrip(%s)=%v equals float32(%s)=%v",
				v, q, v, float64(float32(f)))
		}
		verifyTierWindows(t, "l2-snorig", ch, 1, tier1Gran, fixture.T0+40, fixture.T0+40+2*tier1Gran)
	})
}

// TestLayer2UpdateEvery5 exercises the tier grid arithmetic with a
// non-default update_every: granularity 5×60=300, first aligned end T0+100.
func TestLayer2UpdateEvery5(t *testing.T) {
	trackContract(t, "L2/update-every-5")

	ch := fixture.Series("fixture.l2ue5", "fixture.l2ue5", fixture.T0, 260, 5, modVal, notAnom)

	pushLiveBurst(t, "l2-ue5", guid(48), ch)
	if _, err := td.WaitRetention("l2-ue5", ch.Context, ch.FirstT(), ch.LastT(), 15*time.Second); err != nil {
		t.Fatal(err)
	}
	const gran = 5 * tier1Gran
	// W1 ends T0+100 (20 samples), W2 ends T0+400 (60) — W3/W4 are the margin
	verifyTierWindows(t, "l2-ue5", ch, 1, gran, fixture.T0+100, fixture.T0+100+gran)
}

// TestLayer2Tier2 rolls 17200 replicated samples into tier2 (granularity
// 3600, first aligned end T0+2800): W1 partial (2800), W2 full (3600), W3
// carrying a gap run (samples 6401..6499 only — count 99). The same fixture
// asserts a stretch of tier1 windows around the gap boundary, so both rollup
// levels are pinned on identical data.
func TestLayer2Tier2(t *testing.T) {
	trackContract(t, "L2/tier2")

	ch := fixture.Series("fixture.l2tier2", "fixture.l2tier2", fixture.T0, 17200, 1, func(i int) string {
		return strconv.Itoa(i % 1000)
	}, func(i int) string {
		if i >= 6500 && i <= 10000 {
			return stream.FlagEmpty
		}
		return stream.FlagNotAnomalous
	})
	ch.ValueTolerance = 1e-9 // average buckets are fractional; JSON print tolerance

	pushReplication(t, "l2-tier2", guid(49), ch)
	if _, err := td.WaitRetention("l2-tier2", ch.Context, ch.FirstT(), ch.LastT(), 30*time.Second); err != nil {
		t.Fatal(err)
	}

	verifyTierWindows(t, "l2-tier2", ch, 2, tier2Gran, fixture.T0+2800, fixture.T0+2800+2*tier2Gran)

	// tier1 cross-check on the same data, from before the data (clean start,
	// see CASE-017) through the gap-run boundary: full windows, the partial
	// edge (6401..6499) and all-gap stored-empty windows
	verifyTierWindows(t, "l2-tier2", ch, 1, tier1Gran, fixture.T0+40, fixture.T0+7600)
}
