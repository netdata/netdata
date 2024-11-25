// SPDX-License-Identifier: GPL-3.0-or-later

package testrandom

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

func (tr *TestRandom) collect() (map[string]int64, error) {
	collected := make(map[string]int64)

	for _, chart := range *tr.Charts() {
		tr.collectChart(collected, chart)
	}
	return collected, nil
}

func (tr *TestRandom) collectChart(collected map[string]int64, chart *module.Chart) {
	var num int
	if chart.Opts.Hidden {
		num = tr.Config.HiddenCharts.Dims
	} else {
		num = tr.Config.Charts.Dims
	}

	for i := 0; i < num; i++ {
		name := fmt.Sprintf("random%d", i)
		id := fmt.Sprintf("%s_%s", chart.ID, name)

		if !tr.collectedDims[id] {
			tr.collectedDims[id] = true

			dim := &module.Dim{ID: id, Name: name}
			if err := chart.AddDim(dim); err != nil {
				tr.Warning(err)
			}
			chart.MarkNotCreated()
		}
		if i%2 == 0 {
			collected[id] = tr.randInt()
		} else {
			collected[id] = -tr.randInt()
		}
	}
}
