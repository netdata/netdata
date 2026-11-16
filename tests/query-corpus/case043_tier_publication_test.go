// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

func TestCase043CompletedTierPublication(t *testing.T) {
	for _, component := range []string{"slow-collector", "control"} {
		registerContractComponent(t, "CASE-043/completed-tier-publication", component)
	}
	registerContract(t, "CASE-043/completed-tier-recovery")
	const (
		host           = "tier-publication"
		ue             = int64(60)
		period         = ue * 60 * 60
		spreadCeiling  = int64(65535)
		witnessOrdinal = 3600
		// netdata/netdata @ d7cab55409bc, src/database/rrdset-index-id.c:10-18
		// assigns m=ordinal+1 here. Align both the tier period and m to select
		// a reachable schedule. The oracle uses the spread ceiling.
		base        = 2 * period * (witnessOrdinal + 1)
		completedAt = base + ue
		deadline    = ((completedAt + spreadCeiling + ue - 1) / ue) * ue
		first       = base - 2*period + ue
	)
	dd := startDedicatedStorageDaemon(t, daemon.Options{DisablePulse: true})
	conn, err := stream.Connect(dd.Addr, dd.StreamKey, stream.HostInfo{
		Hostname: host, MachineGUID: guid(10100),
	}, stream.CapsLive)
	if err != nil {
		t.Fatal(err)
	}
	// Disconnect would flush pending tier records and hide the delay.
	t.Cleanup(func() { _ = conn.Close() })
	chart := func(context string) fixture.Chart {
		return fixture.Chart{ID: context, Context: context, Title: "tier publication", Units: "units",
			Family: "fixture", UpdateEvery: int(ue), Dimensions: []fixture.Dimension{{ID: "load"}}}
	}
	control, target := chart("fixture.publication_control"), chart("fixture.publication_slow")
	control.Define(conn)
	// The chart counter is process-wide. Pulse and all collectors are off
	// before this private daemon's first boot; only these charts exist.
	for i := 1; i < witnessOrdinal; i++ {
		seed := chart(fmt.Sprintf("fixture.publication_seed_%d", i))
		seed.Dimensions = nil
		seed.Define(conn)
	}
	target.Define(conn)
	push := func(from, through int64) {
		for ts := from; ts <= through; ts += ue {
			// The control's tier0 endpoint is also a parser-order barrier
			// for the preceding target sample on this same connection.
			for _, ch := range []fixture.Chart{target, control} {
				conn.Begin2(ch.ID, int(ue), ts)
				conn.Set2("load", "7", stream.FlagNotAnomalous)
				conn.End2()
			}
		}
		if err := conn.Flush(); err != nil {
			t.Fatal(err)
		}
		for _, ch := range []fixture.Chart{control, target} {
			until := time.Now().Add(15 * time.Second)
			for {
				p := daemon.DataParamsTier(ch.Context, 0, through-ue, through, 1, "average")
				doc, err := dd.DataV3(host, p)
				if err == nil {
					r := perTierRetention(t, doc)
					if len(r) == 3 && r[0].FirstEntry == first && r[0].LastEntry == through {
						break
					}
				}
				if time.Now().After(until) {
					t.Fatalf("tier0 ingestion barrier did not reach [%d,%d] for %s", first, through, ch.Context)
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
	}
	push(first, deadline)
	for _, tc := range []struct{ name, context string }{
		{"control", control.Context}, {"slow-collector", target.Context},
	} {
		t.Run(tc.name, func(t *testing.T) {
			trackContractComponent(t, "CASE-043/completed-tier-publication", tc.name)
			// Query tier0 only: avoid creating negative tier2 cache entries
			// before the later recovery control observes its published record.
			doc, cols := c040Query(t, dd, host, tc.context, deadline-ue, deadline, 1, "average", "0")
			if !assertExactColumnValues(t, cols, "load", []expectedColumnPoint{wantNumberAt(deadline, 7)}, 0) {
				t.Fatal("source sample was not ingested")
			}
			retention := perTierRetention(t, doc)
			if len(retention) != 3 {
				t.Fatalf("expected three tiers: %v", retention)
			}
			r := retention[2]
			t.Logf("completed=%d observed_at=%d tier2_last=%d", base, deadline, r.LastEntry)
			if r.LastEntry < base {
				t.Errorf("completed tier2 record %d still invisible %ds after completion; spreading ceiling=%ds plus cadence=%ds",
					base, deadline-completedAt, spreadCeiling, ue)
			}
		})
	}
	push(deadline+ue, base+period+ue)
	t.Run("recovery", func(t *testing.T) {
		trackContract(t, "CASE-043/completed-tier-recovery")
		doc, cols := c040Query(t, dd, host, target.Context, base-period, base, 1, "average", "2")
		if !assertTierPresence(t, doc, []bool{false, false, true}) ||
			!assertExactColumnValues(t, cols, "load", []expectedColumnPoint{wantNumberAt(base, 7)}, 0) {
			t.Error("completed constant tier2 record did not become queryable after the next completion")
		}
	})
}
