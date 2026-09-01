// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-025 pins where a wide stored record's sum belongs, and what the
// apportionment must not touch.
//
// A result row receives the share of every stored record whose time interval
// overlaps the row. This is stricter than whole-window conservation: paying a
// record to the wrong row can preserve the total while making both rows false.
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

func c025ExpectedSum(
	base, before, rowSpan int64,
	samples, updateEvery int,
	included func(int) bool,
) []expectedColumnPoint {
	points := (before - base) / rowSpan
	want := make([]expectedColumnPoint, points)
	for row := range want {
		rowStart := base + int64(row)*rowSpan
		rowEnd := rowStart + rowSpan
		var value float64
		partial := false
		for i := 1; i <= samples; i++ {
			recordStart := base + int64((i-1)*updateEvery)
			recordEnd := recordStart + int64(updateEvery)
			overlapStart := rowStart
			if recordStart > overlapStart {
				overlapStart = recordStart
			}
			overlapEnd := rowEnd
			if recordEnd < overlapEnd {
				overlapEnd = recordEnd
			}
			if overlapEnd > overlapStart {
				if included(i) {
					value += float64(i) * float64(overlapEnd-overlapStart) / float64(updateEvery)
				} else {
					partial = true
				}
			}
		}
		if value == 0 {
			want[row] = wantEmptyWithMetadataAt(rowEnd, 0, canon.AnnotationEmpty)
		} else {
			pa := int64(0)
			if partial {
				pa = canon.AnnotationPartial
			}
			want[row] = wantNumberWithMetadataAt(rowEnd, value, 0, pa)
		}
	}
	return want
}

func TestCase025OverlapOracle(t *testing.T) {
	first, second := 2.0, 1.0
	valid := map[string][]canon.Pt{"value": {
		{T: 15, Value: &first},
		{T: 30, Value: &second},
		{T: 45, PA: canon.AnnotationEmpty},
	}}
	want := c025ExpectedSum(0, 45, 15, 2, 10, func(int) bool { return true })
	if !assertExactColumn(t, valid, "value", want, 0) {
		t.Fatal("overlap oracle rejected records (0,10]=1 and (10,20]=2 over 15-second rows")
	}
}

// CASE-025a uses 35-second rows over 10-second records. The dense dimension
// ends mid-row, while the gapped dimension puts one settlement-only row
// between a run and a real empty row. Both shapes require exact row ownership;
// a whole-window total alone cannot distinguish them.
func TestCase025CarrySurvivesGaps(t *testing.T) {
	registerContract(t, "CASE-025/carry-survives-gaps")
	registerContract(t, "CASE-025/gap-evidence-follows-row-overlap")

	const (
		ctx     = "fixture.c025gap"
		host    = "c025gap"
		ue      = 10
		samples = 60 // 600s of data, then nothing
		rowSpan = 35
	)

	base := fixture.T0 - fixture.T0%int64(ue)
	ch := fixture.Series(ctx, ctx, base, samples, ue,
		func(i int) string { return strconv.Itoa(i) },
		func(int) string { return stream.FlagNotAnomalous })
	ch.Dimensions = append(ch.Dimensions, fixture.Dimension{ID: "interior-gap"})
	for i := 1; i <= samples; i++ {
		point := fixture.Point{
			T:         base + int64(i*ue),
			Collected: strconv.Itoa(i),
			Flags:     stream.FlagEmpty,
		}
		if i <= 4 || (i >= 12 && i <= 14) {
			point.Flags = stream.FlagNotAnomalous
		}
		ch.Dimensions[1].Points = append(ch.Dimensions[1].Points, point)
	}

	pushLiveBurst(t, host, guid(250), ch)
	if _, err := td.WaitRetention(host, ch.Context, ch.FirstT(), ch.LastT(), 20*time.Second); err != nil {
		t.Fatal(err)
	}

	valuesOK, evidenceOK := true, true
	for _, tc := range []struct {
		name, dimension string
		before          int64
		want            []expectedColumnPoint
	}{
		{
			name:      "retention-tail",
			dimension: ch.Dimensions[0].ID,
			before:    base + samples*ue + 100,
			want: c025ExpectedSum(
				base, base+samples*ue+100, rowSpan, samples, ue, func(int) bool { return true }),
		},
		{
			name:      "interior-gap",
			dimension: ch.Dimensions[1].ID,
			before:    base + 4*rowSpan,
			want: c025ExpectedSum(
				base, base+4*rowSpan, rowSpan, samples, ue,
				func(i int) bool { return i <= 4 || (i >= 12 && i <= 14) }),
		},
	} {
		after := base
		points := (tc.before - after) / rowSpan
		params := daemon.DataParamsTier(ctx, 0, after, tc.before, points, "sum")
		params.Set("options", "jsonwrap|unaligned")
		params.Set("scope_dimensions", tc.dimension)
		doc, err := td.DataV3(host, params)
		if err != nil {
			t.Fatal(err)
		}
		if !assertSelectedTier(t, doc, 0) {
			valuesOK, evidenceOK = false, false
		}
		if !assertExactView(t, doc, after, tc.before, rowSpan) {
			valuesOK, evidenceOK = false, false
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}
		if !assertOnlyColumn(t, cols, tc.dimension) {
			valuesOK, evidenceOK = false, false
		}
		if !assertExactColumnValues(t, cols, tc.dimension, tc.want, 1e-9) {
			t.Logf("%s: wide-record shares did not land on their exact owning rows", tc.name)
			valuesOK = false
		}
		if !assertExactColumnMetadata(t, cols, tc.dimension, tc.want) {
			t.Logf("%s: stored gap evidence did not follow the rows owning those intervals", tc.name)
			evidenceOK = false
		}
	}

	assertContract(t, "CASE-025/carry-survives-gaps", valuesOK)
	assertContract(t, "CASE-025/gap-evidence-follows-row-overlap", evidenceOK)
}

