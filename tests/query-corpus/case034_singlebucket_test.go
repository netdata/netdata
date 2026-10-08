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

type c034NearLiveWindow struct {
	updateEvery, first, last, delayedLast int64
}

func c034NearLiveFixture(t *testing.T, name, context string, machineGUID int, window c034NearLiveWindow) string {
	t.Helper()

	host := "c034-" + name
	ch := fixture.Chart{
		ID: context, Title: "near-live timestamp grid", Units: "units",
		Family: "fixture", Context: context, UpdateEvery: int(window.updateEvery),
		Dimensions: []fixture.Dimension{
			{ID: "always", Algorithm: "absolute"},
			{ID: "delayed", Algorithm: "absolute"},
		},
	}
	for ts := window.first; ts <= window.last; ts += window.updateEvery {
		ch.Dimensions[0].Points = append(ch.Dimensions[0].Points, fixture.Point{
			T: ts, Collected: "1", Flags: stream.FlagNotAnomalous,
		})
		if ts <= window.delayedLast {
			ch.Dimensions[1].Points = append(ch.Dimensions[1].Points, fixture.Point{
				T: ts, Collected: "2", Flags: stream.FlagNotAnomalous,
			})
		}
	}
	pushLiveBurst(t, host, guid(machineGUID), ch)
	if _, err := td.WaitRetention(host, context, window.first, window.last, 15*time.Second); err != nil {
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
					343, boundary),
				context: "fixture.c034_timestamp_grid.hot_edge_newer",
			},
			{
				host: c034HotEdgeFixture(
					t, "hot-edge-older", "fixture.c034_timestamp_grid.hot_edge_older",
					344, boundary-60),
				context: "fixture.c034_timestamp_grid.hot_edge_older",
			},
		}

		ok := true
		v1GridExact := func(doc map[string]any, want queryExpectedGrid, label string) bool {
			t.Helper()
			gotAfter, afterOK := queryInteger(doc["after"])
			gotBefore, beforeOK := queryInteger(doc["before"])
			gotEvery, everyOK := queryInteger(doc["view_update_every"])
			gotPoints, pointsOK := queryInteger(doc["points"])
			if !afterOK || !beforeOK || !everyOK || !pointsOK ||
				gotAfter != want.after || gotBefore != want.before ||
				gotEvery != want.updateEvery || gotPoints != int64(want.rows) {
				t.Logf("%s v1 grid = %v/%v/%v/%v, want %d/%d/%d/%d",
					label, doc["after"], doc["before"], doc["view_update_every"], doc["points"],
					want.after, want.before, want.updateEvery, want.rows)
				return false
			}
			if err := queryRawTimestampsExact(doc, []int64{want.before}); err != nil {
				t.Logf("%s v1 wire grid: %v", label, err)
				return false
			}
			return true
		}
		for _, aligned := range []bool{true, false} {
			// Both Agents receive the same request-derived cutoff captured before
			// fixture setup, so a minute-boundary crossing cannot change whether
			// either newest point is inside the two-update-every hot edge.
			hotBefore := sourceNow
			hotAfter := hotBefore - 3600
			for _, fixtureHost := range hosts {
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

				v1Params := daemon.DataParams(fixtureHost.context, hotAfter, hotBefore, 1)
				for _, key := range []string{"scope_contexts", "time_group", "group_by", "aggregation"} {
					v1Params.Del(key)
				}
				v1Params.Set("context", fixtureHost.context)
				v1Params.Set("group", "latest")
				v1Params.Set("format", "json")
				v1Params.Set("options", "jsonwrap|seconds|virtual-points")
				if !aligned {
					v1Params.Set("options", "jsonwrap|seconds|unaligned|virtual-points")
				}
				v1Doc, err := td.HostJSON(fixtureHost.host, "api/v1/data", v1Params)
				if err != nil {
					t.Fatal(err)
				}
				if !v1GridExact(
					v1Doc, queryExpectedVirtualGrid(t, hotAfter, hotBefore, 1, aligned),
					fixtureHost.host+" explicit") {
					ok = false
				}
			}
		}

		for _, fixtureHost := range hosts {
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

			v1Relative := daemon.DataParams(fixtureHost.context, -3601, -1, 1)
			for _, key := range []string{"scope_contexts", "time_group", "group_by", "aggregation"} {
				v1Relative.Del(key)
			}
			v1Relative.Set("context", fixtureHost.context)
			v1Relative.Set("group", "latest")
			v1Relative.Set("format", "json")
			v1Relative.Set("options", "jsonwrap|seconds|unaligned|virtual-points")
			v1Started := time.Now().Unix()
			v1Doc, err := td.HostJSON(fixtureHost.host, "api/v1/data", v1Relative)
			v1Finished := time.Now().Unix()
			if err != nil {
				t.Fatal(err)
			}
			v1ResolvedBefore, integer := queryInteger(v1Doc["before"])
			if !integer || v1ResolvedBefore < v1Started-2 || v1ResolvedBefore > v1Finished-2 {
				t.Logf("%s v1 relative before = %v, want query-time range [%d,%d]",
					fixtureHost.host, v1Doc["before"], v1Started-2, v1Finished-2)
				ok = false
			} else if !v1GridExact(
				v1Doc,
				queryExpectedVirtualGrid(t, v1ResolvedBefore-3600, v1ResolvedBefore, 1, false),
				fixtureHost.host+" relative") {
				ok = false
			}
		}
		if !ok {
			t.Errorf("BROKEN %s (hot-edge-data-independence): %s", contract, manifest[contract].Proves)
		}
	})

	t.Run("near-live-partial-data", func(t *testing.T) {
		const (
			trimmingContract = "CASE-034/near-live-partial-data-is-trimmed"
			updateEvery      = int64(60)
		)
		trackContract(t, trimmingContract)

		fixtures := []struct {
			name, context      string
			machineGUID        int
			delayedLastOffset  int64
			rows               int
			trimmedAfterOffset int64
		}{
			{
				name: "near-live-complete", context: "fixture.c034_timestamp_grid.near_live_complete",
				machineGUID: 346, rows: 6,
			},
			{
				name: "near-live-missing-last", context: "fixture.c034_timestamp_grid.near_live_missing_last",
				machineGUID: 347, delayedLastOffset: -2 * updateEvery, rows: 4, trimmedAfterOffset: -updateEvery,
			},
		}

		ok := true
		requireNearLive := func(host string, before int64) {
			t.Helper()
			if now := time.Now().Unix(); before < now-2*updateEvery {
				t.Fatalf("%s fixture aged outside the near-live window: before=%d now=%d update_every=%d",
					host, before, now, updateEvery)
			}
		}
		for _, live := range fixtures {
			before := time.Now().Unix() - 1
			before -= before % updateEvery
			after := before - 6*updateEvery
			grid := queryExpectedVirtualGrid(t, after, before, 6, false)
			delayedLast := before + live.delayedLastOffset
			wantTrimmedAfter := before + live.trimmedAfterOffset
			window := c034NearLiveWindow{
				updateEvery: updateEvery,
				first:       after + updateEvery,
				last:        before,
				delayedLast: delayedLast,
			}
			host := c034NearLiveFixture(t, live.name, live.context, live.machineGUID, window)
			requireNearLive(host, before)
			params := daemon.DataParams(live.context, after, before, 6)
			params.Set("scope_dimensions", "always|delayed")
			params.Set("options", "jsonwrap|unaligned|virtual-points")
			doc, err := td.DataV3(host, params)
			if err != nil {
				t.Fatal(err)
			}
			if !assertViewFields(t, doc, grid.after, grid.before, grid.updateEvery) {
				ok = false
			}
			wantWire := make([]int64, live.rows)
			latest := grid.before - int64(grid.rows-live.rows)*grid.updateEvery
			for i := range wantWire {
				wantWire[i] = latest - int64(i)*grid.updateEvery
			}
			if err := queryRawTimestampsExact(doc, wantWire); err != nil {
				t.Logf("%s near-live trimmed grid: %v", host, err)
				ok = false
			}

			cols, err := canon.Columns(doc)
			if err != nil {
				t.Fatal(err)
			}
			always := make([]expectedColumnPoint, 0, live.rows)
			delayed := make([]expectedColumnPoint, 0, live.rows)
			for row := 1; row <= live.rows; row++ {
				ts := after + int64(row)*updateEvery
				always = append(always, wantNumberAt(ts, 1))
				delayed = append(delayed, wantNumberAt(ts, 2))
			}
			if !assertExactColumn(t, cols, "always", always, 0) ||
				!assertExactColumn(t, cols, "delayed", delayed, 0) {
				ok = false
			}

			debugParams := daemon.DataParams(live.context, after, before, 6)
			debugParams.Set("scope_dimensions", "always|delayed")
			debugParams.Set("options", "jsonwrap|unaligned|virtual-points|debug")
			requireNearLive(host, before)
			debugDoc, err := td.DataV3(host, debugParams)
			if err != nil {
				t.Fatal(err)
			}
			if !assertViewFields(t, debugDoc, grid.after, grid.before, grid.updateEvery) {
				ok = false
			}
			if err := queryRawTimestampsExact(debugDoc, wantWire); err != nil {
				t.Logf("%s debug near-live trimmed grid: %v", host, err)
				ok = false
			}
			view := queryObject(t, debugDoc, "view", "debug view")
			trimming := queryObject(t, view, "partial_data_trimming", "debug partial_data_trimming")
			maxEvery, maxOK := queryInteger(trimming["max_update_every"])
			expectedAfter, expectedOK := queryInteger(trimming["expected_after"])
			trimmedAfter, trimmedOK := queryInteger(trimming["trimmed_after"])
			if !maxOK || !expectedOK || !trimmedOK ||
				maxEvery != 2*updateEvery || expectedAfter != before-2*updateEvery ||
				trimmedAfter != wantTrimmedAfter {
				t.Logf("%s partial_data_trimming = %v, want max=%d expected_after=%d trimmed_after=%d",
					host, trimming, 2*updateEvery, before-2*updateEvery, wantTrimmedAfter)
				ok = false
			}
		}
		if !ok {
			t.Errorf("BROKEN %s: %s", trimmingContract, manifest[trimmingContract].Proves)
		}
	})
}
