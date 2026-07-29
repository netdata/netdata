// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-033 anomaly rate describes the raw samples whose timestamps are in
// the result row. The fixture uses tier 0, where those timestamps are known
// exactly: row (base+50, base+75] contains samples +60 and +70, and only +60
// is anomalous.
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

func TestCase033AnomalyRateCountsSamplesInTheRow(t *testing.T) {
	trackContract(t, "CASE-033/anomaly-rate-counts-samples-in-the-row")

	const (
		context = "fixture.c033arp"
		host    = "c033-arp"
		ue      = 10
	)
	base := int64(fixture.T0) - int64(fixture.T0)%ue
	ch := fixture.Series(context, context, base, 12, ue,
		func(int) string { return "100" },
		func(i int) string {
			if i == 6 {
				return stream.FlagAnomalous
			}
			return stream.FlagNotAnomalous
		})
	pushLiveBurst(t, host, guid(293), ch)
	if _, err := td.WaitRetention(host, context, ch.FirstT(), ch.LastT(), 20*time.Second); err != nil {
		t.Fatal(err)
	}

	ok := true
	for group := range map[string]struct{}{"average": {}, "max": {}, "sum": {}} {
		params := daemon.DataParamsTier(context, 0, base, base+100, 4, group)
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

		col := cols[ch.Dimensions[0].ID]
		if len(col) != 4 {
			t.Logf("%s returned %d rows, want exactly 4", group, len(col))
			ok = false
			continue
		}
		wantARP := []float64{0, 0, 50, 0}
		for i, pt := range col {
			wantT := base + int64(i+1)*25
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
			if pt.ARP != wantARP[i] {
				t.Logf("%s row %d at %d reports anomaly rate %.10g, want %.10g",
					group, i, pt.T, pt.ARP, wantARP[i])
				ok = false
			}
		}
	}

	assertContract(t, "CASE-033/anomaly-rate-counts-samples-in-the-row", ok)
}
