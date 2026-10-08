// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-027 what a series rose by does not depend on how the chart is cut.
//
// time_group=incremental-sum answers "how much did this value change in
// this bucket", so the buckets of a window are a telescoping series: each
// one measures from where the one before it ended, and they must add up to
// the change between the first reading and the last. That is true at any
// resolution - it is the same rise, read through different windows.
//
// The mechanism that makes it true is one line: a bucket hands its last
// sample forward as the next bucket's baseline. When a bucket holds a
// SINGLE sample there is no "last" distinct from the baseline it just
// captured, and a carry that copies the missing one over the real one
// destroys the baseline instead of passing it on. Every bucket then has a
// baseline and nothing to measure against it, so every bucket answers
// EMPTY - and one bucket per collection interval is the most natural way
// there is to draw a chart.
//
// The zooms below bracket that: one bucket per stored record, several
// buckets per record, and several records per bucket. Only the middle
// group is safe by construction; the first two are the shape that blanked.
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

func TestCase027IncrementalSumConservesAcrossZoom(t *testing.T) {
	trackContract(t, "CASE-027/incremental-sum-conserves-across-zoom")

	const (
		ctx     = "fixture.c027inc"
		host    = "c027inc"
		ue      = 10
		samples = 60
		base0   = 100 // first reading
		step    = 7   // and its rise per stored record
	)

	base := fixture.T0 - fixture.T0%int64(ue)
	ch := fixture.Series(ctx, ctx, base, samples, ue,
		func(i int) string { return strconv.Itoa(base0 + (i-1)*step) },
		func(int) string { return stream.FlagNotAnomalous })

	pushLiveBurst(t, host, guid(253), ch)
	if _, err := td.WaitRetention(host, ch.Context, ch.FirstT(), ch.LastT(), 20*time.Second); err != nil {
		t.Fatal(err)
	}

	after := base
	before := base + int64(samples*ue)

	// what the fixture rose by, first reading to last
	want := float64((samples - 1) * step)

	ok := true

	for _, points := range []int64{
		int64(samples * ue), // 1s buckets: ten of them per stored record
		int64(samples),      // one bucket per stored record
		int64(samples / 5),  // five records per bucket
		int64(samples / 10), // ten records per bucket
	} {
		params := daemon.DataParamsTier(ctx, 0, after, before, points, "incremental-sum")
		params.Set("options", "jsonwrap|unaligned")
		params.Set("scope_dimensions", ch.Dimensions[0].ID)
		doc, err := td.DataV3(host, params)
		if err != nil {
			t.Fatal(err)
		}
		if !assertSelectedTier(t, doc, 0) {
			ok = false
		}
		rowSpan := (before - after) / points
		if !assertExactView(t, doc, after, before, rowSpan) {
			ok = false
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}
		if !assertOnlyColumn(t, cols, ch.Dimensions[0].ID) {
			ok = false
		}
		if !assertColumnExactGrid(t, cols, ch.Dimensions[0].ID, after, before, rowSpan) {
			ok = false
		}
		col := cols[ch.Dimensions[0].ID]

		total := 0.0
		empties := 0
		for _, pt := range col {
			if pt.Value == nil {
				empties++
				continue
			}
			total += *pt.Value
		}

		// A bucket containing at most one sample cannot measure the first
		// sample without a predecessor. Coarser buckets contain their own
		// first-to-last delta and therefore have no empty opening row.
		wantEmpties := 0
		if rowSpan <= ue {
			wantEmpties = 1
		}
		if empties != wantEmpties {
			t.Logf("incremental-sum contract not met: at %d buckets, %d of %d answered "+
				"nothing, want exactly %d for rows spanning %d seconds",
				points, empties, len(col), wantEmpties, rowSpan)
			ok = false
		}

		if math.Abs(total-want) > 1e-6 {
			t.Logf("incremental-sum contract not met: at %d buckets the window rises by "+
				"%.4f, but the fixture rose from %d to %d, which is %.4f - the buckets of "+
				"a window measure one after another and must add up to the whole rise",
				points, total, base0, base0+(samples-1)*step, want)
			ok = false
		}

		if points == samples {
			exact := make([]expectedColumnPoint, samples)
			exact[0] = wantEmptyWithMetadataAt(base+ue, 0, canon.AnnotationEmpty)
			for i := 1; i < samples; i++ {
				exact[i] = wantNumberWithMetadataAt(base+int64(i+1)*ue, step, 0, 0)
			}
			if !assertExactColumn(t, cols, ch.Dimensions[0].ID, exact, 0) {
				t.Logf("one-row-per-sample grid did not preserve the previous sample as the next row's baseline")
				ok = false
			}
		}
	}

	assertContract(t, "CASE-027/incremental-sum-conserves-across-zoom", ok)
}
