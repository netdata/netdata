// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-033 anomaly rate describes the raw samples whose timestamps are in
// the result row. The tier-0 fixture places anomalous samples on a row start,
// inside the row, and just after it, so neither neighboring boundary can be
// merged into the exact membership.
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
	const contract = "CASE-033/anomaly-rate-counts-samples-in-the-row"
	trackContractComponent(t, contract, "tier0-row")

	const (
		context = "fixture.c033arp"
		host    = "c033-arp"
		ue      = 10
	)
	base := int64(fixture.T0) - int64(fixture.T0)%ue
	ch := fixture.Series(context, context, base, 12, ue,
		func(int) string { return "100" },
		func(i int) string {
			if i == 5 || i == 6 || i == 8 {
				return stream.FlagAnomalous
			}
			return stream.FlagNotAnomalous
		})
	pushLiveBurst(t, host, guid(293), ch)
	if _, err := td.WaitRetention(host, context, ch.FirstT(), ch.LastT(), 20*time.Second); err != nil {
		t.Fatal(err)
	}

	ok := true
	for _, group := range []string{"average", "max", "sum"} {
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
		if !assertExactView(t, doc, base, base+100, 25) {
			ok = false
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}
		if !assertOnlyColumn(t, cols, ch.Dimensions[0].ID) {
			ok = false
		}
		if !assertColumnExactGrid(t, cols, ch.Dimensions[0].ID, base, base+100, 25) {
			ok = false
		}

		col := cols[ch.Dimensions[0].ID]
		if len(col) != 4 {
			t.Logf("%s returned %d rows, want exactly 4", group, len(col))
			ok = false
			continue
		}
		wantARP := []float64{0, 100.0 / 3.0, 50, 100.0 / 3.0}
		for i, pt := range col {
			if pt.Value == nil || pt.PA&canon.AnnotationEmpty != 0 {
				t.Logf("%s row %d at %d is empty, want a numeric row", group, i, pt.T)
				ok = false
			} else if math.IsNaN(*pt.Value) || math.IsInf(*pt.Value, 0) {
				t.Logf("%s row %d at %d is non-finite: %v", group, i, pt.T, *pt.Value)
				ok = false
			}
			if group != "sum" && pt.Value != nil && *pt.Value != 100 {
				t.Logf("%s row %d at %d reports value %.10g, want the exact constant 100",
					group, i, pt.T, *pt.Value)
				ok = false
			}
			if math.Abs(pt.ARP-wantARP[i]) > 1e-6 {
				t.Logf("%s row %d at %d reports anomaly rate %.10g, want %.10g",
					group, i, pt.T, pt.ARP, wantARP[i])
				ok = false
			}
			if pt.PA != 0 {
				t.Logf("%s row %d at %d has PA %d, want exactly 0", group, i, pt.T, pt.PA)
				ok = false
			}
		}
	}

	if !ok {
		t.Errorf("BROKEN %s (tier0-row): %s", contract, manifest[contract].Proves)
	}
}
