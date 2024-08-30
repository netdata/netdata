// SPDX-License-Identifier: GPL-3.0-or-later

package example

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

func (e *Example) collect() (map[string]int64, error) {
	collected := make(map[string]int64)

	for _, chart := range *e.Charts() {
		e.collectChart(collected, chart)
	}
	return collected, nil
}

func (e *Example) collectChart(collected map[string]int64, chart *module.Chart) {
	var num int
	if chart.Opts.Hidden {
		num = e.Config.HiddenCharts.Dims
	} else {
		num = e.Config.Charts.Dims
	}

	for i := 0; i < num; i++ {
		name := fmt.Sprintf("random%d", i)
		id := fmt.Sprintf("%s_%s", chart.ID, name)

		if !e.collectedDims[id] {
			e.collectedDims[id] = true

			dim := &module.Dim{ID: id, Name: name}
			if err := chart.AddDim(dim); err != nil {
				e.Warning(err)
			}
			chart.MarkNotCreated()
		}
		if i%2 == 0 {
			collected[id] = e.randInt()
		} else {
			collected[id] = -e.randInt()
		}
	}
}
