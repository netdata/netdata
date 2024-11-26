// SPDX-License-Identifier: GPL-3.0-or-later

package testrandom

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

func (c *Collector) collect() (map[string]int64, error) {
	collected := make(map[string]int64)

	for _, chart := range *c.Charts() {
		c.collectChart(collected, chart)
	}
	return collected, nil
}

func (c *Collector) collectChart(collected map[string]int64, chart *module.Chart) {
	var num int
	if chart.Opts.Hidden {
		num = c.Config.HiddenCharts.Dims
	} else {
		num = c.Config.Charts.Dims
	}

	for i := 0; i < num; i++ {
		name := fmt.Sprintf("random%d", i)
		id := fmt.Sprintf("%s_%s", chart.ID, name)

		if !c.collectedDims[id] {
			c.collectedDims[id] = true

			dim := &module.Dim{ID: id, Name: name}
			if err := chart.AddDim(dim); err != nil {
				c.Warning(err)
			}
			chart.MarkNotCreated()
		}
		if i%2 == 0 {
			collected[id] = c.randInt()
		} else {
			collected[id] = -c.randInt()
		}
	}
}
