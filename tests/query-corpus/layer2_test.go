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

	after := firstEnd - granularity
	points := (lastEnd - after) / granularity

	oracles := make(map[string]map[int64]fixture.TierPoint, len(ch.Dimensions))
	for _, d := range ch.Dimensions {
		oracles[d.ID] = d.TierWindows(granularity)
	}

	for _, tg := range []string{"sum", "min", "max", "average"} {
		doc, err := td.DataV3(host, daemon.DataParamsTier(ch.Context, tier, after, lastEnd, points, tg))
		if err != nil {
			t.Fatalf("tier%d %s query: %v", tier, tg, err)
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatalf("tier%d %s decode: %v", tier, tg, err)
		}

		for _, dim := range ch.Dimensions {
			col, ok := cols[dim.ID]
			if !ok {
				t.Fatalf("tier%d %s: dimension %q missing from result (have %v)", tier, tg, dim.ID, keys(cols))
			}
			if len(col) != int(points) {
				t.Fatalf("tier%d %s dim %q: got %d buckets, want %d (view drifted from the tier grid?)",
					tier, tg, dim.ID, len(col), points)
			}
			for i, pt := range col {
				wantEnd := firstEnd + int64(i)*granularity
				if pt.T != wantEnd {
					t.Errorf("tier%d %s dim %q bucket %d: time t0%+d, want t0%+d",
						tier, tg, dim.ID, i, pt.T-fixture.T0, wantEnd-fixture.T0)
					continue
				}

				want, stored := oracles[dim.ID][pt.T]
				if !stored || want.Empty {
					// never-stored and stored-empty windows read identically:
					// null value, EMPTY annotation
					if pt.Value != nil {
						t.Errorf("tier%d %s dim %q t0%+d: value %v, want null (%s window)",
							tier, tg, dim.ID, pt.T-fixture.T0, *pt.Value, emptyKind(stored))
					}
					if pt.PA&canon.AnnotationEmpty == 0 {
						t.Errorf("tier%d %s dim %q t0%+d: EMPTY annotation missing on %s window (pa %d)",
							tier, tg, dim.ID, pt.T-fixture.T0, emptyKind(stored), pt.PA)
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
					t.Errorf("tier%d %s dim %q t0%+d: null, want %v (count %d)",
						tier, tg, dim.ID, pt.T-fixture.T0, exp, want.Count)
					continue
				}
				if !tierValueMatch(*pt.Value, exp, tol) {
					t.Errorf("tier%d %s dim %q t0%+d: value %v, want %v (count %d, tolerance %v)",
						tier, tg, dim.ID, pt.T-fixture.T0, *pt.Value, exp, want.Count, tol)
				}

				expARP := 100 * float64(want.AnomalyCount) / float64(want.Count)
				if !tierValueMatch(pt.ARP, expARP, 0) {
					t.Errorf("tier%d %s dim %q t0%+d: anomaly rate %v, want %v (%d/%d)",
						tier, tg, dim.ID, pt.T-fixture.T0, pt.ARP, expARP, want.AnomalyCount, want.Count)
				}

				// tier pages store no flags: the RESET annotation does not
				// survive to tier1+ — pinned contract (page.c tier1 slot)
				if pt.PA&canon.AnnotationReset != 0 {
					t.Errorf("tier%d %s dim %q t0%+d: RESET annotation present — tier pages gained flags? re-pin deliberately",
						tier, tg, dim.ID, pt.T-fixture.T0)
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

	cases := map[string]struct {
		hostname          string
		guid              string
		chart             fixture.Chart
		firstEnd, lastEnd int64
	}{
		// W1 partial (40 samples), W2..W5 full — plain identity arithmetic
		"complete": {
			hostname: "l2-complete", guid: guid(41),
			chart:    fixture.Series("fixture.l2complete", "fixture.l2complete", fixture.T0, 400, 1, modVal, notAnom),
			firstEnd: b1, lastEnd: b1 + 4*tier1Gran,
		},
		// gap run i=90..170: W2 partial (count 49), W3 all-gap (stored-empty
		// tier point), W4 partial (count 50)
		"interior-gaps": {
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
			hostname: "l2-reset", guid: guid(45),
			chart: fixture.Series("fixture.l2reset", "fixture.l2reset", fixture.T0, 280, 1, modVal, func(i int) string {
				switch i {
				case 50:
					return "R"
				case 110:
					return "AR"
				}
				return stream.FlagNotAnomalous
			}),
			firstEnd: b1, lastEnd: b1 + 2*tier1Gran,
		},
		// mixed-sign fractional values: the float32 write-rounding of
		// sum/min/max is visible and must match the oracle's single cast
		"fractional-f32": {
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

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			pushLiveBurst(t, tc.hostname, tc.guid, tc.chart)
			if _, err := td.WaitRetention(tc.hostname, tc.chart.Context, tc.chart.FirstT(), tc.chart.LastT(), 15*time.Second); err != nil {
				t.Fatal(err)
			}
			verifyTierWindows(t, tc.hostname, tc.chart, 1, tier1Gran, tc.firstEnd, tc.lastEnd)
		})
	}
}

// TestLayer2WholeChartAbsence pins the two flavors of a missing tier window:
// samples exist for W1..W2-part and again from W5, with NOTHING pushed in
// between — W3/W4 are never stored (vs the stored NAN/count-0 point of an
// all-gap window). Both must read back as null + EMPTY; the flanking partial
// windows carry the reduced counts.
func TestLayer2WholeChartAbsence(t *testing.T) {
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
	const v = "16777217"
	ch := fixture.Series("fixture.l2snorig", "fixture.l2snorig", fixture.T0, 280, 1, func(_ int) string {
		return v
	}, notAnom)

	pushLiveBurst(t, "l2-snorig", guid(47), ch)
	settleAndVerify(t, "l2-snorig", ch) // tier0: every value reads SNRoundTrip(16777217) = 16777220

	f, _ := strconv.ParseFloat(v, 64)
	if q := fixture.SNRoundTrip(f); q == float64(float32(f)) {
		t.Fatalf("fixture lost its discriminating power: SNRoundTrip(%s)=%v equals float32(%s)=%v",
			v, q, v, float64(float32(f)))
	}
	verifyTierWindows(t, "l2-snorig", ch, 1, tier1Gran, fixture.T0+40, fixture.T0+40+2*tier1Gran)
}

// TestLayer2UpdateEvery5 exercises the tier grid arithmetic with a
// non-default update_every: granularity 5×60=300, first aligned end T0+100.
func TestLayer2UpdateEvery5(t *testing.T) {
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
