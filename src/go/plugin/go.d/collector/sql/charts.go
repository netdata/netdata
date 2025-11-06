// SPDX-License-Identifier: GPL-3.0-or-later

package sql

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioChart = module.Priority + iota
)

func (c *Collector) addQueryChart(row map[string]string, queryCfg QueryConfig, chartID, queryValue string) {
	chart := &module.Chart{
		ID:       chartID,
		Title:    queryCfg.ChartMeta.Description,
		Units:    queryCfg.ChartMeta.Units,
		Fam:      queryCfg.ChartMeta.Family,
		Ctx:      fmt.Sprintf("sql.%s", queryCfg.ChartMeta.Context),
		Type:     module.ChartType(queryCfg.ChartMeta.Type),
		Priority: prioChart,
	}

	chart.Labels = append(chart.Labels, module.Label{Key: "driver", Value: c.Driver})

	for _, lbl := range queryCfg.Labels {
		if v, ok := row[lbl]; ok {
			chart.Labels = append(chart.Labels, module.Label{Key: lbl, Value: v})
		}
	}

	chart.Dims = append(chart.Dims, &module.Dim{
		ID:   chartID,
		Name: queryValue,
		Algo: module.DimAlgo(queryCfg.ChartMeta.Algorithm),
	})

	if err := c.Charts().Add(chart); err != nil {
		c.Warningf("failed to add chart for query %s (%s): %v", queryCfg.Name, chartID, err)
	}
}
