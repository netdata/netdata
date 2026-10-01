// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

// Values are constant within each coarse record, so rollup loses no information
// needed by the oracle. Different bucket widths expose both overlap and prefix loss.
func TestCase040TierHandoff(t *testing.T) {
	for _, late := range []bool{false, true} {
		t.Run(fmt.Sprintf("late-%v", late), func(t *testing.T) {
			component := "single-tier-control"
			if late {
				component = "fine-to-coarse"
			}
			for _, group := range []string{"sum", "rate-sum", "average", "min", "max", "metadata", "annotations"} {
				registerContractComponent(t, "CASE-040/handoff-"+group, component)
			}
			const host, context = "tier-handoff", "fixture.tier_handoff"
			base := int64(fixture.T0 - fixture.T0%4)
			tiers, initial := 2, 360
			if late {
				tiers, initial = 1, 120
			}
			dd := startDedicatedStorageDaemon(t, daemon.Options{StorageTiers: tiers, TierGrouping: [3]int{0, 4, 0}})
			value := func(timestamp int64) float64 { return float64(((timestamp-base-1)/4)%13 + 1) }
			anomalous := func(timestamp int64) bool {
				return (timestamp-base)%4 == 0 && (timestamp-base)/4%3 == 1
			}
			makeChart := func(start int64, count int) fixture.Chart {
				chart := fixture.Series(context, context, start, count, 1,
					func(i int) string { return fmt.Sprint(value(start + int64(i))) },
					func(i int) string {
						if anomalous(start + int64(i)) {
							return stream.FlagAnomalous
						}
						return stream.FlagNotAnomalous
					})
				chart.Dimensions = append(chart.Dimensions, fixture.Dimension{
					ID: "rate", Algorithm: "incremental", Points: chart.Dimensions[0].Points,
				})
				return chart
			}
			closeFixture := pushDedicatedChart(t, dd, host, guid(9701), makeChart(base, initial))
			if _, err := dd.WaitRetention(host, context, base+1, base+int64(initial), 15*time.Second); err != nil {
				t.Fatal(err)
			}
			if late {
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
				const old = "    storage tiers = 1\n"
				if strings.Count(string(conf), old) != 1 {
					t.Fatal("storage tiers configuration not found exactly once")
				}
				if err := os.WriteFile(path, []byte(strings.Replace(string(conf), old, "    storage tiers = 2\n", 1)), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := dd.Restart(); err != nil {
					t.Fatal(err)
				}
				pushDedicatedChart(t, dd, host, guid(9701), makeChart(base+120, 240))
			}
			waitDedicatedTierLastEntry(t, dd, host, context, 1, base+356)
			for _, group := range []string{"sum", "rate-sum", "average", "min", "max", "metadata", "annotations"} {
				t.Run(group, func(t *testing.T) {
					trackContractComponent(t, "CASE-040/handoff-"+group, component)
					for _, span := range []int64{4, 8, 16, 32, 64} {
						for _, tier := range []string{"", "0"} {
							t.Run(fmt.Sprintf("span-%d/tier-%s", span, tier), func(t *testing.T) {
								after, before := base+4, base+324
								p := daemon.DataParams(context, after, before, (before-after)/span)
								p.Set("options", "jsonwrap|unaligned")
								p.Set("time_group", group)
								dimension := "load"
								if group == "rate-sum" {
									p.Set("time_group", "sum")
									dimension = "rate"
								}
								if group == "metadata" || group == "annotations" {
									p.Set("time_group", "average")
								}
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
								want := make([]expectedColumnPoint, 0, (before-after)/span)
								for end := after + span; end <= before; end += span {
									sum, low, high := 0.0, math.Inf(1), math.Inf(-1)
									anomalyCount := 0
									for ts := end - span + 1; ts <= end; ts++ {
										v := value(ts)
										sum, low, high = sum+v, math.Min(low, v), math.Max(high, v)
										if anomalous(ts) {
											anomalyCount++
										}
									}
									v := sum
									switch group {
									case "average":
										v /= float64(span)
									case "min":
										v = low
									case "max":
										v = high
									}
									if group == "annotations" {
										want = append(want, wantNumberWithPAAt(end, v, 0))
									} else {
										want = append(want, wantNumberWithARPAt(end, v, 100*float64(anomalyCount)/float64(span)))
									}
								}
								if group == "metadata" || group == "annotations" {
									if !assertExactColumnMetadata(t, cols, "load", want) {
										t.Error("tier handoff changed anomaly metadata")
									}
								} else if !assertExactColumnValues(t, cols, dimension, want, 0) {
									t.Error("tier handoff changed fixture arithmetic")
								}
							})
						}
					}
				})
			}
		})
	}
}
