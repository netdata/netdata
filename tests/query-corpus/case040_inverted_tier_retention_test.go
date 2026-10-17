// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"testing"

	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
)

// A constant survives rollup and any plan seam exactly. Enabling tier 1 later
// must not hide history that remains available in tier 0.
func TestCase040InvertedTierRetention(t *testing.T) {
	const contract = "CASE-040/inverted-tier-retention"
	registerContract(t, contract)
	registerContractComponent(t, "CASE-040/inverted-retention-annotations", "late-tier")
	const host, context = "inverted-tier-retention", "fixture.inverted_tier_retention"
	base := int64(fixture.T0 - fixture.T0%4)
	dd := startDedicatedStorageDaemon(t, daemon.Options{StorageTiers: 1, TierGrouping: [3]int{0, 4, 0}})
	closeFixture := pushDedicatedChart(t, dd, host, guid(9601), c040Series(context, base, 1, 120, c040Flat, c040NotAnomalous, 0))
	waitDedicatedTierLastEntry(t, dd, host, context, 0, base+120)
	c040Restart(t, dd, closeFixture, 2, "")
	pushDedicatedChart(t, dd, host, guid(9601), c040Series(context, base, 121, 360, c040Flat, c040NotAnomalous, 0))
	waitDedicatedTierLastEntry(t, dd, host, context, 1, base+324)
	for _, annotations := range []bool{false, true} {
		name := "values"
		if annotations {
			name = "annotations"
		}
		t.Run(name, func(t *testing.T) {
			if annotations {
				trackContractComponent(t, "CASE-040/inverted-retention-annotations", "late-tier")
			} else {
				trackContract(t, contract)
			}
			for _, tc := range []struct {
				name, tier            string
				after, before, points int64
			}{
				{"tier0-control", "0", base + 4, base + 324, 80},
				{"tier1-control", "1", base + 160, base + 324, 41},
				{"automatic", "", base + 4, base + 324, 80},
			} {
				t.Run(tc.name, func(t *testing.T) {
					doc, cols := c040Query(t, dd, host, context, tc.after, tc.before, tc.points, "average", tc.tier)
					retention := perTierRetention(t, doc)
					if len(retention) != 2 || retention[0].FirstEntry >= retention[1].FirstEntry {
						t.Fatalf("fixture lacks inverted retention: %v", retention)
					}
					var want []expectedColumnPoint
					for end := tc.after + 4; end <= tc.before; end += 4 {
						want = append(want, wantNumberWithPAAt(end, 7, 0))
					}
					if annotations {
						if !assertExactColumnMetadata(t, cols, "load", want) {
							t.Error("retained constant history has incorrect annotations")
						}
					} else if !assertExactColumnValues(t, cols, "load", want, 0) {
						t.Error("constant history was lost")
					}
				})
			}
		})
	}
}

// A rotated ALLOC tier and one completed coarse bucket leave an isolated head
// sample. Automatic planning must read it even when the first output row ends there.
func TestCase040IsolatedCoarseHead(t *testing.T) {
	registerContract(t, "CASE-040/isolated-coarse-head")
	registerContract(t, "CASE-040/isolated-coarse-tail")
	registerContractComponent(t, "CASE-040/inverted-retention-annotations", "isolated-head")
	dd := startDedicatedStorageDaemon(t, daemon.Options{
		StorageTiers: 2, StreamMemoryMode: "alloc", TierGrouping: [3]int{0, 1200, 0},
	})
	if err := dd.Stop(); err != nil {
		t.Fatal(err)
	}
	c040ReplaceConfig(t, dd, "netdata.conf", "[db]\n", "[db]\n    retention = 5\n")
	c040ReplaceConfig(t, dd, "stream.conf", "    default memory mode = alloc\n", "    default memory mode = alloc\n    retention = 5\n")
	if err := dd.Restart(); err != nil {
		t.Fatal(err)
	}
	const host, context = "isolated-coarse-head", "fixture.isolated_coarse_head"
	base := int64(fixture.T0 - fixture.T0%1200)
	ch := fixture.Series(context, context, base, 1400, 1, func(int) string { return "7" }, notAnom)
	pushDedicatedChart(t, dd, host, guid(9602), ch)
	waitDedicatedTierLastEntry(t, dd, host, context, 1, base+1200)
	waitDedicatedTierLastEntry(t, dd, host, context, 0, base+1400)
	for _, scope := range []string{"head", "tail", "annotations"} {
		t.Run(scope, func(t *testing.T) {
			if scope == "annotations" {
				trackContractComponent(t, "CASE-040/inverted-retention-annotations", "isolated-head")
			} else {
				trackContract(t, "CASE-040/isolated-coarse-"+scope)
			}
			end := base + 1200
			if scope == "tail" {
				end = base + 1400
			}
			want := []expectedColumnPoint{wantNumberWithPAAt(end, 7, 0)}
			for _, tier := range []string{"control", "automatic"} {
				t.Run(tier, func(t *testing.T) {
					after, before, points, selected := base+1000, base+1400, int64(2), ""
					if tier == "control" {
						after, before, points, selected = end-200, end, 1, "1"
						if scope == "tail" {
							selected = "0"
						}
					}
					doc, cols := c040Query(t, dd, host, context, after, before, points, "average", selected)
					retention := perTierRetention(t, doc)
					if len(retention) != 2 || retention[1].FirstEntry != base+1200 || retention[1].LastEntry != base+1200 ||
						retention[0].FirstEntry <= base+1200 || retention[0].FirstEntry >= base+1400 {
						t.Fatalf("fixture lacks isolated head: %v", retention)
					}
					rows := c040Rows(cols, end, end)
					if scope == "annotations" {
						if !assertExactColumnMetadata(t, rows, "load", want) {
							t.Error("isolated retained head has incorrect annotations")
						}
					} else if !assertExactColumnValues(t, rows, "load", want, 0) {
						t.Error("isolated retained history was lost")
					}
				})
			}
		})
	}
}
