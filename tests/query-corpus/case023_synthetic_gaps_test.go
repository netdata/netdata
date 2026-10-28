// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"fmt"
	"testing"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

// Stopping collection and storing the missing slots explicitly describe the
// same absence. Occurrences count each slot once; sample percentages retain
// the delivered-sample rule when a slot overlaps several result buckets.
func TestCase023SyntheticGapSlots(t *testing.T) {
	const (
		window          = int64(100)
		countContract   = "CASE-023/synthetic-gap-occurrences"
		samplesContract = "CASE-023/synthetic-gap-sample-weight"
	)
	for index, cadence := range []int64{1, 10} {
		t.Run(fmt.Sprintf("ue%d", cadence), func(t *testing.T) {
			for _, shape := range []string{"stopped", "stored"} {
				for _, points := range []int64{1, 4, 10, 100} {
					component := fmt.Sprintf("ue%d/%s/points%d", cadence, shape, points)
					registerContractComponent(t, countContract, component)
					registerContractComponent(t, samplesContract, component)
				}
			}
			host := fmt.Sprintf("c023-synthetic-%d", cadence)
			context := fmt.Sprintf("fixture.c023_synthetic_%d", cadence)
			base := fixture.T0 - fixture.T0%cadence
			makeChart := func(shape string) fixture.Chart {
				return fixture.Chart{
					ID: context + "_" + shape, Title: "missing source slots", Units: "units",
					Family: "fixture", Context: context, UpdateEvery: int(cadence),
					Dimensions: []fixture.Dimension{{ID: "value"}},
				}
			}
			always, stopped, stored := makeChart("always"), makeChart("stopped"), makeChart("stored")
			for slot := int64(1); slot <= window/cadence; slot++ {
				point := fixture.Point{T: base + slot*cadence, Collected: "1", Flags: stream.FlagNotAnomalous}
				always.Dimensions[0].Points = append(always.Dimensions[0].Points, point)
				if slot == 1 {
					stopped.Dimensions[0].Points = append(stopped.Dimensions[0].Points, point)
				} else {
					point.Collected, point.Flags = "", stream.FlagEmpty
				}
				stored.Dimensions[0].Points = append(stored.Dimensions[0].Points, point)
			}
			machineGUID := guid(10010 + index)
			conn := connect(t, host, machineGUID, stream.CapsLive)
			for _, chart := range []fixture.Chart{always, stopped, stored} {
				chart.Define(conn)
				chart.PushLive(conn)
			}
			if err := conn.Flush(); err != nil {
				t.Fatal(err)
			}
			waitChartsRetention(t, host, always, stopped, stored)

			for shape, chart := range map[string]fixture.Chart{"stopped": stopped, "stored": stored} {
				for _, points := range []int64{1, 4, 10, 100} {
					component := fmt.Sprintf("ue%d/%s/points%d", cadence, shape, points)
					for group, contract := range map[string]string{
						"number-of-times":       countContract,
						"percentage-of-samples": samplesContract,
					} {
						t.Run(component+"/"+group, func(t *testing.T) {
							trackContractComponent(t, contract, component)
							params := daemon.DataParamsTier(context, 0, base, base+window, points, group)
							params.Set("time_group_options", "==gap")
							params.Set("scope_instances", chart.ID)
							params.Set("group_by", "instance")
							params.Set("options", "jsonwrap|unaligned|virtual-points")
							doc, err := td.DataV3(host, params)
							if err != nil {
								t.Fatal(err)
							}
							columns, err := canon.Columns(doc)
							if err != nil {
								t.Fatal(err)
							}
							grid := queryExpectedVirtualGrid(t, base, base+window, points, false)
							want := make([]expectedColumnPoint, 0, grid.rows)
							for row := 0; row < grid.rows; row++ {
								end := grid.before - int64(grid.rows-1-row)*grid.updateEvery
								start := max(end-grid.updateEvery, base)
								var delivered, missing, occurrences int
								for slot := int64(1); slot <= window/cadence; slot++ {
									slotStart, slotEnd := base+(slot-1)*cadence, base+slot*cadence
									if slotStart >= end || slotEnd <= start {
										continue
									}
									delivered++
									if slot > 1 {
										missing++
										if slotStart >= start {
											occurrences++
										}
									}
								}
								value := float64(occurrences)
								if group == "percentage-of-samples" {
									value = 100 * float64(missing) / float64(delivered)
								}
								want = append(want, wantNumberAt(end, value))
							}
							column := chart.ID + "@" + machineGUID
							if !assertViewFields(t, doc, grid.after, grid.before, grid.updateEvery) ||
								!assertOnlyColumn(t, columns, column) ||
								!assertExactColumn(t, columns, column, want, 1e-6) {
								t.Error("missing-slot result differs from the literal source slots")
							}
						})
					}
				}
			}
		})
	}
}
