// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
)

// A constant survives rollup and any plan seam exactly. Enabling tier 1 later
// must not hide history that remains available in tier 0.
func TestCase040InvertedTierRetention(t *testing.T) {
	const contract = "CASE-040/inverted-tier-retention"
	trackContract(t, contract)
	const host, context = "inverted-tier-retention", "fixture.inverted_tier_retention"
	base := int64(fixture.T0 - fixture.T0%4)
	dd := startDedicatedStorageDaemon(t, daemon.Options{StorageTiers: 1, TierGrouping: [3]int{0, 4, 0}})
	makeChart := func(start int64, count int) fixture.Chart {
		return fixture.Series(context, context, start, count, 1, func(int) string { return "7" }, notAnom)
	}
	ch := makeChart(base, 120)
	closeFixture := pushDedicatedChart(t, dd, host, guid(9601), ch)
	if _, err := dd.WaitRetention(host, context, base+1, base+120, 15*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := closeFixture(); err != nil {
		t.Fatal(err)
	}
	if err := dd.Stop(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dd.Opts.RunDir, "etc", "netdata.conf")
	conf, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const oldLine = "    storage tiers = 1\n"
	if strings.Count(string(conf), oldLine) != 1 {
		t.Fatal("storage tiers configuration not found exactly once")
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(conf), oldLine, "    storage tiers = 2\n", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := dd.Restart(); err != nil {
		t.Fatal(err)
	}
	pushDedicatedChart(t, dd, host, guid(9601), makeChart(base+120, 240))
	waitDedicatedTierLastEntry(t, dd, host, context, 1, base+324)
	query := func(t *testing.T, tier string, after, before, points int64) map[string]any {
		p := daemon.DataParams(context, after, before, points)
		p.Set("options", "jsonwrap|unaligned")
		if tier != "" {
			p.Set("tier", tier)
		}
		doc, err := dd.DataV3(host, p)
		if err != nil {
			t.Fatal(err)
		}
		return doc
	}
	doc := query(t, "", base+4, base+324, 80)
	retention := perTierRetention(t, doc)
	if len(retention) < 2 || retention[0].FirstEntry >= retention[1].FirstEntry {
		t.Fatalf("fixture lacks inverted retention: %v", retention)
	}
	t.Logf("retention: %v", retention)
	for _, tc := range []struct {
		name, tier            string
		after, before, points int64
	}{
		{"tier0-control", "0", base + 4, base + 324, 80},
		{"tier1-control", "1", base + 160, base + 324, 41},
		{"automatic", "", base + 4, base + 324, 80},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := query(t, tc.tier, tc.after, tc.before, tc.points)
			cols, err := canon.Columns(d)
			if err != nil {
				t.Fatal(err)
			}
			want := make([]expectedColumnPoint, 0, tc.points)
			for end := tc.after + 4; end <= tc.before; end += 4 {
				want = append(want, wantNumberWithPAAt(end, 7, 0))
			}
			if !assertExactColumnValues(t, cols, "load", want, 0) {
				t.Error("constant history was lost")
			}
		})
	}
}
