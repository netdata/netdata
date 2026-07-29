// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-034 a query asking for one bucket still covers exactly the requested
// interval. The sample or stored record ending at `after` is outside
// `(after,before]` and must not enter either the value or the bucket width.
package corpus

import (
	"fmt"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

const (
	c034InsideRate  = 10
	c034OutsideRate = 1000
	c034Samples     = 17200
)

func c034Base(ue int) int64 {
	gran2 := int64(ue) * 3600
	return int64(fixture.T0) - int64(fixture.T0)%gran2
}

func c034Fixture(t *testing.T, ue, machineGUID int) (context, host string) {
	t.Helper()

	context = fmt.Sprintf("fixture.c034_ue%d", ue)
	host = fmt.Sprintf("c034-ue%d", ue)
	base := c034Base(ue)
	ch := fixture.Chart{
		ID: context, Title: "single bucket boundary", Units: "units/s",
		Family: "fixture", Context: context, UpdateEvery: ue,
		Dimensions: []fixture.Dimension{{ID: "rate", Algorithm: "incremental"}},
	}
	for i := 1; i <= c034Samples; i++ {
		rate := c034InsideRate
		if i <= 3600 {
			rate = c034OutsideRate
		}
		ch.Dimensions[0].Points = append(ch.Dimensions[0].Points, fixture.Point{
			T:         base + int64(i*ue),
			Collected: strconv.Itoa(rate),
			Flags:     stream.FlagNotAnomalous,
		})
	}

	pushLiveBurst(t, host, guid(machineGUID), ch)
	if _, err := td.WaitRetention(host, context, ch.FirstT(), ch.LastT(), 60*time.Second); err != nil {
		t.Fatal(err)
	}
	return context, host
}

func TestCase034SingleBucketRespectsRequestedWindow(t *testing.T) {
	const contract = "CASE-034/single-bucket-respects-requested-window"
	cases := map[string]struct {
		ue, machineGUID int
	}{
		"ue1":  {ue: 1, machineGUID: 294},
		"ue10": {ue: 10, machineGUID: 295},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			trackContractComponent(t, contract, name)

			ok := true
			ue := tc.ue
			context, host := c034Fixture(t, ue, tc.machineGUID)
			base := c034Base(ue)
			after := base + int64(ue)*3600
			before := after + int64(ue)*3600
			duration := before - after

			for _, tier := range []int{0, 1, 2} {
				for group := range map[string]struct{}{"average": {}, "sum": {}} {
					params := daemon.DataParamsTier(context, tier, after, before, 1, group)
					params.Set("options", "jsonwrap|unaligned")
					doc, err := td.DataV3(host, params)
					if err != nil {
						t.Fatal(err)
					}

					view, hasView := doc["view"].(map[string]any)
					viewAfter, afterOK := view["after"].(float64)
					viewBefore, beforeOK := view["before"].(float64)
					viewEvery, everyOK := view["update_every"].(float64)
					if !hasView || !afterOK || !beforeOK || !everyOK ||
						int64(viewAfter) != after || int64(viewBefore) != before || int64(viewEvery) != duration {
						t.Logf("ue%d tier%d %s view: after=%v before=%v update_every=%v, want %d/%d/%d",
							ue, tier, group, view["after"], view["before"], view["update_every"],
							after, before, duration)
						ok = false
					}

					cols, err := canon.Columns(doc)
					if err != nil {
						t.Fatal(err)
					}
					col := cols["rate"]
					if len(col) != 1 || col[0].T != before || col[0].Value == nil {
						t.Logf("ue%d tier%d %s returned %v, want one non-null row ending at %d",
							ue, tier, group, col, before)
						ok = false
						continue
					}

					want := float64(c034InsideRate)
					if group == "sum" {
						want *= float64(duration)
					}
					if math.IsNaN(*col[0].Value) || math.IsInf(*col[0].Value, 0) ||
						math.Abs(*col[0].Value-want) > 1e-6 {
						t.Logf("ue%d tier%d %s over (%d,%d] returned %.10g, want %.10g; "+
							"the %d/s record ending at after is outside the requested window",
							ue, tier, group, after, before, *col[0].Value, want, c034OutsideRate)
						ok = false
					}
				}
			}
			if !ok {
				t.Errorf("BROKEN %s (%s): %s", contract, name, manifest[contract].Proves)
			}
		})
	}
}
