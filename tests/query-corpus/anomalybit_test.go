// SPDX-License-Identifier: GPL-3.0-or-later

// S3 — the anomaly-bit surface: options=anomaly-bit replaces every
// fetched point's VALUE with its anomaly rate BEFORE time-grouping
// (query-execute.c use_anomaly_bit_as_value), so:
//   - at tier0 identity the values are exactly 0/100 per sample;
//   - time-grouped buckets aggregate the rates (average = the bucket's
//     anomaly percentage, max = "any anomaly in the bucket");
//   - group-by consumes the rates as values (sum adds them across
//     members);
//   - at tier>0 the per-point rate is FRACTIONAL (100*anomaly_count/
//     count of the tier window) and feeds the grouping as such.
//
// The same fixture pins the jsonwrap-v2 per-dimension anomaly arrays
// (never decoded by the corpus before): view.dimensions.sts.arp and
// db.dimensions.sts.arp.
//
// Fixtures are self-contained (own hosts), so this file has no ordering
// dependency on the layer tests.
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

const abContext = "fixture.anombit"

// abFixture: dim aa (value i) anomalous rows 15-24 with a gap run 51-55;
// dim bb (value 2i) anomalous rows 21-40.
func abFixture() fixture.Chart {
	var aa, bb fixture.Dimension
	aa.ID = "aa"
	bb.ID = "bb"
	for i := 1; i <= 60; i++ {
		fa := stream.FlagNotAnomalous
		switch {
		case i >= 51 && i <= 55:
			fa = stream.FlagEmpty
		case i >= 15 && i <= 24:
			fa = stream.FlagAnomalous
		}
		fb := stream.FlagNotAnomalous
		if i >= 21 && i <= 40 {
			fb = stream.FlagAnomalous
		}
		aa.Points = append(aa.Points, fixture.Point{T: fixture.T0 + int64(i), Collected: strconv.Itoa(i), Flags: fa})
		bb.Points = append(bb.Points, fixture.Point{T: fixture.T0 + int64(i), Collected: strconv.Itoa(2 * i), Flags: fb})
	}
	return fixture.Chart{
		ID: abContext, Title: "anomaly bit", Units: "units", Family: "fixture",
		Context: abContext, UpdateEvery: 1,
		Dimensions: []fixture.Dimension{aa, bb},
	}
}

func abRate(anomalous bool) float64 {
	if anomalous {
		return 100
	}
	return 0
}

// aaAnom/bbAnom/aaGap mirror the fixture shape for the oracles.
func aaAnom(i int) bool { return i >= 15 && i <= 24 }
func bbAnom(i int) bool { return i >= 21 && i <= 40 }
func aaGap(i int) bool  { return i >= 51 && i <= 55 }

