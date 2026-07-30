// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-034 a query asking for one bucket still covers exactly the requested
// interval. The sample or stored record ending at `after` is outside
// `(after,before]` and must not enter either the value or the bucket width.
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
	cases := []struct {
		name            string
		ue, machineGUID int
	}{
		{name: "ue1", ue: 1, machineGUID: 294},
		{name: "ue10", ue: 10, machineGUID: 295},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			trackContractComponent(t, contract, tc.name)

			ok := true
			ue := tc.ue
			context, host := c034Fixture(t, ue, tc.machineGUID)
			base := c034Base(ue)
			after := base + int64(ue)*3600
			before := after + int64(ue)*3600
			duration := before - after

			for _, tier := range []int{0, 1, 2} {
				for _, group := range []string{"average", "sum"} {
					params := daemon.DataParamsTier(context, tier, after, before, 1, group)
					params.Set("options", "jsonwrap|unaligned")
					params.Set("scope_dimensions", "rate")
					doc, err := td.DataV3(host, params)
					if err != nil {
						t.Fatal(err)
					}
					if !assertSelectedTier(t, doc, tier) {
						ok = false
					}

					if !assertExactView(t, doc, after, before, duration) {
						t.Logf("ue%d tier%d %s did not return the exact one-bucket (after,before] view",
							ue, tier, group)
						ok = false
					}

					cols, err := canon.Columns(doc)
					if err != nil {
						t.Fatal(err)
					}
					if !assertOnlyColumn(t, cols, "rate") {
						ok = false
					}

					wantValue := float64(c034InsideRate)
					if group == "sum" {
						wantValue *= float64(duration)
					}
					want := []expectedColumnPoint{wantNumberWithMetadataAt(before, wantValue, 0, 0)}
					if !assertExactColumn(t, cols, "rate", want, 0) {
						t.Logf("ue%d tier%d %s over (%d,%d] did not return exact value %.10g; "+
							"the %d/s record ending at after is outside the requested window",
							ue, tier, group, after, before, wantValue, c034OutsideRate)
						ok = false
					}
				}
			}

			// Natural points changes the executor's query granularity from one
			// second to the selected tier's stored-record cadence. The same
			// outside sentinel must remain excluded in that branch too.
			for _, tier := range []int{1, 2} {
				queryGranularity := int64(ue * 60)
				if tier == 2 {
					queryGranularity = int64(ue * 3600)
				}
				naturalBefore := after + queryGranularity
				for _, group := range []string{"average", "sum"} {
					params := daemon.DataParamsTier(context, tier, after, naturalBefore, 1, group)
					params.Set("options", "jsonwrap|unaligned|natural-points")
					params.Set("scope_dimensions", "rate")
					doc, err := td.DataV3(host, params)
					if err != nil {
						t.Fatal(err)
					}
					if !assertSelectedTier(t, doc, tier) {
						ok = false
					}
					if !assertExactView(t, doc, after, naturalBefore, queryGranularity) {
						t.Logf("ue%d tier%d natural %s did not return the exact one-record view",
							ue, tier, group)
						ok = false
					}
					cols, err := canon.Columns(doc)
					if err != nil {
						t.Fatal(err)
					}
					if !assertOnlyColumn(t, cols, "rate") {
						ok = false
					}
					wantValue := float64(c034InsideRate)
					if group == "sum" {
						wantValue *= float64(queryGranularity)
					}
					want := []expectedColumnPoint{
						wantNumberWithMetadataAt(naturalBefore, wantValue, 0, 0),
					}
					if !assertExactColumn(t, cols, "rate", want, 0) {
						t.Logf("ue%d tier%d natural %s did not return the exact one-record result",
							ue, tier, group)
						ok = false
					}
				}
			}
			if !ok {
				t.Errorf("BROKEN %s (%s): %s", contract, tc.name, manifest[contract].Proves)
			}
		})
	}
}
