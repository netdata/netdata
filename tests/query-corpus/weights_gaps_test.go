// SPDX-License-Identifier: GPL-3.0-or-later
package corpus

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

func TestWeightsAnomalyGaps(t *testing.T) {
	trackContract(t, "W/anomaly-gaps")
	for _, interval := range []int{1, 5} {
		t.Run(fmt.Sprintf("interval-%d", interval), func(t *testing.T) { testWeightsAnomalyGaps(t, interval) })
	}
}

func testWeightsAnomalyGaps(t *testing.T, interval int) {
	context := fmt.Sprintf("fixture.weights_gaps%d", interval)
	host := fmt.Sprintf("weights-gaps%d", interval)
	ch := weightsFixture()
	ch.ID, ch.Context = context, context
	ch.UpdateEvery = interval
	ch.Dimensions = ch.Dimensions[:2]
	ch.Dimensions[0].ID = "sparse"
	ch.Dimensions[1].ID = "complete"
	for d := range ch.Dimensions {
		for i := range ch.Dimensions[d].Points {
			p := &ch.Dimensions[d].Points[i]
			p.Collected, p.Flags = "20", stream.FlagAnomalous
			if d == 0 && p.T != fixture.T0+180 {
				p.Flags = stream.FlagEmpty
			}
			p.T = fixture.T0 + (p.T-fixture.T0)*int64(interval)
		}
	}
	weightsSettle(t, host, guid(9810+interval), ch)
	for _, version := range []string{"v1", "v2", "v3"} {
		for _, method := range []string{"anomaly-rate", "value"} {
			p := weightsV1Params(method, context, "raw", false)
			p.Set("after", strconv.FormatInt(fixture.T0+120*int64(interval), 10))
			p.Set("before", strconv.FormatInt(fixture.T0+240*int64(interval), 10))
			if version != "v1" {
				p.Set("scope_contexts", context)
				p.Set("scope_nodes", host)
			}
			doc, err := td.HostJSON(host, "api/"+version+"/weights", p)
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]float64
			if version == "v1" {
				got = v1ContextsWeights(t, doc, context)
			} else {
				got = map[string]float64{}
				for id, row := range weightsLimitRows(t, doc) {
					got[id] = row[5].(float64)
				}
			}
			// Natural-point grouping uses (duration + 1) / granularity:
			// 121 slots at 1s, 120 slots at 5s for these windows.
			slots := float64((120*interval + 1) / interval)
			want := map[string]float64{"sparse": 100 / slots, "complete": 100}
			if method == "value" {
				want = map[string]float64{"sparse": 20, "complete": 20}
			}
			for id, w := range want {
				if g, ok := got[id]; !ok || !tierValueMatch(g, w, 1e-6) {
					t.Errorf("%s %s %s got %v, want %v", version, method, id, g, w)
				}
			}
		}
	}
}
