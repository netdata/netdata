// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-033 anomaly rate describes the raw samples whose timestamps are in
// the result row. The fixture uses tier 0, where those timestamps are known
// exactly: row (base+50, base+75] contains samples +60 and +70, and only +60
// is anomalous.
package corpus

import (
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
		doc, err := td.DataV3(host, params)
		if err != nil {
			t.Fatal(err)
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}

		found := false
		for _, pt := range cols[ch.Dimensions[0].ID] {
			if pt.T != base+75 {
				continue
			}
			found = true
			if pt.ARP != 50 {
				t.Logf("%s row (t0+50,t0+75] reports anomaly rate %.10g, want 50: "+
					"one of its two raw samples is anomalous", group, pt.ARP)
				ok = false
			}
		}
		if !found {
			t.Logf("%s did not return the row ending at t0+75", group)
			ok = false
		}
	}

	assertContract(t, "CASE-033/anomaly-rate-counts-samples-in-the-row", ok)
}
