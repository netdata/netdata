// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

func TestCase035IntermediatePageVisibility(t *testing.T) {
	const contract = "CASE-035/tier0-page-boundary-keeps-every-sample"
	type phase struct {
		cadence int
		points  [][2]int64
	}
	// Native source shapes: netdata/netdata @ 7f7bfc45c113d0450c0a3ca79e0683bd5373e7f6,
	// src/database/engine/dbengine-unittest.c:1081-1174. Only public visibility
	// is asserted; query warmups do not guarantee a particular eviction state.
	for name, tc := range map[string]struct {
		guidIdx int
		phases  []phase
	}{
		"intermediate-page": {9970, []phase{
			{60, [][2]int64{{100, 1}, {160, 2}}},
			{10, [][2]int64{{170, 99}, {180, 4}, {190, 5}, {200, 6}}},
			{100, [][2]int64{{260, 7}, {360, 8}}},
		}},
		"singleton-bridge": {9971, []phase{
			{60, [][2]int64{{100, 1}, {160, 2}}},
			{100, [][2]int64{{170, 3}}},
			{10, [][2]int64{{180, 99}, {190, 5}, {200, 6}}},
			{100, [][2]int64{{250, 7}, {350, 8}}},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			trackContractComponent(t, contract, name)
			dd := startDedicatedStorageDaemon(t, daemon.Options{StorageTiers: 1})
			base := int64(fixture.T0 - fixture.T0%300)
			host := "intermediate-" + name
			context := fmt.Sprintf("fixture.intermediate_page_%d", tc.guidIdx)
			conn, err := stream.Connect(dd.Addr, dd.StreamKey, stream.HostInfo{
				Hostname: host, MachineGUID: guid(tc.guidIdx),
			}, stream.CapsLive)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = conn.Close() })
			ch := fixture.Chart{ID: context, Context: context, Title: "intermediate page", Units: "units", Family: "fixture"}
			for _, item := range tc.phases {
				ch.UpdateEvery = item.cadence
				dim := fixture.Dimension{ID: "load"}
				for _, point := range item.points {
					dim.Points = append(dim.Points, fixture.Point{
						T: base + point[0], Collected: strconv.FormatInt(point[1], 10), Flags: stream.FlagNotAnomalous,
					})
				}
				ch.Dimensions = []fixture.Dimension{dim}
				ch.Define(conn)
				ch.PushLive(conn)
				if err := conn.Flush(); err != nil {
					t.Fatal(err)
				}
			}
			settle := func() {
				if _, err := dd.WaitRetention(host, context, base+100, ch.LastT(), 15*time.Second); err != nil {
					t.Fatal(err)
				}
			}
			check := func(t *testing.T) {
				p := daemon.DataParamsTier(context, 0, base+1, base+400, 1, "max")
				p.Set("options", "jsonwrap|unaligned|virtual-points")
				doc, err := dd.DataV3(host, p)
				if err != nil {
					t.Fatal(err)
				}
				cols, err := canon.Columns(doc)
				if err != nil {
					t.Fatal(err)
				}
				if !queryTimestampGridExact(t, doc, queryExpectedVirtualGrid(t, base+1, base+400, 1, false)) ||
					!assertOnlyColumn(t, cols, "load") || !assertTierPresence(t, doc, []bool{true}) ||
					!assertExactColumnValues(t, cols, "load", []expectedColumnPoint{wantNumberAt(base+400, 99)}, 0) {
					t.Error("intermediate page lost its unique maximum")
				}
			}
			settle()
			t.Run("hot", check)
			if err := conn.Close(); err != nil {
				t.Fatal(err)
			}
			if err := dd.Restart(); err != nil {
				t.Fatal(err)
			}
			settle()
			t.Run("restarted", check)
			for _, window := range [][2]int64{{1, 100}, {301, 400}} {
				p := daemon.DataParamsTier(context, 0, base+window[0], base+window[1], 1, "max")
				p.Set("options", "jsonwrap|unaligned|virtual-points")
				doc, err := dd.DataV3(host, p)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := canon.Columns(doc); err != nil {
					t.Fatal(err)
				}
			}
			t.Run("warmed", check)
			t.Run("repeated", check)
		})
	}
}
