// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

const (
	c038Context   = "fixture.c038archivedrate"
	c038Host      = "c038-archived-rate"
	c038DenseDim  = "dense"
	c038GappedDim = "gapped"
	c038Rate      = 13
	c038Samples   = 7200
)

// c038StoreArchivedRate writes and flushes the metric before layer 4c's
// quota-driving fixture starts. The first restart is essential: otherwise the
// metric's last partial tier-0 page stays in main cache and cannot rotate.
func c038StoreArchivedRate(t *testing.T, dd *daemon.Daemon) {
	t.Helper()

	base := int64(fixture.T0 - fixture.T0%60)
	ch := fixture.Series(c038Context, c038Context, base, c038Samples, 1,
		func(int) string { return strconv.Itoa(c038Rate) },
		func(int) string { return stream.FlagNotAnomalous })
	ch.Dimensions[0].ID = c038DenseDim
	ch.Dimensions[0].Algorithm = "incremental"
	gapped := ch.Dimensions[0]
	gapped.ID = c038GappedDim
	gapped.Points = append([]fixture.Point(nil), gapped.Points...)
	for i := range gapped.Points {
		if (i+1)%2 == 0 {
			gapped.Points[i].Collected = ""
			gapped.Points[i].Flags = stream.FlagEmpty
		}
	}
	ch.Dimensions = append(ch.Dimensions, gapped)

	conn, err := stream.Connect(dd.Addr, dd.StreamKey, stream.HostInfo{
		Hostname: c038Host, MachineGUID: guid(380),
	}, stream.CapsLive)
	if err != nil {
		t.Fatal(err)
	}
	ch.Define(conn)
	ch.PushLive(conn)
	if err := conn.Flush(); err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}
	if _, err := dd.WaitRetention(c038Host, c038Context, ch.FirstT(), ch.LastT(), 30*time.Second); err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	if err := dd.Restart(); err != nil {
		t.Fatalf("flush archived-rate pages by restart: %v", err)
	}
}

// CASE-038: rate volume remains exact when tier 0 no longer has the metric.
//
// The layer-4c fixture collects archived_rate only near the beginning, then
// keeps writing unrelated incompressible dimensions until tier 0 rotates that
// metric away. Restarting removes live/cache handles, leaving the supported
// state where tier 1 retains the metric and tier 0 does not. The expected
// dense rows cover every second. Gapped rows cover every other second and
// must also carry PARTIAL. This proves legacy tier-1 gap reconstruction does
// not depend on retained tier-0 samples or pre-restart live/cache state.
func c038HigherTierOnlyRateVolume(t *testing.T, dd *daemon.Daemon) (bool, bool) {
	t.Helper()

	const (
		rowSpan = int64(60)
		rows    = int64(60)
	)
	after := int64(fixture.T0 - fixture.T0%60) // absolute tier-1 boundary
	before := after + rows*rowSpan
	query := func(label, dimension string, selected, gapped bool) (bool, bool) {
		t.Helper()

		var params url.Values
		if selected {
			params = daemon.DataParamsTier(c038Context, 1, after, before, rows, "sum")
		} else {
			params = daemon.DataParams(c038Context, after, before, rows)
			params.Set("time_group", "sum")
		}
		params.Set("scope_dimensions", dimension)
		params.Set("options", "jsonwrap|unaligned")

		doc, err := dd.DataV3(c038Host, params)
		if err != nil {
			t.Fatalf("%s query: %v", label, err)
		}
		tiers := perTierRetention(t, doc)
		if len(tiers) < 2 {
			t.Fatalf("%s query reports fewer than two tiers: %+v", label, tiers)
		}
		if tiers[0].FirstEntry != 0 || tiers[0].LastEntry != 0 {
			t.Fatalf("%s precondition failed: tier 0 still retains archived_rate: %+v", label, tiers[0])
		}
		if tiers[1].FirstEntry == 0 || tiers[1].LastEntry < before {
			t.Fatalf("%s precondition failed: tier 1 does not cover (%d,%d]: %+v",
				label, after, before, tiers[1])
		}

		valuesOK, evidenceOK := true, true
		if !assertSelectedTier(t, doc, 1) {
			valuesOK, evidenceOK = false, false
		}
		if !assertExactView(t, doc, after, before, rowSpan) {
			valuesOK, evidenceOK = false, false
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatalf("%s columns: %v", label, err)
		}
		if !assertOnlyColumn(t, cols, dimension) {
			valuesOK, evidenceOK = false, false
		}
		wantPA := int64(0)
		measuredSeconds := rowSpan
		if gapped {
			wantPA = canon.AnnotationPartial
			measuredSeconds /= 2
		}
		want := make([]expectedColumnPoint, rows)
		for i := range want {
			want[i] = wantNumberWithPAAt(
				after+int64(i+1)*rowSpan,
				float64(c038Rate)*float64(measuredSeconds),
				wantPA)
		}
		if !assertExactColumnValues(t, cols, dimension, want, 0) {
			t.Logf("%s did not preserve fixture rate x selected seconds", label)
			valuesOK = false
		}
		if !assertExactColumnMetadata(t, cols, dimension, want) {
			evidenceOK = false
		}
		return valuesOK, evidenceOK
	}

	valuesOK, evidenceOK := true, true
	for _, shape := range []struct {
		dimension string
		gapped    bool
	}{
		{dimension: c038DenseDim},
		{dimension: c038GappedDim, gapped: true},
	} {
		queryValuesOK, queryEvidenceOK := query(
			"forced-tier-1/"+shape.dimension, shape.dimension, true, shape.gapped)
		if !queryValuesOK {
			valuesOK = false
		}
		if !queryEvidenceOK {
			evidenceOK = false
		}
		queryValuesOK, queryEvidenceOK = query(
			"automatic-tier/"+shape.dimension, shape.dimension, false, shape.gapped)
		if !queryValuesOK {
			valuesOK = false
		}
		if !queryEvidenceOK {
			evidenceOK = false
		}
	}
	if !valuesOK {
		t.Logf("expected dense rows to integrate %ds and gapped rows to integrate %ds",
			rowSpan, rowSpan/2)
	}
	return valuesOK, evidenceOK
}
