// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-026 a row that reports a value reports an anomaly rate with it.
//
// Every row of a query answers two things: how much happened (the value)
// and how much of it was anomalous (the anomaly rate). Exactly which
// samples the rate is averaged over is an OPEN CONTRACT - the shipped
// documentation says the raw samples inside the row, the engine merges
// every record its read loop touched for that row, and the two disagree
// whenever records do not line up with rows. This case deliberately does
// not decide that. It asserts the one thing true under every candidate: a
// row whose value came from anomalous seconds cannot report zero.
//
// `sum` is the grouping that can break the pair, because it is the only one
// that pays a row seconds belonging to a record the row was never handed:
// a record wider than the row is delivered to the row it ends inside, and
// the seconds before that row's start are owed backwards. A row can
// therefore be paid entirely by a record it never received - and if the
// anomaly rate is taken only from what the row RECEIVED, such a row reports
// a value out of nowhere and calls it perfectly healthy.
//
// The fixture makes every stored second anomalous, so there is nothing to
// weigh and every candidate answers 100. A row reading 0 is a row whose
// value came from seconds its anomaly rate never looked at.
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

func TestCase026SettlementCarriesAnomaly(t *testing.T) {
	const (
		ctx     = "fixture.c026anom"
		host    = "c026anom"
		ue      = 10
		samples = 60 // 600s of data, every second of it anomalous
	)

	base := fixture.T0 - fixture.T0%int64(ue)
	ch := fixture.Series(ctx, ctx, base, samples, ue,
		func(int) string { return "7" },
		func(int) string { return stream.FlagAnomalous })

	pushLiveBurst(t, host, guid(252), ch)
	if _, err := td.WaitRetention(host, ch.Context, ch.FirstT(), ch.LastT(), 20*time.Second); err != nil {
		t.Fatal(err)
	}

	first := base
	last := base + int64(samples*ue)

	ok := true

	// Buckets of 35s over 10s records: no boundary falls on a record edge,
	// so records straddle rows and owe seconds backwards. The window runs
	// 100s past the data, so the final debt is owed to a row that receives
	// no record of its own at all.
	for _, group := range []string{"sum", "average"} {
		after := first
		before := last + 100
		points := (before - after) / 35

		params := daemon.DataParamsTier(ctx, 0, after, before, points, group)
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
			t.Logf("anomaly-pairing contract not met: %s returned no column", group)
			ok = false
			continue
		}

		for _, pt := range col {
			if pt.Value == nil {
				continue
			}
			if math.Abs(pt.ARP-100.0) > 1e-6 {
				t.Logf("anomaly-pairing contract not met: %s row t0%+d reports %.4f "+
					"with anomaly rate %.4f, want 100 - every stored second under "+
					"this window is anomalous, so a row with a value has no seconds "+
					"left that could be healthy",
					group, pt.T-fixture.T0, *pt.Value, pt.ARP)
				ok = false
			}
		}
	}

	assertContract(t, "CASE-026/anomaly-rate-covers-the-paid-seconds", ok)
}
