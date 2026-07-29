// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-025 what the sum apportionment owes, and what it must not touch.
//
// `sum` pays each bucket the seconds it owns, at the rate of whichever
// stored record owns them. A record wider than a bucket therefore has a
// remainder that belongs to the NEXT bucket, and the engine carries that
// remainder forward rather than re-reading the record.
//
// A carry is a debt, and these are the three ways a debt goes wrong:
// it is never collected (the next bucket never asks), it is collected from
// the wrong creditor (the query moved to another tier in between), or the
// mechanism reaches a caller that never borrowed anything (an option that
// does not go through the apportionment at all).
//
// All three shapes are invisible to a total taken over a whole window,
// which is why L10 and L11 hold while these fail: they are about WHERE the
// seconds land, and about who is left holding nothing.
package corpus

import (
	"math"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

// CASE-025a: a record's remainder is not lost because the next bucket
// happens to be a gap, or because the data ran out.
//
// The bucket after a wide record is where that record's last seconds are
// paid. If nothing is collected there, the bucket answers EMPTY - and the
// seconds it owed are then paid nowhere at all. The window total silently
// drops by up to one stored record at every gap edge and at the end of
// retention, which is exactly where a "how much did we transfer" answer is
// least likely to be double-checked.
//
// The fixture collects a run, stops dead, and the query asks for buckets
// that do NOT divide the collection interval - so the last record before
// the silence is guaranteed to straddle a bucket boundary and to have a
// remainder owed to a bucket that will see no data.
func TestCase025CarrySurvivesGaps(t *testing.T) {
	trackContract(t, "CASE-025/carry-survives-gaps")

	const (
		ctx     = "fixture.c025gap"
		host    = "c025gap"
		ue      = 10
		value   = 7
		samples = 60 // 600s of data, then nothing
	)

	base := fixture.T0 - fixture.T0%int64(ue)
	ch := fixture.Series(ctx, ctx, base, samples, ue,
		func(int) string { return "7" },
		func(int) string { return stream.FlagNotAnomalous })

	pushLiveBurst(t, host, guid(250), ch)
	if _, err := td.WaitRetention(host, ch.Context, ch.FirstT(), ch.LastT(), 20*time.Second); err != nil {
		t.Fatal(err)
	}

	first := base
	last := base + int64(samples*ue)

	ok := true

	// Buckets of 25s over 10s records: every bucket boundary cuts a record,
	// so every bucket carries a remainder into the next one. The window
	// runs 100s PAST the end of the data, so the final carry is owed to
	// buckets that hold nothing.
	for _, span := range []int64{25, 35, 55} {
		after := first
		before := last + 100
		points := (before - after) / span

		params := daemon.DataParamsTier(ctx, 0, after, before, points, "sum")
		params.Set("options", "jsonwrap|unaligned")
		doc, err := td.DataV3(host, params)
		if err != nil {
			t.Fatal(err)
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}

		col, has := cols[ch.Dimensions[0].ID]
		if !has {
			t.Logf("carry contract not met: bucket=%ds returned no column", span)
			ok = false
			continue
		}

		got := 0.0
		for _, pt := range col {
			if pt.Value != nil {
				got += *pt.Value
			}
		}

		// every collected second is worth `value` per record of `ue`
		// seconds, so the whole run is samples*value
		want := float64(samples * value)
		if math.Abs(got-want) > 1e-6 {
			t.Logf("carry contract not met: bucket=%ds over a window that ends 100s "+
				"after the data totals %.4f, but the fixture collected %.4f - "+
				"a remainder owed to a bucket with no data of its own is paid nowhere",
				span, got, want)
			ok = false
		}
	}

	assertContract(t, "CASE-025/carry-survives-gaps", ok)
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

	// tier 1 rolls 60 samples into one stored window, so the step lands
	// inside one window and the windows either side of it are pure 0 / 100
	step := base + int64(samples/2)
	after := step - step%60 + 60 // the first fully-anomalous stored window
	before := after + 60

	ok := true

	// every grouping asks the same question of an anomaly rate, and every
	// bucket here sits inside one fully-anomalous stored window
	for _, group := range []string{"average", "min", "max", "sum"} {
		params := daemon.DataParamsTier(ctx, 1, after, before, 3, group)
		params.Set("options", "jsonwrap|unaligned|anomaly-bit")
		doc, err := td.DataV3(host, params)
		if err != nil {
			t.Fatal(err)
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}

		col, has := cols[ch.Dimensions[0].ID]
		if !has || len(col) != 3 {
			t.Logf("anomaly-bit contract not met: %s returned %d rows, want 3", group, len(col))
			ok = false
			continue
		}
		for i, pt := range col {
			if pt.Value == nil {
				t.Logf("anomaly-bit contract not met: %s bucket %d is empty, want 100", group, i)
				ok = false
				continue
			}
			if math.Abs(*pt.Value-100.0) > 1e-6 {
				t.Logf("anomaly-bit contract not met: %s bucket %d reads %.4f, want 100 - "+
					"every sample under this bucket is anomalous, and the window before it "+
					"is one the bucket does not overlap at all",
					group, i, *pt.Value)
				ok = false
			}
		}
	}

	assertContract(t, "CASE-025/anomaly-bit-not-blended", ok)
}
