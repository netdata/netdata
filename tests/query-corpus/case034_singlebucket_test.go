// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-034 pins the API timestamp grid independently of values and storage.
// The same explicit time selection must have identical geometry on every Agent.
package corpus

import (
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

type c034Shape struct {
	name, context            string
	updateEvery, machineGUID int
	firstOffset, lastOffset  int64
	gapEvery                 int
}

func c034Base() int64 {
	return int64(fixture.T0) - int64(fixture.T0)%3600
}

func c034Fixture(t *testing.T, shape c034Shape) string {
	t.Helper()

	host := "c034-" + shape.name
	base := c034Base()
	ch := fixture.Chart{
		ID: shape.context, Title: "timestamp grid", Units: "units",
		Family: "fixture", Context: shape.context, UpdateEvery: shape.updateEvery,
		Dimensions: []fixture.Dimension{{ID: "value", Algorithm: "absolute"}},
	}
	first := base + shape.firstOffset
	last := base + shape.lastOffset
	for ts, index := first, 0; ts <= last; ts, index = ts+int64(shape.updateEvery), index+1 {
		point := fixture.Point{
			T:         ts,
			Collected: strconv.FormatInt(1+(ts-base)%97, 10),
			Flags:     stream.FlagNotAnomalous,
		}
		if shape.gapEvery > 0 && index > 0 && ts < last && index%shape.gapEvery == 0 {
			point.Collected = ""
			point.Flags = stream.FlagEmpty
		}
		ch.Dimensions[0].Points = append(ch.Dimensions[0].Points, point)
	}

	pushLiveBurst(t, host, guid(shape.machineGUID), ch)
	if _, err := td.WaitRetention(host, shape.context, ch.FirstT(), ch.LastT(), 60*time.Second); err != nil {
		t.Fatal(err)
	}
	return host
}

func c034HotEdgeFixture(t *testing.T, name, context string, machineGUID int, last int64) string {
	t.Helper()

	host := "c034-" + name
	ch := fixture.Chart{
		ID: context, Title: "hot-edge timestamp grid", Units: "units",
		Family: "fixture", Context: context, UpdateEvery: 60,
		Dimensions: []fixture.Dimension{{ID: "value", Algorithm: "absolute"}},
	}
	for i := int64(2); i >= 0; i-- {
		ch.Dimensions[0].Points = append(ch.Dimensions[0].Points, fixture.Point{
			T: last - i*60, Collected: strconv.FormatInt(3-i, 10), Flags: stream.FlagNotAnomalous,
		})
	}
	pushLiveBurst(t, host, guid(machineGUID), ch)
	if _, err := td.WaitRetention(host, context, ch.FirstT(), ch.LastT(), 60*time.Second); err != nil {
		t.Fatal(err)
	}
	return host
}

func c034NearLiveFixture(
	t *testing.T, name, context string, machineGUID int, first, last int64,
) string {
	t.Helper()

	host := "c034-" + name
	ch := fixture.Chart{
		ID: context, Title: "near-live timestamp grid", Units: "units",
		Family: "fixture", Context: context, UpdateEvery: 10,
		Dimensions: []fixture.Dimension{{ID: "value", Algorithm: "absolute"}},
	}
	for ts := first; ts <= last; ts += 10 {
		ch.Dimensions[0].Points = append(ch.Dimensions[0].Points, fixture.Point{
			T: ts, Collected: "1", Flags: stream.FlagNotAnomalous,
		})
	}
	pushLiveBurst(t, host, guid(machineGUID), ch)
	if _, err := td.WaitRetention(host, context, first, last, 15*time.Second); err != nil {
		t.Fatal(err)
	}
	return host
}

func TestCase034APITimestampGridIsImmutable(t *testing.T) {
	const contract = "CASE-034/api-timestamp-grid-is-immutable"
	shapes := []c034Shape{
		{name: "dense-ue1", context: "fixture.c034_timestamp_grid.dense_ue1", updateEvery: 1, machineGUID: 294, firstOffset: 1, lastOffset: 10000},
		{name: "dense-ue10", context: "fixture.c034_timestamp_grid.dense_ue10", updateEvery: 10, machineGUID: 295, firstOffset: 10, lastOffset: 10000},
		{name: "gapped-ue1", context: "fixture.c034_timestamp_grid.gapped_ue1", updateEvery: 1, machineGUID: 298, firstOffset: 1, lastOffset: 10000, gapEvery: 3},
		{name: "partial-ue1", context: "fixture.c034_timestamp_grid.partial_ue1", updateEvery: 1, machineGUID: 299, firstOffset: 3600, lastOffset: 5200},
	}
	queryShapes := []struct {
		name    string
		points  int64
		aligned bool
	}{
		{name: "aligned-1", points: 1, aligned: true},
		{name: "unaligned-1", points: 1, aligned: false},
		{name: "aligned-7", points: 7, aligned: true},
		{name: "unaligned-7", points: 7, aligned: false},
		{name: "aligned-60", points: 60, aligned: true},
	}

	base := c034Base()
	after, before := base+2400, base+6000
	for _, shape := range shapes {
		t.Run(shape.name, func(t *testing.T) {
			trackContractComponent(t, contract, shape.name)
			host := c034Fixture(t, shape)
			ok := true

			for _, queryShape := range queryShapes {
				want := queryExpectedVirtualGrid(
					t, after, before, queryShape.points, queryShape.aligned)
				for _, tier := range []int{-1, 0, 1, 2} {
					for _, group := range []string{"average", "sum", "latest"} {
						params := daemon.DataParams(shape.context, after, before, queryShape.points)
						if tier >= 0 {
							params.Set("tier", strconv.Itoa(tier))
						}
						params.Set("time_group", group)
						params.Set("scope_dimensions", "value")
						if !queryShape.aligned {
							params.Set("options", "jsonwrap|unaligned")
						}

						doc, err := td.DataV3(host, params)
						if err != nil {
							t.Fatal(err)
						}
						if !queryTimestampGridExact(t, doc, want) {
							t.Logf("%s tier=%d group=%s changed the API timestamp grid",
								queryShape.name, tier, group)
							ok = false
						}
					}
				}
			}

			// The v2 and v3 Cloud-facing surfaces share the same engine grid.
			params := daemon.DataParams(shape.context, after, before, 1)
			params.Set("scope_dimensions", "value")
			v2, err := td.HostJSON(host, "api/v2/data", params)
			if err != nil {
				t.Fatal(err)
			}
			if !queryTimestampGridExact(
				t, v2, queryExpectedVirtualGrid(t, after, before, 1, true)) {
				t.Logf("api/v2/data changed the API timestamp grid")
				ok = false
			}

			if !ok {
				t.Errorf("BROKEN %s (%s): %s", contract, shape.name, manifest[contract].Proves)
			}
		})
	}

	t.Run("hot-edge-data-independence", func(t *testing.T) {
		trackContractComponent(t, contract, "hot-edge-data-independence")

		sourceNow := time.Now().Unix() - 1
		boundary := sourceNow - sourceNow%60
		hosts := []struct {
			host, context string
		}{
			{
				host: c034HotEdgeFixture(
					t, "hot-edge-newer", "fixture.c034_timestamp_grid.hot_edge_newer",
					343, boundary-60),
				context: "fixture.c034_timestamp_grid.hot_edge_newer",
			},
			{
				host: c034HotEdgeFixture(
					t, "hot-edge-older", "fixture.c034_timestamp_grid.hot_edge_older",
					344, boundary-120),
				context: "fixture.c034_timestamp_grid.hot_edge_older",
			},
		}

		// Keep the explicit before within one update_every of query execution
		// so the LATEST hot-edge path is exercised on both fixture Agents.
		hotBefore := time.Now().Unix() - 1
		hotAfter := hotBefore - 3600
		ok := true
		for _, fixtureHost := range hosts {
			for _, aligned := range []bool{true, false} {
				params := daemon.DataParams(fixtureHost.context, hotAfter, hotBefore, 1)
				params.Set("time_group", "latest")
				params.Set("scope_dimensions", "value")
				params.Set("options", "jsonwrap|virtual-points")
				if !aligned {
					params.Set("options", "jsonwrap|unaligned|virtual-points")
				}
				doc, err := td.DataV3(fixtureHost.host, params)
				if err != nil {
					t.Fatal(err)
				}
				if !queryTimestampGridExact(
					t, doc, queryExpectedVirtualGrid(t, hotAfter, hotBefore, 1, aligned)) {
					t.Logf("%s aligned=%v made the LATEST hot-edge grid depend on retention",
						fixtureHost.host, aligned)
					ok = false
				}
				if !aligned && !assertTierPresence(t, doc, []bool{false, false, false}) {
					t.Logf("%s aligned=%v did not use the collector-cache fast path",
						fixtureHost.host, aligned)
					ok = false
				}
			}

			// A relative hot-edge request must resolve from the query-time clock,
			// not from this metric's newest stored timestamp. Wall-clock bounds
			// make this deterministic without assuming both HTTP calls land in
			// the same second.
			started := time.Now().Unix()
			relative := daemon.DataParams(fixtureHost.context, -3601, -1, 1)
			relative.Set("time_group", "latest")
			relative.Set("scope_dimensions", "value")
			relative.Set("options", "jsonwrap|unaligned|virtual-points")
			relativeDoc, err := td.DataV3(fixtureHost.host, relative)
			finished := time.Now().Unix()
			if err != nil {
				t.Fatal(err)
			}
			view := queryObject(t, relativeDoc, "view", "relative view")
			resolvedBefore, integer := queryInteger(view["before"])
			if !integer || resolvedBefore < started-2 || resolvedBefore > finished-2 {
				t.Logf("%s relative before = %v, want query-time range [%d,%d]",
					fixtureHost.host, view["before"], started-2, finished-2)
				ok = false
			} else if !queryTimestampGridExact(
				t, relativeDoc, queryExpectedVirtualGrid(t, resolvedBefore-3600, resolvedBefore, 1, false)) {
				t.Logf("%s changed the relative request's resolved timestamp grid", fixtureHost.host)
				ok = false
			}
			if !assertTierPresence(t, relativeDoc, []bool{false, false, false}) {
				t.Logf("%s relative query did not use the collector-cache fast path", fixtureHost.host)
				ok = false
			}
		}
		if !ok {
			t.Errorf("BROKEN %s (hot-edge-data-independence): %s", contract, manifest[contract].Proves)
		}
	})

	t.Run("near-live-partial-data", func(t *testing.T) {
		trackContractComponent(t, contract, "near-live-partial-data")

		before := time.Now().Unix() - 1
		before -= before % 10
		after := before - 60
		fixtures := []struct {
			name, context string
			machineGUID   int
			last          int64
		}{
			{
				name: "near-live-complete", context: "fixture.c034_timestamp_grid.near_live_complete",
				machineGUID: 346, last: before,
			},
			{
				name: "near-live-missing-last", context: "fixture.c034_timestamp_grid.near_live_missing_last",
				machineGUID: 347, last: before - 10,
			},
		}

		ok := true
		for _, live := range fixtures {
			host := c034NearLiveFixture(t, live.name, live.context, live.machineGUID, after+10, live.last)
			params := daemon.DataParams(live.context, after, before, 6)
			params.Set("scope_dimensions", "value")
			params.Set("options", "jsonwrap|unaligned|virtual-points")
			doc, err := td.DataV3(host, params)
			if err != nil {
				t.Fatal(err)
			}
			if !queryTimestampGridExact(t, doc, queryExpectedVirtualGrid(t, after, before, 6, false)) {
				t.Logf("%s changed the near-live timestamp grid based on final-row availability", host)
				ok = false
			}

			cols, err := canon.Columns(doc)
			if err != nil {
				t.Fatal(err)
			}
			want := make([]expectedColumnPoint, 0, 6)
			for ts := after + 10; ts <= before; ts += 10 {
				if ts > live.last {
					want = append(want, wantEmptyAt(ts))
				} else {
					want = append(want, wantNumberAt(ts, 1))
				}
			}
			if !assertExactColumn(t, cols, "value", want, 0) {
				ok = false
			}
		}
		if !ok {
			t.Errorf("BROKEN %s (near-live-partial-data): %s", contract, manifest[contract].Proves)
		}
	})
}
