// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
)

// A coarse output row can skip a short successor without making it the active
// plan. Smoothing must still reach the terminal empty fill after reader EOF.
func TestCase045SkippedPlanTerminal(t *testing.T) {
	const contract = "CASE-045/terminal-smoothing-empty"
	scopes := []struct{ name, tier string }{
		{"single-plan-control", "1"}, {"skipped-successor", ""},
	}
	for _, group := range []string{"ses", "des"} {
		for _, scope := range scopes {
			registerContractComponent(t, contract, group+"/"+scope.name)
		}
	}
	const host, context = "skipped-plan", "fixture.skipped_plan"
	const coarseLast, fineFirst, fineLast = int64(480), int64(500), int64(510)
	const step, duration = int64(300), int64(86400)
	base := int64(fixture.T0 - fixture.T0%60)
	dd := startDedicatedStorageDaemon(t, daemon.Options{
		StorageTiers: 2, TierGrouping: [3]int{0, 60, 0}, DisablePulse: true,
	})
	closeFixture := pushDedicatedChart(t, dd, host, guid(13000),
		c040Series(context, base, 1, 490, c040Flat, c040NotAnomalous, 0))
	waitDedicatedTierLastEntry(t, dd, host, context, 0, base+490)
	if err := closeFixture(); err != nil {
		t.Fatal(err)
	}
	if err := dd.Stop(); err != nil {
		t.Fatal(err)
	}
	// Expire only this fixture's fine history, preserving its coarse records.
	if err := os.Rename(filepath.Join(dd.Opts.RunDir, "cache", "dbengine"),
		filepath.Join(dd.Opts.RunDir, "expired-tier0")); err != nil {
		t.Fatal(err)
	}
	if err := dd.Restart(); err != nil {
		t.Fatal(err)
	}
	pushDedicatedChart(t, dd, host, guid(13000), c040Series(context, base, fineFirst, fineLast,
		func(int64) float64 { return 17 }, c040NotAnomalous, 0))
	waitDedicatedTierLastEntry(t, dd, host, context, 0, base+fineLast)
	doc, fine := c040Query(t, dd, host, context, base+fineFirst-1, base+fineLast,
		fineLast-fineFirst+1, "average", "0")
	c040Retention(t, doc, []daemon.Retention{
		{FirstEntry: base + fineFirst, LastEntry: base + fineLast},
		{FirstEntry: base + 60, LastEntry: base + coarseLast},
	})
	var fineWant []expectedColumnPoint
	for off := fineFirst; off <= fineLast; off++ {
		fineWant = append(fineWant, wantNumberAt(base+off, 17))
	}
	if !assertExactColumnValues(t, fine, "load", fineWant, 0) {
		t.Fatal("forced fine source does not contain the short successor")
	}
	_, coarse := c040Query(t, dd, host, context, base+420, base+coarseLast, 1, "average", "1")
	if !assertExactColumnValues(t, coarse, "load", []expectedColumnPoint{wantNumberAt(base+coarseLast, 7)}, 0) {
		t.Fatal("forced coarse source does not contain the older record")
	}

	// Class B terminal bound: netdata/netdata @ a0ceb4a83d0d1b407642cf6b1a05b1b17aeb2a49,
	// src/web/api/queries/query-execute.c:201-205,239-255,644-652 and
	// src/database/engine/rrdengineapi.c:817-835,961-963. The reader ends at
	// coarseLast; eleven EOF iterations start at the next output row.
	const eofIterations = int64(11)
	firstEmpty := (coarseLast/step + 1 + eofIterations) * step
	var emptyWant []expectedColumnPoint
	for off := firstEmpty; off <= duration; off += step {
		emptyWant = append(emptyWant, wantEmptyAt(base+off))
	}
	for _, group := range []string{"ses", "des"} {
		t.Run(group, func(t *testing.T) {
			for _, tc := range scopes {
				t.Run(tc.name, func(t *testing.T) {
					trackContractComponent(t, contract, group+"/"+tc.name)
					_, rows := c040Query(t, dd, host, context, base, base+duration, duration/step, group, tc.tier)
					if !assertExactColumnValues(t, c040Rows(rows, base+step, base+step), "load",
						[]expectedColumnPoint{wantNumberAt(base+step, 7)}, 0) {
						t.Fatal("smoothing did not consume the older source values")
					}
					if !assertExactColumnValues(t, c040Rows(rows, base+firstEmpty, base+duration), "load", emptyWant, 0) {
						t.Error("smoothing continued past the terminal EOF allowance")
					}
				})
			}
		})
	}
}