func TestAnomalyBitOption(t *testing.T) {
	ch := abFixture()
	pushLiveBurst(t, "anom-bit", guid(69), ch)
	if _, err := td.WaitRetention("anom-bit", abContext, ch.FirstT(), ch.LastT(), 15*time.Second); err != nil {
		t.Fatal(err)
	}

	query := func(extra func(p map[string][]string)) map[string][]canon.Pt {
		t.Helper()
		params := daemon.DataParams(abContext, fixture.T0, fixture.T0+60, 60)
		params.Set("options", "jsonwrap|anomaly-bit")
		if extra != nil {
			extra(params)
		}
		doc, err := td.DataV3("anom-bit", params)
		if err != nil {
			t.Fatal(err)
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}
		return cols
	}

	t.Run("identity", func(t *testing.T) {
		trackContract(t, "L3/anomaly-bit-identity")

		cols := query(nil)
		if !assertExactColumnSet(t, cols, []string{"aa", "bb"}) {
			t.Fail()
		}
		aaWant := make([]expectedColumnPoint, 0, 60)
		bbWant := make([]expectedColumnPoint, 0, 60)
		for i := 1; i <= 60; i++ {
			ts := fixture.T0 + int64(i)
			if aaGap(i) {
				aaWant = append(aaWant, wantEmptyAt(ts))
			} else {
				aaWant = append(aaWant, wantNumberAt(ts, abRate(aaAnom(i))))
			}
			bbWant = append(bbWant, wantNumberAt(ts, abRate(bbAnom(i))))
		}
		if !assertExactColumn(t, cols, "aa", aaWant, 0) ||
			!assertExactColumn(t, cols, "bb", bbWant, 0) {
			t.Fail()
		}
	})

	bucketWant := func(anom func(int) bool, gap func(int) bool, b int, maxMode bool) float64 {
		sum, n := 0.0, 0
		peak := 0.0
		for i := (b-1)*10 + 1; i <= b*10; i++ {
			if gap != nil && gap(i) {
				continue
			}
			r := abRate(anom(i))
			sum += r
			if r > peak {
				peak = r
			}
			n++
		}
		if maxMode {
			return peak
		}
		return sum / float64(n)
	}

	t.Run("buckets", func(t *testing.T) {
		trackContract(t, "L3/anomaly-bit-buckets")

		for _, tg := range []string{"average", "max"} {
			t.Run(tg, func(t *testing.T) {
				cols := query(func(p map[string][]string) {
					p["points"] = []string{"6"}
					p["time_group"] = []string{tg}
				})
				if !assertExactColumnSet(t, cols, []string{"aa", "bb"}) {
					t.Fail()
				}
				aaWant := make([]expectedColumnPoint, 0, 6)
				bbWant := make([]expectedColumnPoint, 0, 6)
				for bucket := 1; bucket <= 6; bucket++ {
					ts := fixture.T0 + int64(bucket*10)
					aaWant = append(aaWant, wantNumberAt(
						ts, bucketWant(aaAnom, aaGap, bucket, tg == "max")))
					bbWant = append(bbWant, wantNumberAt(
						ts, bucketWant(bbAnom, nil, bucket, tg == "max")))
				}
				if !assertExactColumn(t, cols, "aa", aaWant, printTol) ||
					!assertExactColumn(t, cols, "bb", bbWant, printTol) {
					t.Fail()
				}
			})
		}
	})

	t.Run("group-by-sum", func(t *testing.T) {
		trackContract(t, "L3/anomaly-bit-group-by")

		cols := query(func(p map[string][]string) {
			p["group_by"] = []string{"selected"}
			p["aggregation"] = []string{"sum"}
		})
		col := cols["selected"]
		if !assertOnlyColumn(t, cols, "selected") ||
			!assertColumnExactGrid(t, cols, "selected", fixture.T0, fixture.T0+60, 1) {
			t.Fatal("group-by sum returned an incomplete result grid")
		}
		for _, pt := range col {
			i := int(pt.T - fixture.T0)
			want := abRate(bbAnom(i))
			wantAR := want
			gbc := 1
			if !aaGap(i) {
				want += abRate(aaAnom(i))
				wantAR += abRate(aaAnom(i))
				gbc = 2
			}
			wantAR /= float64(gbc)
			if pt.Value == nil || !tierValueMatch(*pt.Value, want, 1e-9) {
				t.Errorf("row %d: %s, want %v", i, fmtPt(pt), want)
			}
			if !tierValueMatch(pt.ARP, wantAR, 1e-9) {
				t.Errorf("row %d: arp %v, want %v", i, pt.ARP, wantAR)
			}
			gotPartial := pt.PA&canon.AnnotationPartial != 0
			if gotPartial != (gbc == 1) {
				t.Errorf("row %d: partial %v, want %v", i, gotPartial, gbc == 1)
			}
		}
	})
}

