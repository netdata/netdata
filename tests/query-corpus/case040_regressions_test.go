// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

func c040ReplaceConfig(t *testing.T, dd *daemon.Daemon, file, old, replacement string) {
	t.Helper()
	path := filepath.Join(dd.Opts.RunDir, "etc", file)
	conf, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(conf), old) != 1 {
		t.Fatalf("configuration marker %q not unique in %s", old, file)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(conf), old, replacement, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func c040Restart(t *testing.T, dd *daemon.Daemon, closeFixture func() error, tiers int, backfill string) {
	t.Helper()
	if err := closeFixture(); err != nil {
		t.Fatal(err)
	}
	if err := dd.Stop(); err != nil {
		t.Fatal(err)
	}
	if tiers > 0 {
		c040ReplaceConfig(t, dd, "netdata.conf", fmt.Sprintf("    storage tiers = %d\n", dd.Opts.StorageTiers),
			fmt.Sprintf("    storage tiers = %d\n", tiers))
		dd.Opts.StorageTiers = tiers
	}
	if backfill != "" {
		c040ReplaceConfig(t, dd, "netdata.conf", "[db]\n", "[db]\n    dbengine tier backfill = "+backfill+"\n")
	}
	if err := dd.Restart(); err != nil {
		t.Fatal(err)
	}
}

func c040Series(context string, base, first, last int64, value func(int64) float64, anomalous func(int64) bool, omit int64) fixture.Chart {
	ch := fixture.Chart{ID: context, Context: context, Title: "tier handoff", Units: "units", Family: "fixture", UpdateEvery: 1}
	dim := fixture.Dimension{ID: "load"}
	for off := first; off <= last; off++ {
		if off == omit {
			continue
		}
		flags := stream.FlagNotAnomalous
		if anomalous(off) {
			flags = stream.FlagAnomalous
		}
		dim.Points = append(dim.Points, fixture.Point{T: base + off, Collected: strconv.FormatFloat(value(off), 'f', -1, 64), Flags: flags})
	}
	ch.Dimensions = []fixture.Dimension{dim}
	return ch
}

func c040Query(t *testing.T, dd *daemon.Daemon, host, context string, after, before, points int64, group, tier string) (map[string]any, map[string][]canon.Pt) {
	t.Helper()
	p := daemon.DataParams(context, after, before, points)
	p.Set("options", "jsonwrap|unaligned")
	p.Set("time_group", group)
	if tier != "" {
		p.Set("tier", tier)
	}
	doc, err := dd.DataV3(host, p)
	if err != nil {
		t.Fatal(err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !assertOnlyColumn(t, cols, "load") || !assertColumnExactGrid(t, cols, "load", after, before, (before-after)/points) {
		t.Fatal("query changed the complete requested column/grid")
	}
	return doc, cols
}

// Keep the full response/grid check separate from a named invariant's interval.
// Other retained intervals can have independent defects on the same binary.
func c040Rows(cols map[string][]canon.Pt, first, last int64) map[string][]canon.Pt {
	selected := make(map[string][]canon.Pt, len(cols))
	for dim, pts := range cols {
		for _, pt := range pts {
			if pt.T >= first && pt.T <= last {
				selected[dim] = append(selected[dim], pt)
			}
		}
	}
	return selected
}

func c040Retention(t *testing.T, doc map[string]any, want []daemon.Retention) {
	t.Helper()
	got := perTierRetention(t, doc)
	if len(got) != len(want) {
		t.Fatalf("retention tiers: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i].FirstEntry != want[i].FirstEntry || got[i].LastEntry != want[i].LastEntry {
			t.Fatalf("retention prerequisite tier%d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func c040Flat(int64) float64      { return 7 }
func c040NotAnomalous(int64) bool { return false }

// Omitting collection at the fine-to-coarse seam must not add a share of a
// coarse record that already represents the other samples in the output row.
func TestCase040MissedSeamSum(t *testing.T) {
	const contract = "CASE-040/missed-seam-sum"
	for _, gran := range []int64{4, 60} {
		t.Run(fmt.Sprintf("grouping-%d", gran), func(t *testing.T) {
			trackContractComponent(t, contract, fmt.Sprintf("grouping-%d", gran))
			base := int64(fixture.T0 - fixture.T0%gran)
			initial, last, after, before := int64(120), int64(360), int64(4), int64(324)
			if gran == 60 {
				initial, last, after, before = 600, 1800, 0, 1800
			}
			seam := initial + gran
			fixtureGUID := 9900
			if gran == 60 {
				fixtureGUID = 9901
			}
			value := func(off int64) float64 { return float64(off*7%23 + 1) }
			const host, context = "missed-seam", "fixture.missed_seam"
			dd := startDedicatedStorageDaemon(t, daemon.Options{StorageTiers: 1, TierGrouping: [3]int{0, int(gran), 0}})
			closeFixture := pushDedicatedChart(t, dd, host, guid(fixtureGUID), c040Series(context, base, 1, initial, value, c040NotAnomalous, 0))
			waitDedicatedTierLastEntry(t, dd, host, context, 0, base+initial)
			c040Restart(t, dd, closeFixture, 2, "")
			pushDedicatedChart(t, dd, host, guid(fixtureGUID), c040Series(context, base, initial+1, last, value, c040NotAnomalous, seam))
			waitDedicatedTierLastEntry(t, dd, host, context, 1, base+last-gran)
			want := 0.0
			for off := initial + 1; off < seam; off++ {
				want += value(off)
			}
			for _, tier := range []string{"0", ""} {
				t.Run("tier-"+tier, func(t *testing.T) {
					_, cols := c040Query(t, dd, host, context, base+after, base+before, (before-after)/gran, "sum", tier)
					if !assertExactColumnValues(t, c040Rows(cols, base+seam, base+seam), "load", []expectedColumnPoint{wantNumberAt(base+seam, want)}, 0) {
						t.Error("missing seam collection contributed to SUM")
					}
				})
			}
		})
	}
}

// A restart-created hole in the first coarse page is an outage, even when
// skipping the covered first record leaves a post-outage read-ahead point.
func TestCase040LateTierGap(t *testing.T) {
	const contract = "CASE-040/late-tier-gap-null"
	const boundaryContract = "CASE-040/late-tier-boundary-average"
	for name, tc := range map[string]struct {
		initialTiers, finalTiers                         int
		grouping                                         [3]int
		initial, middle, resume, last, after, firstEmpty int64
		guid                                             int
	}{
		"tier1": {1, 2, [3]int{0, 60, 0}, 600, 690, 1201, 2400, 0, 780, 9910},
		"tier2": {2, 3, [3]int{0, 10, 6}, 1200, 1290, 3001, 4800, 600, 1380, 9911},
	} {
		t.Run(name, func(t *testing.T) {
			registerContractComponent(t, contract, name)
			registerContractComponent(t, boundaryContract, name)
			base := int64(fixture.T0 - fixture.T0%60)
			const host, context = "late-tier-gap", "fixture.late_tier_gap"
			dd := startDedicatedStorageDaemon(t, daemon.Options{StorageTiers: tc.initialTiers, TierGrouping: tc.grouping})
			closeFixture := pushDedicatedChart(t, dd, host, guid(tc.guid), c040Series(context, base, 1, tc.initial, c040Flat, c040NotAnomalous, 0))
			waitDedicatedTierLastEntry(t, dd, host, context, 0, base+tc.initial)
			c040Restart(t, dd, closeFixture, tc.finalTiers, "none")
			closeFixture = pushDedicatedChart(t, dd, host, guid(tc.guid), c040Series(context, base, tc.initial+1, tc.middle, c040Flat, c040NotAnomalous, 0))
			waitDedicatedTierLastEntry(t, dd, host, context, 0, base+tc.middle)
			c040Restart(t, dd, closeFixture, 0, "")
			pushDedicatedChart(t, dd, host, guid(tc.guid), c040Series(context, base, tc.resume, tc.last, c040Flat, c040NotAnomalous, 0))
			waitDedicatedTierLastEntry(t, dd, host, context, tc.finalTiers-1, base+tc.last-60)
			var want []expectedColumnPoint
			for end := tc.firstEmpty; end < tc.resume; end += 60 {
				want = append(want, wantEmptyAt(base+end))
			}
			t.Run("gap", func(t *testing.T) {
				trackContractComponent(t, contract, name)
				for _, tier := range []string{"0", ""} {
					t.Run("tier-"+tier, func(t *testing.T) {
						_, cols := c040Query(t, dd, host, context, base+tc.after, base+tc.last, (tc.last-tc.after)/60, "average", tier)
						if !assertExactColumnValues(t, c040Rows(cols, want[0].T, want[len(want)-1].T), "load", want, 0) {
							t.Error("query fabricated values in a wholly empty coarse-page hole")
						}
					})
				}
			})
			t.Run("retained-boundary", func(t *testing.T) {
				trackContractComponent(t, boundaryContract, name)
				end := base + tc.firstEmpty - 60
				for _, tier := range []string{"0", ""} {
					t.Run("tier-"+tier, func(t *testing.T) {
						_, cols := c040Query(t, dd, host, context, base+tc.after, base+tc.last, (tc.last-tc.after)/60, "average", tier)
						if !assertExactColumnValues(t, c040Rows(cols, end, end), "load", []expectedColumnPoint{wantNumberAt(end, c040Flat(0))}, 0) {
							t.Error("coarse-page boundary discarded retained fine samples")
						}
					})
				}
			})
		})
	}
}

func c040AllocDaemon(t *testing.T, retention int) *daemon.Daemon {
	t.Helper()
	dd := startDedicatedStorageDaemon(t, daemon.Options{StorageTiers: 2, StreamMemoryMode: "alloc", TierGrouping: [3]int{0, 300, 0}})
	if err := dd.Stop(); err != nil {
		t.Fatal(err)
	}
	c040ReplaceConfig(t, dd, "netdata.conf", "[db]\n", fmt.Sprintf("[db]\n    retention = %d\n", retention))
	c040ReplaceConfig(t, dd, "stream.conf", "    default memory mode = alloc\n", fmt.Sprintf("    default memory mode = alloc\n    retention = %d\n", retention))
	if err := dd.Restart(); err != nil {
		t.Fatal(err)
	}
	return dd
}

func TestCase040RetainedIsland(t *testing.T) {
	for _, group := range []string{"average", "sum"} {
		registerContract(t, "CASE-040/retained-island-"+group)
	}
	base := int64(fixture.T0 - fixture.T0%300)
	const host, context = "retained-island", "fixture.retained_island"
	dd := c040AllocDaemon(t, 200)
	pushDedicatedChart(t, dd, host, guid(9912), c040Series(context, base, 1, 1200, c040Flat, c040NotAnomalous, 0))
	waitDedicatedTierLastEntry(t, dd, host, context, 0, base+1200)
	waitDedicatedTierLastEntry(t, dd, host, context, 1, base+900)
	for _, group := range []string{"average", "sum"} {
		t.Run(group, func(t *testing.T) {
			trackContract(t, "CASE-040/retained-island-"+group)
			for _, tier := range []string{"1", ""} {
				t.Run("tier-"+tier, func(t *testing.T) {
					doc, cols := c040Query(t, dd, host, context, base, base+1200, 60, group, tier)
					c040Retention(t, doc, []daemon.Retention{{FirstEntry: base + 1000, LastEntry: base + 1200}, {FirstEntry: base + 300, LastEntry: base + 900}})
					var want []expectedColumnPoint
					value := 7.0
					if group == "sum" {
						value *= 20
					}
					for end := int64(620); end <= 900; end += 20 {
						want = append(want, wantNumberAt(base+end, value))
					}
					if !assertExactColumnValues(t, c040Rows(cols, base+620, base+900), "load", want, 0) {
						t.Error("retained coarse-island record was truncated")
					}
				})
			}
		})
	}
}

func TestCase040DisjointFineSelectedGap(t *testing.T) {
	trackContract(t, "CASE-040/fine-selected-gap-null")
	base := int64(fixture.T0 - fixture.T0%300)
	const host, context = "disjoint-gap", "fixture.disjoint_gap"
	dd := c040AllocDaemon(t, 5)
	ch := c040Series(context, base, 1, 900, c040Flat, c040NotAnomalous, 0)
	ch.Dimensions[0].Points = append(ch.Dimensions[0].Points, c040Series(context, base, 1201, 1500, c040Flat, c040NotAnomalous, 0).Dimensions[0].Points...)
	pushDedicatedChart(t, dd, host, guid(9913), ch)
	waitDedicatedTierLastEntry(t, dd, host, context, 0, base+1500)
	waitDedicatedTierLastEntry(t, dd, host, context, 1, base+900)
	doc, cols := c040Query(t, dd, host, context, base, base+1500, 1500, "average", "")
	c040Retention(t, doc, []daemon.Retention{{FirstEntry: base + 1495, LastEntry: base + 1500}, {FirstEntry: base + 300, LastEntry: base + 900}})
	var want []expectedColumnPoint
	for end := int64(901); end <= 1495; end++ {
		want = append(want, wantEmptyAt(base+end))
	}
	if !assertExactColumnValues(t, c040Rows(cols, base+901, base+1495), "load", want, 0) {
		t.Error("disconnected retained islands fabricated fine-selected gap data")
	}
}

// Preserve the pre-existing coarse-selected form under its own key. Moving
// owned tier0 files simulates expired retention without deleting the evidence.
func TestCase040DisjointCoarseSelectedGap(t *testing.T) {
	for _, key := range []string{"coarse-selected-gap-null", "coarse-gap-boundary-average", "coarse-gap-boundary-sum", "coarse-gap-resume-average", "coarse-gap-resume-sum"} {
		registerContract(t, "CASE-040/"+key)
	}
	base := int64(fixture.T0 - fixture.T0%60)
	const host, context = "coarse-selected-gap", "fixture.coarse_selected_gap"
	dd := startDedicatedStorageDaemon(t, daemon.Options{StorageTiers: 2, TierGrouping: [3]int{0, 60, 0}})
	closeFixture := pushDedicatedChart(t, dd, host, guid(9914), c040Series(context, base, 1, 1260, c040Flat, c040NotAnomalous, 0))
	waitDedicatedTierLastEntry(t, dd, host, context, 1, base+1200)
	if err := closeFixture(); err != nil {
		t.Fatal(err)
	}
	if err := dd.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(dd.Opts.RunDir, "cache", "dbengine"), filepath.Join(dd.Opts.RunDir, "expired-tier0")); err != nil {
		t.Fatal(err)
	}
	if err := dd.Restart(); err != nil {
		t.Fatal(err)
	}
	resumedValue := func(off int64) float64 { return float64(off - 1500) }
	pushDedicatedChart(t, dd, host, guid(9914), c040Series(context, base, 1501, 1540, resumedValue, c040NotAnomalous, 0))
	waitDedicatedTierLastEntry(t, dd, host, context, 0, base+1540)
	doc, cols := c040Query(t, dd, host, context, base, base+1540, 20, "average", "")
	c040Retention(t, doc, []daemon.Retention{{FirstEntry: base + 1501, LastEntry: base + 1540}, {FirstEntry: base + 60, LastEntry: base + 1200}})
	var want []expectedColumnPoint
	for end := int64(1309); end <= 1463; end += 77 {
		want = append(want, wantEmptyAt(base+end))
	}
	t.Run("gap", func(t *testing.T) {
		trackContract(t, "CASE-040/coarse-selected-gap-null")
		if !assertExactColumnValues(t, c040Rows(cols, base+1309, base+1463), "load", want, 0) {
			t.Error("query fabricated coarse-selected gap data")
		}
	})
	for _, group := range []string{"average", "sum"} {
		t.Run("boundary-"+group, func(t *testing.T) {
			trackContract(t, "CASE-040/coarse-gap-boundary-"+group)
			_, rows := c040Query(t, dd, host, context, base, base+1540, 20, group, "")
			value := 7.0
			if group == "sum" {
				value *= 1200 - 1155
			}
			if !assertExactColumnValues(t, c040Rows(rows, base+1232, base+1232), "load", []expectedColumnPoint{wantNumberAt(base+1232, value)}, 0) {
				t.Error("last retained coarse record lost its partially overlapping row")
			}
		})
		t.Run("resume-"+group, func(t *testing.T) {
			trackContract(t, "CASE-040/coarse-gap-resume-"+group)
			_, rows := c040Query(t, dd, host, context, base, base+1540, 20, group, "")
			value := 40.0 * 41 / 2
			if group == "average" {
				value /= 40
			}
			if !assertExactColumnValues(t, c040Rows(rows, base+1540, base+1540), "load", []expectedColumnPoint{wantNumberAt(base+1540, value)}, 0) {
				t.Error("first post-gap sample did not reach its own output row")
			}
		})
	}
}

// The current coarse cadence may differ from the actual first stored interval.
func TestCase040HistoricalHeadCadence(t *testing.T) {
	trackContract(t, "CASE-040/historical-head-cadence")
	base := int64(fixture.T0 - fixture.T0%16)
	const host, context = "historical-head-cadence", "fixture.historical_head_cadence"
	value := func(off int64) float64 {
		if off >= 117 && off <= 120 {
			return 100
		}
		return 7
	}
	dd := startDedicatedStorageDaemon(t, daemon.Options{StorageTiers: 1, TierGrouping: [3]int{0, 4, 0}})
	closeFixture := pushDedicatedChart(t, dd, host, guid(9919), c040Series(context, base, 1, 120, value, c040NotAnomalous, 0))
	waitDedicatedTierLastEntry(t, dd, host, context, 0, base+120)
	c040Restart(t, dd, closeFixture, 2, "")
	conn, err := stream.Connect(dd.Addr, dd.StreamKey, stream.HostInfo{Hostname: host, MachineGUID: guid(9919)}, stream.CapsLive)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	ch := c040Series(context, base, 121, 200, value, c040NotAnomalous, 0)
	ch.Define(conn)
	ch.PushLive(conn)
	if err := conn.Flush(); err != nil {
		t.Fatal(err)
	}
	waitDedicatedTierLastEntry(t, dd, host, context, 0, base+200)
	ch.UpdateEvery = 2
	ch.Dimensions[0].Points = nil
	for off := int64(202); off <= 400; off += 2 {
		ch.Dimensions[0].Points = append(ch.Dimensions[0].Points, fixture.Point{T: base + off, Collected: "7", Flags: stream.FlagNotAnomalous})
	}
	ch.Define(conn)
	ch.PushLive(conn)
	if err := conn.Flush(); err != nil {
		t.Fatal(err)
	}
	waitDedicatedTierLastEntry(t, dd, host, context, 1, base+392)
	for _, tier := range []string{"0", ""} {
		t.Run("tier-"+tier, func(t *testing.T) {
			doc, cols := c040Query(t, dd, host, context, base+116, base+396, 35, "average", tier)
			if tier == "" {
				points, ok := strictTierPoints(t, doc)
				if !ok || points[0] <= 0 || points[1] <= 0 {
					t.Fatal("historical head control did not cross tiers")
				}
			}
			if !assertExactColumnValues(t, c040Rows(cols, base+124, base+124), "load", []expectedColumnPoint{wantNumberAt(base+124, (4*100+4*7)/8.0)}, 0) {
				t.Error("current coarse cadence replaced the shorter historical head interval")
			}
		})
	}
}

// A stored empty coarse read-ahead interval must delay the next real record
// until its output row, even after covered seam records have been skipped.
func TestCase040PostGapCoarseMin(t *testing.T) {
	trackContract(t, "CASE-040/post-gap-coarse-min")
	base := int64(fixture.T0 - fixture.T0%10)
	const host, context = "post-gap-coarse", "fixture.post_gap_coarse"
	value := func(off int64) float64 { return float64(((off-1)/10)%13 + 1) }
	dd := startDedicatedStorageDaemon(t, daemon.Options{StorageTiers: 1, TierGrouping: [3]int{0, 10, 0}})
	closeFixture := pushDedicatedChart(t, dd, host, guid(9920), c040Series(context, base, 1, 280, value, c040NotAnomalous, 0))
	waitDedicatedTierLastEntry(t, dd, host, context, 0, base+280)
	c040Restart(t, dd, closeFixture, 2, "")
	ch := c040Series(context, base, 301, 901, value, c040NotAnomalous, 0)
	for i := range ch.Dimensions[0].Points {
		pt := &ch.Dimensions[0].Points[i]
		if pt.T <= base+325 {
			pt.Collected, pt.Flags = "0", stream.FlagEmpty
		}
	}
	pushDedicatedChart(t, dd, host, guid(9920), ch)
	waitDedicatedTierLastEntry(t, dd, host, context, 1, base+890)
	want := math.Inf(1)
	for off := int64(326); off <= 340; off++ {
		want = math.Min(want, value(off))
	}
	for _, tier := range []string{"0", ""} {
		t.Run("tier-"+tier, func(t *testing.T) {
			doc, cols := c040Query(t, dd, host, context, base+200, base+400, 10, "min", tier)
			if tier == "" && !assertTierPresence(t, doc, []bool{true, true}) {
				t.Error("gap fixture did not read both fine and coarse tiers")
			}
			if !assertExactColumnValues(t, c040Rows(cols, base+340, base+340), "load", []expectedColumnPoint{wantNumberAt(base+340, want)}, 0) {
				t.Error("empty coarse read-ahead lost the first real post-gap minimum")
			}
		})
	}
}

func TestCase040PartialFirstRecord(t *testing.T) {
	for _, group := range []string{"sum", "average", "min", "max", "metadata"} {
		registerContract(t, "CASE-040/partial-first-record-"+group)
	}
	base := int64(fixture.T0 - fixture.T0%60)
	const host, context = "partial-first-record", "fixture.partial_first_record"
	value := func(off int64) float64 {
		// The first coarse record omits this prefix; MAX must detect that loss.
		if off == 620 {
			return 2000
		}
		return float64(off)
	}
	anomalous := func(off int64) bool { return off >= 601 && off <= 615 }
	dd := startDedicatedStorageDaemon(t, daemon.Options{StorageTiers: 1, TierGrouping: [3]int{0, 60, 0}})
	closeFixture := pushDedicatedChart(t, dd, host, guid(9915), c040Series(context, base, 1, 630, value, anomalous, 0))
	waitDedicatedTierLastEntry(t, dd, host, context, 0, base+630)
	c040Restart(t, dd, closeFixture, 2, "")
	pushDedicatedChart(t, dd, host, guid(9915), c040Series(context, base, 631, 2600, value, anomalous, 0))
	waitDedicatedTierLastEntry(t, dd, host, context, 1, base+2460)
	for _, group := range []string{"sum", "average", "min", "max", "metadata"} {
		t.Run(group, func(t *testing.T) {
			trackContract(t, "CASE-040/partial-first-record-"+group)
			sum, low, high, anom := 0.0, math.Inf(1), math.Inf(-1), 0
			for off := int64(601); off <= 660; off++ {
				sum += value(off)
				low = math.Min(low, value(off))
				high = math.Max(high, value(off))
				if anomalous(off) {
					anom++
				}
			}
			wantValue := sum
			switch group {
			case "average", "metadata":
				wantValue /= 60
			case "min":
				wantValue = low
			case "max":
				wantValue = high
			}
			want := []expectedColumnPoint{wantNumberWithARPAt(base+660, wantValue, 100*float64(anom)/60)}
			for _, tier := range []string{"0", ""} {
				t.Run("tier-"+tier, func(t *testing.T) {
					queryGroup := group
					if group == "metadata" {
						queryGroup = "average"
					}
					_, cols := c040Query(t, dd, host, context, base+600, base+2400, 30, queryGroup, tier)
					selected := c040Rows(cols, base+660, base+660)
					if group == "metadata" {
						if !assertExactColumnMetadata(t, selected, "load", want) {
							t.Error("partial first record omitted retained anomaly flags")
						}
					} else if !assertExactColumnValues(t, selected, "load", want, 0) {
						t.Error("partial coarse record replaced complete retained fine data")
					}
				})
			}
		})
	}
}

// Both tiers are populated from inception. On the aligned 4-second grid,
// the final row contains a fine value absent from the last coarse bucket.
func TestCase040ConventionalTail(t *testing.T) {
	for _, group := range []string{"sum", "average", "min", "max", "metadata"} {
		registerContract(t, "CASE-040/conventional-tail-"+group)
	}
	base := int64(fixture.T0 - fixture.T0%4)
	const host, context = "conventional-tail", "fixture.conventional_tail"
	value := func(off int64) float64 { return float64(((off-1)/4)%13 + 1) }
	anomalous := func(off int64) bool { return off > 356 }
	dd := startDedicatedStorageDaemon(t, daemon.Options{StorageTiers: 2, TierGrouping: [3]int{0, 4, 0}})
	pushDedicatedChart(t, dd, host, guid(9916), c040Series(context, base, 1, 360, value, anomalous, 0))
	waitDedicatedTierLastEntry(t, dd, host, context, 0, base+360)
	waitDedicatedTierLastEntry(t, dd, host, context, 1, base+356)
	for _, group := range []string{"sum", "average", "min", "max", "metadata"} {
		t.Run(group, func(t *testing.T) {
			trackContract(t, "CASE-040/conventional-tail-"+group)
			queryGroup := group
			if group == "metadata" {
				queryGroup = "average"
			}
			for _, tier := range []string{"0", ""} {
				t.Run("tier-"+tier, func(t *testing.T) {
					doc, cols := c040Query(t, dd, host, context, base+4, base+360, 89, queryGroup, tier)
					if tier == "" && !assertTierPresence(t, doc, []bool{true, true}) {
						t.Error("conventional fixture did not execute both fine and coarse reads")
					}
					var want []expectedColumnPoint
					for end := int64(8); end <= 360; end += 4 {
						v := value(end)
						if group == "sum" {
							v *= 4
						}
						arp := 0.0
						if anomalous(end) {
							arp = 100
						}
						want = append(want, wantNumberWithARPAt(base+end, v, arp))
					}
					if group == "metadata" {
						if !assertExactColumnMetadata(t, cols, "load", want) {
							t.Error("conventional tail lost fine-tier anomaly flags")
						}
					} else if !assertExactColumnValues(t, cols, "load", want, 0) {
						t.Error("conventional coarse-to-fine tail lost retained values")
					}
				})
			}
		})
	}
}

// The ruled coarse estimate stays between adjacent coarse-record averages.
// Fine samples still give exact truth; coarse interpolation may blend records
// but must not amplify the seam using a single fine spike as its anchor.
func TestCase040ShiftedConstantSeam(t *testing.T) {
	trackContract(t, "CASE-040/shifted-constant-seam")
	base := int64(fixture.T0 - fixture.T0%60)
	const host, context = "shifted-constant-seam", "fixture.shifted_constant_seam"
	value := func(off int64) float64 {
		if off == 3660 || off == 3780 || off == 4083 {
			return 2000
		}
		return 200
	}
	dd := startDedicatedStorageDaemon(t, daemon.Options{StorageTiers: 1, TierGrouping: [3]int{0, 60, 4}})
	closeFixture := pushDedicatedChart(t, dd, host, guid(9917), c040Series(context, base, 1, 3617, value, c040NotAnomalous, 0))
	waitDedicatedTierLastEntry(t, dd, host, context, 0, base+3617)
	c040Restart(t, dd, closeFixture, 3, "")
	pushDedicatedChart(t, dd, host, guid(9917), c040Series(context, base, 3618, 6617, value, c040NotAnomalous, 0))
	waitDedicatedTierLastEntry(t, dd, host, context, 1, base+6540)
	waitDedicatedTierLastEntry(t, dd, host, context, 2, base+6240)
	want := 0.0
	for off := int64(3661); off <= 3705; off++ {
		want += value(off)
	}
	want /= 45
	coarseAverage := func(first, last int64) float64 {
		total := 0.0
		for off := first; off <= last; off++ {
			total += value(off)
		}
		return total / float64(last-first+1)
	}
	previous := coarseAverage(3618, 3660)
	current := coarseAverage(3661, 3720)
	lower, upper := math.Min(previous, current), math.Max(previous, current)
	for _, tier := range []string{"0", "1", ""} {
		t.Run("tier-"+tier, func(t *testing.T) {
			_, cols := c040Query(t, dd, host, context, base+3300, base+4200, 20, "average", tier)
			row := c040Rows(cols, base+3705, base+3705)
			if tier == "0" {
				if !assertExactColumnValues(t, row, "load", []expectedColumnPoint{wantNumberAt(base+3705, want)}, 0) {
					t.Error("fine control lost the exact constant-row truth")
				}
				return
			}
			if len(row["load"]) != 1 || row["load"][0].Value == nil {
				t.Fatal("coarse estimate did not return the seam row")
			}
			got := *row["load"][0].Value
			if math.IsNaN(got) || math.IsInf(got, 0) || got < lower || got > upper {
				t.Errorf("coarse seam estimate %v outside fixture-derived interval [%v, %v]", got, lower, upper)
			}
		})
	}
}

func TestCase040YoungTierWork(t *testing.T) {
	trackContract(t, "CASE-040/young-tier-work")
	base := int64(fixture.T0 - fixture.T0%3600)
	const host = "young-tier-work"
	const dimensions, coarseInterval, lastCoarse = 2, int64(60), int64(28800)
	const queryAfter, queryBefore, outputRows = int64(21640), int64(28840), int64(60)
	// Five-record lookbehind and inclusive start: netdata/netdata @ b5934b3784,
	// src/web/api/queries/query-internal.h:11, query-plan.c:231-245,
	// and src/web/api/queries/query-window.c:343-355.
	const lookbehind = 5 * coarseInterval
	expandedAfter := queryAfter + 1 - lookbehind
	firstQueryRecord := (expandedAfter + coarseInterval - 1) / coarseInterval * coarseInterval
	pointBudget := func(firstStoredRecord int64) int64 {
		first := max(firstStoredRecord, firstQueryRecord)
		return dimensions * ((lastCoarse-first)/coarseInterval + 1)
	}
	dd := startDedicatedStorageDaemon(t, daemon.Options{StorageTiers: 3})
	conn, err := stream.Connect(dd.Addr, dd.StreamKey, stream.HostInfo{Hostname: host, MachineGUID: guid(9918)}, stream.CapsLive)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	for name, first := range map[string]int64{"young": 22657, "mature": 1} {
		context := "fixture.work_" + name
		ch := c040Series(context, base, first, 28840, c040Flat, c040NotAnomalous, 0)
		ch.Dimensions = append(ch.Dimensions, fixture.Dimension{ID: "second", Points: ch.Dimensions[0].Points})
		ch.Define(conn)
		ch.PushLive(conn)
		if err := conn.Flush(); err != nil {
			t.Fatal(err)
		}
		waitDedicatedTierLastEntry(t, dd, host, context, 1, base+28800)
	}
	for name, tc := range map[string]struct {
		context, tier       string
		queryBudget         [3]int64
		selectedPointBudget int64
	}{
		"young":  {"fixture.work_young", "", [3]int64{dimensions, dimensions, 0}, pointBudget(22680)},
		"forced": {"fixture.work_young", "1", [3]int64{0, dimensions, 0}, pointBudget(22680)},
		"mature": {"fixture.work_mature", "", [3]int64{dimensions, dimensions, 0}, pointBudget(coarseInterval)},
	} {
		t.Run(name, func(t *testing.T) {
			p := daemon.DataParams(tc.context, base+queryAfter, base+queryBefore, outputRows)
			p.Set("options", "jsonwrap|unaligned")
			if tc.tier != "" {
				p.Set("tier", tc.tier)
			}
			doc, err := dd.DataV3(host, p)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := canon.Columns(doc); err != nil {
				t.Fatal(err)
			}
			points, ok := strictTierPoints(t, doc)
			if !ok || len(points) != 3 {
				t.Fatal("invalid work counters")
			}
			if points[1] <= 0 || points[1] > tc.selectedPointBudget {
				t.Errorf("tier1 read %d records, budget %d", points[1], tc.selectedPointBudget)
			}
			db := doc["db"].(map[string]any)
			for _, raw := range db["per_tier"].([]any) {
				entry := raw.(map[string]any)
				tier := int(entry["tier"].(float64))
				if tier >= len(tc.queryBudget) {
					t.Fatalf("unexpected work-counter tier %d", tier)
				}
				q, ok := entry["queries"].(float64)
				if !ok || math.IsNaN(q) || math.IsInf(q, 0) || q < 0 || q != math.Trunc(q) {
					t.Fatalf("invalid tier%d queries: %v", tier, entry["queries"])
				}
				if q > float64(tc.queryBudget[tier]) {
					t.Errorf("tier%d initialized %v queries, budget %d", tier, q, tc.queryBudget[tier])
				}
				t.Logf("tier%d queries=%v points=%d", tier, q, points[tier])
			}
		})
	}
}
