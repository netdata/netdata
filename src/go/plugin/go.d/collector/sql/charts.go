// SPDX-License-Identifier: GPL-3.0-or-later

package sql

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioChart = module.Priority + iota
)

func (c *Collector) createChart(chartID string, m ConfigMetricBlock, ch ConfigChartConfig, row map[string]string) {
	if c.seenCharts[chartID] {
		return
	}
	c.seenCharts[chartID] = true

	chart := &module.Chart{
		ID:       chartID,
		Title:    ch.Title,
		Units:    ch.Units,
		Type:     module.ChartType(ch.Type),
		Ctx:      ch.Context,
		Fam:      ch.Family,
		Priority: prioChart,
	}

	for k, v := range c.StaticLabels {
		chart.Labels = append(chart.Labels, module.Label{Key: k, Value: v})
	}
	chart.Labels = append(chart.Labels, module.Label{Key: "driver", Value: c.Driver})

	for _, lf := range m.LabelsFromRow {
		if v, ok := row[lf.Source]; ok {
			chart.Labels = append(chart.Labels, module.Label{Key: lf.Name, Value: v})
		}
	}

	for _, d := range ch.Dims {
		chart.Dims = append(chart.Dims, &module.Dim{
			ID:   buildDimID(chartID, d.Name),
			Name: d.Name,
			Algo: module.DimAlgo(ch.Algorithm),
		})
	}

	if err := c.Charts().Add(chart); err != nil {
		c.Warningf("failed to add chart %q: %v", chartID, err)
	}
}
