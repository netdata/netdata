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
	trackContractComponent(t, contract, "late-tier")
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

// A rotated ALLOC tier and one completed coarse bucket leave an isolated head
// sample. Automatic planning must read it even when the first output row ends there.
func TestCase040IsolatedCoarseHead(t *testing.T) {
	trackContractComponent(t, "CASE-040/inverted-tier-retention", "isolated-head")
	dd := startDedicatedStorageDaemon(t, daemon.Options{
		StorageTiers: 2, StreamMemoryMode: "alloc", TierGrouping: [3]int{0, 1200, 0},
	})
	if err := dd.Stop(); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ file, old, replacement string }{
		{"netdata.conf", "[db]\n", "[db]\n    retention = 5\n"},
		{"stream.conf", "    default memory mode = alloc\n", "    default memory mode = alloc\n    retention = 5\n"},
	} {
		path := filepath.Join(dd.Opts.RunDir, "etc", c.file)
		conf, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Count(string(conf), c.old) != 1 {
			t.Fatalf("configuration marker not unique in %s", c.file)
		}
		if err := os.WriteFile(path, []byte(strings.Replace(string(conf), c.old, c.replacement, 1)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := dd.Restart(); err != nil {
		t.Fatal(err)
	}
	const host, context = "isolated-coarse-head", "fixture.isolated_coarse_head"
	base := int64(fixture.T0 - fixture.T0%1200)
	ch := fixture.Series(context, context, base, 1400, 1, func(int) string { return "7" }, notAnom)
	pushDedicatedChart(t, dd, host, guid(9602), ch)
	waitDedicatedTierLastEntry(t, dd, host, context, 1, base+1200)
	waitDedicatedTierLastEntry(t, dd, host, context, 0, base+1400)
	for _, tc := range []struct {
		name, tier            string
		after, before, points int64
	}{
		{"coarse-control", "1", base + 1000, base + 1200, 1},
		{"fine-control", "0", base + 1200, base + 1400, 1},
		{"automatic", "", base + 1000, base + 1400, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := daemon.DataParams(context, tc.after, tc.before, tc.points)
			p.Set("options", "jsonwrap|unaligned")
			if tc.tier != "" {
				p.Set("tier", tc.tier)
			}
			doc, err := dd.DataV3(host, p)
			if err != nil {
				t.Fatal(err)
			}
			retention := perTierRetention(t, doc)
			if len(retention) < 2 || retention[1].FirstEntry != base+1200 || retention[1].LastEntry != base+1200 ||
				retention[0].FirstEntry <= base+1200 || retention[0].FirstEntry >= base+1400 {
				t.Fatalf("fixture lacks isolated head: %v", retention)
			}
			cols, err := canon.Columns(doc)
			if err != nil {
				t.Fatal(err)
			}
			var want []expectedColumnPoint
			for end := tc.after + 200; end <= tc.before; end += 200 {
				want = append(want, wantNumberWithPAAt(end, 7, 0))
			}
			if tc.tier == "" {
				if !queryTimestampGridExact(t, doc, queryExpectedVirtualGrid(t, tc.after, tc.before, tc.points, false)) {
					t.Fatal("automatic query changed the requested grid")
				}
				// This fixture guards the isolated head. Tail seam arithmetic has separate corpus contracts.
				if len(cols["load"]) != 2 {
					t.Fatal("automatic query did not return both rows")
				}
				cols["load"] = cols["load"][:1]
				want = want[:1]
			}
			if !assertExactColumnValues(t, cols, "load", want, 0) {
				t.Error("constant history was lost")
			}
		})
	}
}
