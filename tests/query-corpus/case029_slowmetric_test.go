// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-029 a slowly collected metric totals the same at every zoom, from
// tier 0 through a wide higher-tier record.
//
// The zoom inflation `sum` used to have was not a property of tiers - it
// was a property of a stored record being WIDER than the row asking about
// it. Above tier 0 that is always true at fine resolutions, which is where
// it was found, but it is equally true at tier 0 for anything collected
// less often than once a second: a metric collected every ten seconds
// answers ten one-second rows from one stored record.
//
// So a total over a window must not change with the requested resolution
// on tier 0 either. Pinned here because it is a deliberate change to what
// tier 0 answers for slow metrics - a chart of a 10-second metric zoomed to
// one-second rows used to report ten times what was collected.
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

func TestCase029SlowMetricTotalsAtEveryZoom(t *testing.T) {
	trackContract(t, "CASE-029/tier0-slow-metric-totals-at-every-zoom")

	const (
		ctx   = "fixture.c029slow"
		host  = "c029slow"
		ue    = 10
		value = 7
		// The third source span closes the second 10-hour tier-2 record.
		// Query only the first two finalized records below.
		samples      = 10800
		tier2Samples = 7200

		tier0Samples = 60
	)

	base := int64(fixture.T0) - int64(fixture.T0)%int64(ue*3600)
	ch := fixture.Series(ctx, ctx, base, samples, ue,
		func(int) string { return strconv.Itoa(value) },
		func(int) string { return stream.FlagNotAnomalous })

	pushLiveBurst(t, host, guid(255), ch)
	if _, err := td.WaitRetention(host, ch.Context, ch.FirstT(), ch.LastT(), 20*time.Second); err != nil {
		t.Fatal(err)
	}

	// Keep the tier-0 matrix compact: the last 60 raw records are a complete,
	// aligned dense window inside the longer fixture.
	after := base + int64((samples-tier0Samples)*ue)
	before := base + int64(samples*ue)

	// an absolute gauge: the window holds one reading of `value` per stored
	// record, and nothing about asking for narrower rows creates more of them
	want := float64(tier0Samples * value)

	ok := true
	for _, points := range []int64{
		tier0Samples,          // one row per stored record
		tier0Samples * 2,      // 5s rows: two per record
		tier0Samples * ue,     // 1s rows: ten per record
		tier0Samples * ue / 2, // 2s rows: five per record
	} {
		params := daemon.DataParamsTier(ctx, 0, after, before, points, "sum")
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

		total := 0.0
		for _, pt := range cols[ch.Dimensions[0].ID] {
			if pt.Value != nil {
				total += *pt.Value
			}
		}

		if math.Abs(total-want) > want*1e-6 {
			t.Logf("tier-0 zoom contract not met: at %d rows (%ds each) the window totals "+
				"%.4f, but the fixture stored %d readings of %d, which is %.4f - a row "+
				"narrower than a stored record owns part of it, not all of it",
				points, (before-after)/points, total, tier0Samples, value, want)
			ok = false
		}

		exact := make([]expectedColumnPoint, points)
		rowValue := float64(value) * float64(rowSpan) / ue
		for i := range exact {
			exact[i] = wantNumberWithMetadataAt(
				after+int64(i+1)*rowSpan, rowValue, 0, 0)
		}
		if !assertExactColumn(t, cols, ch.Dimensions[0].ID, exact, 1e-9) {
			t.Logf("tier-0 %ds rows did not receive their exact share of each 10-second record", rowSpan)
			ok = false
		}
	}

	// Two complete tier-2 records, each 36,000 seconds wide, are sliced into
	// 60-second rows. Whole-window conservation and every row are exact on a
	// constant fixture.
	tier2After := base
	tier2Before := base + int64(tier2Samples*ue)
	const tier2RowSpan = int64(60)
	tier2Points := (tier2Before - tier2After) / tier2RowSpan
	params := daemon.DataParamsTier(ctx, 2, tier2After, tier2Before, tier2Points, "sum")
	params.Set("options", "jsonwrap|unaligned")
	params.Set("scope_dimensions", ch.Dimensions[0].ID)
	doc, err := td.DataV3(host, params)
	if err != nil {
		t.Fatal(err)
	}
	if !assertSelectedTier(t, doc, 2) {
		ok = false
	}
	if !assertExactView(t, doc, tier2After, tier2Before, tier2RowSpan) {
		ok = false
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !assertOnlyColumn(t, cols, ch.Dimensions[0].ID) {
		ok = false
	}
	tier2Want := make([]expectedColumnPoint, tier2Points)
	for i := range tier2Want {
		tier2Want[i] = wantNumberWithARPAt(
			tier2After+int64(i+1)*tier2RowSpan,
			float64(value)*float64(tier2RowSpan)/ue, 0)
	}
	if !assertExactColumn(t, cols, ch.Dimensions[0].ID, tier2Want, 1e-9) {
		t.Logf("tier-2 wide records did not preserve exact dense per-row sum ownership")
		ok = false
	}

	assertContract(t, "CASE-029/tier0-slow-metric-totals-at-every-zoom", ok)
}
