// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-026 pins the value and metadata of every row when 10-second records
// are sliced into 35-second rows.
//
// At tier 0, anomaly rate is exact: it describes the raw samples whose
// timestamps are inside the result row. Alternating all-healthy and
// all-anomalous rows makes a fetched-record or interpolation-based rate
// visibly wrong. The last stored sample is also a RESET, so the final
// five-second sum share, its anomaly rate, and its annotation must remain
// paired on the settlement row.
package corpus

import (
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

func TestCase026SettlementCarriesAnomaly(t *testing.T) {
	registerContract(t, "CASE-026/anomaly-rate-covers-the-paid-seconds")
	registerContract(t, "CASE-026/settlement-values-belong-to-their-row")

	const (
		ctx     = "fixture.c026anom"
		host    = "c026anom"
		ue      = 10
		value   = 7
		samples = 60
		rowSpan = 35
	)

	base := fixture.T0 - fixture.T0%int64(ue)
	ch := fixture.Series(ctx, ctx, base, samples, ue,
		func(int) string { return "7" },
		func(i int) string {
			// The lower bound is exclusive, so subtract one before assigning
			// a sample timestamp to its 35-second result row.
			anomalous := ((i*ue-1)/rowSpan)%2 == 1
			if i == samples {
				if anomalous {
					return stream.FlagReset
				}
				return stream.FlagNotAnomalous + stream.FlagReset
			}
			if anomalous {
				return stream.FlagAnomalous
			}
			return stream.FlagNotAnomalous
		})

	pushLiveBurst(t, host, guid(252), ch)
	if _, err := td.WaitRetention(host, ch.Context, ch.FirstT(), ch.LastT(), 20*time.Second); err != nil {
		t.Fatal(err)
	}

	last := base + int64(samples*ue)
	valuesOK, evidenceOK := true, true

	for _, group := range []string{"sum", "average"} {
		after := base
		before := last + 100
		points := (before - after) / rowSpan

		params := daemon.DataParamsTier(ctx, 0, after, before, points, group)
		params.Set("options", "jsonwrap|unaligned")
		params.Set("scope_dimensions", ch.Dimensions[0].ID)
		doc, err := td.DataV3(host, params)
		if err != nil {
			t.Fatal(err)
		}
		if !assertSelectedTier(t, doc, 0) {
			valuesOK, evidenceOK = false, false
		}
		if !assertExactView(t, doc, after, before, rowSpan) {
			valuesOK, evidenceOK = false, false
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}
		if !assertOnlyColumn(t, cols, ch.Dimensions[0].ID) {
			valuesOK, evidenceOK = false, false
		}

		want := make([]expectedColumnPoint, points)
		for row := range want {
			rowStart := after + int64(row)*rowSpan
			rowEnd := rowStart + rowSpan
			if rowStart >= last {
				want[row] = wantEmptyWithMetadataAt(rowEnd, 0, canon.AnnotationEmpty)
				continue
			}

			overlapEnd := rowEnd
			if overlapEnd > last {
				overlapEnd = last
			}
			rowValue := float64(value)
			if group == "sum" {
				rowValue *= float64(overlapEnd-rowStart) / ue
			}
			arp := float64(0)
			if (row % 2) == 1 {
				arp = 100
			}
			pa := int64(0)
			if rowStart < last && last <= rowEnd {
				pa = canon.AnnotationReset
			}
			want[row] = wantNumberWithMetadataAt(rowEnd, rowValue, arp, pa)
		}
		if !assertExactColumnValues(t, cols, ch.Dimensions[0].ID, want, 1e-9) {
			t.Logf("%s did not settle each value on its exact owning row", group)
			valuesOK = false
		}
		if !assertExactColumnMetadata(t, cols, ch.Dimensions[0].ID, want) {
			t.Logf("%s did not keep anomaly membership and RESET on their exact owning rows", group)
			evidenceOK = false
		}
	}

	assertContract(t, "CASE-026/anomaly-rate-covers-the-paid-seconds", evidenceOK)
	assertContract(t, "CASE-026/settlement-values-belong-to-their-row", valuesOK)
}
