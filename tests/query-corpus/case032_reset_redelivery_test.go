// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-032 a reset is an event at the stored sample's timestamp. When a
// result grid is finer than the collection interval, the same stored point
// is re-delivered to several rows; that must not turn one reset into several
// resets or annotate a row that ends before the event happened.
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

func TestCase032ResetAnnotationIsNotRedelivered(t *testing.T) {
	trackContract(t, "CASE-032/reset-annotation-is-not-redelivered")

	const (
		context = "fixture.c032reset"
		host    = "c032-reset"
		ue      = 10
		samples = 12
	)
	base := int64(fixture.T0) - int64(fixture.T0)%ue
	ch := fixture.Series(context, context, base, samples, ue,
		func(int) string { return "100" },
		func(i int) string {
			if i == 6 {
				return stream.FlagReset
			}
			return stream.FlagNotAnomalous
		})
	pushLiveBurst(t, host, guid(292), ch)
	if _, err := td.WaitRetention(host, context, ch.FirstT(), ch.LastT(), 20*time.Second); err != nil {
		t.Fatal(err)
	}

	ok := true
	for group := range map[string]struct{}{"average": {}, "sum": {}} {
		params := daemon.DataParamsTier(context, 0, base, base+samples*ue, samples*2, group)
		params.Set("options", "jsonwrap|unaligned")
		params.Set("scope_dimensions", ch.Dimensions[0].ID)
		doc, err := td.DataV3(host, params)
		if err != nil {
			t.Fatal(err)
		}
		if !assertSelectedTier(t, doc, 0) {
			ok = false
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}
		if !assertOnlyColumn(t, cols, ch.Dimensions[0].ID) {
			ok = false
		}
		col, has := cols[ch.Dimensions[0].ID]
		if !has || len(col) != samples*2 {
			t.Logf("%s returned %d rows for the reset dimension, want exactly %d",
				group, len(col), samples*2)
			ok = false
			continue
		}

		var resetRows []int64
		for i, pt := range col {
			wantT := base + int64(i+1)*(ue/2)
			if pt.T != wantT {
				t.Logf("%s row %d ends at %d, want %d", group, i, pt.T, wantT)
				ok = false
			}
			if pt.Value == nil || pt.PA&canon.AnnotationEmpty != 0 {
				t.Logf("%s row %d at %d is empty, want a numeric row", group, i, pt.T)
				ok = false
			} else if math.IsNaN(*pt.Value) || math.IsInf(*pt.Value, 0) {
				t.Logf("%s row %d at %d is non-finite: %v", group, i, pt.T, *pt.Value)
				ok = false
			}
			if pt.PA&canon.AnnotationReset != 0 {
				resetRows = append(resetRows, pt.T)
			}
		}
		wantT := base + 6*ue
		if len(resetRows) != 1 || resetRows[0] != wantT {
			t.Logf("%s annotated RESET at offsets %v, want exactly [%d]: one event belongs "+
				"to the row containing its sample timestamp",
				group, offsetsFrom(resetRows, base), wantT-base)
			ok = false
		}
	}

	assertContract(t, "CASE-032/reset-annotation-is-not-redelivered", ok)
}

func offsetsFrom(timestamps []int64, base int64) []int64 {
	offsets := make([]int64, len(timestamps))
	for i, ts := range timestamps {
		offsets[i] = ts - base
	}
	return offsets
}
