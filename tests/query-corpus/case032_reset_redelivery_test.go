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
			if i == 8 || i == samples {
				return stream.FlagNotAnomalous + stream.FlagReset
			}
			return stream.FlagNotAnomalous
		})
	pushLiveBurst(t, host, guid(292), ch)
	if _, err := td.WaitRetention(host, context, ch.FirstT(), ch.LastT(), 20*time.Second); err != nil {
		t.Fatal(err)
	}

	ok := true
	for _, shape := range []struct {
		name    string
		before  int64
		rowSpan int64
	}{
		{name: "upsample", before: base + samples*ue, rowSpan: ue / 2},
		{name: "downsample", before: base + samples*ue, rowSpan: 30},
		{name: "metadata-handoff", before: base + 100, rowSpan: 25},
		{name: "nondividing-tail", before: base + 140, rowSpan: 35},
	} {
		resetRows := make(map[int64]bool, 2)
		for _, resetSample := range []int64{8, samples} {
			resetOffset := resetSample * ue
			resetRows[base+((resetOffset+shape.rowSpan-1)/shape.rowSpan)*shape.rowSpan] = true
		}
		for _, group := range []string{"average", "sum"} {
			points := (shape.before - base) / shape.rowSpan
			params := daemon.DataParamsTier(context, 0, base, shape.before, points, group)
			params.Set("options", "jsonwrap|unaligned")
			params.Set("scope_dimensions", ch.Dimensions[0].ID)
			doc, err := td.DataV3(host, params)
			if err != nil {
				t.Fatal(err)
			}
			if !assertSelectedTier(t, doc, 0) {
				ok = false
			}
			if !assertExactView(t, doc, base, shape.before, shape.rowSpan) {
				ok = false
			}
			cols, err := canon.Columns(doc)
			if err != nil {
				t.Fatal(err)
			}
			if !assertOnlyColumn(t, cols, ch.Dimensions[0].ID) {
				ok = false
			}
			if !assertColumnExactGrid(
				t, cols, ch.Dimensions[0].ID, base, shape.before, shape.rowSpan) {
				ok = false
			}
			col := cols[ch.Dimensions[0].ID]
			for i, pt := range col {
				if pt.Value == nil || pt.PA&canon.AnnotationEmpty != 0 {
					t.Logf("%s/%s row %d at %d is empty, want a numeric row",
						shape.name, group, i, pt.T)
					ok = false
				} else if math.IsNaN(*pt.Value) || math.IsInf(*pt.Value, 0) {
					t.Logf("%s/%s row %d at %d is non-finite: %v",
						shape.name, group, i, pt.T, *pt.Value)
					ok = false
				}
				if pt.ARP != 0 {
					t.Logf("%s/%s row %d at %d has ARP %.10g, want exactly 0",
						shape.name, group, i, pt.T, pt.ARP)
					ok = false
				}
				wantPA := int64(0)
				if resetRows[pt.T] {
					wantPA = canon.AnnotationReset
				}
				if pt.PA != wantPA {
					t.Logf("%s/%s row %d at %d has PA %d, want exactly %d",
						shape.name, group, i, pt.T, pt.PA, wantPA)
					ok = false
				}
			}
		}
	}

	assertContract(t, "CASE-032/reset-annotation-is-not-redelivered", ok)
}