// CASE-025b: a bucket that lies entirely inside one stored window reports
// THAT window's anomaly rate, un-blended.
//
// options=anomaly-bit answers about the anomaly RATE, and sum's seconds-owed
// arithmetic is skipped for it - the rate says nothing about the metric's
// magnitude. What must NOT happen is the other half of the boundary
// machinery reaching it: blending the rate with the window before it.
//
// A bucket carved inside a fully-anomalous stored window contains no sample
// from the window before it, so blending would report the metric as less
// anomalous than every sample in the bucket actually was. The value is 100
// because all 60 samples under it are anomalous, and it stays 100 however
// finely the window is cut.
//
// The fixture puts a hard 0 -> 100 step on a stored window boundary and asks
// for three buckets inside the first fully-anomalous window. All three are
// 100, under every grouping - a blended 33/67/100 would be the step smeared
// backwards into seconds it never touched.
func TestCase025AnomalyBitNotBlended(t *testing.T) {
	trackContract(t, "CASE-025/anomaly-bit-not-blended")

	const (
		ctx     = "fixture.c025anom"
		host    = "c025anom"
		ue      = 1
		samples = 600
	)

	base := fixture.T0 - fixture.T0%int64(60)
	// the first half is never anomalous, the second half always is
	ch := fixture.Series(ctx, ctx, base, samples, ue,
		func(int) string { return "10" },
		func(i int) string {
			if i > samples/2 {
				return stream.FlagAnomalous
			}
			return stream.FlagNotAnomalous
		})

	pushLiveBurst(t, host, guid(251), ch)
	if _, err := td.WaitRetention(host, ch.Context, ch.FirstT(), ch.LastT(), 20*time.Second); err != nil {
		t.Fatal(err)
	}

	// tier 1 rolls 60 samples into one stored window. Query the first fully
	// anomalous window so its predecessor is the final healthy window: HOLD
	// returns 100/100/100, LINEAR returns 33/67/100, and TOTAL returns
	// 33/33/33.
	step := base + int64(samples/2)
	after := step
	before := after + 60

	ok := true

	// every grouping asks the same question of an anomaly rate, and every
	// bucket here sits inside one fully-anomalous stored window
	for _, group := range []string{"average", "min", "max", "sum"} {
		params := daemon.DataParamsTier(ctx, 1, after, before, 3, group)
		params.Set("options", "jsonwrap|unaligned|anomaly-bit")
		params.Set("scope_dimensions", ch.Dimensions[0].ID)
		doc, err := td.DataV3(host, params)
		if err != nil {
			t.Fatal(err)
		}
		if !assertSelectedTier(t, doc, 1) {
			ok = false
		}
		if !assertExactView(t, doc, after, before, 20) {
			ok = false
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}
		if !assertOnlyColumn(t, cols, ch.Dimensions[0].ID) {
			ok = false
		}
		want := []expectedColumnPoint{
			wantNumberWithMetadataAt(after+20, 100, 100, 0),
			wantNumberWithMetadataAt(after+40, 100, 100, 0),
			wantNumberWithMetadataAt(after+60, 100, 100, 0),
		}
		if !assertExactColumn(t, cols, ch.Dimensions[0].ID, want, 1e-9) {
			t.Logf("anomaly-bit contract not met: %s changed a pure 100%% window", group)
			ok = false
		}
	}

	assertContract(t, "CASE-025/anomaly-bit-not-blended", ok)
}
