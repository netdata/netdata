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
	registerContractComponent(t, contract, "tier0-row")
	registerContractComponent(t, contract, "tier0-exact-endpoint")

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

	t.Run("tier0-row", func(t *testing.T) {
		trackContractComponent(t, contract, "tier0-row")
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
				if pt.Value == nil {
					t.Logf("%s row %d at %d is empty, want a numeric row", group, i, pt.T)
					ok = false
				} else if math.IsNaN(*pt.Value) || math.IsInf(*pt.Value, 0) {
					t.Logf("%s row %d at %d is non-finite: %v", group, i, pt.T, *pt.Value)
					ok = false
				}
				if math.Abs(pt.ARP-wantARP[i]) > 1e-6 {
					t.Logf("%s row %d at %d reports anomaly rate %.10g, want %.10g",
						group, i, pt.T, pt.ARP, wantARP[i])
					ok = false
				}
			}
		}

		if !ok {
			t.Errorf("BROKEN %s (tier0-row): %s", contract, manifest[contract].Proves)
		}
	})

	t.Run("tier0-exact-endpoint", func(t *testing.T) {
		trackContractComponent(t, contract, "tier0-exact-endpoint")
		endpointOK := true
		params := daemon.DataParamsTier(context, 0, base, base+100, 20, "average")
		params.Set("options", "jsonwrap|unaligned")
		params.Set("scope_dimensions", ch.Dimensions[0].ID)
		doc, err := td.DataV3(host, params)
		if err != nil {
			t.Fatal(err)
		}
		if !assertSelectedTier(t, doc, 0) {
			endpointOK = false
		}
		if !assertExactView(t, doc, base, base+100, 5) {
			endpointOK = false
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}
		if !assertOnlyColumn(t, cols, ch.Dimensions[0].ID) {
			endpointOK = false
		}
		if !assertColumnExactGrid(t, cols, ch.Dimensions[0].ID, base, base+100, 5) {
			endpointOK = false
		}

		col := cols[ch.Dimensions[0].ID]
		if len(col) != 20 {
			t.Logf("exact-endpoint upsampling returned %d rows, want exactly 20", len(col))
			endpointOK = false
		}
		for i, pt := range col {
			if pt.Value == nil {
				t.Logf("exact-endpoint row %d at %d is empty, want a numeric row", i, pt.T)
				endpointOK = false
			} else if math.IsNaN(*pt.Value) || math.IsInf(*pt.Value, 0) {
				t.Logf("exact-endpoint row %d at %d is non-finite: %v", i, pt.T, *pt.Value)
				endpointOK = false
			}

			offset := int64(i+1) * 5
			wantARP := float64(0)
			if offset%ue == 0 {
				sample := int(offset / ue)
				if sample == 5 || sample == 6 || sample == 8 {
					wantARP = 100
				}
			}
			if math.Abs(pt.ARP-wantARP) > 1e-6 {
				t.Logf("exact-endpoint row %d at %d reports anomaly rate %.10g, want %.10g",
					i, pt.T, pt.ARP, wantARP)
				endpointOK = false
			}
		}

		if !endpointOK {
			t.Errorf("BROKEN %s (tier0-exact-endpoint): %s", contract, manifest[contract].Proves)
		}
	})
}