// TestAnomalyStsArrays pins the jsonwrap-v2 per-dimension anomaly
// arrays on a PLAIN query (no anomaly-bit): view.dimensions.sts.arp is
// the per-dimension mean of the plotted rows' anomaly rates;
// db.dimensions.sts.arp is the anomaly rate of the FETCHED db points.
func TestAnomalyStsArrays(t *testing.T) {
	trackContract(t, "L5/anomaly-statistics")

	ch := abFixture()
	pushLiveBurst(t, "anom-sts", guid(423), ch)
	if _, err := td.WaitRetention("anom-sts", abContext, ch.FirstT(), ch.LastT(), 15*time.Second); err != nil {
		t.Fatal(err)
	}
	params := daemon.DataParams(abContext, fixture.T0, fixture.T0+60, 60)
	doc, err := td.DataV3("anom-sts", params)
	if err != nil {
		t.Fatal(err)
	}

	// aa: 10 anomalous rows of 55 present; bb: 20 of 60
	wantView := map[string]float64{
		"aa": 100.0 * 10 / 55,
		"bb": 100.0 * 20 / 60,
	}
	wantIDs := keys2(wantView)
	view, viewOK := strictDimensionStats(t, doc, "view", wantIDs, []string{"arp"})
	if !viewOK {
		t.Errorf("view anomaly statistics are malformed")
	}
	for id, want := range wantView {
		got, ok := view[id]["arp"]
		if !ok {
			t.Errorf("view sts arp missing for %q (have %v)", id, view[id])
			continue
		}
		if !tierValueMatch(got, want, 1e-9) {
			t.Errorf("view arp[%s] = %v, want %v", id, got, want)
		}
	}

	dbSts, dbOK := strictDimensionStats(t, doc, "db", wantIDs, []string{"arp"})
	if !dbOK {
		t.Errorf("db anomaly statistics are malformed")
	}
	for id, want := range wantView {
		got, ok := dbSts[id]["arp"]
		if !ok {
			t.Errorf("db sts arp missing for %q (have %v)", id, dbSts[id])
			continue
		}
		if !tierValueMatch(got, want, 1e-9) {
			t.Errorf("db arp[%s] = %v, want %v", id, got, want)
		}
	}
}

// TestAnomalyBitTierRates pins the fractional tier>0 rates: with
// options=anomaly-bit over a forced tier1 query, every view point is
// the tier window's 100*anomaly_count/count.
func TestAnomalyBitTierRates(t *testing.T) {
	trackContract(t, "L3/anomaly-bit-tier-rates")

	ch := fixture.Series("fixture.anomtier", "fixture.anomtier", fixture.T0, 600, 1, strconv.Itoa, func(i int) string {
		if i >= 100 && i <= 159 {
			return stream.FlagAnomalous
		}
		return stream.FlagNotAnomalous
	})
	pushReplication(t, "anom-tier", guid(71), ch)
	if _, err := td.WaitRetention("anom-tier", ch.Context, ch.FirstT(), ch.LastT(), 15*time.Second); err != nil {
		t.Fatal(err)
	}

	windows := ch.Dimensions[0].TierWindows(60, int64(ch.UpdateEvery))

	// same bounds discipline as layer 2: assert aligned window ends
	// [T0+40 .. T0+520] and leave the tail out (the most recent
	// completed tier window is not queryable yet)
	const firstEnd = fixture.T0 + 40
	const lastEnd = fixture.T0 + 520
	after := int64(firstEnd - 60)
	points := (lastEnd - after) / 60

	params := daemon.DataParamsTier(ch.Context, 1, after, lastEnd, points, "average")
	params.Set("options", "jsonwrap|anomaly-bit")
	doc, err := td.DataV3("anom-tier", params)
	if err != nil {
		t.Fatal(err)
	}
	if !assertSelectedTier(t, doc, 1) {
		t.Fatal("tier anomaly-rate query was not served only by forced tier 1")
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}
	col := cols[ch.Dimensions[0].ID]
	if !assertOnlyColumn(t, cols, ch.Dimensions[0].ID) ||
		!assertColumnExactGrid(t, cols, ch.Dimensions[0].ID, after, lastEnd, 60) {
		t.Fatal("tier anomaly-rate query returned an incomplete result grid")
	}
	for _, pt := range col {
		w, ok := windows[pt.T]
		if !ok || w.Count == 0 {
			t.Errorf("window t0%+d: no oracle window", pt.T-fixture.T0)
			continue
		}
		want := 100 * float64(w.AnomalyCount) / float64(w.Count)
		if pt.Value == nil || !tierValueMatch(*pt.Value, want, 1e-9) {
			t.Errorf("window t0%+d: %s, want %v (%d/%d)", pt.T-fixture.T0, fmtPt(pt), want, w.AnomalyCount, w.Count)
		}
	}
}
